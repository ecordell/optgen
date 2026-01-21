// Package helpers provides utility functions for optgen-generated code.
//
// The helpers package contains functions used by generated option code,
// primarily for creating safe debug representations of values.
package helpers

import (
	"fmt"
	"reflect"
)

type withDebugMap interface {
	DebugMap() map[string]any
}

// DebugValue returns a safe debug representation of a value.
//
// For primitive types (string, int, bool, etc.), it returns the value directly.
// For maps and slices, it returns a size indicator unless fmtValue is true.
// For types implementing the withDebugMap interface, it calls their DebugMap method.
//
// Parameters:
//   - value: The value to convert for debug output
//   - fmtValue: If true, formats complex values using fmt.Sprintf instead of size indicators
//
// Returns a debug-safe representation suitable for logging.
func DebugValue(value any, fmtValue bool) any {
	if value == nil {
		return "nil"
	}

	if value == "" {
		return "(empty)"
	}

	if wdm, ok := value.(withDebugMap); ok {
		return wdm.DebugMap()
	}

	switch reflect.TypeOf(value).Kind() {
	case reflect.Map:
		if fmtValue {
			return fmt.Sprintf("%v", value)
		}

		return fmt.Sprintf("(map of size %d)", reflect.ValueOf(value).Len())

	case reflect.Slice:
		slce, ok := value.([]any)
		if !ok {
			if fmtValue {
				return fmt.Sprintf("%v", value)
			}

			return fmt.Sprintf("(slice of size %d)", reflect.ValueOf(value).Len())
		}

		updated := make([]any, 0, len(slce))
		for _, vle := range slce {
			updated = append(updated, DebugValue(vle, fmtValue))
		}
		return updated

	case reflect.String:
		fallthrough

	case reflect.Int:
		fallthrough

	case reflect.Int8:
		fallthrough

	case reflect.Int16:
		fallthrough

	case reflect.Int32:
		fallthrough

	case reflect.Int64:
		fallthrough

	case reflect.Uint:
		fallthrough

	case reflect.Uint8:
		fallthrough

	case reflect.Uint16:
		fallthrough

	case reflect.Uint32:
		fallthrough

	case reflect.Uint64:
		fallthrough

	case reflect.Bool:
		fallthrough

	case reflect.Float32:
		fallthrough

	case reflect.Float64:
		return value

	default:
		if fmtValue {
			return fmt.Sprintf("%v", value)
		}
		return "(value)"
	}
}

// SensitiveDebugValue returns a safe placeholder for sensitive values.
//
// This function should be used for passwords, API keys, tokens, and other
// sensitive data that should not appear in logs or debug output.
//
// Returns:
//   - "nil" if value is nil
//   - "(empty)" if value is empty string
//   - "(sensitive)" otherwise
func SensitiveDebugValue(value any) any {
	if value == nil {
		return "nil"
	}

	if value == "" {
		return "(empty)"
	}

	return "(sensitive)"
}

// Flatten recursively flattens nested debug maps into a single-level map.
//
// Nested keys are joined with dots. For example:
//
//	{"server": {"host": "localhost"}} becomes {"server.host": "localhost"}
//
// This is useful for structured logging systems that prefer flat key-value pairs.
func Flatten(debugMap map[string]any) map[string]any {
	flattened := make(map[string]any, len(debugMap))
	for key, value := range debugMap {
		childMap, ok := value.(map[string]any)
		if ok {
			for fk, fv := range Flatten(childMap) {
				flattened[key+"."+fk] = fv
			}
			continue
		}

		flattened[key] = value
	}
	return flattened
}
