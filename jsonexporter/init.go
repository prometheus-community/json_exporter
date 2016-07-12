package jsonexporter

import (
	"fmt"
	"github.com/urfave/cli"
	"github.com/kawamuray/prometheus-exporter-harness/harness"
	"github.com/prometheus/client_golang/prometheus"
)

type ScrapeType struct {
	Configure  func(*Config, *harness.MetricRegistry)
	NewScraper func(*Config) (JsonScraper, error)
}

var ScrapeTypes = map[string]*ScrapeType{
	"object": {
		Configure: func(config *Config, reg *harness.MetricRegistry) {
			for subName := range config.Values {
				name := harness.MakeMetricName(config.Name, subName)
				reg.Register(
					name,
					prometheus.NewGaugeVec(prometheus.GaugeOpts{
						Name: name,
						Help: config.Help + " - " + subName,
					}, config.labelNames()),
				)
			}
		},
		NewScraper: NewObjectScraper,
	},
	"value": {
		Configure: func(config *Config, reg *harness.MetricRegistry) {
			reg.Register(
				config.Name,
				prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name: config.Name,
					Help: config.Help,
				}, config.labelNames()),
			)
		},
		NewScraper: NewValueScraper,
	},
}

var DefaultScrapeType = "value"

func Init(c *cli.Context, reg *harness.MetricRegistry) (harness.Collector, error) {
	args := c.Args()

	if len(args) < 2 {
		cli.ShowAppHelp(c)
		return nil, fmt.Errorf("not enough arguments")
	}

	var (
		endpoint   = args[0]
		configPath = args[1]
	)

	configs, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	scrapers := make([]JsonScraper, len(configs))
	for i, config := range configs {
		tpe := ScrapeTypes[config.Type]
		if tpe == nil {
			return nil, fmt.Errorf("unknown scrape type;type:<%s>", config.Type)
		}
		tpe.Configure(config, reg)
		scraper, err := tpe.NewScraper(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create scraper;name:<%s>,err:<%s>", config.Name, err)
		}
		scrapers[i] = scraper
	}

	return NewCollector(endpoint, scrapers), nil
}
