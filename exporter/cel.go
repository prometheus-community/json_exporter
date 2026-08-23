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
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/common/types/traits"
	"google.golang.org/protobuf/types/known/structpb"
)

// celRootVariable is the name under which the whole scraped document is made
// available to CEL expressions. It allows expressions on documents that are not
// JSON objects, e.g. a top-level array. For JSON objects the top-level members
// are bound as variables as well, so that '.foo' and 'root.foo' are equivalent.
// A document member actually named 'root' takes precedence over the binding.
const celRootVariable = "root"

var (
	celEnvOnce sync.Once
	celEnv     *cel.Env
	celEnvErr  error

	// celPrograms caches the compiled program of every expression seen so far.
	// Expressions come from the config file, so the cache is bounded by it.
	celPrograms sync.Map // string -> *celProgram
)

type celProgram struct {
	program cel.Program
	err     error
}

// celEnvironment returns the process-wide CEL environment.
func celEnvironment() (*cel.Env, error) {
	celEnvOnce.Do(func() {
		celEnv, celEnvErr = cel.NewEnv()
	})
	return celEnv, celEnvErr
}

// compileCELExpression parses an expression and caches the resulting program.
// The expression is not type-checked, as the shape of the scraped document is
// only known at scrape time: unknown members are reported when the expression
// is evaluated, not when it is compiled.
func compileCELExpression(expression string) (cel.Program, error) {
	if cached, ok := celPrograms.Load(expression); ok {
		p := cached.(*celProgram)
		return p.program, p.err
	}

	p := &celProgram{}
	env, err := celEnvironment()
	if err != nil {
		p.err = fmt.Errorf("failed to set up CEL environment: %w", err)
	} else if ast, issues := env.Parse(expression); issues != nil && issues.Err() != nil {
		p.err = fmt.Errorf("failed to parse CEL expression %q: %w", expression, issues.Err())
	} else if p.program, err = env.Program(ast); err != nil {
		p.err = fmt.Errorf("failed to build CEL program for expression %q: %w", expression, err)
	}

	cached, _ := celPrograms.LoadOrStore(expression, p)
	stored := cached.(*celProgram)
	return stored.program, stored.err
}

// extractValueCEL returns the result of evaluating the CEL expression against
// the given JSON document, and a flag telling whether the expression referred
// to something the document does not contain while allowMissingKey is set.
func extractValueCEL(logger *slog.Logger, data []byte, expression string, enableJSONOutput, allowMissingKey bool) (string, bool, error) {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		logger.Error("Failed to unmarshal data to json", "err", err, "data", string(data))
		return "", false, err
	}

	program, err := compileCELExpression(expression)
	if err != nil {
		logger.Error("Failed to compile CEL expression", "err", err, "expression", expression)
		return "", false, err
	}

	out, _, err := program.Eval(celActivation(document))
	if err != nil {
		if allowMissingKey && isCELMissingValueError(err) {
			return "", true, nil
		}
		logger.Error("Failed to evaluate CEL expression", "err", err, "expression", expression)
		return "", false, err
	}

	var value string
	if enableJSONOutput {
		value, err = celValueToJSONList(out)
	} else {
		value, err = celValueToString(out)
	}
	return value, false, err
}

// isCELMissingValueError reports whether an evaluation failed because the
// document does not contain what the expression selects. CEL has no typed
// errors for this, so the messages it builds for a missing variable, a missing
// map key and an out-of-range index are matched instead. The corresponding test
// catches it if CEL ever rewords them.
func isCELMissingValueError(err error) bool {
	message := err.Error()
	for _, prefix := range []string{"no such attribute", "no such key", "no such field", "index out of bounds"} {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}

// celActivation binds the document to the variables an expression can refer to.
func celActivation(document any) map[string]any {
	object, ok := document.(map[string]any)
	if !ok {
		return map[string]any{celRootVariable: document}
	}
	activation := make(map[string]any, len(object)+1)
	activation[celRootVariable] = document
	// Members of the document shadow the root binding.
	maps.Copy(activation, object)
	return activation
}

// celValueToString renders a CEL value the way the rest of the exporter expects
// it: as something SanitizeValue can turn into a float64.
func celValueToString(value ref.Val) (string, error) {
	switch v := value.(type) {
	case types.Null:
		// Matches the JSONPath engine, where a null becomes NaN.
		return "<nil>", nil
	case types.Double:
		// Every JSON number decodes to a double. Format it without an exponent,
		// so that whole numbers stay parseable as integers, e.g. as timestamps.
		return strconv.FormatFloat(float64(v), 'f', -1, 64), nil
	case traits.Lister, traits.Mapper:
		return celValueToJSON(value)
	default:
		return fmt.Sprintf("%v", value.Value()), nil
	}
}

// celValueToJSON marshals a CEL value to JSON.
func celValueToJSON(value ref.Val) (string, error) {
	native, err := celValueToNative(value)
	if err != nil {
		return "", err
	}
	return marshalJSON(native)
}

// celValueToJSONList marshals a CEL value as a JSON list. Object scrapes
// iterate over the result, and a result that is not a list is scraped as a
// single object, just like a JSONPath expression matching a single object.
func celValueToJSONList(value ref.Val) (string, error) {
	native, err := celValueToNative(value)
	if err != nil {
		return "", err
	}
	if _, ok := native.([]any); !ok {
		native = []any{native}
	}
	return marshalJSON(native)
}

// celValueToNative converts a CEL value to plain Go types. It takes the detour
// through a structpb value, as that is the only conversion CEL implements for
// every type a JSON document can hold.
func celValueToNative(value ref.Val) (any, error) {
	converted, err := value.ConvertToNative(reflect.TypeOf(&structpb.Value{}))
	if err != nil {
		return nil, fmt.Errorf("failed to convert CEL value to JSON: %w", err)
	}
	return converted.(*structpb.Value).AsInterface(), nil
}

func marshalJSON(value any) (string, error) {
	marshalled, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to marshal CEL value to JSON: %w", err)
	}
	return string(marshalled), nil
}
