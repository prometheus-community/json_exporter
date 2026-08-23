json_exporter
========================
[![Build Status](https://github.com/prometheus-community/json_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/prometheus-community/json_exporter/actions/workflows/ci.yml)


A [prometheus](https://prometheus.io/) exporter which scrapes remote JSON by JSONPath or [CEL (Common Expression Language)](https://github.com/google/cel-spec).

- [Supported JSONPath Syntax](https://kubernetes.io/docs/reference/kubectl/jsonpath/)
- [CEL Language Definition](https://github.com/google/cel-spec/blob/master/doc/langdef.md)
- [Examples configurations](/examples)

## Example Usage

```console
## SETUP

$ make build
$ ./json_exporter --config.file examples/config.yml &
$ python3 -m http.server 8000 &
Serving HTTP on :: port 8000 (http://[::]:8000/) ...


## TEST with 'default' module

$ curl "http://localhost:7979/probe?module=default&target=http://localhost:8000/examples/data.json"
# HELP example_cel_global_value Example of a top-level global value scrape in the json using cel
# TYPE example_cel_global_value gauge
example_cel_global_value{environment="beta",location="planet-mars"} 1234
# HELP example_cel_timestamped_value Example of a timestamped value scrape in the json
# TYPE example_cel_timestamped_value untyped
example_cel_timestamped_value{environment="beta"} 2 1657568506000
# HELP example_cel_value_active Example of sub-level value scrapes from a json
# TYPE example_cel_value_active untyped
example_cel_value_active{environment="beta",id="id-A"} 1
example_cel_value_active{environment="beta",id="id-C"} 1
# HELP example_cel_value_boolean Example of sub-level value scrapes from a json
# TYPE example_cel_value_boolean untyped
example_cel_value_boolean{environment="beta",id="id-A"} 1
example_cel_value_boolean{environment="beta",id="id-C"} 0
# HELP example_cel_value_count Example of sub-level value scrapes from a json
# TYPE example_cel_value_count untyped
example_cel_value_count{environment="beta",id="id-A"} 1
example_cel_value_count{environment="beta",id="id-C"} 3
# HELP example_global_value Example of a top-level global value scrape in the json
# TYPE example_global_value untyped
example_global_value{environment="beta",location="planet-mars"} 1234
# HELP example_timestamped_value Example of a timestamped value scrape in the json
# TYPE example_timestamped_value untyped
example_timestamped_value{environment="beta"} 2 1657568506000
# HELP example_value_active Example of sub-level value scrapes from a json
# TYPE example_value_active untyped
example_value_active{environment="beta",id="id-A"} 1
example_value_active{environment="beta",id="id-C"} 1
# HELP example_value_boolean Example of sub-level value scrapes from a json
# TYPE example_value_boolean untyped
example_value_boolean{environment="beta",id="id-A"} 1
example_value_boolean{environment="beta",id="id-C"} 0
# HELP example_value_count Example of sub-level value scrapes from a json
# TYPE example_value_count untyped
example_value_count{environment="beta",id="id-A"} 1
example_value_count{environment="beta",id="id-C"} 3


## TEST with a different module for different json file

$ curl "http://localhost:7979/probe?module=animals&target=http://localhost:8000/examples/animal-data.json"
# HELP animal_cel_population Example of top-level lists in a separate module
# TYPE animal_cel_population untyped
animal_cel_population{name="deer",predator="false"} 456
animal_cel_population{name="lion",predator="true"} 123
animal_cel_population{name="pigeon",predator="false"} 789
# HELP animal_population Example of top-level lists in a separate module
# TYPE animal_population untyped
animal_population{name="deer",predator="false"} 456
animal_population{name="lion",predator="true"} 123
animal_population{name="pigeon",predator="false"} 789


## TEST through prometheus:

$ docker run --rm -it -p 9090:9090 -v $PWD/examples/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus
```
Then head over to http://localhost:9090/graph?g0.range_input=1h&g0.expr=example_value_active&g0.tab=1 or http://localhost:9090/targets to check the scraped metrics or the targets.

## Expression engines

Every metric is scraped with one of two expression engines, selected by the
`engine` field of the metric:

| `engine`             | Expressions                                                                             |
|----------------------|-----------------------------------------------------------------------------------------|
| `jsonpath` (default) | [JSONPath](https://kubernetes.io/docs/reference/kubectl/jsonpath/), e.g. `{ .counter }`  |
| `cel`                | [CEL](https://github.com/google/cel-spec/blob/master/doc/langdef.md), e.g. `.counter`    |

Both engines can be mixed freely within a module, but a single metric uses one
engine for all of its expressions: its `path`, its `labels` and its `values`.

### Using CEL

The members of the scraped JSON document are bound as variables, so a value is
selected by naming it, and the document itself is bound as `root`:

```yaml
- name: example_cel_global_value
  engine: cel
  path: '.counter'                     # 'counter' and 'root.counter' are equivalent
  valuetype: gauge
  labels:
    environment: '"beta"'              # a static label is a CEL string literal
    location: '"planet-" + .location'  # a dynamic label is any expression
```

`root` is what makes documents that are not JSON objects reachable, such as a
top-level list:

```yaml
- name: animal_cel
  engine: cel
  type: object
  path: 'root'
  labels:
    name: '.noun'
  values:
    population: '.population'
```

Note that labels and values are expressions as well, which is why a constant
label has to be quoted twice: `'"beta"'` is the CEL string literal `"beta"`,
while `'beta'` would be a reference to a member named `beta`.

With `allow_missing_key: true`, an expression that refers to something the
document does not contain — an unknown member, an unknown key, an index out of
bounds — skips the metric instead of reporting an error. Any other evaluation
error, such as a type mismatch, is still reported.

Expressions are parsed, but not type-checked, when the config is loaded, since
the shape of the scraped document is only known at scrape time. Use
`--config.check` to verify that all expressions of a config file parse.

## Using custom timestamps

This exporter allows you to use a field of the metric as the (unix/epoch) timestamp for the data, as an int64 in milliseconds. The timestamp is looked up in the same document as the value, so a top-level timestamp can only be used by a value scrape. However, this may lead to unexpected behaviour, as the prometheus implements a [Staleness](https://prometheus.io/docs/prometheus/latest/querying/basics/#staleness) mechanism.

:warning: Including timestamps in metrics disables the staleness handling and can make data visible for longer than expected.

## Exposing metrics through HTTPS

TLS configuration supported by this exporter can be found at [exporter-toolkit/web](https://github.com/prometheus/exporter-toolkit/blob/v0.9.0/docs/web-configuration.md)

## Sending body content for HTTP `POST`

If `modules.<module_name>.body` paramater is set in config, it will be sent by the exporter as the body content in the scrape request. The HTTP method will also be set as 'POST' in this case.
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
$ docker run -v $PWD/examples/config.yml:/config.yml quay.io/prometheuscommunity/json-exporter --config.file=/config.yml
```
