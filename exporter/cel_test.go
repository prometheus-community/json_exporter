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
	"math"
	"testing"

	"github.com/prometheus/common/promslog"
)

const celTestDocument = `{
	"counter": 1234,
	"timestamp": 1657568506,
	"location": "mars",
	"nothing": null,
	"values": [
		{"id": "id-A", "count": 1, "some_boolean": true, "state": "ACTIVE"},
		{"id": "id-B", "count": 2, "some_boolean": true, "state": "INACTIVE"}
	]
}`

func TestExtractValueCEL(t *testing.T) {
	tests := []struct {
		Name             string
		Data             string
		Expression       string
		EnableJSONOutput bool
		ExpectedOutput   string
		ShouldSucceed    bool
	}{
		{"leading dot", celTestDocument, ".counter", false, "1234", true},
		{"bare identifier", celTestDocument, "counter", false, "1234", true},
		{"root variable", celTestDocument, "root.counter", false, "1234", true},
		{"string literal", celTestDocument, `"beta"`, false, "beta", true},
		{"string concatenation", celTestDocument, `"planet-" + .location`, false, "planet-mars", true},
		{"boolean", celTestDocument, `.values[0].some_boolean`, false, "true", true},
		{"arithmetic", celTestDocument, ".counter / 2.0", false, "617", true},
		{"comparison", celTestDocument, ".counter > 1000", false, "true", true},
		{"macro", celTestDocument, ".values.filter(v, v.state == \"ACTIVE\").size()", false, "1", true},
		// Whole numbers must not be rendered in exponent notation, timestamps
		// are parsed as integers.
		{"large whole number", celTestDocument, ".timestamp", false, "1657568506", true},
		{"fraction", celTestDocument, ".counter / 8.0", false, "154.25", true},
		// A top-level array is only reachable through the root variable.
		{"top-level array", `[{"population": 123}]`, "root[0].population", false, "123", true},
		{"top-level scalar", `42`, "root", false, "42", true},
		// A member named 'root' shadows the root variable.
		{"shadowed root", `{"root": "shadow"}`, "root", false, "shadow", true},
		{"json output", celTestDocument, `.values.filter(v, v.state == "INACTIVE")`, true,
			`[{"count":2,"id":"id-B","some_boolean":true,"state":"INACTIVE"}]`, true},
		// A single object is scraped as a list of one, like a JSONPath
		// expression matching a single object.
		{"json output of a single object", celTestDocument, `.values[1]`, true,
			`[{"count":2,"id":"id-B","some_boolean":true,"state":"INACTIVE"}]`, true},
		{"unknown member", celTestDocument, ".missing", false, "", false},
		{"syntax error", celTestDocument, ".counter +", false, "", false},
		{"empty expression", celTestDocument, "", false, "", false},
		{"invalid json", `not json`, ".counter", false, "", false},
		{"type error", celTestDocument, `.counter + .location`, false, "", false},
	}

	logger := promslog.NewNopLogger()
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			actualOutput, _, err := extractValueCEL(logger, []byte(test.Data), test.Expression, test.EnableJSONOutput, false)
			if test.ShouldSucceed && err != nil {
				t.Fatalf("CEL extraction of %q failed with an unexpected error: %s", test.Expression, err)
			}
			if !test.ShouldSucceed {
				if err == nil {
					t.Fatalf("CEL extraction of %q succeeded unexpectedly, got %q", test.Expression, actualOutput)
				}
				return
			}
			if actualOutput != test.ExpectedOutput {
				t.Fatalf("CEL extraction of %q fails unexpectedly.\nGOT:\n%s\nEXPECTED:\n%s", test.Expression, actualOutput, test.ExpectedOutput)
			}
		})
	}
}

// A JSON null must end up as NaN, just like with the JSONPath engine.
func TestExtractValueCELNull(t *testing.T) {
	value, _, err := extractValueCEL(promslog.NewNopLogger(), []byte(celTestDocument), ".nothing", false, false)
	if err != nil {
		t.Fatal(err)
	}
	floatValue, err := SanitizeValue(value)
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsNaN(floatValue) {
		t.Fatalf("CEL extraction of a null value returned %q, expected NaN", value)
	}
}

// allow_missing_key turns the errors CEL reports for data the document does not
// contain into a skipped metric. Any other evaluation error is still an error.
func TestExtractValueCELAllowMissingKey(t *testing.T) {
	tests := []struct {
		Name            string
		Expression      string
		ExpectedMissing bool
		ShouldSucceed   bool
	}{
		{"missing member", ".missing", true, true},
		{"missing member of a member", ".values[0].missing", true, true},
		{"index out of bounds", ".values[99]", true, true},
		{"missing key of a map", `.values[0]["missing"]`, true, true},
		{"present member", ".counter", false, true},
		{"type error", `.counter + .location`, false, false},
	}

	logger := promslog.NewNopLogger()
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			_, missing, err := extractValueCEL(logger, []byte(celTestDocument), test.Expression, false, true)
			if test.ShouldSucceed && err != nil {
				t.Fatalf("CEL extraction of %q failed with an unexpected error: %s", test.Expression, err)
			}
			if !test.ShouldSucceed && err == nil {
				t.Fatalf("CEL extraction of %q succeeded unexpectedly", test.Expression)
			}
			if missing != test.ExpectedMissing {
				t.Fatalf("CEL extraction of %q reported missing=%t, expected %t", test.Expression, missing, test.ExpectedMissing)
			}
		})
	}
}

func TestCompileCELExpressionIsCached(t *testing.T) {
	const expression = `"cache me"`

	first, err := compileCELExpression(expression)
	if err != nil {
		t.Fatal(err)
	}
	second, err := compileCELExpression(expression)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("compileCELExpression returned a new program for an already compiled expression")
	}

	// Failures are cached as well, so that a broken expression does not compile
	// on every scrape.
	if _, err := compileCELExpression("("); err == nil {
		t.Fatal("Compilation of an invalid expression succeeded unexpectedly")
	}
	if _, err := compileCELExpression("("); err == nil {
		t.Fatal("Repeated compilation of an invalid expression succeeded unexpectedly")
	}
}
