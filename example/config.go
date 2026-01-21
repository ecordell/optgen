package example

// Config represents a configuration struct for testing optgen
type Config struct {
	Name     string                 `debugmap:"visible"`
	Port     int                    `debugmap:"visible"`
	Enabled  bool                   `debugmap:"visible"`
	Timeout  *int                   `debugmap:"visible"`
	Tags     []string               `debugmap:"visible-format"`
	Metadata map[string]interface{} `debugmap:"visible-format"`
	Debug    bool                   `debugmap:"visible"`
}

// Server represents another test struct
type Server struct {
	Host    string `debugmap:"visible"`
	Port    int    `debugmap:"visible"`
	TLS     bool   `debugmap:"visible"`
	Cert    string `debugmap:"sensitive"`
	Key     string `debugmap:"sensitive"`
	Workers int    `debugmap:"visible"`
}
