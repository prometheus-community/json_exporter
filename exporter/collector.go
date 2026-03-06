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
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus-community/json_exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/util/jsonpath"
)

type JSONMetricCollector struct {
	JSONMetrics []JSONMetric
	Data        []byte
	Logger      *slog.Logger
}

type JSONMetric struct {
	Desc                   *prometheus.Desc
	Type                   config.ScrapeType
	KeyJSONPath            string
	ValueJSONPath          string
	LabelsJSONPaths        []string
	ValueType              prometheus.ValueType
	EpochTimestampJSONPath string

	keyParser       *jsonpath.JSONPath
	valueParser     *jsonpath.JSONPath
	labelsParsers   []*jsonpath.JSONPath
	timestampParser *jsonpath.JSONPath
}

func (mc JSONMetricCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range mc.JSONMetrics {
		ch <- m.Desc
	}
}

func (mc JSONMetricCollector) Collect(ch chan<- prometheus.Metric) {
	var jsonData interface{}
	if err := json.Unmarshal(mc.Data, &jsonData); err != nil {
		mc.Logger.Error("Failed to unmarshal data to json", "err", err)
		return
	}

	for _, m := range mc.JSONMetrics {
		switch m.Type {
		case config.ValueScrape:
			value, err := extractValue(mc.Logger, jsonData, m.KeyJSONPath, m.keyParser)
			if err != nil {
				mc.Logger.Error("Failed to extract value for metric", "path", m.KeyJSONPath, "err", err, "metric", m.Desc)
				continue
			}

			if floatValue, err := SanitizeValue(value); err == nil {
				metric := prometheus.MustNewConstMetric(
					m.Desc,
					m.ValueType,
					floatValue,
					extractLabels(mc.Logger, jsonData, m.LabelsJSONPaths, m.labelsParsers)...,
				)
				ch <- timestampMetric(mc.Logger, m, jsonData, metric)
			} else {
				mc.Logger.Error("Failed to convert extracted value to float64", "path", m.KeyJSONPath, "value", value, "err", err, "metric", m.Desc)
				continue
			}

		case config.ObjectScrape:
			values, err := extractValue(mc.Logger, jsonData, m.KeyJSONPath, m.keyParser)
			if err != nil {
				mc.Logger.Error("Failed to extract json objects for metric", "err", err, "metric", m.Desc)
				continue
			}

			var jsonData []interface{}
			if err := json.Unmarshal([]byte(values), &jsonData); err == nil {
				for _, data := range jsonData {
					value, err := extractValue(mc.Logger, data, m.ValueJSONPath, m.valueParser)
					if err != nil {
						mc.Logger.Error("Failed to extract value for metric", "path", m.ValueJSONPath, "err", err, "metric", m.Desc)
						continue
					}

					if floatValue, err := SanitizeValue(value); err == nil {
						metric := prometheus.MustNewConstMetric(
							m.Desc,
							m.ValueType,
							floatValue,
							extractLabels(mc.Logger, data, m.LabelsJSONPaths, m.labelsParsers)...,
						)
						ch <- timestampMetric(mc.Logger, m, data, metric)
					} else {
						mc.Logger.Error("Failed to convert extracted value to float64", "path", m.ValueJSONPath, "value", value, "err", err, "metric", m.Desc)
						continue
					}
				}
			} else {
				mc.Logger.Error("Failed to convert extracted objects to json", "err", err, "metric", m.Desc)
				continue
			}
		default:
			mc.Logger.Error("Unknown scrape config type", "type", m.Type, "metric", m.Desc)
			continue
		}
	}
}

func (m *JSONMetric) compileJSONPaths() error {
	var err error
	m.keyParser, err = compileJSONPath(m.KeyJSONPath, m.Type == config.ObjectScrape)
	if err != nil {
		return fmt.Errorf("compile key jsonpath %q: %w", m.KeyJSONPath, err)
	}

	if m.ValueJSONPath != "" {
		m.valueParser, err = compileJSONPath(m.ValueJSONPath, false)
		if err != nil {
			return fmt.Errorf("compile value jsonpath %q: %w", m.ValueJSONPath, err)
		}
	}

	m.labelsParsers = make([]*jsonpath.JSONPath, len(m.LabelsJSONPaths))
	for i, labelPath := range m.LabelsJSONPaths {
		m.labelsParsers[i], err = compileJSONPath(labelPath, false)
		if err != nil {
			return fmt.Errorf("compile label jsonpath %q: %w", labelPath, err)
		}
	}

	if m.EpochTimestampJSONPath != "" {
		m.timestampParser, err = compileJSONPath(m.EpochTimestampJSONPath, false)
		if err != nil {
			return fmt.Errorf("compile timestamp jsonpath %q: %w", m.EpochTimestampJSONPath, err)
		}
	}

	return nil
}

func compileJSONPath(path string, enableJSONOutput bool) (*jsonpath.JSONPath, error) {
	j := jsonpath.New("jp")
	if enableJSONOutput {
		j.EnableJSONOutput(true)
	}
	if err := j.Parse(path); err != nil {
		return nil, err
	}
	return j, nil
}

// Returns the last matching value at the given json path.
func extractValue(logger *slog.Logger, data interface{}, path string, parser *jsonpath.JSONPath) (string, error) {
	buf := new(bytes.Buffer)
	if err := parser.Execute(buf, data); err != nil {
		logger.Error("Failed to execute jsonpath", "err", err, "path", path, "data", data)
		return "", err
	}

	// Since we are finally going to extract only float64, unquote if necessary
	if res, err := jsonpath.UnquoteExtend(buf.String()); err == nil {
		return res, nil
	}

	return buf.String(), nil
}

// Returns the list of labels created from the list of provided json paths
func extractLabels(logger *slog.Logger, data interface{}, paths []string, parsers []*jsonpath.JSONPath) []string {
	labels := make([]string, len(paths))
	for i, path := range paths {
		if result, err := extractValue(logger, data, path, parsers[i]); err == nil {
			labels[i] = result
		} else {
			logger.Error("Failed to extract label value", "err", err, "path", path, "data", data)
		}
	}
	return labels
}

func timestampMetric(logger *slog.Logger, m JSONMetric, data interface{}, pm prometheus.Metric) prometheus.Metric {
	if m.EpochTimestampJSONPath == "" {
		return pm
	}
	ts, err := extractValue(logger, data, m.EpochTimestampJSONPath, m.timestampParser)
	if err != nil {
		logger.Error("Failed to extract timestamp for metric", "path", m.KeyJSONPath, "err", err, "metric", m.Desc)
		return pm
	}
	epochTime, err := SanitizeIntValue(ts)
	if err != nil {
		logger.Error("Failed to parse timestamp for metric", "path", m.KeyJSONPath, "err", err, "metric", m.Desc)
		return pm
	}
	timestamp := time.UnixMilli(epochTime)
	return prometheus.NewMetricWithTimestamp(timestamp, pm)
}
