package harness

import (
	"github.com/urfave/cli"
	"os"
)

var (
	defaultOpts = []cli.Flag{
		cli.IntFlag{
			Name:  "port",
			Usage: "The port number used to expose metrics via http",
			Value: 7979,
		},
		cli.StringFlag{
			Name:  "log-level",
			Usage: "Set Logging level",
			Value: "info",
		},
	}

	defaultTickOpt = cli.IntFlag{
		Name:  "interval",
		Usage: "Interval to fetch metrics from the endpoint in second",
		Value: 60,
	}
)

func MakeApp(opts *ExporterOpts) *cli.App {
	exp := &exporter{opts}

	app := cli.NewApp()
	app.Name = opts.Name
	app.Version = opts.Version
	app.Usage = "A prometheus " + opts.Name
	app.UsageText = opts.Usage
	app.Action = exp.main
	app.Flags = buildOptsWithDefault(opts.Flags, defaultOpts)

	if opts.Tick && !contains(app.Flags, defaultTickOpt) {
		app.Flags = append(app.Flags, defaultTickOpt)
	}

	return app
}

func buildOptsWithDefault(opts []cli.Flag, defaultOpts []cli.Flag) []cli.Flag {
	for _, opt := range defaultOpts {
		if !contains(opts, opt) {
			opts = append(opts, opt)
		}
	}
	return opts
}

func contains(opts []cli.Flag, opt cli.Flag) bool {
	for _, o := range opts {
		if o.GetName() == opt.GetName() {
			return true
		}
	}
	return false
}

func Main(opts *ExporterOpts) {
	err := MakeApp(opts).Run(os.Args)
	if err != nil {
		os.Exit(1)
	}
}
