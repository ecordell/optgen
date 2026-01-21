package testdata

// Container is a generic container type
type Container[T any] struct {
	Value T
}

// Pair is a generic type with two type parameters
type Pair[K comparable, V any] struct {
	Key   K
	Value V
}

// GenericConfig demonstrates various generic field types
type GenericConfig struct {
	// Single type parameter
	StringContainer Container[string] `debugmap:"visible"`
	IntContainer    Container[int]    `debugmap:"visible"`

	// Multiple type parameters
	StringIntPair Pair[string, int] `debugmap:"visible"`

	// Slices of generic types
	Containers []Container[string] `debugmap:"visible"`
	Pairs      []Pair[int, string] `debugmap:"visible"`

	// Pointers to generic types
	OptionalContainer *Container[bool] `debugmap:"visible"`

	// Map with generic value type
	ContainerMap map[string]Container[int] `debugmap:"visible"`
}
