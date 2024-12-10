-- name: Epoch :one
select epoch from migration;

-- name: BatchById :one
select *
from batch
	where id = ?;
-- name: SubBatches :many
select *
from batch
	where super = ?;
-- name: SubBatchesCompleted :many
select *
from batch
	where super = ?
		and completed_at is not null;
-- name: SubBatchesPending :many
select *
from batch
	where super = ?
		and completed_at is null
		and canceled_at is null;

-- name: InsertBatch :exec
insert into batch (id, super, model, body)
	values (?, ?, ?, ?);
-- name: InsertBatchOp :exec
insert into batch_op (batch, custom_id, request, implicit, deferred)
	values (?, ?, ?, ?, ?);

-- name: BatchOps :many
select request from batch_op where batch = ?;
-- name: CountBatchOps :one
select
	count(*) as total,
	count(*) filter (where completed_at is not null) as completed,
	count(*) filter (where canceled_at is not null) as canceled
from batch_op
	where batch = ?;
-- name: DeleteBatchOps :exec
delete from batch_op where batch = ?;
-- name: BatchOpsCompleted :many
select response
from batch_op
where batch = ? and completed_at is not null
limit @limit offset @offset;

-- name: UpdateBatch :exec
update batch
set body = ?,
	updated_at = current_timestamp,
	canceled_at = ?,
	completed_at = ?
where id = ?;
-- name: CancelBatch :exec
update batch
	set canceled_at = current_timestamp
	where (id = @id or super = @id) and completed_at is null;
-- name: CancelBatchOps :exec
update batch_op
	set canceled_at = current_timestamp
	where batch = @id and completed_at is null;
