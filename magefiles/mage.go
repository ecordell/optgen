//go:build mage

package main

import (
	"fmt"

	"github.com/magefile/mage/mg"
)

// Default target runs tests
var Default = Test.All

// CI runs all checks for continuous integration
func CI() error {
	fmt.Println("Running CI checks...")
	mg.Deps(Test.All, Lint.All, Gen.Verify)
	return nil
}
