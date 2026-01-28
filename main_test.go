package main

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
		{"optgen tags", "testdata/optgen_tags", "OptgenTagTest"},
		{"unexported fields", "testdata/unexported", "UnexportedTest"},
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

			// Run optgen
			cmd := exec.Command("./optgen_testbin", "-output="+outputFile, tt.inputDir, tt.structName)
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
