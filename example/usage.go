package example

import "fmt"

// ExampleUsage demonstrates how to use the generated option functions
func ExampleUsage() {
	// Create a config with functional options
	config := NewConfigWithOptions(
		WithConfigName("my-app"),
		WithConfigPort(3000),
		WithConfigEnabled(true),
		WithConfigTags("production"),
		WithConfigTags("v1.0"),
	)

	fmt.Printf("Config created: %s on port %d\n", config.Name, config.Port)

	// Use DebugMap for safe logging (hides sensitive fields)
	fmt.Printf("Config debug: %+v\n", config.DebugMap())

	// Update existing config with more options
	updatedConfig := config.WithOptions(
		WithConfigName("my-app-updated"),
		WithConfigDebug(true),
	)

	fmt.Printf("Updated config: %s (debug=%v)\n", updatedConfig.Name, updatedConfig.Debug)

	// Create config with defaults
	configWithDefaults := NewConfigWithOptionsAndDefaults(
		WithConfigName("default-app"),
	)

	fmt.Printf("Config with defaults: %+v\n", configWithDefaults)
}
