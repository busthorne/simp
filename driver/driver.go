package driver

import "strings"

var Drivers = []string{"openai", "anthropic", "gemini", "dify"}

func ListString() string {
	return strings.Join(Drivers, ", ")
}
