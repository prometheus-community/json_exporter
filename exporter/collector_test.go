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

package exporter

import (
	"io"
	"log/slog"
	"testing"
)

func TestExtractValue(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		Name           string
		Data           string
		Path           string
		ExpectedOutput string
	}{
		{
			Name:           "large integer is not rendered in scientific notation",
			Data:           `{"id": 1655371}`,
			Path:           "{ .id }",
			ExpectedOutput: "1655371",
		},
		{
			Name:           "float keeps decimals without scientific notation",
			Data:           `{"value": 1234567.89}`,
			Path:           "{ .value }",
			ExpectedOutput: "1234567.89",
		},
		{
			Name:           "small integer is unaffected",
			Data:           `{"count": 42}`,
			Path:           "{ .count }",
			ExpectedOutput: "42",
		},
		{
			Name:           "string value is preserved",
			Data:           `{"name": "foo"}`,
			Path:           "{ .name }",
			ExpectedOutput: "foo",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			value, _, err := extractValue(logger, []byte(test.Data), test.Path, false, false)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if value != test.ExpectedOutput {
				t.Fatalf("got %q, expected %q", value, test.ExpectedOutput)
			}
		})
	}
}
