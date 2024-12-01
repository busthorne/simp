package books

import (
	"time"

	openai "github.com/sashabaranov/go-openai"
)

func BatchUpdates(batch openai.Batch) (upd UpdateBatchParams) {
	upd.ID = batch.ID
	upd.Body = batch
	if batch.FailedAt != 0 {
		t := time.Unix(batch.FailedAt, 0)
		upd.CanceledAt = &t
	}
	if batch.CancelledAt != 0 {
		t := time.Unix(batch.CancelledAt, 0)
		upd.CanceledAt = &t
	}
	if batch.CompletedAt != 0 {
		t := time.Unix(batch.CompletedAt, 0)
		upd.CompletedAt = &t
	}
	return
}
