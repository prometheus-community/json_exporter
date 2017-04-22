package jsonexporter

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/kawamuray/jsonpath" // Originally: "github.com/NickSardo/jsonpath"
	"github.com/kawamuray/prometheus-exporter-harness/harness"
	"io/ioutil"
	"net/http"
)

type Collector struct {
	modules []*Module
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

func NewCollector(modules []*Module) *Collector {
	return &Collector{
		modules: modules,
	}
}

func (module *Module) fetchJson() ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", module.endpoint, nil)
	for name, value := range module.headers{
		req.Header.Add(name, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch json from endpoint;endpoint:<%s>,err:<%s>", module.endpoint, err)
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body;err:<%s>", err)
	}

	return data, nil
}

func (col *Collector) Collect(reg *harness.MetricRegistry) {
    for _, module := range col.modules {
        json, err := module.fetchJson()
        if err != nil {
            log.Error(err)
            return
        }

        for _, scraper := range module.scrapers {
            if err := scraper.Scrape(json, reg); err != nil {
                log.Errorf("error while scraping json;err:<%s>", err)
                continue
            }
        }
    }
}
