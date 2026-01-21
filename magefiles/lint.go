//go:build mage

package main

import (
	"fmt"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Lint mg.Namespace

// Go runs golangci-lint on the codebase
func (Lint) Go() error {
	fmt.Println("Running golangci-lint...")
	return sh.RunV("golangci-lint", "run", "--timeout=5m", "./...")
}

// Format checks if code is properly formatted
func (Lint) Format() error {
	fmt.Println("Checking code formatting...")
	return sh.RunV("gofmt", "-l", "-s", ".")
}

// Imports checks if imports are properly organized
func (Lint) Imports() error {
	fmt.Println("Checking import organization...")
	return sh.RunV("goimports", "-l", ".")
}

// All runs all linting checks
func (Lint) All() error {
	mg.Deps(Lint.Go, Lint.Format, Lint.Imports)
	return nil
}
