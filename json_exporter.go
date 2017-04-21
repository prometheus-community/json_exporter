package main

import (
	"github.com/kawamuray/prometheus-exporter-harness/harness"
	"github.com/kawamuray/prometheus-json-exporter/jsonexporter"
)

func main() {
	opts := harness.NewExporterOpts("json_exporter", jsonexporter.Version)
	opts.Usage = "[OPTIONS] CONFIG_PATH"
	opts.Init = jsonexporter.Init
	harness.Main(opts)
}
