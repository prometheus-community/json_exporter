package jsonexporter

import (
	"fmt"
	"github.com/urfave/cli"
	"github.com/kawamuray/prometheus-exporter-harness/harness"
	"github.com/prometheus/client_golang/prometheus"
)

type Module struct {
	endpoint  string
	headers   map[string]string
	scrapers  []JsonScraper
}

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

	moduleConfigs, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	modules := make([]*Module, len(moduleConfigs))
	for i, moduleConfig := range moduleConfigs {
        modules[i] = &Module{endpoint: moduleConfig.Endpoint, headers: moduleConfig.Headers}
		modules[i].scrapers = make([]JsonScraper, len(moduleConfig.Mappings))
		for j, mapping := range moduleConfig.Mappings {
			tpe := ScrapeTypes[mapping.Type]
			if tpe == nil {
				return nil, fmt.Errorf("unknown scrape type;type:<%s>", mapping.Type)
			}
			tpe.Configure(mapping, reg)
			scraper, err := tpe.NewScraper(mapping)
			if err != nil {
				return nil, fmt.Errorf("failed to create scraper;name:<%s>,err:<%s>", mapping.Name, err)
			}
			modules[i].scrapers[j] = scraper
		}
	}

	return NewCollector(modules), nil
}
