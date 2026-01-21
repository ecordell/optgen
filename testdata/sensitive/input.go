package testdata

// Credentials tests sensitive field handling
type Credentials struct {
	Username string `debugmap:"visible"`
	Password string `debugmap:"sensitive"`
	APIKey   string `debugmap:"sensitive"`
	Host     string `debugmap:"visible"`
}
