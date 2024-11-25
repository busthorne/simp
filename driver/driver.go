package driver

import "strings"

var Drivers = []string{"openai", "anthropic", "gemini", "dify", "vertex"}

const (
	chatCompletions = "/v1/chat/completions"
	embeddings      = "/v1/embeddings"
)

func ListString() string {
	return strings.Join(Drivers, ", ")
}
