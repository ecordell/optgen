package testdata

// BasicConfig is a simple struct for testing basic field types
type BasicConfig struct {
	Name    string `debugmap:"visible"`
	Port    int    `debugmap:"visible"`
	Enabled bool   `debugmap:"visible"`
	Timeout *int   `debugmap:"visible"`
}
