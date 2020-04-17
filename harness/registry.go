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
