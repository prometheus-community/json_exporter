json_exporter
========================
[![Build Status](https://github.com/prometheus-community/json_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/prometheus-community/json_exporter/actions/workflows/ci.yml)


A [prometheus](https://prometheus.io/) exporter which scrapes remote JSON by JSONPath.

- [Supported JSONPath Syntax](https://kubernetes.io/docs/reference/kubectl/jsonpath/)
- [Examples configurations](/examples)

## Dynamic object keys

Extract dynamic object keys as Prometheus labels using the special JSONPath `{__name__}`:

```yaml
labels:
  provider_name: '{__name__}'  # Extracts dynamic object key as label
```

**Perfect for monitoring APIs with dynamic object structures:**
- Cloud providers with dynamic instance/account names  
- Service discovery endpoints with generated names
- Infrastructure components with environment-specific identifiers

**Example:**
```json
{"providers": [{"aws-prod-123": {"ok": true}}, {"gcp-dev-456": {"ok": false}}]}
```

Generates:
```
provider_status{provider_name="aws-prod-123"} 1
provider_status{provider_name="gcp-dev-456"} 0
```

### Anatomy of a dynamic-key object

Given `examples/health-data.json`, the path `{.regions[*]}` yields one object per list item:

```
{ "alpha__svc_b" : { "ok": false, "error": "Timeout contacting 10.0.0.10", "href": "/admin/region/2/" } }
    └──── {__name__} ────┘   └──────────────────────────── value (nested object) ───────────────────────────┘
                                     └ {.ok}                └ {.error} (don't use as label)   └ {.href}
```

Each item must be an object with a **single** key. Items with several keys are ambiguous and are never matched by `key_filter`; items that are not objects are skipped when `key_filter` is set.

| What you want | Where it goes | Expression | Result for the object above |
|---|---|---|---|
| The dynamic key | `labels` | `'{__name__}'` | `region="alpha__svc_b"` |
| A field of the value as a label | `labels` | `'{.href}'` | `href="/admin/region/2/"` |
| A field of the value as the metric value | `values` | `'{.ok}'` | `0` (`true`/`false` become `1`/`0`) |
| Keep only some keys | `key_filter` | regex on the key | `'^alpha__'` keeps this entry |
| Keep only some values | `value_filter` | regex on the extracted value | `'^false$'` keeps this entry |

`key_filter` and `value_filter` apply to `type: object` metrics only (using them on a `type: value` metric is a config error) and can be combined. `value_filter` is matched against the value of each entry in `values` (the string before it is converted to a number, so booleans match `true`/`false`). Entries that do not match are skipped. Do not use free-text fields such as `error` as labels: they can be large and have unbounded cardinality.

Ready-to-run examples (value, dynamic key, key filter, value filter, both filters, mixed labels, non-2xx status) are in [`examples/health-config.yml`](examples/health-config.yml), using [`examples/health-data.json`](examples/health-data.json):

```yaml
- name: region
  type: object
  path: '{.regions[*]}'
  key_filter: '^alpha__'    # only the "alpha" group
  value_filter: '^false$'     # only failing entries
  labels:
    region: '{__name__}'
  values:
    up: '{.ok}'
```

```
region_up{region="alpha__svc_b"} 0
```

If the endpoint returns a non-2xx status together with a valid JSON body (e.g. 418 when any check fails), list the accepted codes with `valid_status_codes: [200, 418]` in the module.

## Example Usage

```console
## SETUP

$ make build
$ ./json_exporter --config.file examples/config.yml &
$ python3 -m http.server 8000 &
Serving HTTP on :: port 8000 (http://[::]:8000/) ...


## TEST with 'default' module

$ curl "http://localhost:7979/probe?module=default&target=http://localhost:8000/examples/data.json"
# HELP example_global_value Example of a top-level global value scrape in the json
# TYPE example_global_value untyped
example_global_value{environment="beta",location="planet-mars"} 1234
# HELP example_timestamped_value_count Example of a timestamped value scrape in the json
# TYPE example_timestamped_value_count untyped
example_timestamped_value_count{environment="beta"} 2
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
# HELP animal_population Example of top-level lists in a separate module
# TYPE animal_population untyped
animal_population{name="deer",predator="false"} 456
animal_population{name="lion",predator="true"} 123
animal_population{name="pigeon",predator="false"} 789


## TEST through prometheus:

$ docker run --rm -it -p 9090:9090 -v $PWD/examples/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus
```
Then head over to http://localhost:9090/graph?g0.range_input=1h&g0.expr=example_value_active&g0.tab=1 or http://localhost:9090/targets to check the scraped metrics or the targets.

## Using custom timestamps

This exporter allows you to use a field of the metric as the (unix/epoch) timestamp for the data as an int64. However, this may lead to unexpected behaviour, as the prometheus implements a [Staleness](https://prometheus.io/docs/prometheus/latest/querying/basics/#staleness) mechanism.

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
