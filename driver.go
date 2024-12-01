package simp

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

// Driver is a roughly OpenAI-compatible inference backend.
type Driver interface {
	List(context.Context) ([]openai.Model, error)
	Embed(context.Context, openai.EmbeddingRequest) (openai.EmbeddingResponse, error)
	Complete(context.Context, openai.CompletionRequest) (openai.CompletionResponse, error)
	Chat(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// BatchDriver is a driver that supports some variant of Batch API.
//
// Think OpenAI, Anthropic, Vertex, etc.
type BatchDriver interface {
	// Usually: validate input and upload vendor-specific JSONL
	BatchUpload(context.Context, *openai.Batch, []openai.BatchInput) error
	// Schedule the batch for execution with the provider
	BatchSend(context.Context, *openai.Batch) error
	// Update the status on the batch
	BatchRefresh(context.Context, *openai.Batch) error
	// Usually: download the file (JSONL) and convert to OpenAI format
	BatchReceive(context.Context, *openai.Batch) ([]openai.BatchOutput, error)
	// Cancel the batch, if possible
	BatchCancel(context.Context, *openai.Batch) error
}
