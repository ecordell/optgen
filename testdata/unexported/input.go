package testdata

// UnexportedTest demonstrates unexported field generation with visibility control
type UnexportedTest struct {
	// Public field, public setter (default)
	Host string `debugmap:"visible"`

	// Private field, public setter
	maxRetries int `optgen:"generate,public" debugmap:"visible"`

	// Private field, private setter (default for unexported with generate)
	buffer []byte `optgen:"generate" debugmap:"hidden"`

	// Public field, private setter (unusual but supported)
	Cache map[string]any `optgen:"generate,private" debugmap:"hidden"`

	// Private readonly with public visibility
	id string `optgen:"readonly,public" debugmap:"visible"`
}
