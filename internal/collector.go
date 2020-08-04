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
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/kawamuray/jsonpath" // Originally: "github.com/NickSardo/jsonpath"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

type JsonGaugeCollector struct {
	*prometheus.GaugeVec
	KeyJsonPath    string
	ValueJsonPath  string
	LabelsJsonPath map[string]string
}

func Scrape(collectors []JsonGaugeCollector, json []byte) {

	for _, collector := range collectors {
		if collector.ValueJsonPath == "" { // ScrapeType is 'value'

			// Since this is a 'value' type metric, there should be exactly one element in results
			// If there are more, just return the first one
			// TODO: Better handling/logging for this scenario
			floatValue, err := extractValue(json, collector.KeyJsonPath)
			if err != nil {
				log.Error(err)
				continue
			}

			collector.With(extractLabels(json, collector.LabelsJsonPath)).Set(floatValue)
		} else { // ScrapeType is 'object'
			path, err := compilePath(collector.KeyJsonPath)
			if err != nil {
				log.Errorf("Failed to compile path: '%s', ERROR: '%s'", collector.KeyJsonPath, err)
				continue
			}

			eval, err := jsonpath.EvalPathsInBytes(json, []*jsonpath.Path{path})
			if err != nil {
				log.Errorf("Failed to create evaluator for JSON Path: %s, ERROR: '%s'", collector.KeyJsonPath, err)
				continue
			}
			for {
				if result, ok := eval.Next(); ok {
					floatValue, err := extractValue(result.Value, collector.ValueJsonPath)
					if err != nil {
						log.Error(err)
						continue
					}

					collector.With(extractLabels(result.Value, collector.LabelsJsonPath)).Set(floatValue)
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
func extractValue(json []byte, path string) (float64, error) {
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
		return floatValue, fmt.Errorf("Failed to compile path: '%s', ERROR: '%s'", path, err)
	}

	eval, err := jsonpath.EvalPathsInBytes(json, []*jsonpath.Path{p})
	if err != nil {
		return floatValue, fmt.Errorf("Failed to create evaluator for JSON Path: %s, ERROR: '%s'", path, err)
	}

	result, ok := eval.Next()
	if result == nil || !ok {
		if eval.Error != nil {
			return floatValue, fmt.Errorf("Failed to evaluate json. ERROR: '%s', PATH: '%s', JSON: '%s'", eval.Error, path, string(json))
		} else {
			log.Debugf("Could not find path. PATH: '%s', JSON: '%s'", path, string(json))
			return floatValue, fmt.Errorf("Could not find path. PATH: '%s'", path)
		}
	}

	return SanitizeValue(result)
}

func extractLabels(json []byte, l map[string]string) map[string]string {
	labels := make(map[string]string)
	for label, path := range l {

		if len(path) < 1 || path[0] != '$' {
			// Static value
			labels[label] = path
			continue
		}

		// Dynamic value
		p, err := compilePath(path)
		if err != nil {
			log.Errorf("Failed to compile path for label: '%s', PATH: '%s', ERROR: '%s'", label, path, err)
			labels[label] = ""
			continue
		}

		eval, err := jsonpath.EvalPathsInBytes(json, []*jsonpath.Path{p})
		if err != nil {
			log.Errorf("Failed to create evaluator for JSON Path: %s, ERROR: '%s'", path, err)
			labels[label] = ""
			continue
		}

		result, ok := eval.Next()
		if result == nil || !ok {
			if eval.Error != nil {
				log.Errorf("Failed to evaluate json for label: '%s', ERROR: '%s', PATH: '%s', JSON: '%s'", label, eval.Error, path, string(json))
			} else {
				log.Debugf("Could not find path in json for label: '%s', PATH: '%s', JSON: '%s'", label, path, string(json))
				log.Warnf("Could not find path in json for label: '%s', PATH: '%s'", label, path)
			}
			continue
		}

		l, err := strconv.Unquote(string(result.Value))
		if err == nil {
			labels[label] = l
		} else {
			labels[label] = string(result.Value)
		}
	}
	return labels
}

func FetchJson(ctx context.Context, endpoint string, headers map[string]string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", endpoint, nil)
	req = req.WithContext(ctx)
	if err != nil {
		log.Errorf("Error creating request. ERROR: '%s'", err)
		return nil, err
	}

	for key, value := range headers {
		req.Header.Add(key, value)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Add("Accept", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch json from endpoint;endpoint:<%s>,err:<%s>", endpoint, err)
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body;err:<%s>", err)
	}

	return data, nil
}