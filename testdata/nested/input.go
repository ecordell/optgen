package testdata

// NestedConfig has sensitive fields — optgen will generate DebugMap() for it.
type NestedConfig struct {
	URI    string `debugmap:"sensitive"`
	Engine string `debugmap:"visible"`
}

// OuterConfig references NestedConfig as both a value and a pointer.
type OuterConfig struct {
	Name      string        `debugmap:"visible"`
	Nested    NestedConfig  `debugmap:"visible"`
	NestedPtr *NestedConfig `debugmap:"visible"`
}
