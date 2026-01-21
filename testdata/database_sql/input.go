package testdata

import (
	"database/sql"
)

// DatabaseConfig tests database/sql package types
type DatabaseConfig struct {
	ConnectionString sql.NullString `debugmap:"sensitive"`
	MaxConnections   sql.NullInt64  `debugmap:"visible"`
	Enabled          bool           `debugmap:"visible"`
}
