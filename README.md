prometheus-json-exporter
========================

A [prometheus](https://prometheus.io/) exporter which scrapes remote JSON by JSONPath.

Build
=====
```sh
./gow get .
./gow build -o json_exporter .
```

Example Usage
=============
```sh
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

See Also
========
- [nicksardo/jsonpath](https://github.com/nicksardo/jsonpath) : For syntax reference of JSONPath
