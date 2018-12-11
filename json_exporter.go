package main

import (
	"github.com/kawamuray/prometheus-exporter-harness/harness"
	"github.com/kawamuray/prometheus-json-exporter/jsonexporter"
	"github.com/urfave/cli"
)

func main() {
	opts := harness.NewExporterOpts("json_exporter", jsonexporter.Version)
	opts.Usage = "[OPTIONS] HTTP_ENDPOINT CONFIG_PATH"
	opts.Init = jsonexporter.Init
	opts.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "userName",
			Value: "",
			Usage: "User name for http basic auth.",
		},
		cli.StringFlag{
			Name:  "password",
			Value: "",
			Usage: "Password for http basic auth.",
		}}
	harness.Main(opts)
}
