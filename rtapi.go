package main

import (
	"bytes"
	"encoding/json"
	"github.com/gobuffalo/packr/v2"
	"github.com/jung-kurt/gofpdf"
	"github.com/tsenart/vegeta/v12/lib"
	"github.com/urfave/cli/v2"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type endpointDetails struct {
	Target  endpointTarget `json:"target" yaml:"target"`
	Query   endpointQuery  `json:"query_parameters" yaml:"query_parameters"`
	metrics vegeta.Metrics
}

type endpointTarget struct {
	Method string      `json:"method" yaml:"method"`
	URL    string      `json:"url" yaml:"url"`
	Body   string      `json:"body" yaml:"body"`
	Header http.Header `json:"header" yaml:"header"`
}

type endpointQuery struct {
	Threads     uint64 `json:"threads" yaml:"threads"`
	MaxThreads  uint64 `json:"max_threads" yaml:"max_threads"`
	Connections int    `json:"connections" yaml:"connections"`
	Duration    string `json:"duration" yaml:"duration"`
	RequestRate int    `json:"request_rate" yaml:"request_rate"`
}

func main() {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "select a JSON or YAML file to load",
		},
		&cli.StringFlag{
			Name:    "data",
			Aliases: []string{"d"},
			Usage:   "input API parameters directly as a JSON string",
		},
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "output query results in easy to grasp PDF report",
		},
		&cli.BoolFlag{
			Name:    "print",
			Aliases: []string{"p"},
			Usage:   "output technical query results to terminal",
		},
	}

	app := &cli.App{
		Name:    "Real time API latency analyzer",
		Version: "v0.2.0",
		Usage:   "Create a PDF report and HDR histogram of Your APIs",
		Flags:   flags,
		Action: func(c *cli.Context) error {
			// Check if there's any input data
			var endpointList []endpointDetails
			if !c.IsSet("file") && !c.IsSet("data") {
				log.Fatal("No data found")
			} else if c.IsSet("file") && c.IsSet("data") {
				log.Fatal("Please only use either file or data as your input source")
			} else if !c.IsSet("output") && !c.Bool("print") {
				log.Fatal("You did not specify any type of output")
			} else if c.IsSet("file") {
				if filepath.Ext(c.String("file")) == ".json" {
					endpointList = parseJSON(c.String("file"))
				} else if filepath.Ext(c.String("file")) == ".yml" || filepath.Ext(c.String("file")) == ".yaml" {
					endpointList = parseYAML(c.String("file"))
				}
			} else if c.IsSet("data") {
				endpointList = parseJSONString(c.String("data"))
			}
			// Query each endpoint specified
			for i := range endpointList {
				endpointList[i].metrics = queryAPI(endpointList[i])
			}
			// Print text report
			if c.Bool("print") {
				printText(endpointList)
			}
			// Create a PDF with some informative text and the graph we've just created
			if c.IsSet("output") {
				createPDF(endpointList, c.String("output"))
			}
			return nil
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func parseJSON(file string) []endpointDetails {
	jsonFile, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer jsonFile.Close()

	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		panic(err)
	}
	var temp []endpointDetails
	err = json.Unmarshal(byteValue, &temp)
	if err != nil {
		panic(err)
	}
	return temp
}

func parseYAML(file string) []endpointDetails {
	yamlFile, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer yamlFile.Close()

	byteValue, err := ioutil.ReadAll(yamlFile)
	if err != nil {
		panic(err)
	}
	var temp []endpointDetails
	err = yaml.Unmarshal(byteValue, &temp)
	if err != nil {
		panic(err)
	}
	return temp
}

func parseJSONString(value string) []endpointDetails {
	var temp []endpointDetails
	err := json.Unmarshal([]byte(value), &temp)
	if err != nil {
		panic(err)
	}
	return temp
}

// Override the default JSON unmarshal behavior to set some default query parameters
// if they are not specified in the input JSON
func (details *endpointDetails) UnmarshalJSON(b []byte) error {
	type tempDetails endpointDetails
	temp := &tempDetails{
		Query: endpointQuery{
			Threads:     2,
			MaxThreads:  2,
			Connections: 10,
			Duration:    "10s",
			RequestRate: 500,
		},
	}
	if err := json.Unmarshal(b, temp); err != nil {
		return err
	}
	*details = endpointDetails(*temp)
	return nil
}

// Override the default YAML unmarshal behavior to set some default query parameters
// if they are not specified in the input YAML
func (details *endpointDetails) UnmarshalYAML(node *yaml.Node) error {
	type tempDetails endpointDetails
	temp := &tempDetails{
		Query: endpointQuery{
			Threads:     2,
			MaxThreads:  2,
			Connections: 10,
			Duration:    "10s",
			RequestRate: 500,
		},
	}
	if err := node.Decode(temp); err != nil {
		return err
	}
	*details = endpointDetails(*temp)
	return nil
}

func queryAPI(endpoint endpointDetails) vegeta.Metrics {
	rate := vegeta.Rate{
		Freq: endpoint.Query.RequestRate,
		Per:  time.Second,
	}
	duration, err := time.ParseDuration(endpoint.Query.Duration)
	if err != nil {
		log.Fatal(err)
	}
	targeter := vegeta.NewStaticTargeter(
		vegeta.Target{
			URL:    endpoint.Target.URL,
			Method: endpoint.Target.Method,
			Body:   []byte(endpoint.Target.Body),
			Header: endpoint.Target.Header,
		},
	)
	workers := vegeta.Workers(endpoint.Query.Threads)
	maxWorkers := vegeta.MaxWorkers(endpoint.Query.MaxThreads)
	connections := vegeta.Connections(endpoint.Query.Connections)
	body := vegeta.MaxBody(0)
	attacker := vegeta.NewAttacker(workers, maxWorkers, connections, body)
	var metrics vegeta.Metrics
	for response := range attacker.Attack(targeter, rate, duration, "") {
		metrics.Add(response)
	}
	metrics.Close()
	return metrics
}

func printText(endpoints []endpointDetails) {
	os.Stdout.Write([]byte("====================================\n"))
	os.Stdout.Write([]byte("NGINX — Real-Time API Latency Report\n"))
	os.Stdout.Write([]byte("====================================\n\n"))
	text := [...]string{
		"APIs lie at the very heart of modern applications and evolving digital architectures.\n" +
			"In today’s landscape, where the barrier of switching to a digital competitor is very low,\n" +
			"it is of the upmost importance for consumers to have positive experiences.\n\n",
		"Therefore, at NGINX, we define a real-time API as one that can process end-to-end API calls in 30ms or less (see " +
			"\"https://www.nginx.com/blog/how-real-time-apis-power-our-lives\" for more information).\n\n",
		"To get started, let’s assess how your API endpoints stack up.\n\n",
		"Learn more, talk to an NGINX expert, and discover how NGINX can help you on " +
			"your journey towards real-time APIs at \"https://www.nginx.com/real-time-api\"\n",
	}
	os.Stdout.Write([]byte(text[0]))
	os.Stdout.Write([]byte(text[1]))
	os.Stdout.Write([]byte(text[2]))
	for i := range endpoints {
		reporter := vegeta.NewTextReporter(&endpoints[i].metrics)
		os.Stdout.Write([]byte("------------------------------------\n"))
		os.Stdout.Write([]byte("API Endpoint: " + endpoints[i].Target.URL + "\n"))
		os.Stdout.Write([]byte("------------------------------------\n"))
		reporter.Report(os.Stdout)
		os.Stdout.Write([]byte("------------------------------------\n\n"))
	}
	os.Stdout.Write([]byte(text[3]))
}

func createPDF(endpoints []endpointDetails, output string) {
	text := [...]string{
		"<center><b>NGINX — Real-Time API Latency Report</b></center>",
		"<b>Why API Performance Matters</b>",
		"APIs lie at the very heart of modern applications and evolving digital architectures. " +
			"In today’s landscape, where the barrier of switching to a digital competitor is very low, " +
			"it is of the upmost importance for consumers to have positive experiences. " +
			"This is ultimately driven by responsive, healthy, and adaptable APIs. " +
			"If you get this right, and your API call is faster than your competitor’s, " +
			"developers will choose you.",
		"However, it’s a major challenge for most businesses to process API calls in " +
			"as near to real time as possible. According to the IDC report " +
			"<i><a href=\"https://www.nginx.com/resources/library/idc-report-apis-success-failure-digital-business/\">" +
			"APIs — The Determining Agents Between Success or Failure of Digital Business</a></i>, " +
			"over 90% of organizations expect a latency of under 50 milliseconds, " +
			"while almost 60% expect latency of 20 milliseconds or less. " +
			"At NGINX, we’ve used this data, together with some end-to-end analysis of the API lifecycle, " +
			"to define a <a href=\"https://www.nginx.com/blog/how-real-time-apis-power-our-lives/\">" +
			"real-time API</a> as one with latency of 30ms or less. " +
			"(Latency is defined as the amount of time it takes for your API infrastructure " +
			"to respond to an API call – from the moment a request arrives at the API gateway " +
			"to when the first byte of a response is returned to the client.)",
		"So, how do your APIs measure up? Are they already fast enough to be considered real time, " +
			"or do they need to improve? Does your product feel a bit sluggish, but you can’t quite " +
			"place why that is? Maybe you don’t know for sure what your API latency looks like? " +
			"Whether you’re using an API as the interface for microservices deployments, " +
			"building a revenue stream with an external API, or something totally new, we’re here to help.",
		"<b>Your API Performance</b>",
		"We have run a simple HTTP benchmark using the query parameters you specified on " +
			"each of the target API endpoints you listed and created an " +
			"<a href=\"https://hdrhistogram.github.io/HdrHistogram/\">Hdr Histogram</a> graph " +
			"that shows the latency of your API endpoints. Ideally, the latency at the 99th percentile " +
			"(<b>99%</b> on the graph) is less than 30ms for your API to be considered real time.",
		"Is your API’s latency below 30ms? We can help you improve it no matter where it is!",
		"Learn more, talk to an NGINX expert, and discover how NGINX can help you on " +
			"your journey towards real-time APIs at <a href=\"https://www.nginx.com/real-time-api\">" +
			"https://www.nginx.com/real-time-api</a>",
	}

	// Pack binary data into the go binary
	box := packr.New("NGINX", "./data")
	arialBytes, err := box.Find("arial.ttf")
	if err != nil {
		log.Fatal(err)
	}
	arialItalicBytes, err := box.Find("arial_italic.ttf")
	if err != nil {
		log.Fatal(err)
	}
	arialBoldBytes, err := box.Find("arial_bold.ttf")
	if err != nil {
		log.Fatal(err)
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetMargins(25.4, 25.4, 25.4)
	pdf.AddUTF8FontFromBytes("ArialTrue", "", arialBytes)
	pdf.AddUTF8FontFromBytes("ArialTrue", "I", arialItalicBytes)
	pdf.AddUTF8FontFromBytes("ArialTrue", "B", arialBoldBytes)
	pdf.SetFont("ArialTrue", "", 16)
	pt := pdf.PointConvert(6)
	html := pdf.HTMLBasicNew()

	options := gofpdf.ImageOptions{
		ImageType: "png",
		ReadDpi:   true,
	}
	logoBytes, err := box.Find("nginx_logo.png")
	if err != nil {
		log.Fatal(err)
	}
	logo := bytes.NewReader(logoBytes)
	pdf.RegisterImageOptionsReader("logo", options, logo)
	pdf.ImageOptions("logo", 26, 13.5, 10.6, 12.03, false, options, 0, "")

	_, lineHt := pdf.GetFontSize()
	lineSpacing := 1.25
	lineHt *= lineSpacing
	html.Write(lineHt, text[0])
	pdf.Ln(pt)
	pdf.SetFontSize(11)
	_, lineHt = pdf.GetFontSize()
	lineSpacing = 1.2
	lineHt *= lineSpacing
	html.Write(lineHt, text[1])
	pdf.Ln(lineHt + pt)
	pdf.SetFontSize(10)
	_, lineHt = pdf.GetFontSize()
	lineHt *= lineSpacing
	html.Write(lineHt, text[2])
	pdf.Ln(lineHt + pt)
	html.Write(lineHt, text[3])
	pdf.Ln(lineHt + pt)
	html.Write(lineHt, text[4])
	pdf.Ln(lineHt + pt)
	pdf.SetFontSize(11)
	_, lineHt = pdf.GetFontSize()
	lineHt *= lineSpacing
	html.Write(lineHt, text[5])
	pdf.Ln(lineHt + pt)
	pdf.SetFontSize(10)
	_, lineHt = pdf.GetFontSize()
	lineHt *= lineSpacing
	html.Write(lineHt, text[6])
	pdf.Ln(lineHt + pt)

	// Create a graph with all the endpoint query results
	buffer := createGraph(endpoints)
	graph := bytes.NewReader(buffer.Bytes())
	pdf.RegisterImageOptionsReader("graph", options, graph)
	pdf.ImageOptions("graph", 45, 0, 120, 120, true, options, 0, "")

	html.Write(lineHt, text[7])
	pdf.Ln(lineHt + pt)
	html.Write(lineHt, text[8])
	pdf.Ln(lineHt + pt)

	err = pdf.OutputFileAndClose(output)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write([]byte("PDF report generated successfully!\n"))
}

func createGraph(endpoints []endpointDetails) *bytes.Buffer {
	// Rearrange HdrHistogram data to plottable data
	var stringArray [][]string
	var points []plotter.XYs
	for i := range endpoints {
		reporter := vegeta.NewHDRHistogramPlotReporter(&endpoints[i].metrics)
		buffer := new(bytes.Buffer)
		reporter.Report(buffer)
		bufferString := buffer.String()
		stringArray = append(stringArray, strings.Split(bufferString, "\n")[1:])
		points = append(points, make(plotter.XYs, len(stringArray[i])-1))
		for j := range stringArray[i] {
			values := strings.Fields(stringArray[i][j])
			if len(values) == 4 {
				x, err := strconv.ParseFloat(values[3], 64)
				if err != nil {
					log.Fatal(err)
				}
				y, err := strconv.ParseFloat(values[0], 64)
				if err != nil {
					log.Fatal(err)
				}
				points[i][j].X = x
				points[i][j].Y = y
			}
		}
	}
	// Create a new graph and populate it with the HdrHistogram data
	p, err := plot.New()
	if err != nil {
		panic(err)
	}
	p.X.Label.Text = "Percentile (%)"
	p.X.Label.TextStyle.Font.Size = vg.Length(15)
	p.X.Scale = plot.LogScale{}
	p.X.Tick.Marker = customXTicks{}
	p.Y.Label.Text = "Latency (ms)"
	p.Y.Label.TextStyle.Font.Size = vg.Length(15)
	p.Y.Label.Padding = vg.Length(-20)
	p.Y.Min = 0
	p.Y.Tick.Marker = customYTicks{}
	p.Add(plotter.NewGrid())

	// Plot the Hdr Histogram for each API endpoint
	for i := range points {
		lpLine, lpPoints, err := plotter.NewLinePoints(points[i])
		if err != nil {
			panic(err)
		}
		// Start at +1 to skip the red color (and avoid confusion with the 30ms threshold line)
		lpLine.Color = plotutil.Color(i + 1)
		lpLine.Dashes = plotutil.Dashes(i + 1)
		lpPoints.Color = plotutil.Color(i + 1)
		lpPoints.Shape = plotutil.Shape(i + 1)
		p.Add(lpLine, lpPoints)
		p.Legend.Add(endpoints[i].Target.URL, [2]plot.Thumbnailer{lpLine, lpPoints}[0], [2]plot.Thumbnailer{lpLine, lpPoints}[1])
	}
	// Label the latency at 99% for each API endpoint
	for i := range endpoints {
		lineX, err := plotter.NewLine(
			plotter.XYs{
				plotter.XY{
					X: p.X.Min,
					Y: float64(endpoints[i].metrics.Latencies.P99) / 1000000,
				},
				plotter.XY{
					X: 100,
					Y: float64(endpoints[i].metrics.Latencies.P99) / 1000000,
				},
			},
		)
		if err != nil {
			panic(err)
		}
		lineX.LineStyle = draw.LineStyle{
			Color: plotutil.Color(0),
			Width: vg.Length(2),
			Dashes: []vg.Length{
				vg.Length(4),
			},
		}
		p.Add(lineX)
		labels, err := plotter.NewLabels(
			plotter.XYLabels{
				plotter.XYs{
					plotter.XY{
						X: 100,
						Y: float64(float64(endpoints[i].metrics.Latencies.P99) / 1000000),
					},
				},
				[]string{
					strconv.FormatFloat(float64(endpoints[i].metrics.Latencies.P99) / 1000000, 'f', 3, 64) + "ms @ 99%",
				},
			},
		)
		if err != nil {
			panic(err)
		}
		labels.TextStyle[0].Color = plotutil.Color(0)
		labels.TextStyle[0].Font.Size = vg.Length(14)
		p.Add(labels)
	}
	// Add a line to highlight the 30ms and 99% thresholds
	line30ms, err := plotter.NewLine(
		plotter.XYs{
			plotter.XY{
				X: 1,
				Y: 30,
			},
			plotter.XY{
				X: 10000000,
				Y: 30,
			},
		},
	)
	if err != nil {
		panic(err)
	}
	line30ms.LineStyle = draw.LineStyle{
		Width: vg.Length(1),
		Dashes: []vg.Length{
			vg.Length(4),
		},
		DashOffs: vg.Length(8),
	}
	p.Add(line30ms)
	line99, err := plotter.NewLine(
		plotter.XYs{
			plotter.XY{
				X: 100,
				Y: p.Y.Min,
			},
			plotter.XY{
				X: 100,
				Y: p.Y.Max,
			},
		},
	)
	if err != nil {
		panic(err)
	}
	line99.LineStyle = draw.LineStyle{
		Width: vg.Length(1),
		Dashes: []vg.Length{
			vg.Length(4),
		},
		DashOffs: vg.Length(8),
	}
	p.Add(line99)

	// Save the graph data into a buffer
	buffer := new(bytes.Buffer)
	wrt, err := p.WriterTo(25*vg.Centimeter, 25*vg.Centimeter, "png")
	if err != nil {
		panic(err)
	}
	wrt.WriteTo(buffer)
	return buffer
}

type customXTicks struct{}

func (customXTicks) Ticks(min, max float64) []plot.Tick {
	return []plot.Tick{
		plot.Tick{
			Value: 1,
			Label: "0%",
		},
		plot.Tick{
			Value: 10,
			Label: "90%",
		},
		plot.Tick{
			Value: 100,
			Label: "99%",
		},
		plot.Tick{
			Value: 1000,
			Label: "99.9%",
		},
		plot.Tick{
			Value: 10000,
			Label: "99.99%",
		},
		plot.Tick{
			Value: 100000,
			Label: "99.999%",
		},
		plot.Tick{
			Value: 1000000,
			Label: "99.9999%",
		},
		plot.Tick{
			Value: 10000000,
			Label: "99.99999%",
		},
	}
}

type customYTicks struct{}

func (customYTicks) Ticks(min, max float64) []plot.Tick {
	ticks := make([]plot.Tick, 0)
	for i := 0; float64(i) <= max; i += 50 {
		ticks = append(
			ticks,
			plot.Tick{
				Value: float64(i),
				Label: strconv.Itoa(i) + "ms",
			},
		)
	}
	ticks = append(
		ticks,
		plot.Tick{
			Value: float64(30),
			Label: "Real-Time -- " + strconv.Itoa(30) + "ms",
		},
	)
	return ticks
}
