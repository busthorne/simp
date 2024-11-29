-- name: Epoch :one
select epoch from migration;

-- name: BatchById :one
select * from batch where id = ?;
-- name: SubBatches :many
select * from batch where super = ? order by updated_at nulls first;
-- name: CreateBatch :exec
insert into batch (id, super, quo, body) values (?, ?, ?, ?);
-- name: CreateBatchOp :exec
insert into batch_op (batch, custom_id, op, request) values (?, ?, ?, ?);
