package helpers

import (
	"fmt"
	"reflect"
)

type withDebugMap interface {
	DebugMap() map[string]any
}

// DebugValue returns the debug value for the given raw Go value. If the value
// is a primitive, it is directly returned. If the value is itself a generated
// Config with a DebugMap function, the DebugMap is invoked. Otherwise, "(value)"
// is returned, unless fmtValue is specified, in which case it is returned as the
// result of fmt.Sprintf.
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

// SensitiveDebugValue returns the string "nil" if the value is nil, "(empty)"
// if empty and otherwise returns "(sensitive)".
func SensitiveDebugValue(value any) any {
	if value == nil {
		return "nil"
	}

	if value == "" {
		return "(empty)"
	}

	return "(sensitive)"
}

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
