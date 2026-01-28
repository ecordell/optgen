package testdata

// ServerMetadata contains metadata about a server
type ServerMetadata struct {
	Name  string `optgen:"generate" debugmap:"visible"`
	Owner string `optgen:"generate" debugmap:"visible"`
}

// Config is a configuration with nested struct
type Config struct {
	Port     int            `optgen:"generate" debugmap:"visible"`
	Metadata ServerMetadata `optgen:"generate,recursive" debugmap:"visible"`
}
