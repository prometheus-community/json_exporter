json_exporter
========================
[![CircleCI](https://circleci.com/gh/prometheus-community/json_exporter.svg?style=svg)](https://circleci.com/gh/prometheus-community/json_exporter)

A [prometheus](https://prometheus.io/) exporter which scrapes remote JSON by JSONPath.

# Build

```sh
make build
```

# Example Usage

```sh
$ cat example/data.json
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
        },
    ]
}

$ cat example/config.yml
- name: example_global_value
  path: $.counter
  labels:
    environment: beta # static label

- name: example_value
  type: object
  path: $.values[*]?(@.state == "ACTIVE")
  labels:
    environment: beta # static label
    id: $.id          # dynamic label
  values:
    active: 1      # static value
    count: $.count # dynamic value
    boolean: $.some_boolean

$ python -m SimpleHTTPServer 8000 &
Serving HTTP on 0.0.0.0 port 8000 ...

$ ./json_exporter examples/config.yml &

$ curl "http://localhost:7979/probe?target=http://localhost:8000/examples/data.json" | grep ^example
example_global_value{environment="beta",location="mars"} 1234
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

# Docker

```console
docker run \
  -v config.yml:/config.yml
  quay.io/prometheuscommunity/json-exporter \
    http://example.com/target.json \
    /config.yml
```

# See Also
- [kawamuray/jsonpath](https://github.com/kawamuray/jsonpath#path-syntax) : For syntax reference of JSONPath.
  Originally forked from nicksardo/jsonpath(now is https://github.com/NodePrime/jsonpath).
