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

package transformers

import "testing"

func TestNewTransformerFactory(t *testing.T) {
	// Define test cases for multiple jq transformations
	tests := []struct {
		Config         TransformationConfig
		Input          string
		ExpectedOutput string
		ShouldSucceed  bool
	}{
		{
			Config: TransformationConfig{
				Type:  "jq",
				Query: `.result[] | select(.name == "pool1")`,
			},
			Input: `{
				"result": [
					{"name":"pool1","origins":[{"name":"origin1","healthy":true},{"name":"origin2","healthy":false}]},
					{"name":"pool2","origins":[{"name":"origin3","healthy":true}]}
				]
			}`,
			ExpectedOutput: `[{"name":"pool1","origins":[{"healthy":true,"name":"origin1"},{"healthy":false,"name":"origin2"}]}]`,
			ShouldSucceed:  true,
		},
		{
			Config: TransformationConfig{
				Type:  "jq",
				Query: `.result[] | select(has("origins")) | .origins[] | select(.healthy == true)`,
			},
			Input: `{
				"result": [
					{"name":"pool1","origins":[{"name":"origin1","healthy":true},{"name":"origin2","healthy":false}]},
					{"name":"pool2","origins":[{"name":"origin3","healthy":true}]}
				]
			}`,
			ExpectedOutput: `[{"healthy":true,"name":"origin1"},{"healthy":true,"name":"origin3"}]`,
			ShouldSucceed:  true,
		},
		{
			Config: TransformationConfig{
				Type:  "jq",
				Query: `.result[] | .name as $poolName | .id as $poolId | .origins[] | {endpoint_name: .name, endpoint_health: .healthy, pool_name: $poolName, address: .address, pool_id: $poolId}`,
			},
			Input:          `{"result":[{"name":"pool1","id":"1","origins":[{"name":"origin1","healthy":true, "address":"127.0.0.1"}]}]}`,
			ExpectedOutput: `[{"address":"127.0.0.1","endpoint_health":true,"endpoint_name":"origin1","pool_id":"1","pool_name":"pool1"}]`,
			ShouldSucceed:  true,
		},
		{
			Config: TransformationConfig{
				Type:  "jq",
				Query: `.result[] | select(.name == "pool2")`,
			},
			Input:          `{"result":[{"name":"pool1","id":"1","origins":[{"name":"origin1","healthy":true}]},{"name":"pool2","id":"2","origins":[{"name":"origin2","healthy":true}]}]}`,
			ExpectedOutput: `[{"id":"2","name":"pool2","origins":[{"healthy":true,"name":"origin2"}]}]`,
			ShouldSucceed:  true,
		},
		{
			Config: TransformationConfig{
				Type:  "jq",
				Query: `.result[] | select(.name == "pool2")`,
			},
			Input:          `{"result":[{"name":"pool1","id":"1","origins":[{"name":"origin1","healthy":true}]}]}`,
			ExpectedOutput: `null`,
			ShouldSucceed:  true,
		},
		{
			Config: TransformationConfig{
				Type:  "jq",
				Query: `.result[] | .origins[]`,
			},
			Input:          `{"result":[{"name":"pool1"}]}`,
			ExpectedOutput: ``,
			ShouldSucceed:  false,
		},
	}

	// Loop through each test case
	for i, test := range tests {
		// Create the transformer using NewTransformer
		transformer, err := NewTransformer(test.Config)
		if err != nil && test.ShouldSucceed {
			t.Fatalf("Failed to create transformer %d: %s", i, err)
		}

		// Apply the transformation
		output, err := transformer.Transform([]byte(test.Input))
		if err != nil && test.ShouldSucceed {
			t.Fatalf("Transformation %d failed: %s", i, err)
		}

		// Compare the actual output with the expected output
		if string(output) != test.ExpectedOutput {
			t.Fatalf("Transformation %d failed. Expected: %s, Got: %s", i, test.ExpectedOutput, string(output))
		}

		// Log the successful transformation
		t.Logf("Transformation %d succeeded. Output: %s", i, string(output))
	}
}
