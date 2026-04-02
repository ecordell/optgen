package main_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	basic "github.com/ecordell/optgen/testdata/basic"
	hidden "github.com/ecordell/optgen/testdata/hidden"
	nested "github.com/ecordell/optgen/testdata/nested"
	sensitive "github.com/ecordell/optgen/testdata/sensitive"
)

var update = flag.Bool("update", false, "update golden files")

func TestGoldenFiles(t *testing.T) {
	// Build the tool first
	buildCmd := exec.Command("go", "build", "-o", "optgen_testbin", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build optgen: %v", err)
	}
	defer func() {
		_ = os.Remove("optgen_testbin")
	}()

	tests := []struct {
		name       string
		inputDir   string
		structName string
	}{
		{"basic types", "testdata/basic", "BasicConfig"},
		{"slices and maps", "testdata/slices_maps", "SlicesAndMaps"},
		{"sensitive fields", "testdata/sensitive", "Credentials"},
		{"visible-format", "testdata/visible_format", "FormatTest"},
		{"hidden fields", "testdata/hidden", "HiddenFields"},
		{"cross package types", "testdata/cross_package", "CrossPackage"},
		{"database/sql types", "testdata/database_sql", "DatabaseConfig"},
		{"generic types", "testdata/generics", "GenericConfig"},
		{"nested struct delegation", "testdata/nested", "NestedConfig OuterConfig"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup paths
			outputFile := filepath.Join(tt.inputDir, "output_test.go")
			goldenFile := filepath.Join(tt.inputDir, "golden.go")

			// Clean up any existing output
			defer func() {
				_ = os.Remove(outputFile)
			}()

			// Run optgen (structName may be space-separated for multiple structs)
			args := append([]string{"-output=" + outputFile, tt.inputDir}, strings.Fields(tt.structName)...)
			cmd := exec.Command("./optgen_testbin", args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("generation failed: %v\nOutput: %s", err, output)
			}

			// Read generated output
			generated, err := os.ReadFile(outputFile)
			if err != nil {
				t.Fatalf("failed to read generated file: %v", err)
			}

			// Update golden file if flag is set
			if *update {
				err := os.WriteFile(goldenFile, generated, 0o644)
				if err != nil {
					t.Fatalf("failed to update golden file: %v", err)
				}
				t.Logf("Updated golden file: %s", goldenFile)
				return
			}

			// Compare with golden file
			golden, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("failed to read golden file: %v", err)
			}

			if !bytes.Equal(generated, golden) {
				t.Errorf("Generated output differs from golden file.\nRun 'go test -update' to update golden files.\nGolden: %s\nGenerated: %s", goldenFile, outputFile)
			}
		})
	}
}

type debugMapper interface {
	DebugMap() map[string]any
	FlatDebugMap() map[string]any
}

func ptr[T any](v T) *T { return &v }

func TestDebugMap(t *testing.T) {
	tests := []struct {
		name     string
		obj      debugMapper
		want     string
		wantFlat string
	}{
		// BasicConfig
		{
			name:     "basic/all fields",
			obj:      &basic.BasicConfig{Name: "myservice", Port: 8080, Enabled: true, Timeout: ptr(30)},
			want:     `map[Enabled:true Name:myservice Port:8080 Timeout:30]`,
			wantFlat: `map[Enabled:true Name:myservice Port:8080 Timeout:30]`,
		},
		{
			name:     "basic/empty string and nil pointer",
			obj:      &basic.BasicConfig{},
			want:     `map[Enabled:false Name:(empty) Port:0 Timeout:nil]`,
			wantFlat: `map[Enabled:false Name:(empty) Port:0 Timeout:nil]`,
		},

		// Credentials
		{
			name:     "sensitive/fields redacted",
			obj:      &sensitive.Credentials{Username: "alice", Password: "s3cr3t", APIKey: "key-abc-123", Host: "db.example.com"},
			want:     `map[APIKey:(sensitive) Host:db.example.com Password:(sensitive) Username:alice]`,
			wantFlat: `map[APIKey:(sensitive) Host:db.example.com Password:(sensitive) Username:alice]`,
		},

		// HiddenFields
		{
			name:     "hidden/fields absent from map",
			obj:      &hidden.HiddenFields{PublicName: "visible", HiddenField: "secret", AnotherName: "also visible"},
			want:     `map[AnotherName:also visible PublicName:visible]`,
			wantFlat: `map[AnotherName:also visible PublicName:visible]`,
		},

		// NestedConfig
		{
			name:     "nested/NestedConfig redacts sensitive sub-field",
			obj:      &nested.NestedConfig{URI: "postgresql://user:s3cr3t@host/db", Engine: "postgres"},
			want:     `map[Engine:postgres URI:(sensitive)]`,
			wantFlat: `map[Engine:postgres URI:(sensitive)]`,
		},

		// OuterConfig
		{
			name: "nested/OuterConfig value field delegates, nil pointer",
			obj: &nested.OuterConfig{
				Name:   "svc",
				Nested: nested.NestedConfig{URI: "postgresql://user:s3cr3t@host/db", Engine: "postgres"},
			},
			want:     `map[Name:svc Nested:map[Engine:postgres URI:(sensitive)] NestedPtr:nil]`,
			wantFlat: `map[Name:svc Nested.Engine:postgres Nested.URI:(sensitive) NestedPtr:nil]`,
		},
		{
			name: "nested/OuterConfig value and pointer fields both delegate",
			obj: &nested.OuterConfig{
				Name:      "svc",
				Nested:    nested.NestedConfig{URI: "postgresql://user:s3cr3t@host/db", Engine: "postgres"},
				NestedPtr: &nested.NestedConfig{URI: "redis://:password@host:6379", Engine: "redis"},
			},
			want:     `map[Name:svc Nested:map[Engine:postgres URI:(sensitive)] NestedPtr:map[Engine:redis URI:(sensitive)]]`,
			wantFlat: `map[Name:svc Nested.Engine:postgres Nested.URI:(sensitive) NestedPtr.Engine:redis NestedPtr.URI:(sensitive)]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmt.Sprintf("%v", tt.obj.DebugMap()); got != tt.want {
				t.Errorf("DebugMap:\ngot  %s\nwant %s", got, tt.want)
			}
			if got := fmt.Sprintf("%v", tt.obj.FlatDebugMap()); got != tt.wantFlat {
				t.Errorf("FlatDebugMap:\ngot  %s\nwant %s", got, tt.wantFlat)
			}
		})
	}
}
