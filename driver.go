package simp

// Driver is a roughly OpenAI-compatible inference backend.
type Driver interface {
	List(Context) ([]Model, error)
	Embed(Context, Embed) (Embeddings, error)
	Complete(Context, Complete) (Completion, error)
}

// BatchDriver is a driver that supports some variant of Batch API.
//
// Think OpenAI, Anthropic, Vertex, etc.
type BatchDriver interface {
	// Usually: validate input and upload vendor-specific JSONL
	BatchCreate(Context, *Batch, []BatchInput) error
	// Schedule the batch for execution with the provider
	BatchSend(Context, *Batch) error
	// Update the status on the batch
	BatchRefresh(Context, *Batch) error
	// Usually: download the file (JSONL) and convert to OpenAI format
	BatchReceive(Context, *Batch) ([]BatchOutput, error)
	// Cancel the batch, if possible
	BatchCancel(Context, *Batch) error
}

type BatchInput struct {
	ID       string
	Complete Complete
	Embed    Embed
}

type BatchOutput struct {
	Completion Completion
	Embeddings Embeddings
}
