package testdata

// OptgenTagTest demonstrates optgen tag usage
type OptgenTagTest struct {
	// Normal field - generates With function
	Name string `optgen:"generate" debugmap:"visible"`

	// Skip generation - no With* function
	Internal int `optgen:"skip" debugmap:"hidden"`

	// Readonly - only in ToOption, not With*
	ID string `optgen:"readonly" debugmap:"visible"`

	// No tag - defaults to generate for exported
	Port int `debugmap:"visible"`

	// Slice with skip
	InternalData []byte `optgen:"skip" debugmap:"hidden"`
}
