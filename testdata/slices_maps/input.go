package testdata

// SlicesAndMaps tests slice and map field handling
type SlicesAndMaps struct {
	Tags     []string               `debugmap:"visible-format"`
	Metadata map[string]interface{} `debugmap:"visible-format"`
	Ports    []int                  `debugmap:"visible-format"`
}
