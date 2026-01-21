package testdata

// FormatTest tests visible-format tag
type FormatTest struct {
	Name  string            `debugmap:"visible"`
	Data  map[string]string `debugmap:"visible-format"`
	Count int               `debugmap:"visible"`
}
