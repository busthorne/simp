// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.27.0
// source: batch.sql

package books

import (
	"context"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const batchById = `-- name: BatchById :one
select id, super, model, body, created_at, updated_at, completed_at, canceled_at
from batch
	where id = ?
`

func (q *Queries) BatchById(ctx context.Context, id string) (Batch, error) {
	row := q.db.QueryRowContext(ctx, batchById, id)
	var i Batch
	err := row.Scan(
		&i.ID,
		&i.Super,
		&i.Model,
		&i.Body,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.CompletedAt,
		&i.CanceledAt,
	)
	return i, err
}

const batchOps = `-- name: BatchOps :many
select request from batch_op where batch = ?
`

func (q *Queries) BatchOps(ctx context.Context, batch string) ([]openai.BatchInput, error) {
	rows, err := q.db.QueryContext(ctx, batchOps, batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []openai.BatchInput
	for rows.Next() {
		var request openai.BatchInput
		if err := rows.Scan(&request); err != nil {
			return nil, err
		}
		items = append(items, request)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const batchOpsCompleted = `-- name: BatchOpsCompleted :many
select response from batch_op where batch = ? and completed_at is not null
`

func (q *Queries) BatchOpsCompleted(ctx context.Context, batch string) ([]openai.BatchOutput, error) {
	rows, err := q.db.QueryContext(ctx, batchOpsCompleted, batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []openai.BatchOutput
	for rows.Next() {
		var response openai.BatchOutput
		if err := rows.Scan(&response); err != nil {
			return nil, err
		}
		items = append(items, response)
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
	set canceled_at = current_timestamp
	where (id = ?1 or super = ?1) and completed_at is null
`

func (q *Queries) CancelBatch(ctx context.Context, id string) error {
	_, err := q.db.ExecContext(ctx, cancelBatch, id)
	return err
}

const cancelBatchOps = `-- name: CancelBatchOps :exec
update batch_op
	set canceled_at = current_timestamp
	where batch = ?1 and completed_at is null
`

func (q *Queries) CancelBatchOps(ctx context.Context, id string) error {
	_, err := q.db.ExecContext(ctx, cancelBatchOps, id)
	return err
}

const countBatchOps = `-- name: CountBatchOps :one
select
	count(*) as total,
	count(*) filter (where completed_at is not null) as completed,
	count(*) filter (where canceled_at is not null) as canceled
from batch_op
	where batch = ?
`

type CountBatchOpsRow struct {
	Total     int64 `db:"total" json:"total"`
	Completed int64 `db:"completed" json:"completed"`
	Canceled  int64 `db:"canceled" json:"canceled"`
}

func (q *Queries) CountBatchOps(ctx context.Context, batch string) (CountBatchOpsRow, error) {
	row := q.db.QueryRowContext(ctx, countBatchOps, batch)
	var i CountBatchOpsRow
	err := row.Scan(&i.Total, &i.Completed, &i.Canceled)
	return i, err
}

const deleteBatchOps = `-- name: DeleteBatchOps :exec
delete from batch_op where batch = ?
`

func (q *Queries) DeleteBatchOps(ctx context.Context, batch string) error {
	_, err := q.db.ExecContext(ctx, deleteBatchOps, batch)
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

const insertBatch = `-- name: InsertBatch :exec
insert into batch (id, super, model, body)
	values (?, ?, ?, ?)
`

type InsertBatchParams struct {
	ID    string       `db:"id" json:"id"`
	Super *string      `db:"super" json:"super"`
	Model string       `db:"model" json:"model"`
	Body  openai.Batch `db:"body" json:"body"`
}

func (q *Queries) InsertBatch(ctx context.Context, arg InsertBatchParams) error {
	_, err := q.db.ExecContext(ctx, insertBatch,
		arg.ID,
		arg.Super,
		arg.Model,
		arg.Body,
	)
	return err
}

const insertBatchOp = `-- name: InsertBatchOp :exec
insert into batch_op (batch, custom_id, request, implicit, deferred)
	values (?, ?, ?, ?, ?)
`

type InsertBatchOpParams struct {
	Batch    string            `db:"batch" json:"batch"`
	CustomID string            `db:"custom_id" json:"custom_id"`
	Request  openai.BatchInput `db:"request" json:"request"`
	Implicit bool              `db:"implicit" json:"implicit"`
	Deferred bool              `db:"deferred" json:"deferred"`
}

func (q *Queries) InsertBatchOp(ctx context.Context, arg InsertBatchOpParams) error {
	_, err := q.db.ExecContext(ctx, insertBatchOp,
		arg.Batch,
		arg.CustomID,
		arg.Request,
		arg.Implicit,
		arg.Deferred,
	)
	return err
}

const subBatches = `-- name: SubBatches :many
select id, super, model, body, created_at, updated_at, completed_at, canceled_at
from batch
	where super = ?
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
select id, super, model, body, created_at, updated_at, completed_at, canceled_at
from batch
	where super = ?
		and completed_at is not null
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

const subBatchesPending = `-- name: SubBatchesPending :many
select id, super, model, body, created_at, updated_at, completed_at, canceled_at
from batch
	where super = ?
		and completed_at is null
		and canceled_at is null
`

func (q *Queries) SubBatchesPending(ctx context.Context, super *string) ([]Batch, error) {
	rows, err := q.db.QueryContext(ctx, subBatchesPending, super)
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
	updated_at = current_timestamp,
	canceled_at = ?,
	completed_at = ?
where id = ?
`

type UpdateBatchParams struct {
	Body        openai.Batch `db:"body" json:"body"`
	CanceledAt  *time.Time   `db:"canceled_at" json:"canceled_at"`
	CompletedAt *time.Time   `db:"completed_at" json:"completed_at"`
	ID          string       `db:"id" json:"id"`
}

func (q *Queries) UpdateBatch(ctx context.Context, arg UpdateBatchParams) error {
	_, err := q.db.ExecContext(ctx, updateBatch,
		arg.Body,
		arg.CanceledAt,
		arg.CompletedAt,
		arg.ID,
	)
	return err
}
