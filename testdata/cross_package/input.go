package testdata

import (
	"time"
)

// CrossPackage tests cross-package types
type CrossPackage struct {
	Name      string    `debugmap:"visible"`
	Timestamp time.Time `debugmap:"visible"`
	Duration  time.Duration `debugmap:"visible"`
}
