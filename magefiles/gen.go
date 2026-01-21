//go:build mage

package main

import (
	"fmt"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Gen mg.Namespace

// Example regenerates the example config options
func (Gen) Example() error {
	fmt.Println("Regenerating example options...")
	return sh.RunV("go", "run", ".", "-output=example/config_options.go", "example", "Config", "Server")
}

// Verify regenerates examples and checks if files changed
func (Gen) Verify() error {
	fmt.Println("Verifying generated files are up to date...")
	mg.Deps(Gen.Example)

	// Check if git shows any changes
	out, err := sh.Output("git", "status", "--porcelain", "example/")
	if err != nil {
		return err
	}

	if out != "" {
		return fmt.Errorf("generated files are out of date, run 'mage gen:example'")
	}

	fmt.Println("Generated files are up to date!")
	return nil
}
