package testdata

// Address contains address information
type Address struct {
	Street string `optgen:"generate" debugmap:"visible"`
	City   string `optgen:"generate" debugmap:"visible"`
}

// Config is a configuration with flattened nested struct
type Config struct {
	Name    string  `optgen:"generate" debugmap:"visible"`
	Address Address `optgen:"generate,flatten" debugmap:"visible"`
}
