package main

import (
	"github.com/kawamuray/prometheus-exporter-harness/harness"
	"github.com/peakgames/prometheus-json-exporter/jsonexporter"
)

func main() {
	opts := harness.NewExporterOpts("json_exporter", jsonexporter.Version)
	opts.Usage = "[OPTIONS] HTTP_ENDPOINT|JSON_FILE CONFIG_PATH"
	opts.Init = jsonexporter.Init
	harness.Main(opts)
}
