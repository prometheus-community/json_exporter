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
