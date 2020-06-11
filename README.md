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
            "state": "ACTIVE"
        },
        {
            "id": "id-B",
            "count": 2,
            "state": "INACTIVE"
        },
        {
            "id": "id-C",
            "count": 3,
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

$ python -m SimpleHTTPServer 8000 &
Serving HTTP on 0.0.0.0 port 8000 ...

$ ./json_exporter http://localhost:8000/example/data.json example/config.yml &
INFO[2016-02-08T22:44:38+09:00] metric registered;name:<example_global_value>
INFO[2016-02-08T22:44:38+09:00] metric registered;name:<example_value_active>
INFO[2016-02-08T22:44:38+09:00] metric registered;name:<example_value_count>
127.0.0.1 - - [08/Feb/2016 22:44:38] "GET /example/data.json HTTP/1.1" 200 -

$ curl http://localhost:7979/metrics | grep ^example
example_global_value{environment="beta"} 1234
example_value_active{environment="beta",id="id-A"} 1
example_value_active{environment="beta",id="id-C"} 1
example_value_count{environment="beta",id="id-A"} 1
example_value_count{environment="beta",id="id-C"} 3
```

# global options as arguments

* `port`: port exposing the metrics. Default: 7979
* `interval`: scraping interval of the json api request. Default: 60 seconds
* `log-level`: logging level. Default: info

The following example sets the json api scraping to 30 seconds.

```sh
$ ./json_exporter --interval=30  http://localhost:8000/example/data.json example/config.yml &
```

# Docker

```console
docker run \
  -v config.yml:/config.yml
  json_exporter \
    http://example.com/target.json \
    /config.yml
```

# See Also
- [kawamuray/jsonpath](https://github.com/kawamuray/jsonpath#path-syntax) : For syntax reference of JSONPath.
  Originally forked from nicksardo/jsonpath(now is https://github.com/NodePrime/jsonpath).
