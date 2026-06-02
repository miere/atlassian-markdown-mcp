// Args parsing: turns --flag VALUE pairs into a map keyed by the property
// names declared in a tool's InputSchema. Boolean flags and positional
// arguments are intentionally unsupported — every tool flag carries a value.
// Validation against the schema (required fields, enums, types) is left to
// the tool or a future shared validator.
package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// parseFlags converts ["--kebab-case", "value", ...] pairs into
// map[string]any keyed by snake_case property names that the tool's schema
// declares. Schema property names are the source of truth.
func parseFlags(schema *jsonschema.Schema, args []string) (map[string]any, error) {
	if schema == nil {
		if len(args) > 0 {
			return nil, fmt.Errorf("unexpected arguments: %v", args)
		}
		return nil, nil
	}
	out := make(map[string]any, len(args)/2)
	for i := 0; i < len(args); i++ {
		raw := args[i]
		if !strings.HasPrefix(raw, "--") {
			return nil, fmt.Errorf("expected --flag, got %q", raw)
		}
		key := snakeFromKebab(strings.TrimPrefix(raw, "--"))
		prop, ok := lookupProp(schema, key)
		if !ok {
			return nil, fmt.Errorf("unknown flag: --%s", strings.TrimPrefix(raw, "--"))
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("flag --%s requires a value", strings.TrimPrefix(raw, "--"))
		}
		val := args[i+1]
		i++
		v, err := coerce(val, prop)
		if err != nil {
			return nil, fmt.Errorf("--%s: %w", strings.TrimPrefix(raw, "--"), err)
		}
		out[key] = v
	}
	return out, nil
}

// snakeFromKebab maps --page-id → page_id, leaving names without dashes
// untouched.
func snakeFromKebab(s string) string { return strings.ReplaceAll(s, "-", "_") }

// lookupProp returns the schema for the given property name. Property names
// are matched case-sensitively against schema.Properties.
func lookupProp(schema *jsonschema.Schema, name string) (*jsonschema.Schema, bool) {
	if schema == nil || schema.Properties == nil {
		return nil, false
	}
	p, ok := schema.Properties[name]
	return p, ok
}

// coerce parses a raw CLI string into the Go type the property expects.
// Only the types the tools actually use today are supported.
func coerce(raw string, prop *jsonschema.Schema) (any, error) {
	t := prop.Type
	if t == "" && len(prop.Types) > 0 {
		t = prop.Types[0]
	}
	switch t {
	case "", "string":
		return raw, nil
	case "integer":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("expected integer, got %q", raw)
		}
		return n, nil
	case "number":
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("expected number, got %q", raw)
		}
		return f, nil
	case "boolean":
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("expected boolean, got %q", raw)
		}
		return b, nil
	default:
		return raw, nil
	}
}
