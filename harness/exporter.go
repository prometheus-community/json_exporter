package harness

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const DefaultMetricsPath = "/metrics"

type ExporterOpts struct {
	// The representative name of exporter
	Name string
	// The version of exporter
	Version string
	// The HTTP endpoint path which used to provide metrics
	MetricsPath string
	// Whether to call Collect() of collector periodically
	Tick bool
	// Whether to reset all metrics per tick
	ResetOnTick bool
	// Command line usage
	Usage string
	// Additional command line flags which can be accepted
	Flags []cli.Flag
	// Function to instantiate collector
	Init func(*cli.Context, *MetricRegistry) (Collector, error)
}

func NewExporterOpts(name string, version string) *ExporterOpts {
	return &ExporterOpts{
		Name:        name,
		Version:     version,
		MetricsPath: DefaultMetricsPath,
		Tick:        true,
		ResetOnTick: true,
		Usage:       "",
	}
}

type exporter struct {
	*ExporterOpts
}

func setupLogging(c *cli.Context) {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	levelString := c.String("log-level")
	level, err := log.ParseLevel(levelString)
	if err != nil {
		log.Fatalf("could not set log level to '%s';err:<%s>", levelString, err)
	}
	log.SetLevel(level)
}

func (exp *exporter) main(c *cli.Context) {
	setupLogging(c)

	registry := newRegistry()

	collector, err := exp.Init(c, registry)
	if err != nil {
		log.Fatal(err)
	}

	if exp.Tick {
		collector.Collect(registry)
		interval := c.Int("interval")
		go func() {
			for _ = range time.Tick(time.Duration(interval) * time.Second) {
				if exp.ResetOnTick {
					registry.Reset()
				}
				collector.Collect(registry)
			}
		}()
	}

	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Add("Location", exp.MetricsPath)
		w.WriteHeader(http.StatusFound)
	})
	http.Handle(exp.MetricsPath, promhttp.Handler())
	if err := http.ListenAndServe(fmt.Sprintf(":%d", c.Int("port")), nil); err != nil {
		log.Fatal(err)
	}
}
