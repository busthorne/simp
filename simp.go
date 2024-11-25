package simp

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/sashabaranov/go-openai"
)

// I hate long type names.
type (
	Context     = context.Context
	Model       = openai.Model
	Complete    = openai.ChatCompletionRequest
	Completions = openai.ChatCompletionResponse
	Embed       = openai.EmbeddingRequest
	Embeddings  = openai.EmbeddingResponse
	Batch       = openai.Batch
)

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
	// ErrBatchIncomplete is returned when a batch is not completed.
	ErrBatchIncomplete = errors.New("batch is incomplete")
)

func init() {
	Path = os.Getenv("SIMPPATH")
	if Path == "" {
		Path = filepath.Join(os.Getenv("HOME"), ".simp")
	}
}

// Map runs `f` on each element of `a` and returns a slice of the results.
func Map[T any, M any](a []T, f func(T) M) []M {
	n := make([]M, len(a))
	for i, e := range a {
		n[i] = f(e)
	}
	return n
}
