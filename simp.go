package simp

import (
	"context"
	"encoding/json"
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
}

type BatchDriver interface {
}

var (
	// Path is $SIMPPATH defaulting to $HOME/.simp
	Path string

	// ErrNotImplemented is returned when a driver does not support a method.
	ErrNotImplemented = errors.New("not implemented")
	// ErrUnsupportedMime is usually returned when a driver does not support image type.
	ErrUnsupportedMime = errors.New("mime type is not supported")
	// ErrUnsupportedRole is returned when role is neither "user" nor "assistant".
	ErrUnsupportedRole = errors.New("role is not supported")
	// ErrNotFound is returned when a model or alias is not found.
	ErrNotFound = errors.New("model or alias is not found")
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

type Batch []BatchRequest

type BatchRequest struct {
	ID        string `json:"custom_id"`
	Method    string `json:"method"`
	URL       string `json:"url"`
	MaxTokens int    `json:"max_tokens,omitempty"`

	Body     json.RawMessage `json:"body,omitempty"`
	Embed    *Embed          `json:"embed,omitempty"`
	Complete *Complete       `json:"complete,omitempty"`
}

// Map runs `f` on each element of `a` and returns a slice of the results.
func Map[T any, M any](a []T, f func(T) M) []M {
	n := make([]M, len(a))
	for i, e := range a {
		n[i] = f(e)
	}
	return n
}
