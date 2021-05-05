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
	"time"

	"github.com/araddon/dateparse"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/util/jsonpath"
)

type JsonMetricCollector struct {
	JsonMetrics []JsonMetric
	Data        []byte
	Logger      log.Logger
}

type JsonMetric struct {
	Desc            *prometheus.Desc
	KeyJsonPath     string
	ValueJsonPath   string
	LabelsJsonPaths []string
	TimestampPath   string
	Timezone        string
}

func (mc JsonMetricCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range mc.JsonMetrics {
		ch <- m.Desc
	}
}

func (mc JsonMetricCollector) Collect(ch chan<- prometheus.Metric) {
	for _, m := range mc.JsonMetrics {
		if m.TimestampPath != "" && m.Timezone != "" { // manage timezone
			loc, err := time.LoadLocation(m.Timezone)
			if err != nil {
				level.Error(mc.Logger).Log("msg", "Failed to extract timezone for metric", "path", m.KeyJsonPath, "err", err, "metric", m.Desc) //nolint:errcheck
				continue
			}
			time.Local = loc
		}

		if m.ValueJsonPath == "" { // ScrapeType is 'value'
			value, err := extractValue(mc.Logger, mc.Data, m.KeyJsonPath, false)
			if err != nil {
				level.Error(mc.Logger).Log("msg", "Failed to extract value for metric", "path", m.KeyJsonPath, "err", err, "metric", m.Desc) //nolint:errcheck
				continue
			}

			if floatValue, err := SanitizeValue(value); err == nil {

				metric := prometheus.MustNewConstMetric(
					m.Desc,
					prometheus.UntypedValue,
					floatValue,
					extractLabels(mc.Logger, mc.Data, m.LabelsJsonPaths)...,
				)
				if m.TimestampPath == "" { // manage timestamp
					ch <- metric
				} else {
					ts, err := extractValue(mc.Logger, mc.Data, m.TimestampPath, false)
					if err != nil {
						level.Error(mc.Logger).Log("msg", "Failed to extract timestamp for metric", "path", m.KeyJsonPath, "err", err, "metric", m.Desc) //nolint:errcheck
						continue
					}
					timestamp, err := dateparse.ParseLocal(ts)
					if err != nil {
						level.Error(mc.Logger).Log("msg", "Failed to convert timestamp", "path", m.KeyJsonPath, "err", err, "metric", m.Desc) //nolint:errcheck
						continue
					}
					ch <- prometheus.NewMetricWithTimestamp(timestamp, metric)
				}
			} else {
				level.Error(mc.Logger).Log("msg", "Failed to convert extracted value to float64", "path", m.KeyJsonPath, "value", value, "err", err, "metric", m.Desc) //nolint:errcheck
				continue
			}
		} else { // ScrapeType is 'object'
			values, err := extractValue(mc.Logger, mc.Data, m.KeyJsonPath, true)
			if err != nil {
				level.Error(mc.Logger).Log("msg", "Failed to extract json objects for metric", "err", err, "metric", m.Desc) //nolint:errcheck
				continue
			}

			var jsonData []interface{}
			if err := json.Unmarshal([]byte(values), &jsonData); err == nil {
				for _, data := range jsonData {
					jdata, err := json.Marshal(data)
					if err != nil {
						level.Error(mc.Logger).Log("msg", "Failed to marshal data to json", "path", m.ValueJsonPath, "err", err, "metric", m.Desc, "data", data) //nolint:errcheck
						continue
					}
					value, err := extractValue(mc.Logger, jdata, m.ValueJsonPath, false)
					if err != nil {
						level.Error(mc.Logger).Log("msg", "Failed to extract value for metric", "path", m.ValueJsonPath, "err", err, "metric", m.Desc) //nolint:errcheck
						continue
					}

					if floatValue, err := SanitizeValue(value); err == nil {
						metric := prometheus.MustNewConstMetric(
							m.Desc,
							prometheus.UntypedValue,
							floatValue,
							extractLabels(mc.Logger, jdata, m.LabelsJsonPaths)...,
						)
						if m.TimestampPath == "" { // manage timestamp
							ch <- metric
						} else {
							ts, err := extractValue(mc.Logger, jdata, m.TimestampPath, false)
							if err != nil {
								level.Error(mc.Logger).Log("msg", "Failed to extract timestamp for metric", "path", m.KeyJsonPath, "err", err, "metric", m.Desc) //nolint:errcheck
								continue
							}
							timestamp, err := dateparse.ParseLocal(ts)
							if err != nil {
								level.Error(mc.Logger).Log("msg", "Failed to convert timestamp", "path", m.KeyJsonPath, "err", err, "metric", m.Desc) //nolint:errcheck
								continue
							}
							ch <- prometheus.NewMetricWithTimestamp(timestamp, metric)
						}
					} else {
						level.Error(mc.Logger).Log("msg", "Failed to convert extracted value to float64", "path", m.ValueJsonPath, "value", value, "err", err, "metric", m.Desc) //nolint:errcheck
						continue
					}
				}
			} else {
				level.Error(mc.Logger).Log("msg", "Failed to convert extracted objects to json", "err", err, "metric", m.Desc) //nolint:errcheck
				continue
			}
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
		level.Error(logger).Log("msg", "Failed to unmarshal data to json", "err", err, "data", data) //nolint:errcheck
		return "", err
	}

	if err := j.Parse(path); err != nil {
		level.Error(logger).Log("msg", "Failed to parse jsonpath", "err", err, "path", path, "data", data) //nolint:errcheck
		return "", err
	}

	if err := j.Execute(buf, jsonData); err != nil {
		level.Error(logger).Log("msg", "Failed to execute jsonpath", "err", err, "path", path, "data", data) //nolint:errcheck
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
			level.Error(logger).Log("msg", "Failed to extract label value", "err", err, "path", path, "data", data) //nolint:errcheck
		}
	}
	return labels
}
