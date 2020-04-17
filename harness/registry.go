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

package harness

import (
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

type MetricRegistry struct {
	metrics map[string]prometheus.Collector
}

func newRegistry() *MetricRegistry {
	return &MetricRegistry{
		metrics: make(map[string]prometheus.Collector),
	}
}

func (reg *MetricRegistry) Register(name string, metric prometheus.Collector) {
	log.Infof("metric registered;name:<%s>", name)
	reg.metrics[name] = metric
	prometheus.MustRegister(metric)
}

func (reg *MetricRegistry) Unregister(name string) {
	if metric := reg.metrics[name]; metric != nil {
		log.Infof("metric unregistered;name:<%s>", name)
		prometheus.Unregister(metric)
		delete(reg.metrics, name)
	}
}

func (reg *MetricRegistry) Get(name string) prometheus.Collector {
	return reg.metrics[name]
}

// Since prometheus.MetricVec is a struct but not interface,
// need to intrduce an interface to check if we can call Reset() on a metric.
type resettable interface {
	Reset()
}

func (reg *MetricRegistry) Reset() {
	for name, metric := range reg.metrics {
		if vec, ok := metric.(resettable); ok {
			log.Debugf("resetting metric;name:<%s>", name)
			vec.Reset()
		}
	}
}
