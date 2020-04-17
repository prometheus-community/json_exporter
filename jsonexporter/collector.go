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
	"io/ioutil"
	"net/http"

	"github.com/kawamuray/jsonpath" // Originally: "github.com/NickSardo/jsonpath"
	"github.com/prometheus-community/json_exporter/harness"
	log "github.com/sirupsen/logrus"
)

type Collector struct {
	Endpoint string
	scrapers []JsonScraper
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

func compilePaths(paths map[string]string) (map[string]*jsonpath.Path, error) {
	compiledPaths := make(map[string]*jsonpath.Path)
	for name, value := range paths {
		if len(value) < 1 || value[0] != '$' {
			// Static value
			continue
		}
		compiledPath, err := compilePath(value)
		if err != nil {
			return nil, fmt.Errorf("failed to parse path;path:<%s>,err:<%s>", value, err)
		}
		compiledPaths[name] = compiledPath
	}
	return compiledPaths, nil
}

func NewCollector(endpoint string, scrapers []JsonScraper) *Collector {
	return &Collector{
		Endpoint: endpoint,
		scrapers: scrapers,
	}
}

func (col *Collector) fetchJson() ([]byte, error) {
	resp, err := http.Get(col.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch json from endpoint;endpoint:<%s>,err:<%s>", col.Endpoint, err)
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body;err:<%s>", err)
	}

	return data, nil
}

func (col *Collector) Collect(reg *harness.MetricRegistry) {
	json, err := col.fetchJson()
	if err != nil {
		log.Error(err)
		return
	}

	for _, scraper := range col.scrapers {
		if err := scraper.Scrape(json, reg); err != nil {
			log.Errorf("error while scraping json;err:<%s>", err)
			continue
		}
	}
}
