package testdata

// HiddenFields tests hidden tag
type HiddenFields struct {
	PublicName  string `debugmap:"visible"`
	HiddenField string `debugmap:"hidden"`
	AnotherName string `debugmap:"visible"`
}
