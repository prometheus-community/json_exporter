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

func TestSanitizeIntValue(t *testing.T) {
	tests := []struct {
		Input          string
		ExpectedOutput int64
		ShouldSucceed  bool
	}{
		// Baseline int64 parsing.
		{"1234", 1234, true},
		{"0", 0, true},
		{"-1234", -1234, true},
		{"9223372036854775807", 9223372036854775807, true}, // math.MaxInt64
		{"-9223372036854775808", -9223372036854775808, true},

		// Rust-style "u64" suffix should be stripped before parsing.
		{"1234u64", 1234, true},
		{"0u64", 0, true},
		{"9223372036854775807u64", 9223372036854775807, true},

		// Values exceeding int64 range must still fail even after stripping.
		{"9223372036854775808u64", 0, false},  // math.MaxInt64 + 1
		{"18446744073709551615u64", 0, false}, // math.MaxUint64

		// Suffix stripping is exact: only trailing "u64", case-sensitive, whole suffix.
		{"1234U64", 0, false}, // uppercase not stripped
		{"1234u32", 0, false}, // only u64 handled
		{"u641234", 0, false}, // suffix must be at end
		{"u64", 0, false},     // empty after stripping

		// Non-numeric and float inputs are not integers.
		{"abcd", 0, false},
		{"1234.5", 0, false},
		{"1234.5u64", 0, false}, // stripped to "1234.5" - still not an int
		{"", 0, false},
		{"true", 0, false},
	}

	for i, test := range tests {
		actualOutput, err := SanitizeIntValue(test.Input)
		if err != nil && test.ShouldSucceed {
			t.Fatalf("Int value sanitization test %d failed with an unexpected error.\nINPUT:\n%q\nERR:\n%s", i, test.Input, err)
		}
		if err == nil && !test.ShouldSucceed {
			t.Fatalf("Int value sanitization test %d succeeded unexpectedly.\nINPUT:\n%q\nGOT:\n%d", i, test.Input, actualOutput)
		}
		if test.ShouldSucceed && actualOutput != test.ExpectedOutput {
			t.Fatalf("Int value sanitization test %d fails unexpectedly.\nINPUT:\n%q\nGOT:\n%d\nEXPECTED:\n%d", i, test.Input, actualOutput, test.ExpectedOutput)
		}
	}
}
