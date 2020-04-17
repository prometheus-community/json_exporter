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

type Config struct {
	Name   string            `yaml:name`
	Path   string            `yaml:path`
	Labels map[string]string `yaml:labels`
	Type   string            `yaml:type`
	Help   string            `yaml:help`
	Values map[string]string `yaml:values`
}

func (config *Config) labelNames() []string {
	labelNames := make([]string, 0, len(config.Labels))
	for name := range config.Labels {
		labelNames = append(labelNames, name)
	}
	return labelNames
}

func loadConfig(configPath string) ([]*Config, error) {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config;path:<%s>,err:<%s>", configPath, err)
	}

	var configs []*Config
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("failed to parse yaml;err:<%s>", err)
	}
	// Complete defaults
	for _, config := range configs {
		if config.Type == "" {
			config.Type = DefaultScrapeType
		}
		if config.Help == "" {
			config.Help = config.Name
		}
	}

	return configs, nil
}
