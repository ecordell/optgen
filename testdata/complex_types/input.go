package testdata

// Nested is a struct type used as a field in ComplexConfig.
// It does not have a DebugMap method, so the fallback fmt.Sprintf("%+v") is used.
type Nested struct {
	Host string
	Port int
}

// Stringer is an interface type
type Stringer interface {
	String() string
}

// ComplexConfig tests complex field types: structs, interfaces, and functions
type ComplexConfig struct {
	// Struct value - should use DebugMap() if available, else %+v
	Database Nested `debugmap:"visible"`

	// Interface field - should show type name, not pointer address
	Logger Stringer `debugmap:"visible"`

	// Function field - should show type name, not pointer address
	OnError func(error) `debugmap:"visible"`

	// Named type from same package (non-primitive ident) - should use %+v
	Backup Nested `debugmap:"visible"`
}
