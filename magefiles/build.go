//go:build mage

package main

import (
	"fmt"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Build mg.Namespace

// Binary builds the optgen binary
func (Build) Binary() error {
	fmt.Println("Building optgen binary...")
	return sh.RunV("go", "build", "-o", "bin/optgen", ".")
}

// Install installs optgen to GOPATH/bin
func (Build) Install() error {
	fmt.Println("Installing optgen...")
	return sh.RunV("go", "install", ".")
}

// Clean removes built artifacts
func (Build) Clean() error {
	fmt.Println("Cleaning build artifacts...")
	return sh.Rm("bin")
}
