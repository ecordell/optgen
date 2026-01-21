//go:build mage

package main

import (
	"fmt"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Test mg.Namespace

// Unit runs all unit tests
func (Test) Unit() error {
	fmt.Println("Running unit tests...")
	return sh.RunV("go", "test", "-v", "./...")
}

// Coverage runs tests with coverage report
func (Test) Coverage() error {
	fmt.Println("Running tests with coverage...")
	if err := sh.RunV("go", "test", "-coverprofile=coverage.out", "./..."); err != nil {
		return err
	}
	return sh.RunV("go", "tool", "cover", "-html=coverage.out", "-o", "coverage.html")
}

// UpdateGolden updates golden files for tests
func (Test) UpdateGolden() error {
	fmt.Println("Updating golden files...")
	return sh.RunV("go", "test", "-update")
}

// All runs all tests and checks
func (Test) All() error {
	mg.Deps(Test.Unit)
	return nil
}
