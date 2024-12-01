-- name: Epoch :one
select epoch from migration;

-- name: BatchById :one
select * from batch where id = ?;
-- name: SubBatches :many
select * from batch where super = ? order by updated_at nulls first;
-- name: SubBatchesCompleted :many
select * from batch where super = ? and status = 'completed';
-- name: CreateBatch :exec
insert into batch (id, super, status, model, body) values (?, ?, ?, ?, ?);
-- name: CreateBatchDirect :exec
insert into batch_direct (batch, custom_id, op, request) values (?, ?, ?, ?);
-- name: BatchDirectCompleted :many
select
	custom_id,
	response
from batch_direct
where batch = ? and completed_at is not null;
-- name: UpdateBatch :exec
update batch
set body = ?,
	status = ?,
	updated_at = current_timestamp,
	canceled_at = ?
where id = ?;
-- name: CancelBatch :exec
update batch
set status = 'cancelled', canceled_at = current_timestamp
where id = ?;