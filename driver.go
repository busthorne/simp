package simp

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

// Driver is a roughly OpenAI-compatible inference backend.
type Driver interface {
	List(context.Context) ([]Model, error)
	Embed(context.Context, Embed) (Embeddings, error)
	Complete(context.Context, Complete) (Completions, error)
}

// BatchDriver is a driver that supports some variant of Batch API.
//
// Think OpenAI, Anthropic, Vertex, etc.
type BatchDriver interface {
	// Usually: validate input and upload vendor-specific JSONL
	BatchUpload(context.Context, *Batch, Magazine) error
	// Schedule the batch for execution with the provider
	BatchSend(context.Context, *Batch) error
	// Update the status on the batch
	BatchRefresh(context.Context, *Batch) error
	// Usually: download the file (JSONL) and convert to OpenAI format
	BatchReceive(context.Context, *Batch) (Magazine, error)
	// Cancel the batch, if possible
	BatchCancel(context.Context, *Batch) error
}

// BatchUnion is a union type of possible batch inputs and outputs.
type BatchUnion struct {
	Id   string
	Cin  *Complete
	Ein  *Embed
	Cout *Completions
	Eout *Embeddings
}

// Magazine is a batch content-vector: one shoe fits all.
type Magazine []BatchUnion

func (m Magazine) Endpoint() openai.BatchEndpoint {
	if len(m) == 0 {
		panic("empty magazine")
	}
	switch {
	case m[0].Cin != nil:
		return openai.BatchEndpointChatCompletions
	case m[0].Ein != nil:
		return openai.BatchEndpointEmbeddings
	default:
		panic("unsupported batch op")
	}
}
