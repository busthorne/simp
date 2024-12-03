-- name: KeyringList :many
select key
	from keyring
	where ring = ? and ns = ?;

-- name: KeyringGet :one
select value
	from keyring
	where ring = ? and ns = ? and key = ?;

-- name: KeyringSet :exec
insert into keyring (ring, ns, key, value, updated_at)
	values (?, ?, ?, ?, current_timestamp)
	on conflict (ring, ns, key)
		do update set
			value = excluded.value,
			updated_at = current_timestamp;

-- name: KeyringDelete :exec
delete from keyring
	where ring = ? and ns = ? and key = ?;
