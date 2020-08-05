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
	"net/http"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus-community/json_exporter/config"
	"github.com/prometheus-community/json_exporter/internal"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/version"
	"github.com/urfave/cli"
)

var (
	defaultOpts = []cli.Flag{
		cli.IntFlag{
			Name:  "port",
			Usage: "The port number used to expose metrics via http",
			Value: 7979,
		},
		cli.StringFlag{
			Name:  "log-level",
			Usage: "Set Logging level",
			Value: "info",
		},
	}
)

func MakeApp() *cli.App {

	app := cli.NewApp()
	app.Name = "json_exporter"
	app.Usage = "A prometheus exporter for scraping metrics from JSON REST API endpoints"
	app.UsageText = "[OPTIONS] CONFIG_PATH"
	app.Action = main
	app.Flags = defaultOpts

	return app
}

func main(c *cli.Context) {

	promlogConfig := &promlog.Config{}
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Starting json_exporter", "version", version.Info()) //nolint:errcheck
	level.Info(logger).Log("msg", "Build context", "build", version.BuildContext())    //nolint:errcheck

	internal.Init(logger, c)

	config, err := config.LoadConfig(c.Args()[0])
	if err != nil {
		level.Error(logger).Log("msg", "Error loading config", "err", err) //nolint:errcheck
		os.Exit(1)
	}
	configJson, err := json.Marshal(config)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to marshal config to JOSN", "err", err) //nolint:errcheck
	}
	level.Info(logger).Log("msg", "Loaded config file", "config", configJson) //nolint:errcheck

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/probe", func(w http.ResponseWriter, req *http.Request) {
		probeHandler(w, req, logger, config)
	})
	if err := http.ListenAndServe(fmt.Sprintf(":%d", c.Int("port")), nil); err != nil {
		level.Error(logger).Log("msg", "failed to start the server", "err", err) //nolint:errcheck
	}
}

func probeHandler(w http.ResponseWriter, r *http.Request, logger log.Logger, config config.Config) {

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(config.Global.TimeoutSeconds*float64(time.Second)))
	defer cancel()
	r = r.WithContext(ctx)

	registry := prometheus.NewPedanticRegistry()

	metrics, err := internal.CreateMetricsList(registry, config)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create metrics list from config", "err", err) //nolint:errcheck
	}

	probeSuccessGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_success",
		Help: "Displays whether or not the probe was a success",
	})
	probeDurationGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_duration_seconds",
		Help: "Returns how long the probe took to complete in seconds",
	})

	target := r.URL.Query().Get("target")
	if target == "" {
		http.Error(w, "Target parameter is missing", http.StatusBadRequest)
		return
	}

	start := time.Now()
	registry.MustRegister(probeSuccessGauge)
	registry.MustRegister(probeDurationGauge)

	data, err := internal.FetchJson(ctx, logger, target, config.Headers)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to fetch JSON response", "err", err) //nolint:errcheck
		duration := time.Since(start).Seconds()
		level.Error(logger).Log("msg", "Probe failed", "duration_seconds", duration) //nolint:errcheck
	} else {
		internal.Scrape(logger, metrics, data)

		duration := time.Since(start).Seconds()
		probeDurationGauge.Set(duration)
		probeSuccessGauge.Set(1)
		//level.Info(logger).Log("msg", "Probe succeeded", "duration_seconds", duration) // Too noisy
	}

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)

}
