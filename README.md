# Real Time API (RTAPI) Latency Testing Tool

# This repository has been archived. There will likely be no further development on the project and security vulnerabilities may be unaddressed.


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
    v0.2.0

COMMANDS:
    help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
    --file value, -f value    select a JSON or YAML file to load
    --data value, -d value    input API parameters directly as a JSON string
    --output value, -o value  output query results in easy to grasp PDF report
    --print, -p               output technical query results to terminal (default: false)
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

### Default Values

Only the `target.url` parameter is required. If no method is specified the default is "GET", while in the case of the body and headers these will simply remain empty during the benchmark.

The default `query_parameters` closely follow the default query parameters found in [`wrk2`](https://github.com/giltene/wrk2).

```
threads: 2
max_threads: 2
connections: 10
duration: 10s
request_rate: 500
```
