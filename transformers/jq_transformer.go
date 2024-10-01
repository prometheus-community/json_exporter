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

import (
	"encoding/json"
	"github.com/itchyny/gojq"
)

// JQTransformer struct for jq transformation
type JQTransformer struct {
	Query string
}

// NewJQTransformer creates a new JQTransformer with a given query
func NewJQTransformer(query string) JQTransformer {
	return JQTransformer{Query: query}
}

// Transform applies the jq filter transformation to the input data
func (jq JQTransformer) Transform(data []byte) ([]byte, error) {
	return applyJQFilter(data, jq.Query)
}

// applyJQFilter uses gojq to apply a jq transformation to the input data
func applyJQFilter(jsonData []byte, jqQuery string) ([]byte, error) {
	var input interface{}
	if err := json.Unmarshal(jsonData, &input); err != nil {
		return nil, err
	}

	query, err := gojq.Parse(jqQuery)
	if err != nil {
		return nil, err
	}

	iter := query.Run(input)
	var results []interface{}
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return nil, err
		}
		results = append(results, v)
	}

	// Convert the transformed result back to JSON []byte
	transformedJSON, err := json.Marshal(results)
	if err != nil {
		return nil, err
	}

	return transformedJSON, nil
}
