package simp

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

// Driver is a roughly OpenAI-compatible inference backend.
//
// The OpenAI format is used as the least common denominator for all normal
// LLM inference operations: the embeddings, completions, and chat
// completions.
//
// Note: the driver implementations are not required to populate the response
// fields pertaining to bookkeeping, like ID, Object, Created timestamp, and
// others.
//
// Let's keep the drivers simple, and let the daemon handle the bookkeeping.
//
// The model-agnostic tool-use API will be implemented in a future design.
type Driver interface {
	// List returns the currently-available models from provider's directory.
	//
	// For example, Anthropic doesn't have a directory, so the driver must
	// either maintain a list of known model versions, or never implement
	// this method in the first place.
	//
	// If this operation is not supported, it will return `ErrNotImplemented`.
	//
	// The config supersedes the list: drivers don't have to provide exclusive
	// model list for it to work, although it certainly helps whenever the
	// user config is incomplete.
	List(context.Context) ([]openai.Model, error)

	// Embed computes the vector embedding of a single, or multiple texts.
	// The daemon is not aware of the provider's limits, so it will submit
	// all requests as-is.
	//
	// If this operation is not supported, it will return `ErrNotImplemented`.
	//
	// The context contains model configuration: see `KeyModel`.
	Embed(context.Context, openai.EmbeddingRequest) (openai.EmbeddingResponse, error)

	// Complete is a vanilla completion. Most drivers won't implement it because
	// of how prevalent the instruction-following chat models are; there is no
	// system prompt, no tools, no functions, no nothing.
	//
	// If this operation is not supported, it will return `ErrNotImplemented`.
	//
	// The context contains model configuration: see `KeyModel`.
	Complete(context.Context, openai.CompletionRequest) (openai.CompletionResponse, error)

	// Chat is a chat completion on a message history producing a new message.
	//
	// Models that don't support system instructions, see Gemma, are supposed
	// to incorporate the `system` message in the `user` messages. Similarly,
	// if the upstream provider doesn't support URL inputs for images,
	// the provider should perform HTTP GET using the provided context
	// and incorporate the image data appropriately.
	//
	// If this operation is not supported, it will return `ErrNotImplemented`.
	//
	// Model-agnostic tool use is largely contingent on a future design,
	// however until such design is available, the driver should try to
	// translate the tool-calls to and from OpenAI semantics, if
	// possible.
	//
	// The context contains model configuration: see `KeyModel`.
	Chat(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// BatchDriver is a driver that also supports some variant of Batch API.
//
// Think OpenAI, Anthropic, Vertex, etc.
//
// The drivers that do not implement this interface, will still be batchable,
// however in that case the daemon will process inputs at configured rate as-if
// they were submitted sequentially.
//
// Independent embedding requests whereas task and late_chunking is not set in
// the request, may be inlined; allowing the daemon to process them seemingly
// faster than the configured rate. In that case, usage statistics per-request
// will be reported approximately: the aggregated count in constituent
// requests is split proportionally.
//
// BatchDriver is allowed to mutate the inputs, if necessary.
//
// For example, Vertex requires vision inputs to be well-formed bucket URIs,
// so the driver has to upload them to Cloud Storage bucket, and rewrite the
// inputs before uploading them to BigQuery.
type BatchDriver interface {
	Driver

	// BatchUpload validates input and is used to upload vendor-specific JSONL.
	//
	// The context contains model configuration: see KeyModel.
	//
	// The batch descriptors needs not be fully-specified; that is to say, its
	// ID is set by caller. The drivers are only supposed to set the input and
	// output files, metadata, and status (during and after BatchSend!) updates
	// according to OpenAI semantics.
	//
	// If the batching driver would not support batching on given inputs for
	// some reason, it should return ErrNotImplemented. For example, the OpenAI
	// driver with non-empty BaseURL will do so unless the `batch` setting is
	// also set to true explicitly in the config.
	//
	// This behaviour enables upstream OpenAI multiplexers.
	//
	// If the provider doesn't use uploads, like Anthropic, it should return
	// ErrBatchDeferred to indicate that the batch must be deferred until
	// send time. In that case, the backend will commit the batch to the
	// database temporarily.
	BatchUpload(context.Context, *openai.Batch, []openai.BatchInput) error

	// BatchSend submits the underlying batch for execution with the provider.
	//
	// The context contains model configuration: see `KeyModel`.
	//
	// If the batch has been deferred, the inputs will be included in the
	// context, see `KeyBatchInputs`. The jury is still out on whether it
	// should be provided as third argument to `BatchSend`, however given
	// that the majority of batching drivers will use uploads, the current
	// design looks just fine.
	BatchSend(context.Context, *openai.Batch) error

	// BatchRefresh will inquire the most recent batch status from the provider.
	//
	// The provider is only required to update the Status field on the batch
	// and ensure that the metadata contains all the information it needs to
	// download the batch later.
	//
	// Successful status updates are not expected to return errors.
	BatchRefresh(context.Context, *openai.Batch) error

	// BatchReceive will download the file (JSONL) and convert to OpenAI format.
	//
	// It's recommended that `Usage` is set to reflect token spending.
	BatchReceive(context.Context, *openai.Batch) ([]openai.BatchOutput, error)

	// BatchCancel will cancel the batch, if it's possible.
	//
	// The backend will not call this if the batch is already in terminal
	// state. If the batch could not be cancelled, the driver should
	// return an error.
	BatchCancel(context.Context, *openai.Batch) error
}

// Keys are used for accessing context values of the string content type.
type Key string

const (
	KeyModel       Key = "config.Model"
	KeyBatchInputs Key = "[]config.BatchInput"
)
