package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/books"
	"github.com/busthorne/simp/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

var (
	errNoid        = fmt.Errorf("missing custom_id")
	errBadMethod   = fmt.Errorf("POST method is required")
	errMeatNorFish = fmt.Errorf("neither a chat completion nor an embedding")
)

func notkeep(err error, format string, args ...any) error {
	return fmt.Errorf("%w: %s: %v", simp.ErrBookkeeping, fmt.Sprintf(format, args...), err)
}

func BatchUpload(c *fiber.Ctx) error {
	ctx := c.Context()

	switch purpose := c.FormValue("purpose"); purpose {
	case "batch":
	default:
		return fmt.Errorf("%s purpose is %w", purpose, notImplemented(c))
	}
	ff, err := c.FormFile("file")
	if err != nil {
		return err
	}
	f, err := ff.Open()
	if err != nil {
		return err
	}
	defer f.Close()

	var (
		// driver (nil, if doesn't support batching) by model name
		drivers = map[string]simp.BatchDriver{}
		// magazine by model name
		inputs = map[string][]openai.BatchInput{}
		// model config by model name
		models = map[string]config.Model{}
		// will parse one request at a time
		lines = json.NewDecoder(f)
		// ids
		ids = map[string]bool{}
	)
	for i := 0; ; i++ {
		var input openai.BatchInput

		// a bit of a courtesy handler
		malformed := func(err error) error {
			return fmt.Errorf("request/%d (%s) is malformed: %w", i, input.CustomID, err)
		}

		// preliminary validation
		switch err := lines.Decode(&input); err {
		case nil:
			if input.CustomID == "" {
				return malformed(errNoid)
			}
			if _, ok := ids[input.CustomID]; ok {
				return malformed(fmt.Errorf("duplicate custom_id %q", input.CustomID))
			}
			switch input.Method {
			case "", "POST", "post":
				input.Method = "POST"
			default:
				return malformed(errBadMethod)
			}
			ids[input.CustomID] = true
		case io.EOF:
			goto eof
		default:
			return malformed(err)
		}
		model := input.Model()
		d, m, err := findWaldo(model)
		if err != nil {
			return malformed(fmt.Errorf("model %q: %w", model, err))
		}
		models[model] = m
		// cache the batch driver variant
		if bd, ok := d.(simp.BatchDriver); ok {
			drivers[model] = bd
		}
		// contextual validation
		switch {
		case input.ChatCompletion != nil:
			if m.Embedding {
				return malformed(fmt.Errorf("model %q is not a chat model", model))
			}
			alt := ""
			for i, m := range input.ChatCompletion.Messages {
				switch m.Role {
				case "system":
					if i != 0 {
						return malformed(fmt.Errorf("system message/%d is misplaced", i))
					}
				case "user", "assistant":
					if alt == m.Role {
						return malformed(fmt.Errorf("message/%d is not alternating", i))
					}
				default:
					return malformed(fmt.Errorf("message/%d unsupported role %q", i, m.Role))
				}
				alt = m.Role
			}
			input.ChatCompletion.Model = m.Name
		case input.Embedding != nil:
			if !m.Embedding {
				return malformed(fmt.Errorf("model %q is not an embedding model", model))
			}
			input.Embedding.Model = m.Name
		default:
			return malformed(errMeatNorFish)
		}
		inputs[model] = append(inputs[model], input)
	}

	// the super batch has been partitioned into sub-batches
eof:
	if len(inputs) == 0 {
		return fmt.Errorf("no requests to batch")
	}
	id := uuid.New().String()
	super := openai.Batch{
		ID:               id,
		Object:           "batch",
		CompletionWindow: "24h",
		CreatedAt:        time.Now().Unix(),
		RequestCounts:    openai.BatchRequestCounts{},
		Metadata:         map[string]any{},
		OutputFileID:     id,
	}

	log.Debugf("batch %q partitions:\n", super.ID)
	for model, inputs := range inputs {
		log.Debugf("%d %s (%T)\n", len(inputs), model, drivers[model])
		super.RequestCounts.Total += len(inputs)
	}

	tx, err := books.DB.BeginTx(c.Context(), nil)
	if err != nil {
		return notkeep(err, "begin")
	}
	defer tx.Rollback()

	book := books.Session().WithTx(tx)
	err = book.InsertBatch(ctx, books.InsertBatchParams{
		ID:   super.ID,
		Body: super,
	})
	if err != nil {
		return notkeep(err, "insert super batch")
	}
	for model, inputs := range inputs {
		var (
			implicit, deferred bool

			parent = super
			bd, ok = drivers[model]
		)
		const chunkSize = 25000
		if ok {
			splits := make([][]openai.BatchInput, 0, len(inputs)/chunkSize+1)
			for i := 0; i < len(inputs); i += chunkSize {
				end := i + chunkSize
				if end > len(inputs) {
					end = len(inputs)
				}
				splits = append(splits, inputs[i:end])
			}
			for _, inputs := range splits {
				log.Debugf("batch %q split length %d\n", super.ID, len(inputs))
				b := openai.Batch{
					ID:       uuid.New().String(),
					Endpoint: inputs[0].URL,
					RequestCounts: openai.BatchRequestCounts{
						Total: len(inputs),
					},
					Metadata: map[string]any{},
				}
				ctx := context.WithValue(ctx, simp.KeyModel, models[model])
				switch err := bd.BatchUpload(ctx, &b, inputs); err {
				case simp.ErrNotImplemented:
					// i.e. openai-compatible providers that do not support batching
					implicit = true
				case simp.ErrBatchDeferred:
					// i.e. anthropic which doesn't require an upload
					b.Metadata["deferred"] = true
					deferred = true
					parent = b
					fallthrough
				case nil:
					err := book.InsertBatch(ctx, books.InsertBatchParams{
						ID:    b.ID,
						Super: &super.ID,
						Model: model,
						Body:  b,
					})
					if err != nil {
						return notkeep(err, "create batch")
					}
				default:
					return fmt.Errorf("batch upload failed for model %q: %w", model, err)
				}
			}
		}
		if !deferred && !implicit {
			continue
		}
		// create implicit and deferred batch ops
		for i, input := range inputs {
			err := book.InsertBatchOp(ctx, books.InsertBatchOpParams{
				Batch:    parent.ID,
				CustomID: input.CustomID,
				Request:  input,
				Implicit: implicit,
				Deferred: deferred,
			})
			if err != nil {
				return notkeep(err, "create batch op/%d", i)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return notkeep(err, "commit")
	}
	return c.JSON(openai.File{
		ID:       super.ID,
		Object:   "file",
		Bytes:    int(ff.Size),
		FileName: ff.Filename,
		Purpose:  "batch",
	})
}

func BatchSend(c *fiber.Ctx) error {
	var req openai.CreateBatchRequest
	if err := c.BodyParser(&req); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}

	ctx := c.Context()
	book := books.Session()
	now := time.Now()

	// super batches only have id column set at this point
	row, err := book.BatchById(ctx, req.InputFileID)
	if err != nil {
		return fmt.Errorf("batch not found: %w", err)
	}
	super := row.Body
	if super.Status != "" {
		return fmt.Errorf("batch %q is already %s", super.ID, super.Status)
	}
	// fetch all sub-batches (this does not include implicit ops)
	subs, err := book.SubBatches(ctx, &super.ID)
	if err != nil {
		return fmt.Errorf("empty batch content, will not create")
	}

	// submit a sub-batch, and do the bookkeeping on it
	for _, sub := range subs {
		batch := sub.Body

		bd, m, err := findBaldo(sub.Model)
		if err != nil {
			return fmt.Errorf("model %q is not available for batching", sub.Model)
		}

		var ctx context.Context = ctx

		// pre-deferred
		_, deferred := batch.Metadata["deferred"]
		if deferred {
			inputs, err := book.BatchOps(ctx, batch.ID)
			if err != nil {
				return notkeep(err, "fetch deferred ops")
			}
			ctx = context.WithValue(ctx, simp.KeyBatchInputs, inputs)
		}
		ctx = context.WithValue(ctx, simp.KeyModel, m)

		// send
		if err := bd.BatchSend(ctx, &batch); err != nil {
			berr := openai.BatchError{Message: err.Error()}
			if super.Errors == nil {
				super.Errors = &openai.BatchErrors{}
			}
			super.Errors.Data = append(super.Errors.Data, berr)
			batch.Status = openai.BatchStatusCancelled
			batch.CancelledAt = now.Unix()
		} else {
			batch.Status = openai.BatchStatusInProgress
			batch.InProgressAt = now.Unix()
		}

		// post-deferred
		if deferred {
			if err := book.DeleteBatchOps(ctx, batch.ID); err != nil {
				return notkeep(err, "delete deferred ops")
			}
		}
		if err := book.UpdateBatch(ctx, books.BatchUpdates(batch)); err != nil {
			return notkeep(err, "update sub-batch")
		}
	}

	if super.Errors != nil && len(super.Errors.Data) == len(subs) {
		super.Status = openai.BatchStatusFailed
		super.CancelledAt = now.Unix()
	} else {
		super.Status = openai.BatchStatusInProgress
		super.InProgressAt = now.Unix()
	}
	if err := book.UpdateBatch(ctx, books.BatchUpdates(super)); err != nil {
		return notkeep(err, "update super batch")
	}
	return c.JSON(super)
}

func BatchRefresh(c *fiber.Ctx) error {
	ctx := c.Context()
	book := books.Session()
	id := c.Params("id")
	row, err := book.BatchById(ctx, id)
	if err != nil {
		return fmt.Errorf("batch not found: %w", err)
	}
	super := row.Body
	switch super.Status {
	case openai.BatchStatusCompleted, openai.BatchStatusFailed, openai.BatchStatusCancelled:
		return c.JSON(super)
	}

	subs, err := book.SubBatchesPending(ctx, &super.ID)
	if err != nil {
		return notkeep(err, "fetch pending sub-batches")
	}
	elapsed := 0
	for i, sub := range subs {
		batch := &subs[i].Body

		bd, _, err := findBaldo(sub.Model)
		if err != nil {
			return notkeep(err, "model %q is not available for batching", sub.Model)
		}
		if err := bd.BatchRefresh(ctx, batch); err != nil {
			return fmt.Errorf("refresh %s sub-batch failed: %w", sub.Model, err)
		}
		switch now := time.Now().Unix(); batch.Status {
		case openai.BatchStatusFailed:
			batch.FailedAt = now
		case openai.BatchStatusCancelled:
			batch.CancelledAt = now
		case openai.BatchStatusCompleted:
			batch.CompletedAt = now
		}
		if err := book.UpdateBatch(ctx, books.BatchUpdates(*batch)); err != nil {
			return notkeep(err, "update sub-batch")
		}
		if batch.Status != openai.BatchStatusInProgress {
			elapsed++
		}
	}
	if elapsed == len(subs) {
		ops, err := book.CountBatchOps(ctx, super.ID)
		switch err {
		case nil:
		case sql.ErrNoRows:
		default:
			return notkeep(err, "count batch ops")
		}
		if ops.Total == ops.Completed+ops.Canceled {
			super.Status = openai.BatchStatusCompleted
			super.CompletedAt = time.Now().Unix()
		}
	}
	if err := book.UpdateBatch(ctx, books.BatchUpdates(super)); err != nil {
		return notkeep(err, "update super batch")
	}
	return c.JSON(super)
}

func BatchReceive(c *fiber.Ctx) error {
	ctx := c.Context()
	book := books.Session()
	superid := c.Params("id")
	subs, err := book.SubBatchesCompleted(ctx, &superid)
	if err != nil {
		return fmt.Errorf("batch not found: %w", err)
	}
	c.Set("Content-Type", "application/jsonl")
	w := json.NewEncoder(c.Response().BodyWriter())

	drivers := map[string]simp.BatchDriver{}
	for _, sub := range subs {
		batch := sub.Body
		bd, ok := drivers[sub.Model]
		if !ok {
			d, _, err := findBaldo(sub.Model)
			if err != nil {
				continue
			}
			bd = d
		}
		outputs, err := bd.BatchReceive(ctx, &batch)
		if err != nil {
			continue
		}
		log.Debugf("batch %q received %d outputs\n", batch.ID, len(outputs))
		for _, output := range outputs {
			w.Encode(output)
		}
	}

	const chunkSize = 10000

	for i := int64(0); ; i += chunkSize {
		outputs, err := book.BatchOpsCompleted(ctx, books.BatchOpsCompletedParams{
			Batch:  superid,
			Limit:  chunkSize,
			Offset: i,
		})
		switch err {
		case nil:
			if len(outputs) == 0 {
				return nil
			}
			for _, output := range outputs {
				w.Encode(output)
			}
		case sql.ErrNoRows:
			return nil
		default:
			return fmt.Errorf("batch not found: %w", err)
		}
	}
}

func BatchCancel(c *fiber.Ctx) error {
	ctx := c.Context()
	book := books.Session()
	id := c.Params("id")
	super, err := book.BatchById(ctx, id)
	if err != nil {
		return fmt.Errorf("batch not found or already cancelled: %w", err)
	}
	if super.CompletedAt != nil || super.CanceledAt != nil {
		return fmt.Errorf("batch %q is already %s", super.ID, super.Body.Status)
	}
	subs, err := book.SubBatchesPending(ctx, &super.ID)
	switch err {
	case nil:
	case sql.ErrNoRows:
	default:
		return notkeep(err, "fetch pending sub-batches")
	}
	for _, sub := range subs {
		bd, _, err := findBaldo(sub.Model)
		if err != nil {
			return fmt.Errorf("model %q is not available for batching", sub.Model)
		}
		batch := sub.Body
		if err := bd.BatchCancel(ctx, &batch); err != nil {
			return fmt.Errorf("cancel on %q failed: %w", sub.Model, err)
		}
		batch.Status = openai.BatchStatusCancelled
		batch.CancelledAt = time.Now().Unix()
		if err := book.UpdateBatch(ctx, books.BatchUpdates(batch)); err != nil {
			return notkeep(err, "update sub-batch")
		}
	}
	if err := book.CancelBatchOps(ctx, id); err != nil {
		return notkeep(err, "cancel batch ops")
	}
	batch := super.Body
	batch.Status = openai.BatchStatusCancelled
	batch.CancelledAt = time.Now().Unix()
	if err := book.UpdateBatch(ctx, books.BatchUpdates(batch)); err != nil {
		return notkeep(err, "update superbatch")
	}
	return c.JSON(batch)
}
