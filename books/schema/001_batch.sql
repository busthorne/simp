create table migration (epoch integer not null);

create table batch (
	id text primary key,
	super text,
	model text not null,
	body text,
	created_at timestamp not null default current_timestamp,
	updated_at timestamp,
	completed_at timestamp,
	canceled_at timestamp
);

create index batch_super on batch (super);

create table batch_op (
	batch text not null,
	custom_id text not null,
	request text not null,
	response text,
	implicit boolean not null, -- true for in-batch unbatchable tasks 
	deferred boolean not null, -- true for no-upload batch apis
	created_at timestamp not null default current_timestamp,
	updated_at timestamp,
	completed_at timestamp,
	canceled_at timestamp,

	primary key (batch, custom_id)
);
