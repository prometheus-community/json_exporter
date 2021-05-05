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
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus-community/json_exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	pconfig "github.com/prometheus/common/config"
)

func MakeMetricName(parts ...string) string {
	return strings.Join(parts, "_")
}

func SanitizeValue(s string) (float64, error) {
	var value float64
	var resultErr string

	if value, err := strconv.ParseFloat(s, 64); err == nil {
		return value, nil
	} else {
		resultErr = fmt.Sprintf("%s", err)
	}

	if boolValue, err := strconv.ParseBool(s); err == nil {
		if boolValue {
			return 1.0, nil
		} else {
			return 0.0, nil
		}
	} else {
		resultErr = resultErr + "; " + fmt.Sprintf("%s", err)
	}

	if s == "<nil>" {
		return math.NaN(), nil
	}
	return value, fmt.Errorf(resultErr)
}

func CreateMetricsList(c config.Config) ([]JsonMetric, error) {
	var metrics []JsonMetric
	for _, metric := range c.Metrics {
		switch metric.Type {
		case config.ValueScrape:
			var variableLabels, variableLabelsValues []string
			for k, v := range metric.Labels {
				variableLabels = append(variableLabels, k)
				variableLabelsValues = append(variableLabelsValues, v)
			}
			jsonMetric := JsonMetric{
				Desc: prometheus.NewDesc(
					metric.Name,
					metric.Help,
					variableLabels,
					nil,
				),
				KeyJsonPath:     metric.Path,
				LabelsJsonPaths: variableLabelsValues,
				TimestampPath:   metric.Timestamp,
				Timezone:        metric.Timezone,
			}
			metrics = append(metrics, jsonMetric)
		case config.ObjectScrape:
			for subName, valuePath := range metric.Values {
				name := MakeMetricName(metric.Name, subName)
				var variableLabels, variableLabelsValues []string
				for k, v := range metric.Labels {
					variableLabels = append(variableLabels, k)
					variableLabelsValues = append(variableLabelsValues, v)
				}
				jsonMetric := JsonMetric{
					Desc: prometheus.NewDesc(
						name,
						metric.Help,
						variableLabels,
						nil,
					),
					KeyJsonPath:     metric.Path,
					ValueJsonPath:   valuePath,
					LabelsJsonPaths: variableLabelsValues,
					TimestampPath:   metric.Timestamp,
					Timezone:        metric.Timezone,
				}
				metrics = append(metrics, jsonMetric)
			}
		default:
			return nil, fmt.Errorf("Unknown metric type: '%s', for metric: '%s'", metric.Type, metric.Name)
		}
	}
	return metrics, nil
}

func FetchJson(ctx context.Context, logger log.Logger, endpoint string, config config.Config) ([]byte, error) {
	httpClientConfig := config.HTTPClientConfig
	client, err := pconfig.NewClientFromConfig(httpClientConfig, "fetch_json", true, false)
	if err != nil {
		level.Error(logger).Log("msg", "Error generating HTTP client", "err", err) //nolint:errcheck
		return nil, err
	}
	req, err := http.NewRequest("GET", endpoint, nil)
	req = req.WithContext(ctx)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create request", "err", err) //nolint:errcheck
		return nil, err
	}

	for key, value := range config.Headers {
		req.Header.Add(key, value)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Add("Accept", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if _, err := io.Copy(ioutil.Discard, resp.Body); err != nil {
			level.Error(logger).Log("msg", "Failed to discard body", "err", err) //nolint:errcheck
		}
		resp.Body.Close()
	}()

	if resp.StatusCode/100 != 2 {
		return nil, errors.New(resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}
