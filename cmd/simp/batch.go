package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/busthorne/simp"
	"github.com/busthorne/simp/books"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

var (
	errNoid        = fmt.Errorf("missing custom_id")
	errBadMethod   = fmt.Errorf("POST method is required")
	errMeatNorFish = fmt.Errorf("neither a chat completion nor an embedding")
)

type BatchRequest struct {
	ID        string          `json:"custom_id"`
	Method    string          `json:"method"`
	URL       string          `json:"url"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Body      json.RawMessage `json:"body,omitempty"`
}

func batchUpload(c *fiber.Ctx) error {
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
		mags = map[string]simp.Magazine{}
		// will parse one request at a time
		lines = json.NewDecoder(f)
		// ids
		ids = map[string]bool{}
	)
	for i := 1; ; i++ {
		var req BatchRequest

		// a bit of a courtesy handler
		malformed := func(err error) error {
			return fmt.Errorf("request %s/%d is malformed: %w", req.ID, i, err)
		}

		// preliminary validation
		switch err := lines.Decode(&req); err {
		case nil:
			if req.ID == "" {
				return malformed(errNoid)
			}
			if _, ok := ids[req.ID]; ok {
				return malformed(fmt.Errorf("duplicate custom_id %q", req.ID))
			}
			switch req.Method {
			case "":
			case "POST":
			case "post":
			default:
				return malformed(errBadMethod)
			}

			ids[req.ID] = true
		case io.EOF:
			goto eof
		default:
			return malformed(err)
		}

		// mags consist of batch-unions of chat/embedding input/output
		u := simp.BatchUnion{Id: req.ID}
		model := ""
		// conditional unmarshal
		switch openai.BatchEndpoint(req.URL) {
		case openai.BatchEndpointChatCompletions:
			if err = json.Unmarshal(req.Body, &u.Cin); err != nil {
				return malformed(err)
			}
			model = u.Cin.Model
		case openai.BatchEndpointEmbeddings:
			if err = json.Unmarshal(req.Body, &u.Ein); err != nil {
				return malformed(err)
			}
			model = u.Ein.Model
		default:
			return malformed(errMeatNorFish)
		}
		// TODO: add alias cache in findWaldo
		d, m, err := findWaldo(model)
		if err != nil {
			return malformed(fmt.Errorf("model %q: %w", model, err))
		}
		if _, ok := drivers[model]; !ok {
			bd, _ := d.(simp.BatchDriver)
			drivers[model] = bd
		}
		// contextual validation
		switch {
		case u.Cin != nil:
			if m.Embedding {
				return malformed(fmt.Errorf("model %q is not a chat model", model))
			}
			alt := ""
			for i, m := range u.Cin.Messages {
				switch m.Role {
				case "system":
					if i != 0 {
						return malformed(fmt.Errorf("system message/%d is misplaced", i+1))
					}
				case "user", "assistant":
					if alt == m.Role {
						return malformed(fmt.Errorf("message/%d is not alternating", i+1))
					}
				default:
					return malformed(fmt.Errorf("message/%d unsupported role %q", i+1, m.Role))
				}
				alt = m.Role
			}
			u.Cin.Model = m.Name
		case u.Ein != nil:
			if !m.Embedding {
				return malformed(fmt.Errorf("model %q is not an embedding model", model))
			}
			u.Ein.Model = m.Name
		default:
			panic("unsupported batch op")
		}
		mags[model] = append(mags[model], u)
	}

	// the aggregate batch has been partitioned
eof:
	tx, err := books.DB.BeginTx(c.Context(), nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	book := books.Session().WithTx(tx)
	super := uuid.New().String()
	err = book.CreateBatch(ctx, books.CreateBatchParams{
		ID: super,
	})
	if err != nil {
		return fmt.Errorf("create super batch: %w", err)
	}

	for model, mag := range mags {
		bd, ok := drivers[model]
		if ok {
			b := openai.Batch{
				ID:       uuid.New().String(),
				Endpoint: mag.Endpoint(),
			}
			switch err := bd.BatchUpload(ctx, &b, mag); err {
			// i.e. openai-compatible providers
			case simp.ErrNotImplemented:
			case nil:
				// if batching is supported, upload each model's magazine
				err := book.CreateBatch(ctx, books.CreateBatchParams{
					ID:     b.ID,
					Super:  &super,
					Status: b.Status,
					Model:  model,
					Body:   b,
				})
				if err != nil {
					return fmt.Errorf("insert batch: %w", err)
				}
				continue
			default:
				return fmt.Errorf("batch upload failed for model %q: %w", model, err)
			}
		}

		// if batching is not supported, create a batch op for each request
		for _, u := range mag {
			var body json.RawMessage
			var op openai.BatchEndpoint
			switch {
			case u.Cin != nil:
				op = openai.BatchEndpointChatCompletions
				body, _ = json.Marshal(u.Cin)
			case u.Ein != nil:
				op = openai.BatchEndpointEmbeddings
				body, _ = json.Marshal(u.Ein)
			default:
				panic("unsupported batch op")
			}
			err := book.CreateBatchDirect(ctx, books.CreateBatchDirectParams{
				Batch:    super,
				CustomID: u.Id,
				Op:       op,
				Request:  body,
			})
			if err != nil {
				return simp.ErrBookkeeping
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return simp.ErrBookkeeping
	}
	return c.JSON(openai.File{
		ID:       super,
		Object:   "file",
		Bytes:    int(ff.Size),
		FileName: ff.Filename,
		Purpose:  "batch",
	})
}

func batchSend(c *fiber.Ctx) error {
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
	// fetch all sub-batches (this does not include direct batch ops)
	subs, err := book.SubBatches(ctx, &super.ID)
	if err != nil {
		return fmt.Errorf("empty batch content, will not create")
	}

	// submit a sub-batch, and do the bookkeeping on it
	for i, sub := range subs {
		batch := &subs[i].Body

		d, _, err := findWaldo(sub.Model)
		if err != nil {
			return fmt.Errorf("model %q: %w", sub.Model, err)
		}
		bd, ok := d.(simp.BatchDriver)
		if !ok {
			return fmt.Errorf("model %q is not batchable", sub.Model)
		}

		upd := books.UpdateBatchParams{ID: batch.ID}

		if err := bd.BatchSend(ctx, batch); err != nil {
			batch.Status = openai.BatchStatusCancelled
			batch.CancelledAt = now.Unix()
			upd.CanceledAt = &now

			berr := openai.BatchError{Message: err.Error()}
			if super.Errors == nil {
				super.Errors = &openai.BatchErrors{}
			}
			super.Errors.Data = append(super.Errors.Data, berr)
		} else {
			batch.Status = openai.BatchStatusInProgress
			batch.InProgressAt = now.Unix()
		}
		upd.Status = batch.Status
		upd.Body = *batch
		book.UpdateBatch(ctx, upd)
	}

	if super.Errors != nil && len(super.Errors.Data) == len(subs) {
		super.Status = openai.BatchStatusFailed
	} else {
		super.Status = openai.BatchStatusInProgress
	}

	err = book.UpdateBatch(ctx, books.UpdateBatchParams{
		ID:     super.ID,
		Status: super.Status,
		Body:   super,
	})
	if err != nil {
		return simp.ErrBookkeeping
	}
	return c.JSON(super)
}

func batchCancel(c *fiber.Ctx) error {
	ctx := c.Context()
	book := books.Session()
	id := c.Params("id")
	row, err := book.BatchById(ctx, id)
	if err != nil {
		return fmt.Errorf("batch not found or already cancelled: %w", err)
	}
	if err := book.CancelBatch(ctx, id); err != nil {
		return simp.ErrBookkeeping
	}
	batch := row.Body
	batch.Status = openai.BatchStatusCancelled
	batch.CancelledAt = time.Now().Unix()
	return c.JSON(batch)
}

func batchRefresh(c *fiber.Ctx) error {
	ctx := c.Context()
	book := books.Session()
	id := c.Params("id")
	row, err := book.BatchById(ctx, id)
	if err != nil {
		return fmt.Errorf("batch not found: %w", err)
	}
	batch := row.Body
	if batch.Status != openai.BatchStatusInProgress {
		return c.JSON(batch)
	}

	// TODO: check sub-batches and direct ops
	return simp.ErrNotImplemented
}
