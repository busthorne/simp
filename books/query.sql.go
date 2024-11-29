// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.27.0
// source: query.sql

package books

import (
	"context"
	"encoding/json"

	openai "github.com/sashabaranov/go-openai"
)

const batchById = `-- name: BatchById :one
select id, super, quo, body, created_at, updated_at, canceled_at from batch where id = ?
`

func (q *Queries) BatchById(ctx context.Context, id string) (Batch, error) {
	row := q.db.QueryRowContext(ctx, batchById, id)
	var i Batch
	err := row.Scan(
		&i.ID,
		&i.Super,
		&i.Quo,
		&i.Body,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.CanceledAt,
	)
	return i, err
}

const createBatch = `-- name: CreateBatch :exec
insert into batch (id, super, quo, body) values (?, ?, ?, ?)
`

type CreateBatchParams struct {
	ID    string
	Super *string
	Quo   openai.BatchStatus
	Body  openai.Batch
}

func (q *Queries) CreateBatch(ctx context.Context, arg CreateBatchParams) error {
	_, err := q.db.ExecContext(ctx, createBatch,
		arg.ID,
		arg.Super,
		arg.Quo,
		arg.Body,
	)
	return err
}

const createBatchOp = `-- name: CreateBatchOp :exec
insert into batch_op (batch, custom_id, op, request) values (?, ?, ?, ?)
`

type CreateBatchOpParams struct {
	Batch    string
	CustomID string
	Op       openai.BatchEndpoint
	Request  json.RawMessage
}

func (q *Queries) CreateBatchOp(ctx context.Context, arg CreateBatchOpParams) error {
	_, err := q.db.ExecContext(ctx, createBatchOp,
		arg.Batch,
		arg.CustomID,
		arg.Op,
		arg.Request,
	)
	return err
}

const epoch = `-- name: Epoch :one
select epoch from migration
`

func (q *Queries) Epoch(ctx context.Context) (int64, error) {
	row := q.db.QueryRowContext(ctx, epoch)
	var epoch int64
	err := row.Scan(&epoch)
	return epoch, err
}

const subBatches = `-- name: SubBatches :many
select id, super, quo, body, created_at, updated_at, canceled_at from batch where super = ? order by updated_at nulls first
`

func (q *Queries) SubBatches(ctx context.Context, super *string) ([]Batch, error) {
	rows, err := q.db.QueryContext(ctx, subBatches, super)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Batch
	for rows.Next() {
		var i Batch
		if err := rows.Scan(
			&i.ID,
			&i.Super,
			&i.Quo,
			&i.Body,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.CanceledAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
