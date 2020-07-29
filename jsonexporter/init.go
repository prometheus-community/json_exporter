// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package jsonexporter

import (
	"fmt"

	"github.com/prometheus-community/json_exporter/harness"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/urfave/cli"
)

type ScrapeType struct {
	Configure  func(*Metric, *harness.MetricRegistry)
	NewScraper func(*Metric) (JsonScraper, error)
}

var ScrapeTypes = map[string]*ScrapeType{
	"object": {
		Configure: func(metric *Metric, reg *harness.MetricRegistry) {
			for subName := range metric.Values {
				name := harness.MakeMetricName(metric.Name, subName)
				reg.Register(
					name,
					prometheus.NewGaugeVec(prometheus.GaugeOpts{
						Name: name,
						Help: metric.Help + " - " + subName,
					}, metric.labelNames()),
				)
			}
		},
		NewScraper: NewObjectScraper,
	},
	"value": {
		Configure: func(metric *Metric, reg *harness.MetricRegistry) {
			reg.Register(
				metric.Name,
				prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name: metric.Name,
					Help: metric.Help,
				}, metric.labelNames()),
			)
		},
		NewScraper: NewValueScraper,
	},
}

var DefaultScrapeType = "value"

func Init(c *cli.Context, reg *harness.MetricRegistry) (harness.Collector, error) {
	args := c.Args()

	if len(args) < 2 {
		cli.ShowAppHelp(c) //nolint:errcheck
		return nil, fmt.Errorf("not enough arguments")
	}

	var (
		endpoint   = args[0]
		configPath = args[1]
	)

	config, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	scrapers := make([]JsonScraper, len(config.Metrics))
	for i, metric := range config.Metrics {
		tpe := ScrapeTypes[metric.Type]
		if tpe == nil {
			return nil, fmt.Errorf("unknown scrape type;type:<%s>", metric.Type)
		}
		tpe.Configure(&config.Metrics[i], reg)
		scraper, err := tpe.NewScraper(&config.Metrics[i])
		if err != nil {
			return nil, fmt.Errorf("failed to create scraper;name:<%s>,err:<%s>", metric.Name, err)
		}
		scrapers[i] = scraper
	}

	return NewCollector(endpoint, config.Headers, scrapers), nil
}
