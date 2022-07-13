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

package exporter

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus-community/json_exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/util/jsonpath"
)

type JSONMetricCollector struct {
	JSONMetrics []JSONMetric
	Data        []byte
	Logger      log.Logger
}

type JSONMetric struct {
	Desc            *prometheus.Desc
	Type            config.ScrapeType
	KeyJSONPath     string
	ValueJSONPath   string
	LabelsJSONPaths []string
	ValueType       prometheus.ValueType
	ValueConverter	map[string]map[string]string
}

func (mc JSONMetricCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range mc.JSONMetrics {
		ch <- m.Desc
	}
}

func (mc JSONMetricCollector) Collect(ch chan<- prometheus.Metric) {
	for _, m := range mc.JSONMetrics {
		switch m.Type {
		case config.ValueScrape:
			value, err := extractValue(mc.Logger, mc.Data, m.KeyJSONPath, false)
			if err != nil {
				level.Error(mc.Logger).Log("msg", "Failed to extract value for metric", "path", m.KeyJSONPath, "err", err, "metric", m.Desc)
				continue
			}

			if floatValue, err := SanitizeValue(value); err == nil {

				ch <- prometheus.MustNewConstMetric(
					m.Desc,
					m.ValueType,
					floatValue,
					extractLabels(mc.Logger, mc.Data, m.LabelsJSONPaths)...,
				)
			} else {
				level.Error(mc.Logger).Log("msg", "Failed to convert extracted value to float64", "path", m.KeyJSONPath, "value", value, "err", err, "metric", m.Desc)
				continue
			}

		case config.ObjectScrape:
			values, err := extractValue(mc.Logger, mc.Data, m.KeyJSONPath, true)
			if err != nil {
				level.Error(mc.Logger).Log("msg", "Failed to extract json objects for metric", "err", err, "metric", m.Desc)
				continue
			}

			var jsonData []interface{}
			if err := json.Unmarshal([]byte(values), &jsonData); err == nil {
				for _, data := range jsonData {
					jdata, err := json.Marshal(data)
					if err != nil {
						level.Error(mc.Logger).Log("msg", "Failed to marshal data to json", "path", m.ValueJSONPath, "err", err, "metric", m.Desc, "data", data)
						continue
					}
					value, err := extractValue(mc.Logger, jdata, m.ValueJSONPath, false)

					if err != nil {
						level.Error(mc.Logger).Log("msg", "Failed to extract value for metric", "path", m.ValueJSONPath, "err", err, "metric", m.Desc)
						continue
					}
					
					//convert dynamic value if it's in the valueconverter
					if m.ValueConverter != nil {
						if value_mappings, ok := m.ValueConverter[m.ValueJSONPath]; ok {
							value = strings.ToLower(value)

							if _, ok := value_mappings[value]; ok { 
								value = value_mappings[value]
							}
						}
					}

					if floatValue, err := SanitizeValue(value); err == nil {
						ch <- prometheus.MustNewConstMetric(
							m.Desc,
							m.ValueType,
							floatValue,
							extractLabels(mc.Logger, jdata, m.LabelsJSONPaths)...,
						)
					} else {
						level.Error(mc.Logger).Log("msg", "Failed to convert extracted value to float64", "path", m.ValueJSONPath, "value", value, "err", err, "metric", m.Desc)
						continue
					}
				}
			} else {
				level.Error(mc.Logger).Log("msg", "Failed to convert extracted objects to json", "err", err, "metric", m.Desc)
				continue
			}
		default:
			level.Error(mc.Logger).Log("msg", "Unknown scrape config type", "type", m.Type, "metric", m.Desc)
			continue
		}
	}
}

// Returns the last matching value at the given json path
func extractValue(logger log.Logger, data []byte, path string, enableJSONOutput bool) (string, error) {
	var jsonData interface{}
	buf := new(bytes.Buffer)

	j := jsonpath.New("jp")
	if enableJSONOutput {
		j.EnableJSONOutput(true)
	}

	if err := json.Unmarshal(data, &jsonData); err != nil {
		level.Error(logger).Log("msg", "Failed to unmarshal data to json", "err", err, "data", data)
		return "", err
	}

	if err := j.Parse(path); err != nil {
		level.Error(logger).Log("msg", "Failed to parse jsonpath", "err", err, "path", path, "data", data)
		return "", err
	}

	if err := j.Execute(buf, jsonData); err != nil {
		level.Error(logger).Log("msg", "Failed to execute jsonpath", "err", err, "path", path, "data", data)
		return "", err
	}

	// Since we are finally going to extract only float64, unquote if necessary
	if res, err := jsonpath.UnquoteExtend(buf.String()); err == nil {
		return res, nil
	}

	return buf.String(), nil
}

// Returns the list of labels created from the list of provided json paths
func extractLabels(logger log.Logger, data []byte, paths []string) []string {
	labels := make([]string, len(paths))
	for i, path := range paths {
		if result, err := extractValue(logger, data, path, false); err == nil {
			labels[i] = result
		} else {
			level.Error(logger).Log("msg", "Failed to extract label value", "err", err, "path", path, "data", data)
		}
	}
	return labels
}
