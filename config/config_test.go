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

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write temp config: %s", err)
	}
	return path
}

func TestLoadConfigDefaults(t *testing.T) {
	c, err := LoadConfig(writeConfig(t, `
modules:
  default:
    metrics:
    - name: example
      path: '{.value}'
`))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	metric := c.Modules["default"].Metrics[0]
	if metric.Type != ValueScrape {
		t.Errorf("expected 'type' to default to %q, got %q", ValueScrape, metric.Type)
	}
	if metric.ValueType != ValueTypeUntyped {
		t.Errorf("expected 'valuetype' to default to %q, got %q", ValueTypeUntyped, metric.ValueType)
	}
	if metric.Help != "example" {
		t.Errorf("expected 'help' to default to the metric name, got %q", metric.Help)
	}
}

func TestLoadConfigValid(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, `
modules:
  default:
    metrics:
    - name: gauge_value
      path: '{.value}'
      valuetype: gauge
    - name: object_value
      type: object
      path: '{.values[*]}'
      valuetype: counter
      values:
        count: '{.count}'
`))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
}

func TestLoadConfigInvalid(t *testing.T) {
	for _, tc := range []struct {
		name       string
		config     string
		wantErrSub string
	}{
		{
			// Common mistake: 'type' is for the scrape mode, not the
			// Prometheus metric type. See prometheus-community/json_exporter#393.
			name: "prometheus metric type set on 'type'",
			config: `
modules:
  default:
    metrics:
    - name: example
      path: '{.value}'
      type: counter
`,
			wantErrSub: "valuetype",
		},
		{
			name: "unknown scrape type",
			config: `
modules:
  default:
    metrics:
    - name: example
      path: '{.value}'
      type: bogus
`,
			wantErrSub: `invalid 'type'`,
		},
		{
			name: "unknown valuetype",
			config: `
modules:
  default:
    metrics:
    - name: example
      path: '{.value}'
      valuetype: histogram
`,
			wantErrSub: `invalid 'valuetype'`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, tc.config))
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil", tc.wantErrSub)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("expected error to contain %q, got: %s", tc.wantErrSub, err)
			}
		})
	}
}
