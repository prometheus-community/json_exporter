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
	"reflect"
	"strconv"
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

	if enableJSONOutput {
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

	// For the non-JSON-output path we render the results ourselves instead of
	// relying on jsonpath's default text formatting, which prints float64 values
	// (all JSON numbers unmarshal to float64) using %v and thus renders large
	// integers in scientific notation, e.g. "1.655371e+06" instead of "1655371".
	fullResults, err := j.FindResults(jsonData)
	if err != nil {
		logger.Error("Failed to execute jsonpath", "err", err, "path", path, "data", data)
		return "", false, err
	}
	out := resultsToText(fullResults)
	if len(out) == 0 && allowMissingKey {
		return "", true, nil
	}

	// Since we are finally going to extract only float64, unquote if necessary
	if res, err := jsonpath.UnquoteExtend(out); err == nil {
		return res, false, nil
	}

	return out, false, nil
}

// resultsToText renders jsonpath results as space-separated text, formatting
// float64 leaf values without scientific notation.
func resultsToText(fullResults [][]reflect.Value) string {
	buf := new(bytes.Buffer)
	for _, results := range fullResults {
		for i, r := range results {
			text := resultToText(r)
			if i != len(results)-1 {
				text = append(text, ' ')
			}
			buf.Write(text)
		}
	}
	return buf.String()
}

func resultToText(r reflect.Value) []byte {
	v := r
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Map, reflect.Array, reflect.Slice, reflect.Struct:
		if b, err := json.Marshal(v.Interface()); err == nil {
			return b
		}
	case reflect.Float64, reflect.Float32:
		return []byte(strconv.FormatFloat(v.Float(), 'f', -1, 64))
	}
	if !v.IsValid() {
		return []byte("<nil>")
	}
	var b bytes.Buffer
	fmt.Fprint(&b, v.Interface())
	return b.Bytes()
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
