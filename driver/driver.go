package driver

import "strings"

var Drivers = []string{"openai", "anthropic", "gemini", "dify", "vertex"}

func ListString() string {
	return strings.Join(Drivers, ", ")
}
