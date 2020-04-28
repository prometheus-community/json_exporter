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
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

// Metric contains values that define a metric
type Metric struct {
	Name   string
	Path   string
	Labels map[string]string
	Type   string
	Help   string
	Values map[string]string
}

// Header containes a key and a value for a header
type Header map[string]string

// Config contains metrics and headers defining
// a configuration
type Config struct {
	Headers []Header         
	Metrics []Metric
}

func (metric *Metric) labelNames() []string {
	labelNames := make([]string, 0, len(metric.Labels))
	for name := range metric.Labels {
		labelNames = append(labelNames, name)
	}
	return labelNames
}

func loadConfig(configPath string) (*Config, error) {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config;path:<%s>,err:<%s>", configPath, err)
	}

	var config *Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse yaml;err:<%s>", err)
	}

	// Complete Defaults
	for i := 0; i < len(config.Metrics); i++ {
		if config.Metrics[i].Type == "" {
			config.Metrics[i].Type = DefaultScrapeType
		}
		if config.Metrics[i].Help == "" {
			config.Metrics[i].Help = config.Metrics[i].Name
		}
	}

	return config, nil
}
