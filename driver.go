package simp

// Driver is a roughly OpenAI-compatible inference backend.
type Driver interface {
	List(Context) ([]Model, error)
	Embed(Context, Embed) (Embeddings, error)
	Complete(Context, Complete) (Completions, error)
}

// BatchDriver is a driver that supports some variant of Batch API.
//
// Think OpenAI, Anthropic, Vertex, etc.
type BatchDriver interface {
	// Usually: validate input and upload vendor-specific JSONL
	BatchUpload(Context, *Batch, Magazine) error
	// Schedule the batch for execution with the provider
	BatchSend(Context, *Batch) error
	// Update the status on the batch
	BatchRefresh(Context, *Batch) error
	// Usually: download the file (JSONL) and convert to OpenAI format
	BatchReceive(Context, *Batch) (Magazine, error)
	// Cancel the batch, if possible
	BatchCancel(Context, *Batch) error
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
