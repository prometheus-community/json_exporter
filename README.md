json_exporter
========================
[![CircleCI](https://circleci.com/gh/prometheus-community/json_exporter.svg?style=svg)](https://circleci.com/gh/prometheus-community/json_exporter)

A [prometheus](https://prometheus.io/) exporter which scrapes remote JSON by JSONPath.
For checking the JSONPath configuration supported by this exporter please head over [here](https://kubernetes.io/docs/reference/kubectl/jsonpath/).  
Checkout the [examples](/examples) directory for sample exporter configuration, prometheus configuration and expected data format.  

#### :warning: The configuration syntax has changed in version `0.3.x`. If you are migrating from `0.2.x`, then please use the above mentioned JSONPath guide for correct configuration syntax.

## Example Usage

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

$ cat examples/config.yml
---
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

$ python -m SimpleHTTPServer 8000 &
Serving HTTP on 0.0.0.0 port 8000 ...

$ ./json_exporter --config.file examples/config.yml &

$ curl "http://localhost:7979/probe?target=http://localhost:8000/examples/data.json" | grep ^example
example_global_value{environment="beta",location="planet-mars"} 1234
example_value_active{environment="beta",id="id-A"} 1
example_value_active{environment="beta",id="id-C"} 1
example_value_boolean{environment="beta",id="id-A"} 1
example_value_boolean{environment="beta",id="id-C"} 0
example_value_count{environment="beta",id="id-A"} 1
example_value_count{environment="beta",id="id-C"} 3

# To test through prometheus:
$ docker run --rm -it -p 9090:9090 -v $PWD/examples/prometheus.yml:/etc/prometheus/prometheus.yml --network host prom/prometheus
```
Then head over to http://localhost:9090/graph?g0.range_input=1h&g0.expr=example_value_active&g0.tab=1 or http://localhost:9090/targets to check the scraped metrics or the targets.

## Exposing metrics through HTTPS

TLS configuration supported by this exporter can be found at [exporter-toolkit/web](https://github.com/prometheus/exporter-toolkit/blob/v0.5.1/docs/web-configuration.md)

## Build

```sh
make build
```

## Docker

```console
docker run \
  -v $PWD/examples/config.yml:/config.yml \
  quay.io/prometheuscommunity/json-exporter \
  --config.file=/config.yml
```

