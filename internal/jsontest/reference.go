// Package jsontest provides reference JSON formatting for tests and benchmarks.
package jsontest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
)

// ReferenceFormat formats jq results using encoding/json.
func ReferenceFormat(results []any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	for i, result := range results {
		if i > 0 {
			buf.WriteByte('\n')
		}
		start := buf.Len()
		if err := enc.Encode(result); err != nil {
			buf.Truncate(start)
			fmt.Fprintf(&buf, "%v", result)
			continue
		}
		buf.Truncate(buf.Len() - 1)
	}
	return buf.String()
}

// DrainAndFormat consumes an iterator and formats its values. The first jq
// error value is returned without formatting partial output.
func DrainAndFormat(iter gojq.Iter) (string, error) {
	var results []any
	for {
		value, ok := iter.Next()
		if !ok {
			return ReferenceFormat(results), nil
		}
		if err, isErr := value.(error); isErr {
			return "", err
		}
		results = append(results, value)
	}
}

// RunAndFormat evaluates a filter through the service iterator and formats it.
func RunAndFormat(ctx context.Context, service interface {
	Iter(context.Context, string) (gojq.Iter, error)
}, filter string) (string, error) {
	iter, err := service.Iter(ctx, filter)
	if err != nil {
		return "", err
	}
	return DrainAndFormat(iter)
}
