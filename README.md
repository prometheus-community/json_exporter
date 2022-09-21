json_exporter
========================
[![CircleCI](https://circleci.com/gh/prometheus-community/json_exporter.svg?style=svg)](https://circleci.com/gh/prometheus-community/json_exporter)

A [prometheus](https://prometheus.io/) exporter which scrapes remote JSON by JSONPath.
For checking the JSONPath configuration supported by this exporter please head over [here](https://kubernetes.io/docs/reference/kubectl/jsonpath/).  
Checkout the [examples](/examples) directory for sample exporter configuration, prometheus configuration and expected data format.  

#### :warning: The configuration syntax has changed in version `0.3.x`. If you are migrating from `0.2.x`, then please use the above mentioned JSONPath guide for correct configuration syntax.

## Example Usage
### Preview the example json data
```console
$ cat examples/data.json
{
    "counter": 1234,
    "values": [
        {
            "id": "id-A",
            "count": 1,
            "some_boolean": true,
            "state": "ACTIVE"
        },
        {
            "id": "id-B",
            "count": 2,
            "some_boolean": true,
            "state": "INACTIVE"
        },
        {
            "id": "id-C",
            "count": 3,
            "some_boolean": false,
            "state": "ACTIVE"
        }
    ],
    "location": "mars"
}
```

### Preview the example config yaml
```console
$ cat examples/config.yml
---
modules:
  default:
    metrics:
    - name: example_global_value
      path: "{ .counter }"
      help: Example of a top-level global value scrape in the json
      labels:
        environment: beta # static label
        location: "planet-{.location}"          # dynamic label

    - name: example_value
      type: object
      help: Example of sub-level value scrapes from a json
      path: '{.values[?(@.state == "ACTIVE")]}'
      labels:
        environment: beta # static label
        id: '{.id}'          # dynamic label
      values:
        active: 1      # static value
        count: '{.count}' # dynamic value
        boolean: '{.some_boolean}'

    headers:
      X-Dummy: my-test-header
```

### Test run 
Spin up a simple server
```console
$ python -m SimpleHTTPServer 8000 &
Serving HTTP on 0.0.0.0 port 8000 ...
```
In Python 3, try the following otherwise you might encounter errors like "No module named 'SimpleHTTPServer'"
```
$ python -m http.server 8000
```

Build and run the json_exporter
```
$ make build
$ ./json_exporter --config.file examples/config.yml &
```

Validate it's running correctly
```
$ curl "http://localhost:7979/probe?module=default&target=http://localhost:8000/examples/data.json" | grep ^example
example_global_value{environment="beta",location="planet-mars"} 1234
example_value_active{environment="beta",id="id-A"} 1
example_value_active{environment="beta",id="id-C"} 1
example_value_boolean{environment="beta",id="id-A"} 1
example_value_boolean{environment="beta",id="id-C"} 0
example_value_count{environment="beta",id="id-A"} 1
example_value_count{environment="beta",id="id-C"} 3
```

### To test through prometheus:
Preview the prometheus config file and make changes if necessary.

NOTE: If you use Docker for Mac or Docker for Windows, you can’t use `localhost:9090`, but must use `host.docker.internal:9090`. This has to do with the virtual machines used to implement Docker on those operating systems. You should not use this in production. More information check [here](https://prometheus.io/docs/guides/multi-target-exporter/#querying-multi-target-exporters-with-prometheus)

```console
$ cat examples/prometheus.yml
scrape_configs:

  ## gather metrics of prometheus itself
- job_name: prometheus
  static_configs:
    - targets:
      - localhost:9090  ## For Windows and macOS replace with - host.docker.internal:9090

  ## gather the metrics of json_exporter application itself
- job_name: json_exporter
  static_configs:
    - targets:
      - localhost:7979 ## Location of the json exporter's real <hostname>:<port>. For Windows and macOS replace with - host.docker.internal:7979

  ## gather the metrics from third party json sources, via the json exporter
- job_name: json
  metrics_path: /probe
  params:
    module: [default]
  static_configs:
    - targets:
      - http://host-1.foobar.com/dummy/data.json
      - http://host-2:8000/other-examples/data.json
      - http://localhost:8000/examples/data.json ## Used from the example steps in Readme
  relabel_configs:
    - source_labels: [__address__]
      target_label: __param_target
    - source_labels: [__param_target]
      target_label: instance
    - target_label: __address__
      replacement: localhost:7979 ## Location of the json exporter's real <hostname>:<port>. For Windows and macOS replace with - host.docker.internal:7979
```

Run through docker on Linux
```
$ docker run --rm -it -p 9090:9090 -v $PWD/examples/prometheus.yml:/etc/prometheus/prometheus.yml --network host prom/prometheus
```
Run through docker on Windows and macOS
```
$ docker run --rm -it -p 9090:9090 -v $PWD/examples/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus
```

Then head over 
* to check the scraped metrics at: http://localhost:9090/graph?g0.range_input=1h&g0.expr=example_value_active&g0.tab=1 
* to check the targets at: http://localhost:9090/targets 

## Using custom timestamps

This exporter allows you to use a field of the metric as the (unix/epoch) timestamp for the data as an int64. However, this may lead to unexpected behaviour, as the prometheus implements a [Staleness](https://prometheus.io/docs/prometheus/latest/querying/basics/#staleness) mechanism. Including timestamps in metrics disabled this staleness handling and can make data visible for longer than expected.

## Exposing metrics through HTTPS

TLS configuration supported by this exporter can be found at [exporter-toolkit/web](https://github.com/prometheus/exporter-toolkit/blob/v0.5.1/docs/web-configuration.md)

## Build

```sh
make build
```

## Sending body content for HTTP `POST`

If `body` paramater is set in config, it will be sent by the exporter as the body content in the scrape request. The HTTP method will also be set as 'POST' in this case.
```yaml
body:
  content: |
    My static information: {"time_diff": "1m25s", "anotherVar": "some value"}
```

The body content can also be a [Go Template](https://golang.org/pkg/text/template). All the functions from the [Sprig library](https://masterminds.github.io/sprig/) can be used in the template.
All the query parameters sent by prometheus in the scrape query to the exporter, are available as values while rendering the template.

Example using template functions:
```yaml
body:
  content: |
    {"time_diff": "{{ duration `95` }}","anotherVar": "{{ randInt 12 30 }}"}
  templatize: true
```

Example using template functions with values from the query parameters:
```yaml
body:
  content: |
    {"time_diff": "{{ duration `95` }}","anotherVar": "{{ .myVal | first }}"}
  templatize: true
```
Then `curl "http://exporter:7979/probe?target=http://scrape_target:8080/test/data.json&myVal=something"`, would result in sending the following body as the HTTP POST payload to `http://scrape_target:8080/test/data.json`:
```
{"time_diff": "1m35s","anotherVar": "something"}.
```

## Docker

```console
docker run \
  -v $PWD/examples/config.yml:/config.yml \
  quay.io/prometheuscommunity/json-exporter \
  --config.file=/config.yml
```

