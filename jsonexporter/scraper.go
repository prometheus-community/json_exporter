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

package jsonexporter

import (
	"fmt"
	"math"
	"strconv"

	"github.com/kawamuray/jsonpath" // Originally: "github.com/NickSardo/jsonpath"
	"github.com/prometheus-community/json_exporter/harness"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

type JsonScraper interface {
	Scrape(data []byte, reg *harness.MetricRegistry) error
}

type ValueScraper struct {
	*Metric
	valueJsonPath *jsonpath.Path
}

func NewValueScraper(metric *Metric) (JsonScraper, error) {
	valuepath, err := compilePath(metric.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse path;path:<%s>,err:<%s>", metric.Path, err)
	}

	scraper := &ValueScraper{
		Metric:        metric,
		valueJsonPath: valuepath,
	}
	return scraper, nil
}

func (vs *ValueScraper) parseValue(bytes []byte) (float64, error) {
	value, err := strconv.ParseFloat(string(bytes), 64)
	if err != nil {
		return -1.0, fmt.Errorf("failed to parse value as float;value:<%s>", bytes)
	}
	return value, nil
}

func (vs *ValueScraper) forTargetValue(data []byte, handle func(*jsonpath.Result)) error {
	eval, err := jsonpath.EvalPathsInBytes(data, []*jsonpath.Path{vs.valueJsonPath})
	if err != nil {
		return fmt.Errorf("failed to eval jsonpath;path:<%v>,json:<%s>", vs.valueJsonPath, data)
	}

	for {
		result, ok := eval.Next()
		if !ok {
			break
		}
		handle(result)
	}
	return nil
}

func (vs *ValueScraper) Scrape(data []byte, reg *harness.MetricRegistry) error {
	isFirst := true
	return vs.forTargetValue(data, func(result *jsonpath.Result) {
		if !isFirst {
			log.Infof("ignoring non-first value;path:<%v>", vs.valueJsonPath)
			return
		}
		isFirst = false

		var value float64
		var boolValue bool
		var err error
		switch result.Type {
		case jsonpath.JsonNumber:
			value, err = vs.parseValue(result.Value)
		case jsonpath.JsonString:
			// If it is a string, lets pull off the quotes and attempt to parse it as a number
			value, err = vs.parseValue(result.Value[1 : len(result.Value)-1])
		case jsonpath.JsonNull:
			value = math.NaN()
		case jsonpath.JsonBool:
			if boolValue, err = strconv.ParseBool(string(result.Value)); boolValue {
				value = 1
			} else {
				value = 0
			}
		default:
			log.Warnf("skipping not numerical result;path:<%v>,value:<%s>",
				vs.valueJsonPath, result.Value)
			return
		}
		if err != nil {
			// Should never happen.
			log.Errorf("could not parse numerical value as float;path:<%v>,value:<%s>",
				vs.valueJsonPath, result.Value)
			return
		}

		log.Debugf("metric updated;name:<%s>,labels:<%s>,value:<%.2f>", vs.Name, vs.Labels, value)
		reg.Get(vs.Name).(*prometheus.GaugeVec).With(vs.Labels).Set(value)
	})
}

type ObjectScraper struct {
	*ValueScraper
	labelJsonPaths map[string]*jsonpath.Path
	valueJsonPaths map[string]*jsonpath.Path
}

func NewObjectScraper(metric *Metric) (JsonScraper, error) {
	valueScraper, err := NewValueScraper(metric)
	if err != nil {
		return nil, err
	}

	labelPaths, err := compilePaths(metric.Labels)
	if err != nil {
		return nil, err
	}
	valuePaths, err := compilePaths(metric.Values)
	if err != nil {
		return nil, err
	}
	scraper := &ObjectScraper{
		ValueScraper:   valueScraper.(*ValueScraper),
		labelJsonPaths: labelPaths,
		valueJsonPaths: valuePaths,
	}
	return scraper, nil
}

func (obsc *ObjectScraper) newLabels() map[string]string {
	labels := make(map[string]string)
	for name, value := range obsc.Labels {
		if _, ok := obsc.labelJsonPaths[name]; !ok {
			// Static label value.
			labels[name] = value
		}
	}
	return labels
}

func (obsc *ObjectScraper) extractFirstValue(data []byte, path *jsonpath.Path) (*jsonpath.Result, error) {
	eval, err := jsonpath.EvalPathsInBytes(data, []*jsonpath.Path{path})
	if err != nil {
		return nil, fmt.Errorf("failed to eval jsonpath;err:<%s>", err)
	}

	result, ok := eval.Next()
	if !ok {
		return nil, fmt.Errorf("no value found for path")
	}
	return result, nil
}

func (obsc *ObjectScraper) Scrape(data []byte, reg *harness.MetricRegistry) error {
	return obsc.forTargetValue(data, func(result *jsonpath.Result) {
		if result.Type != jsonpath.JsonObject && result.Type != jsonpath.JsonArray {
			log.Warnf("skipping not structual result;path:<%v>,value:<%s>",
				obsc.valueJsonPath, result.Value)
			return
		}

		labels := obsc.newLabels()
		for name, path := range obsc.labelJsonPaths {
			firstResult, err := obsc.extractFirstValue(result.Value, path)
			if err != nil {
				log.Warnf("could not find value for label path;path:<%v>,json:<%s>,err:<%s>", path, result.Value, err)
				continue
			}
			value := firstResult.Value
			if firstResult.Type == jsonpath.JsonString {
				// Strip quotes
				value = value[1 : len(value)-1]
			}
			labels[name] = string(value)
		}

		for name, configValue := range obsc.Values {
			var metricValue float64
			path := obsc.valueJsonPaths[name]

			if path == nil {
				// Static value
				value, err := obsc.parseValue([]byte(configValue))
				if err != nil {
					log.Errorf("could not use configured value as float number;err:<%s>", err)
					continue
				}
				metricValue = value
			} else {
				// Dynamic value
				firstResult, err := obsc.extractFirstValue(result.Value, path)
				if err != nil {
					log.Warnf("could not find value for value path;path:<%v>,json:<%s>,err:<%s>", path, result.Value, err)
					continue
				}

				var value float64
				switch firstResult.Type {
				case jsonpath.JsonNumber:
					value, err = obsc.parseValue(firstResult.Value)
				case jsonpath.JsonString:
					// If it is a string, lets pull off the quotes and attempt to parse it as a number
					value, err = obsc.parseValue(firstResult.Value[1 : len(firstResult.Value)-1])
				case jsonpath.JsonNull:
					value = math.NaN()
				default:
					log.Warnf("skipping not numerical result;path:<%v>,value:<%s>",
						obsc.valueJsonPath, result.Value)
					continue
				}
				if err != nil {
					// Should never happen.
					log.Errorf("could not parse numerical value as float;path:<%v>,value:<%s>",
						obsc.valueJsonPath, firstResult.Value)
					continue
				}
				metricValue = value
			}

			fqn := harness.MakeMetricName(obsc.Name, name)
			log.Debugf("metric updated;name:<%s>,labels:<%s>,value:<%.2f>", fqn, labels, metricValue)
			reg.Get(fqn).(*prometheus.GaugeVec).With(labels).Set(metricValue)
		}
	})
}
