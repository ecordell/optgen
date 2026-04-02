package testdata

import "testing"

// TestDebugMapRedactsNestedSensitiveFields verifies that DebugMap() on a struct
// with nested complex-type fields correctly delegates to the nested type's DebugMap(),
// redacting sensitive sub-fields — for both value and pointer field types.
func TestDebugMapRedactsNestedSensitiveFields(t *testing.T) {
	cfg := OuterConfig{
		Name: "myservice",
		Nested: NestedConfig{
			URI:    "postgresql://user:s3cr3t@host/db",
			Engine: "postgres",
		},
		NestedPtr: &NestedConfig{
			URI:    "postgresql://user:s3cr3t@host/db",
			Engine: "postgres",
		},
	}

	m := cfg.DebugMap()

	// Value field: Nested NestedConfig
	nestedMap, ok := m["Nested"].(map[string]any)
	if !ok {
		t.Fatalf("Nested: want map[string]any, got %T (%v)", m["Nested"], m["Nested"])
	}
	if nestedMap["URI"] != "(sensitive)" {
		t.Errorf("Nested.URI = %v, want (sensitive)", nestedMap["URI"])
	}
	if nestedMap["Engine"] != "postgres" {
		t.Errorf("Nested.Engine = %v, want postgres", nestedMap["Engine"])
	}

	// Pointer field: NestedPtr *NestedConfig
	nestedPtrMap, ok := m["NestedPtr"].(map[string]any)
	if !ok {
		t.Fatalf("NestedPtr: want map[string]any, got %T (%v)", m["NestedPtr"], m["NestedPtr"])
	}
	if nestedPtrMap["URI"] != "(sensitive)" {
		t.Errorf("NestedPtr.URI = %v, want (sensitive)", nestedPtrMap["URI"])
	}
	if nestedPtrMap["Engine"] != "postgres" {
		t.Errorf("NestedPtr.Engine = %v, want postgres", nestedPtrMap["Engine"])
	}
}
