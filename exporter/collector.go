// Copyright 2025 abgharbi
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
	ValuesMap              map[string]float64
	ValueType              prometheus.ValueType
	EpochTimestampJSONPath string
	AllowMissingKey        bool
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
			value, missing, err := extractValue(mc.Logger, mc.Data, m.KeyJSONPath, false, m.AllowMissingKey)
			if err != nil {
				mc.Logger.Error("Failed to extract value for metric", "path", m.KeyJSONPath, "err", err, "metric", m.Desc)
				continue
			}
			if missing {
				continue
			}

			floatValue, err := resolveValue(value, m.ValuesMap)
			if err != nil {
				mc.Logger.Error("Failed to convert extracted value to float64", "path", m.KeyJSONPath, "value", value, "err", err, "metric", m.Desc)
				continue
			}
			metric := prometheus.MustNewConstMetric(
				m.Desc,
				m.ValueType,
				floatValue,
				extractLabels(mc.Logger, mc.Data, m.LabelsJSONPaths)...,
			)
			ch <- timestampMetric(mc.Logger, m, mc.Data, metric)

		case config.ObjectScrape:
			values, missing, err := extractValue(mc.Logger, mc.Data, m.KeyJSONPath, true, m.AllowMissingKey)
			if err != nil {
				mc.Logger.Error("Failed to extract json objects for metric", "err", err, "metric", m.Desc)
				continue
			}
			if missing {
				continue
			}

			var jsonData []interface{}
			if err := json.Unmarshal([]byte(values), &jsonData); err == nil {
				for _, data := range jsonData {
					// Flat JSON object with {@key}/{@value} iteration (e.g. {"app1":5,"app2":3})
					if mapData, ok := data.(map[string]interface{}); ok && m.ValueJSONPath == "{@value}" {
						for k, v := range mapData {
							floatValue, err := interfaceToFloat64(v)
							if err != nil {
								mc.Logger.Error("Failed to convert map value to float64", "key", k, "value", v, "err", err, "metric", m.Desc)
								continue
							}
							labels := make([]string, len(m.LabelsJSONPaths))
							for i, lp := range m.LabelsJSONPaths {
								switch lp {
								case "{@key}":
									labels[i] = k
								case "{@value}":
									labels[i] = fmt.Sprintf("%v", v)
								default:
									jdata, _ := json.Marshal(data)
									if result, _, err := extractValue(mc.Logger, jdata, lp, false, m.AllowMissingKey); err == nil {
										labels[i] = result
									}
								}
							}
							metric := prometheus.MustNewConstMetric(m.Desc, m.ValueType, floatValue, labels...)
							ch <- metric
						}
						continue
					}

					jdata, err := json.Marshal(data)
					if err != nil {
						mc.Logger.Error("Failed to marshal data to json", "path", m.ValueJSONPath, "err", err, "metric", m.Desc, "data", data)
						continue
					}
					value, missing, err := extractValue(mc.Logger, jdata, m.ValueJSONPath, false, m.AllowMissingKey)
					if err != nil {
						mc.Logger.Error("Failed to extract value for metric", "path", m.ValueJSONPath, "err", err, "metric", m.Desc)
						continue
					}
					if missing {
						continue
					}

					if floatValue, err := SanitizeValue(value); err == nil {
						metric := prometheus.MustNewConstMetric(
							m.Desc,
							m.ValueType,
							floatValue,
							extractLabels(mc.Logger, jdata, m.LabelsJSONPaths)...,
						)
						ch <- timestampMetric(mc.Logger, m, jdata, metric)
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

// Returns the last matching value at the given json path and a flag if the path was missing
func extractValue(logger *slog.Logger, data []byte, path string, enableJSONOutput, allowMissingKey bool) (string, bool, error) {
	var jsonData interface{}
	buf := new(bytes.Buffer)

	j := jsonpath.New("jp")
	if enableJSONOutput {
		j.EnableJSONOutput(true)
	}
	if allowMissingKey {
		j.AllowMissingKeys(true)
	}

	if err := json.Unmarshal(data, &jsonData); err != nil {
		logger.Error("Failed to unmarshal data to json", "err", err, "data", data)
		return "", false, err
	}

	if err := j.Parse(path); err != nil {
		logger.Error("Failed to parse jsonpath", "err", err, "path", path, "data", data)
		return "", false, err
	}

	if err := j.Execute(buf, jsonData); err != nil {
		logger.Error("Failed to execute jsonpath", "err", err, "path", path, "data", data)
		return "", false, err
	}
	if buf.Len() == 0 && allowMissingKey {
		return "", true, nil
	}

	// Since we are finally going to extract only float64, unquote if necessary
	if res, err := jsonpath.UnquoteExtend(buf.String()); err == nil {
		return res, false, nil
	}

	return buf.String(), false, nil
}

// Returns the list of labels created from the list of provided json paths
func extractLabels(logger *slog.Logger, data []byte, paths []string) []string {
	labels := make([]string, len(paths))
	for i, path := range paths {
		if result, _, err := extractValue(logger, data, path, false, false); err == nil {
			labels[i] = result
		} else {
			logger.Error("Failed to extract label value", "err", err, "path", path, "data", data)
		}
	}
	return labels
}

// resolveValue converts a string to float64, consulting the ValuesMap first.
func resolveValue(s string, valuesMap map[string]float64) (float64, error) {
	if v, ok := valuesMap[s]; ok {
		return v, nil
	}
	return SanitizeValue(s)
}

// interfaceToFloat64 converts an unmarshalled JSON value to float64.
func interfaceToFloat64(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case bool:
		if val {
			return 1.0, nil
		}
		return 0.0, nil
	case string:
		return SanitizeValue(val)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

func timestampMetric(logger *slog.Logger, m JSONMetric, data []byte, pm prometheus.Metric) prometheus.Metric {
	if m.EpochTimestampJSONPath == "" {
		return pm
	}
	ts, missing, err := extractValue(logger, data, m.EpochTimestampJSONPath, false, m.AllowMissingKey)
	if err != nil {
		logger.Error("Failed to extract timestamp for metric", "path", m.KeyJSONPath, "err", err, "metric", m.Desc)
		return pm
	}

	if missing {
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
