# Real Time API (RTAPI) Latency Testing Tool

This tool measures the latency response of a series of API endpoints and creates a PDF report with an HDR histogram of all API endpoints.

## How to use

`rtapi` takes either a JSON/YAML file or a JSON string containing endpoint data and query parameters (optional), queries each endpoint using the query parameters (or default query values if no parameters have been specified), and outputs a PDF report containing all the endpoint query results plotted in an HDR histogram.

```
$ ./rtapi -h
NAME:
    Real time API latency analyzer - Create a PDF report and HDR histogram of Your APIs

USAGE:
    rtapi [global options] command [command options] [arguments...]

VERSION:
    v0.0.1

COMMANDS:
    help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
    --file value, -f value    Select a JSON or YAML file to load
    --data value, -d value    Pass API parameters directly as a JSON string
    --output value, -o value  PDF report file name
    --help, -h                show help (default: false)
    --version, -v             print the version (default: false)
```

## Sample Input

### JSON

```json
[
  {
    "target": {
      "url": "https://www.example.com",
      "method": "POST",
      "body": "{\"id\":\"0\"}",
      "header": {
        "Content-Type": [
          "application/json"
        ]
      }
    },
    "query_parameters": {
      "threads": 2,
      "max_threads": 2,
      "connections": 12,
      "duration": "10s",
      "request_rate": 500
    }
  }
]
```

### YAML

```yaml
- target:
    url: https://www.example.com
    method: POST
    body: '{"id":"0"}'
    header:
      Content-Type:
        - application/json
  query_parameters:
    threads: 2
    max_threads: 2
    connections: 12
    duration: 10s
    request_rate: 500
```

## Default Query Values

The default query parameters closely follow the default query parameters found in [`wrk2`](https://github.com/giltene/wrk2).

```
threads: 2
max_threads: 2
connections: 10
duration: 10s
request_rate: 500
```
