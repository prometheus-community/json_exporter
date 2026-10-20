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
	"regexp"
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
	AllowMissingKey        bool
	KeyFilter              *regexp.Regexp
	ValueFilter            *regexp.Regexp
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

			if floatValue, err := SanitizeValue(value); err == nil {
				metric := prometheus.MustNewConstMetric(
					m.Desc,
					m.ValueType,
					floatValue,
					extractLabels(mc.Logger, mc.Data, m.LabelsJSONPaths)...,
				)
				ch <- timestampMetric(mc.Logger, m, mc.Data, metric)
			} else {
				mc.Logger.Error("Failed to convert extracted value to float64", "path", m.KeyJSONPath, "value", value, "err", err, "metric", m.Desc)
				continue
			}

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
					jdata, err := json.Marshal(data)
					if err != nil {
						mc.Logger.Error("Failed to marshal data to json", "path", m.ValueJSONPath, "err", err, "metric", m.Desc, "data", data)
						continue
					}
					if m.KeyFilter != nil && !matchesKey(m.KeyFilter, data) {
						continue
					}

					// Use dynamic label extraction to support object keys as labels
					dynamicLabels := extractDynamicLabels(mc.Logger, data, m.LabelsJSONPaths)

					value, missing, err := extractDynamicValue(mc.Logger, data, m.ValueJSONPath, m.AllowMissingKey)
					if err != nil {
						mc.Logger.Error("Failed to extract value for metric", "path", m.ValueJSONPath, "err", err, "metric", m.Desc)
						continue
					}
					if missing {
						continue
					}
					if m.ValueFilter != nil && !m.ValueFilter.MatchString(value) {
						continue
					}

					if floatValue, err := SanitizeValue(value); err == nil {
						metric := prometheus.MustNewConstMetric(
							m.Desc,
							m.ValueType,
							floatValue,
							dynamicLabels...,
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

var quietLogger = slog.New(slog.DiscardHandler)

// matchesKey reports whether the key of a single-key object matches the filter.
// Objects with several keys are ambiguous (label and value come from one key), so they never match.
func matchesKey(filter *regexp.Regexp, data interface{}) bool {
	objMap, ok := data.(map[string]interface{})
	if !ok || len(objMap) != 1 {
		return false
	}
	for key := range objMap {
		return filter.MatchString(key)
	}
	return false
}

// extractDynamicLabels handles extraction of labels including dynamic object keys
func extractDynamicLabels(logger *slog.Logger, data interface{}, paths []string) []string {
	labels := make([]string, len(paths))
	for i, path := range paths {
		if path == "{__name__}" {
			// Special path to extract object key as label
			if objMap, ok := data.(map[string]interface{}); ok {
				for key := range objMap {
					labels[i] = key
					break // Take the first key as label
				}
			}
		} else {
			// Try to extract from original data first (for regular objects)
			jdata, err := json.Marshal(data)
			if err != nil {
				logger.Error("Failed to marshal data for label extraction", "err", err, "data", data)
				continue
			}

			attemptLogger := logger
			if _, isObj := data.(map[string]interface{}); isObj {
				attemptLogger = quietLogger
			}
			if result, _, err := extractValue(attemptLogger, jdata, path, false, false); err == nil {
				labels[i] = result
			} else {
				// If that fails and this is a dynamic object, try extracting from nested values
				if objMap, ok := data.(map[string]interface{}); ok {
					found := false
					for _, value := range objMap {
						nestedData, err := json.Marshal(value)
						if err != nil {
							continue
						}
						if result, _, err := extractValue(quietLogger, nestedData, path, false, false); err == nil {
							labels[i] = result
							found = true
							break
						}
					}
					if !found {
						logger.Error("Failed to extract label value from any nested object", "path", path, "data", data)
					}
				} else {
					logger.Error("Failed to extract label value", "err", err, "path", path, "data", data)
				}
			}
		}
	}
	return labels
}

// extractDynamicValue handles extraction of values from dynamic objects
func extractDynamicValue(logger *slog.Logger, data interface{}, path string, allowMissingKey bool) (string, bool, error) {
	// Try to extract from original data first (for regular objects)
	jdata, err := json.Marshal(data)
	if err != nil {
		logger.Error("Failed to marshal data for value extraction", "err", err, "data", data)
		return "", false, err
	}

	// Attempts on dynamic objects are expected to fail before the fallback, so keep them quiet
	_, isObj := data.(map[string]interface{})
	attemptLogger := logger
	if isObj {
		attemptLogger = quietLogger
	}

	if result, missing, err := extractValue(attemptLogger, jdata, path, false, allowMissingKey); err == nil && !missing {
		return result, false, nil
	}

	// If that fails and this is a dynamic object, try extracting from nested values
	if objMap, ok := data.(map[string]interface{}); ok {
		for _, value := range objMap {
			nestedData, err := json.Marshal(value)
			if err != nil {
				continue
			}
			if result, missing, err := extractValue(quietLogger, nestedData, path, false, allowMissingKey); err == nil && !missing {
				return result, false, nil
			}
		}
	}

	if allowMissingKey {
		return "", true, nil
	}
	return "", false, fmt.Errorf("value not found for path: %s", path)
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
