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
	Endpoint string
	UserName string
	Password string
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

func NewCollector(endpoint string, userName string, password string, scrapers []JsonScraper) *Collector {
	return &Collector{
		Endpoint: endpoint,
		UserName: userName,
		Password: password,
		scrapers: scrapers,
	}
}

func (col *Collector) fetchJson() ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", col.Endpoint, nil)
	if col.UserName != "" && col.Password != "" {
		req.SetBasicAuth(col.UserName, col.Password)
	}
	resp, err := client.Do(req)
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
