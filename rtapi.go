package main

import (
	"bytes"
	"encoding/json"
	"github.com/gobuffalo/packr/v2"
	"github.com/jung-kurt/gofpdf"
	"github.com/tsenart/vegeta/lib"
	"github.com/urfave/cli/v2"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
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
	Target       endpointTarget `json:"target" yaml:"target"`
	Query        endpointQuery  `json:"query_parameters" yaml:"query_parameters"`
	hdrhistogram string
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
			Usage:   "Select a JSON or YAML file to load",
		},
		&cli.StringFlag{
			Name:    "data",
			Aliases: []string{"d"},
			Usage:   "Pass API parameters directly as a JSON string",
		},
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "PDF report file name",
		},
	}

	app := &cli.App{
		Name:    "Real time API latency analyzer",
		Version: "v0.1.0",
		Usage:   "Create a PDF report and HDR histogram of Your APIs",
		Flags:   flags,
		Action: func(c *cli.Context) error {
			// Check if there's any input data
			var endpointList []endpointDetails
			if !c.IsSet("file") && !c.IsSet("data") {
				log.Fatal("No data found")
			} else if c.IsSet("file") && c.IsSet("data") {
				log.Fatal("Please only use either file or data as your input source")
			} else if !c.IsSet("output") {
				log.Fatal("You did not specify any output file name")
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
				endpointList[i].hdrhistogram = queryAPI(endpointList[i])
			}
			// Create a graph with all the endpoint query results
			buffer := createGraph(endpointList)
			// Create a PDF with some informative text and the graph we've just created
			createPDF(buffer, c.String("output"))
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

func queryAPI(endpoint endpointDetails) string {
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
	reporter := vegeta.NewHDRHistogramPlotReporter(&metrics)
	buffer := new(bytes.Buffer)
	reporter.Report(buffer)
	return buffer.String()
}

func createGraph(endpoints []endpointDetails) *bytes.Buffer {
	// Rearrange HdrHistogram data to plottable data
	var stringArray [][]string
	var points []plotter.XYs
	for i := range endpoints {
		stringArray = append(stringArray, strings.Split(endpoints[i].hdrhistogram, "\n")[1:])
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
	p.X.Label.TextStyle.Font.Size = 0.5 * vg.Centimeter
	p.X.Scale = plot.LogScale{}
	p.X.Tick.Marker = customTicks{}
	p.Y.Label.Text = "Latency (ms)"
	p.Y.Label.TextStyle.Font.Size = 0.5 * vg.Centimeter
	p.Y.Min = 0
	p.Add(plotter.NewGrid())
	for i := range points {
		lpLine, lpPoints, err := plotter.NewLinePoints(points[i])
		lpLine.Color = plotutil.Color(i)
		lpLine.Dashes = plotutil.Dashes(i)
		lpPoints.Color = plotutil.Color(i)
		lpPoints.Shape = plotutil.Shape(i)
		p.Add(lpLine, lpPoints)
		p.Legend.Add(endpoints[i].Target.URL, [2]plot.Thumbnailer{lpLine, lpPoints}[0], [2]plot.Thumbnailer{lpLine, lpPoints}[1])
		if err != nil {
			panic(err)
		}
	}
	buffer := new(bytes.Buffer)
	wrt, err := p.WriterTo(25*vg.Centimeter, 25*vg.Centimeter, "png")
	wrt.WriteTo(buffer)
	return buffer
}

func createPDF(buffer *bytes.Buffer, output string) {
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
			"while almost 60% expect latency of 20 milliseconds or less. Therefore, we " +
			"define a <a href=\"https://www.nginx.com/blog/how-real-time-apis-power-our-lives/\">" +
			"real-time API</a> as one that can process end-to-end API calls in 30ms or less.",
		"Whether you’re using an API as the interface for microservices deployments, " +
			"building a revenue stream with an external API, or something totally new, we’re here to help.",
		"To get started, let’s assess how your APIs stack up.",
		"<b>Your API Performance</b>",
		"We have run a simple HTTP benchmark using the parameters you specified on " +
			"each of the API endpoints you listed and created an " +
			"<a href=\"https://hdrhistogram.github.io/HdrHistogram/\">HDR histogram</a> graph " +
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
	pdf.SetFontSize(12)
	_, lineHt = pdf.GetFontSize()
	lineHt *= lineSpacing
	html.Write(lineHt, text[1])
	pdf.Ln(lineHt + pt)
	pdf.SetFontSize(11)
	_, lineHt = pdf.GetFontSize()
	lineHt *= lineSpacing
	html.Write(lineHt, text[2])
	pdf.Ln(lineHt + pt)
	html.Write(lineHt, text[3])
	pdf.Ln(lineHt + pt)
	html.Write(lineHt, text[4])
	pdf.Ln(lineHt + pt)
	html.Write(lineHt, text[5])
	pdf.Ln(lineHt + pt)
	pdf.SetFontSize(12)
	_, lineHt = pdf.GetFontSize()
	lineHt *= lineSpacing
	html.Write(lineHt, text[6])
	pdf.Ln(lineHt + pt)
	pdf.SetFontSize(11)
	_, lineHt = pdf.GetFontSize()
	lineHt *= lineSpacing
	html.Write(lineHt, text[7])
	pdf.Ln(lineHt + pt)

	graph := bytes.NewReader(buffer.Bytes())
	pdf.RegisterImageOptionsReader("graph", options, graph)
	pdf.ImageOptions("graph", 45, 0, 120, 120, true, options, 0, "")

	html.Write(lineHt, text[8])
	pdf.Ln(lineHt + pt)
	html.Write(lineHt, text[9])
	pdf.Ln(lineHt + pt)

	err = pdf.OutputFileAndClose(output)
	if err != nil {
		log.Fatal(err)
	}
}

type customTicks struct{}

func (customTicks) Ticks(min, max float64) []plot.Tick {
	return []plot.Tick{
		plot.Tick{
			Value: 1, Label: "0%",
		},
		plot.Tick{
			Value: 10, Label: "90%",
		},
		plot.Tick{
			Value: 100, Label: "99%",
		},
		plot.Tick{
			Value: 1000, Label: "99.9%",
		},
		plot.Tick{
			Value: 10000, Label: "99.99%",
		},
		plot.Tick{
			Value: 100000, Label: "99.999%",
		},
		plot.Tick{
			Value: 1000000, Label: "99.9999%",
		},
		plot.Tick{
			Value: 10000000, Label: "99.99999%",
		},
	}
}
