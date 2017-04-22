package jsonexporter

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

type ModuleConfig struct {
    Endpoint string            `yaml:"endpoint"`
    Headers  map[string]string `yaml:"headers"`
    Mappings []*Mapping        `yaml:"mappings"`
}

type Mapping struct {
	Name   string              `yaml:name`
	Path   string              `yaml:path`
	Labels map[string]string   `yaml:labels`
	Type   string              `yaml:type`
	Help   string              `yaml:help`
	Values map[string]string   `yaml:values`
}

func (mapping *Mapping) labelNames() []string {
	labelNames := make([]string, 0, len(mapping.Labels))
	for name := range mapping.Labels {
		labelNames = append(labelNames, name)
	}
	return labelNames
}

func loadConfig(configPath string) ([]*ModuleConfig, error) {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config;path:<%s>,err:<%s>", configPath, err)
	}

	var moduleConfigs []*ModuleConfig
	if err := yaml.Unmarshal(data, &moduleConfigs); err != nil {
		return nil, fmt.Errorf("failed to parse yaml;err:<%s>", err)
	}
	// Complete defaults
	for _, moduleConfig := range moduleConfigs {
		for _, mapping := range moduleConfig.Mappings {
			if mapping.Type == "" {
				mapping.Type = DefaultScrapeType
			}
			if mapping.Help == "" {
				mapping.Help = mapping.Name
			}
		}
	}

	return moduleConfigs, nil
}
