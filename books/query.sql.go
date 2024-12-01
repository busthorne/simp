// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.27.0
// source: query.sql

package books

import (
	"context"
	"encoding/json"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const batchById = `-- name: BatchById :one
select id, super, status, model, body, created_at, updated_at, completed_at, canceled_at from batch where id = ?
`

func (q *Queries) BatchById(ctx context.Context, id string) (Batch, error) {
	row := q.db.QueryRowContext(ctx, batchById, id)
	var i Batch
	err := row.Scan(
		&i.ID,
		&i.Super,
		&i.Status,
		&i.Model,
		&i.Body,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.CompletedAt,
		&i.CanceledAt,
	)
	return i, err
}

const batchDirectCompleted = `-- name: BatchDirectCompleted :many
select
	custom_id,
	response
from batch_direct
where batch = ? and completed_at is not null
`

type BatchDirectCompletedRow struct {
	CustomID string
	Response json.RawMessage
}

func (q *Queries) BatchDirectCompleted(ctx context.Context, batch string) ([]BatchDirectCompletedRow, error) {
	rows, err := q.db.QueryContext(ctx, batchDirectCompleted, batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []BatchDirectCompletedRow
	for rows.Next() {
		var i BatchDirectCompletedRow
		if err := rows.Scan(&i.CustomID, &i.Response); err != nil {
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

const cancelBatch = `-- name: CancelBatch :exec
update batch
set status = 'cancelled', canceled_at = current_timestamp
where id = ?
`

func (q *Queries) CancelBatch(ctx context.Context, id string) error {
	_, err := q.db.ExecContext(ctx, cancelBatch, id)
	return err
}

const createBatch = `-- name: CreateBatch :exec
insert into batch (id, super, status, model, body) values (?, ?, ?, ?, ?)
`

type CreateBatchParams struct {
	ID     string
	Super  *string
	Status openai.BatchStatus
	Model  string
	Body   openai.Batch
}

func (q *Queries) CreateBatch(ctx context.Context, arg CreateBatchParams) error {
	_, err := q.db.ExecContext(ctx, createBatch,
		arg.ID,
		arg.Super,
		arg.Status,
		arg.Model,
		arg.Body,
	)
	return err
}

const createBatchDirect = `-- name: CreateBatchDirect :exec
insert into batch_direct (batch, custom_id, op, request) values (?, ?, ?, ?)
`

type CreateBatchDirectParams struct {
	Batch    string
	CustomID string
	Op       openai.BatchEndpoint
	Request  json.RawMessage
}

func (q *Queries) CreateBatchDirect(ctx context.Context, arg CreateBatchDirectParams) error {
	_, err := q.db.ExecContext(ctx, createBatchDirect,
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
select id, super, status, model, body, created_at, updated_at, completed_at, canceled_at from batch where super = ? order by updated_at nulls first
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
			&i.Status,
			&i.Model,
			&i.Body,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.CompletedAt,
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

const subBatchesCompleted = `-- name: SubBatchesCompleted :many
select id, super, status, model, body, created_at, updated_at, completed_at, canceled_at from batch where super = ? and status = 'completed'
`

func (q *Queries) SubBatchesCompleted(ctx context.Context, super *string) ([]Batch, error) {
	rows, err := q.db.QueryContext(ctx, subBatchesCompleted, super)
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
			&i.Status,
			&i.Model,
			&i.Body,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.CompletedAt,
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

const updateBatch = `-- name: UpdateBatch :exec
update batch
set body = ?,
	status = ?,
	updated_at = current_timestamp,
	canceled_at = ?
where id = ?
`

type UpdateBatchParams struct {
	Body       openai.Batch
	Status     openai.BatchStatus
	CanceledAt *time.Time
	ID         string
}

func (q *Queries) UpdateBatch(ctx context.Context, arg UpdateBatchParams) error {
	_, err := q.db.ExecContext(ctx, updateBatch,
		arg.Body,
		arg.Status,
		arg.CanceledAt,
		arg.ID,
	)
	return err
}
