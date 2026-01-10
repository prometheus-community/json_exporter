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
	"log/slog"
	"math"
	"os"
	"testing"
)

func TestSanitizeValue(t *testing.T) {
	tests := []struct {
		Input          string
		ExpectedOutput float64
		ShouldSucceed  bool
	}{
		{"1234", 1234.0, true},
		{"1234.5", 1234.5, true},
		{"true", 1.0, true},
		{"TRUE", 1.0, true},
		{"False", 0.0, true},
		{"FALSE", 0.0, true},
		{"abcd", 0, false},
		{"{}", 0, false},
		{"[]", 0, false},
		{"", 0, false},
		{"''", 0, false},
	}

	for i, test := range tests {
		actualOutput, err := SanitizeValue(test.Input)
		if err != nil && test.ShouldSucceed {
			t.Fatalf("Value snitization test %d failed with an unexpected error.\nINPUT:\n%q\nERR:\n%s", i, test.Input, err)
		}
		if test.ShouldSucceed && actualOutput != test.ExpectedOutput {
			t.Fatalf("Value sanitization test %d fails unexpectedly.\nGOT:\n%f\nEXPECTED:\n%f", i, actualOutput, test.ExpectedOutput)
		}
	}
}

func TestSanitizeValueNaN(t *testing.T) {
	actualOutput, err := SanitizeValue("<nil>")
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsNaN(actualOutput) {
		t.Fatalf("Value sanitization test for %f fails unexpectedly.", math.NaN())
	}
}

func TestExtractDynamicLabels(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name     string
		data     interface{}
		paths    []string
		expected []string
	}{
		{
			name: "Extract object key as label",
			data: map[string]interface{}{
				"Yokoy Expenses -> HiBob": map[string]interface{}{
					"ok":    true,
					"error": "Number of failing accounts in the last sync (valid and synced): 0",
				},
			},
			paths:    []string{"{__name__}"},
			expected: []string{"Yokoy Expenses -> HiBob"},
		},
		{
			name: "Extract multiple labels with object key",
			data: map[string]interface{}{
				"LDAP-AVIFORS -> Yokoy": map[string]interface{}{
					"ok":    false,
					"error": "Connection timeout",
				},
			},
			paths:    []string{"{__name__}", "{.ok}"},
			expected: []string{"LDAP-AVIFORS -> Yokoy", "false"},
		},
		{
			name: "Regular JSONPath extraction without dynamic keys",
			data: map[string]interface{}{
				"status": "active",
				"count":  42,
			},
			paths:    []string{"{.status}"},
			expected: []string{"active"},
		},
		{
			name: "Extract boolean from dynamic object",
			data: map[string]interface{}{
				"Test Flow -> Failed": map[string]interface{}{
					"ok":    false,
					"error": "Connection timeout occurred",
				},
			},
			paths:    []string{"{__name__}", "{.ok}"},
			expected: []string{"Test Flow -> Failed", "false"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDynamicLabels(logger, tt.data, tt.paths)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d labels, got %d", len(tt.expected), len(result))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected label[%d] = '%s', got '%s'", i, expected, result[i])
				}
			}
		})
	}
}
