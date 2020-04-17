package harness

type Collector interface {
	Collect(*MetricRegistry)
}
