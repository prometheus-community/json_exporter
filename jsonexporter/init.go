package jsonexporter

import (
	"fmt"
	"github.com/urfave/cli"
	"github.com/kawamuray/prometheus-exporter-harness/harness"
	"github.com/prometheus/client_golang/prometheus"
)

type ScrapeType struct {
	Configure  func(*Mapping, *harness.MetricRegistry)
	NewScraper func(*Mapping) (JsonScraper, error)
}

var ScrapeTypes = map[string]*ScrapeType{
	"object": {
		Configure: func(mapping *Mapping, reg *harness.MetricRegistry) {
			for subName := range mapping.Values {
				name := harness.MakeMetricName(mapping.Name, subName)
				reg.Register(
					name,
					prometheus.NewGaugeVec(prometheus.GaugeOpts{
						Name: name,
						Help: mapping.Help + " - " + subName,
					}, mapping.labelNames()),
				)
			}
		},
		NewScraper: NewObjectScraper,
	},
	"value": {
		Configure: func(mapping *Mapping, reg *harness.MetricRegistry) {
			reg.Register(
				mapping.Name,
				prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name: mapping.Name,
					Help: mapping.Help,
				}, mapping.labelNames()),
			)
		},
		NewScraper: NewValueScraper,
	},
}

var DefaultScrapeType = "value"

func Init(c *cli.Context, reg *harness.MetricRegistry) (harness.Collector, error) {
	args := c.Args()

	if len(args) < 1 {
		cli.ShowAppHelp(c)
		return nil, fmt.Errorf("not enough arguments")
	}

	var (
		configPath = args[0]
	)

	config, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	scrapers := make([]JsonScraper, len(config.Mappings))
	for i, mapping := range config.Mappings {
		tpe := ScrapeTypes[mapping.Type]
		if tpe == nil {
			return nil, fmt.Errorf("unknown scrape type;type:<%s>", mapping.Type)
		}
		tpe.Configure(mapping, reg)
		scraper, err := tpe.NewScraper(mapping)
		if err != nil {
			return nil, fmt.Errorf("failed to create scraper;name:<%s>,err:<%s>", mapping.Name, err)
		}
		scrapers[i] = scraper
	}

	return NewCollector(config.Endpoint, config.Headers, scrapers), nil
}
