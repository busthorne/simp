package simp

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/sashabaranov/go-openai"
)

// Driver is a roughly OpenAI-compatible inference backend.
type Driver interface {
	List(context.Context) ([]Model, error)
	Embed(context.Context, Embed) (Embeddings, error)
	Complete(context.Context, Complete) (*Completion, error)
	// CompleteBatch(context.Context, []Complete) ([]Completion, error)
}

var (
	// Drivers is how we reference drivers by name.
	Drivers = map[string]Driver{}

	// Path is $SIMPPATH defaulting to $HOME/.simp
	Path string

	// ErrUnsupported is returned when a driver does not support a method.
	ErrUnsupported = errors.New("unsupported op")
	// ErrUnsupportedMime is usually returned when a driver does not support image type.
	ErrUnsupportedMime = errors.New("unsupported mime type")
	// ErrUnsupportedRole is returned when role is neither "user" nor "assistant".
	ErrUnsupportedRole = errors.New("unsupported role")
)

func init() {
	Path = os.Getenv("SIMPPATH")
	if Path == "" {
		Path = filepath.Join(os.Getenv("HOME"), ".simp")
	}
}

// I hate long type names.
type (
	Model      = openai.Model
	Complete   = openai.ChatCompletionRequest
	Embed      = openai.EmbeddingRequest
	Embeddings = openai.EmbeddingResponse
)

// Completion is like openai.ChatCompletionResponse if it did streaming well.
//
// It was always a horrible idea to have two of them.
type Completion struct {
	openai.ChatCompletionResponse
	Stream chan openai.ChatCompletionStreamResponse `json:"-"`

	Err error `json:"-"`
}
