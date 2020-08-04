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
	"time"

	"github.com/prometheus-community/json_exporter/config"
	"github.com/prometheus-community/json_exporter/internal"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
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
	app.Version = internal.Version
	app.Usage = "A prometheus exporter for scraping metrics from JSON REST API endpoints"
	app.UsageText = "[OPTIONS] CONFIG_PATH"
	app.Action = main
	app.Flags = defaultOpts

	return app
}

func main(c *cli.Context) {
	setupLogging(c.String("log-level"))

	internal.Init(c)

	config, err := config.LoadConfig(c.Args()[0])
	if err != nil {
		log.Fatal(err)
	}
	configJson, err := json.MarshalIndent(config, "", "\t")
	if err != nil {
		log.Errorf("Failed to marshal loaded config to JSON. ERROR: '%s'", err)
	}
	log.Infof("Config:\n%s", string(configJson))

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/probe", func(w http.ResponseWriter, req *http.Request) {
		probeHandler(w, req, config)
	})
	if err := http.ListenAndServe(fmt.Sprintf(":%d", c.Int("port")), nil); err != nil {
		log.Fatal(err)
	}
}

func probeHandler(w http.ResponseWriter, r *http.Request, config config.Config) {

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(config.Global.TimeoutSeconds*float64(time.Second)))
	defer cancel()
	r = r.WithContext(ctx)

	registry := prometheus.NewPedanticRegistry()

	metrics, err := internal.CreateMetricsList(registry, config)
	if err != nil {
		log.Fatalf("Failed to create metrics from config. Error: %s", err)
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

	data, err := internal.FetchJson(ctx, target, config.Headers)
	if err != nil {
		log.Error(err)
		duration := time.Since(start).Seconds()
		log.Errorf("Probe failed. duration_seconds: %f", duration)
	} else {
		internal.Scrape(metrics, data)

		duration := time.Since(start).Seconds()
		probeDurationGauge.Set(duration)
		probeSuccessGauge.Set(1)
	}

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)

}

func setupLogging(level string) {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	logLevel, err := log.ParseLevel(level)
	if err != nil {
		log.Fatalf("could not set log level to '%s';err:<%s>", level, err)
	}
	log.SetLevel(logLevel)
}
