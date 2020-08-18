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

package internal

import (
	"errors"
	"strconv"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kawamuray/jsonpath" // Originally: "github.com/NickSardo/jsonpath"
	"github.com/prometheus/client_golang/prometheus"
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
}

func (mc JsonMetricCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range mc.JsonMetrics {
		ch <- m.Desc
	}
}

func (mc JsonMetricCollector) Collect(ch chan<- prometheus.Metric) {
	for _, m := range mc.JsonMetrics {
		if m.ValueJsonPath == "" { // ScrapeType is 'value'
			floatValue, err := extractValue(mc.Logger, mc.Data, m.KeyJsonPath)
			if err != nil {
				// Avoid noise and continue silently if it was a missing path error
				if err.Error() == "Path not found" {
					level.Debug(mc.Logger).Log("msg", "Failed to extract float value for metric", "path", m.KeyJsonPath, "err", err, "metric", m.Desc) //nolint:errcheck
					continue
				}
				level.Error(mc.Logger).Log("msg", "Failed to extract float value for metric", "path", m.KeyJsonPath, "err", err, "metric", m.Desc) //nolint:errcheck
				continue
			}

			ch <- prometheus.MustNewConstMetric(
				m.Desc,
				prometheus.UntypedValue,
				floatValue,
				extractLabels(mc.Logger, mc.Data, m.LabelsJsonPaths)...,
			)
		} else { // ScrapeType is 'object'
			path, err := compilePath(m.KeyJsonPath)
			if err != nil {
				level.Error(mc.Logger).Log("msg", "Failed to compile path", "path", m.KeyJsonPath, "err", err) //nolint:errcheck
				continue
			}

			eval, err := jsonpath.EvalPathsInBytes(mc.Data, []*jsonpath.Path{path})
			if err != nil {
				level.Error(mc.Logger).Log("msg", "Failed to create evaluator for json path", "path", m.KeyJsonPath, "err", err) //nolint:errcheck
				continue
			}
			for {
				if result, ok := eval.Next(); ok {
					floatValue, err := extractValue(mc.Logger, result.Value, m.ValueJsonPath)
					if err != nil {
						level.Error(mc.Logger).Log("msg", "Failed to extract value", "path", m.ValueJsonPath, "err", err) //nolint:errcheck
						continue
					}

					ch <- prometheus.MustNewConstMetric(
						m.Desc,
						prometheus.UntypedValue,
						floatValue,
						extractLabels(mc.Logger, result.Value, m.LabelsJsonPaths)...,
					)
				} else {
					break
				}
			}
		}
	}
}

func compilePath(path string) (*jsonpath.Path, error) {
	// All paths in this package is for extracting a value.
	// Complete trailing '+' sign if necessary.
	if path[len(path)-1] != '+' {
		path += "+"
	}

	paths, err := jsonpath.ParsePaths(path)
	if err != nil {
		return nil, err
	}
	return paths[0], nil
}

// Returns the first matching float value at the given json path
func extractValue(logger log.Logger, json []byte, path string) (float64, error) {
	var floatValue = -1.0
	var result *jsonpath.Result
	var err error

	if len(path) < 1 || path[0] != '$' {
		// Static value
		return parseValue([]byte(path))
	}

	// Dynamic value
	p, err := compilePath(path)
	if err != nil {
		return floatValue, err
	}

	eval, err := jsonpath.EvalPathsInBytes(json, []*jsonpath.Path{p})
	if err != nil {
		return floatValue, err
	}

	result, ok := eval.Next()
	if result == nil || !ok {
		if eval.Error != nil {
			return floatValue, eval.Error
		} else {
			level.Debug(logger).Log("msg", "Path not found", "path", path, "json", string(json)) //nolint:errcheck
			return floatValue, errors.New("Path not found")
		}
	}

	return SanitizeValue(result)
}

// Returns the list of labels created from the list of provided json paths
func extractLabels(logger log.Logger, json []byte, paths []string) []string {
	labels := make([]string, len(paths))
	for i, path := range paths {

		// Dynamic value
		p, err := compilePath(path)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to compile path for label", "path", path, "err", err) //nolint:errcheck
			continue
		}

		eval, err := jsonpath.EvalPathsInBytes(json, []*jsonpath.Path{p})
		if err != nil {
			level.Error(logger).Log("msg", "Failed to create evaluator for json", "path", path, "err", err) //nolint:errcheck
			continue
		}

		result, ok := eval.Next()
		if result == nil || !ok {
			if eval.Error != nil {
				level.Error(logger).Log("msg", "Failed to evaluate", "json", string(json), "err", eval.Error) //nolint:errcheck
			} else {
				level.Warn(logger).Log("msg", "Label path not found in json", "path", path)                        //nolint:errcheck
				level.Debug(logger).Log("msg", "Label path not found in json", "path", path, "json", string(json)) //nolint:errcheck
			}
			continue
		}

		l, err := strconv.Unquote(string(result.Value))
		if err == nil {
			labels[i] = l
		} else {
			labels[i] = string(result.Value)
		}
	}
	return labels
}
