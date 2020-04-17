package harness

import (
	"strings"
)

func MakeMetricName(parts ...string) string {
	return strings.Join(parts, "_")
}
