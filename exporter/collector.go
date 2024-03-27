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
	"reflect"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
	"github.com/prometheus-community/json_exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	structpb "google.golang.org/protobuf/types/known/structpb"
	"k8s.io/client-go/util/jsonpath"
)

type JSONMetricCollector struct {
	JSONMetrics []JSONMetric
	Data        []byte
	Logger      log.Logger
}

type JSONMetric struct {
	Desc                   *prometheus.Desc
	Type                   config.ScrapeType
	EngineType             config.EngineType
	KeyJSONPath            string
	ValueJSONPath          string
	LabelsJSONPaths        []string
	ValueType              prometheus.ValueType
	EpochTimestampJSONPath string
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
			level.Debug(mc.Logger).Log("msg", "Extracting value for metric", "path", m.KeyJSONPath, "metric", m.Desc)
			value, err := extractValue(mc.Logger, m.EngineType, mc.Data, m.KeyJSONPath, false)
			if err != nil {
				level.Error(mc.Logger).Log("msg", "Failed to extract value for metric", "path", m.KeyJSONPath, "err", err, "metric", m.Desc)
				continue
			}

			if floatValue, err := SanitizeValue(value); err == nil {
				metric := prometheus.MustNewConstMetric(
					m.Desc,
					m.ValueType,
					floatValue,
					extractLabels(mc.Logger, m.EngineType, mc.Data, m.LabelsJSONPaths)...,
				)
				ch <- timestampMetric(mc.Logger, m, mc.Data, metric)
			} else {
				level.Error(mc.Logger).Log("msg", "Failed to convert extracted value to float64", "path", m.KeyJSONPath, "value", value, "err", err, "metric", m.Desc)
				continue
			}

		case config.ObjectScrape:
			level.Debug(mc.Logger).Log("msg", "Extracting object for metric", "path", m.KeyJSONPath, "metric", m.Desc)
			values, err := extractValue(mc.Logger, m.EngineType, mc.Data, m.KeyJSONPath, true)
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
					value, err := extractValue(mc.Logger, m.EngineType, jdata, m.ValueJSONPath, false)
					if err != nil {
						level.Error(mc.Logger).Log("msg", "Failed to extract value for metric", "path", m.ValueJSONPath, "err", err, "metric", m.Desc)
						continue
					}

					if floatValue, err := SanitizeValue(value); err == nil {
						metric := prometheus.MustNewConstMetric(
							m.Desc,
							m.ValueType,
							floatValue,
							extractLabels(mc.Logger, m.EngineType, jdata, m.LabelsJSONPaths)...,
						)
						ch <- timestampMetric(mc.Logger, m, jdata, metric)
					} else {
						level.Error(mc.Logger).Log("msg", "Failed to convert extracted value to float64", "path", m.ValueJSONPath, "value", value, "err", err, "metric", m.Desc)
						continue
					}
				}
			} else {
				level.Error(mc.Logger).Log("msg", "Failed to convert extracted objects to json", "value", values, "err", err, "metric", m.Desc)
				continue
			}
		default:
			level.Error(mc.Logger).Log("msg", "Unknown scrape config type", "type", m.Type, "metric", m.Desc)
			continue
		}
	}
}

func extractValue(logger log.Logger, engine config.EngineType, data []byte, path string, enableJSONOutput bool) (string, error) {
	switch engine {
	case config.EngineTypeJSONPath:
		return extractValueJSONPath(logger, data, path, enableJSONOutput)
	case config.EngineTypeCEL:
		return extractValueCEL(logger, data, path, enableJSONOutput)
	default:
		return "", fmt.Errorf("Unknown engine type: %s", engine)
	}
}

// Returns the last matching value at the given json path
func extractValueJSONPath(logger log.Logger, data []byte, path string, enableJSONOutput bool) (string, error) {
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

// Returns the last matching value at the given json path
func extractValueCEL(logger log.Logger, data []byte, expression string, enableJSONOutput bool) (string, error) {

	var jsonData map[string]any

	err := json.Unmarshal(data, &jsonData)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to unmarshal data to json", "err", err, "data", data)
		return "", err
	}

	inputVars := make([]cel.EnvOption, 0, len(jsonData))
	for k := range jsonData {
		inputVars = append(inputVars, cel.Variable(k, cel.DynType))
	}

	env, err := cel.NewEnv(inputVars...)

	if err != nil {
		level.Error(logger).Log("msg", "Failed to set up CEL environment", "err", err, "data", data)
		return "", err
	}

	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		level.Error(logger).Log("CEL type-check error", issues.Err(), "expression", expression)
		return "", err
	}
	prg, err := env.Program(ast)
	if err != nil {
		level.Error(logger).Log("CEL program construction error", err)
		return "", err
	}

	out, _, err := prg.Eval(jsonData)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to evaluate cel query", "err", err, "expression", expression, "data", jsonData)
		return "", err
	}

	// Since we are finally going to extract only float64, unquote if necessary

	//res, err := jsonpath.UnquoteExtend(fmt.Sprintf("%g", out))
	//if err == nil {
	//	level.Error(logger).Log("msg","Triggered")
	//	return res, nil
	//}
	level.Error(logger).Log("msg", "Triggered later", "val", out)
	if enableJSONOutput {
		res, err := valueToJSON(out)
		if err != nil {
			return "", err
		}
		return res, nil
	}

	return fmt.Sprintf("%v", out), nil
}

// Returns the list of labels created from the list of provided json paths
func extractLabels(logger log.Logger, engine config.EngineType, data []byte, paths []string) []string {
	labels := make([]string, len(paths))
	for i, path := range paths {
		if result, err := extractValue(logger, engine, data, path, false); err == nil {
			labels[i] = result
		} else {
			level.Error(logger).Log("msg", "Failed to extract label value", "err", err, "path", path, "data", data)
		}
	}
	return labels
}

func timestampMetric(logger log.Logger, m JSONMetric, data []byte, pm prometheus.Metric) prometheus.Metric {
	if m.EpochTimestampJSONPath == "" {
		return pm
	}
	ts, err := extractValue(logger, m.EngineType, data, m.EpochTimestampJSONPath, false)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to extract timestamp for metric", "path", m.KeyJSONPath, "err", err, "metric", m.Desc)
		return pm
	}
	epochTime, err := SanitizeIntValue(ts)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to parse timestamp for metric", "path", m.KeyJSONPath, "err", err, "metric", m.Desc)
		return pm
	}
	timestamp := time.UnixMilli(epochTime)
	return prometheus.NewMetricWithTimestamp(timestamp, pm)
}

// valueToJSON converts the CEL type to a protobuf JSON representation and
// marshals the result to a string.
func valueToJSON(val ref.Val) (string, error) {
	v, err := val.ConvertToNative(reflect.TypeOf(&structpb.Value{}))
	if err != nil {
		return "", err
	}
	marshaller := protojson.MarshalOptions{Indent: "    "}
	bytes, err := marshaller.Marshal(v.(proto.Message))
	if err != nil {
		return "", err
	}
	return string(bytes), err
}
