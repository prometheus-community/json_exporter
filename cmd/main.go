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

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus-community/json_exporter/config"
	"github.com/prometheus-community/json_exporter/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

var (
	configFile  = kingpin.Flag("config.file", "JSON exporter configuration file.").Default("config.yml").ExistingFile()
	configCheck = kingpin.Flag("config.check", "If true validate the config file and then exit.").Default("false").Bool()
	metricsPath = kingpin.Flag(
		"web.telemetry-path",
		"Path under which to expose metrics.",
	).Default("/metrics").String()
	toolkitFlags = kingpinflag.AddFlags(kingpin.CommandLine, ":7979")
)

func Run() {

	promslogConfig := &promslog.Config{}

	flag.AddFlags(kingpin.CommandLine, promslogConfig)
	kingpin.Version(version.Print("json_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promslog.New(promslogConfig)

	logger.Info("Starting json_exporter", "version", version.Info())
	logger.Info("Build context", "build", version.BuildContext())

	logger.Info("Loading config file", "file", *configFile)
	config, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Error("Error loading config", "err", err)
		os.Exit(1)
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		logger.Error("Failed to marshal config to JSON", "err", err)
	}
	logger.Info("Loaded config file", "config", string(configJSON))

	if *configCheck {
		os.Exit(0)
	}

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/probe", func(w http.ResponseWriter, req *http.Request) {
		probeHandler(w, req, logger, config)
	})
	if *metricsPath != "/" && *metricsPath != "" {
		landingConfig := web.LandingConfig{
			Name:        "JSON Exporter",
			Description: "Prometheus Exporter for converting json to metrics",
			Version:     version.Info(),
			Links: []web.LandingLinks{
				{
					Address: *metricsPath,
					Text:    "Metrics",
				},
			},
		}
		landingPage, err := web.NewLandingPage(landingConfig)
		if err != nil {
			logger.Error("error creating landing page", "err", err)
			os.Exit(1)
		}
		http.Handle("/", landingPage)
	}

	server := &http.Server{}
	if err := web.ListenAndServe(server, toolkitFlags, logger); err != nil {
		logger.Error("Failed to start the server", "err", err)
		os.Exit(1)
	}
}

func probeHandler(w http.ResponseWriter, r *http.Request, logger *slog.Logger, config config.Config) {

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx)

	module := r.URL.Query().Get("module")
	if module == "" {
		module = "default"
	}
	if _, ok := config.Modules[module]; !ok {
		http.Error(w, fmt.Sprintf("Unknown module %q", module), http.StatusBadRequest)
		logger.Debug("Unknown module", "module", module)
		return
	}

	registry := prometheus.NewPedanticRegistry()

	metrics, err := exporter.CreateMetricsList(config.Modules[module])
	if err != nil {
		logger.Error("Failed to create metrics list from config", "err", err)
	}

	jsonMetricCollector := exporter.JSONMetricCollector{JSONMetrics: metrics}
	jsonMetricCollector.Logger = logger

	target := r.URL.Query().Get("target")
	if target == "" {
		http.Error(w, "Target parameter is missing", http.StatusBadRequest)
		return
	}

	fetcher := exporter.NewJSONFetcher(ctx, logger, config.Modules[module], r.URL.Query())
	data, err := fetcher.FetchJSON(target)
	if err != nil {
		http.Error(w, "Failed to fetch JSON response. TARGET: "+target+", ERROR: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	jsonMetricCollector.Data = data

	registry.MustRegister(jsonMetricCollector)
	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)

}
