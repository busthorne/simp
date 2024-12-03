create table keyring (
	ring text not null,
	ns text not null,
	key text not null,
	value blob not null,
	updated_at timestamp not null default current_timestamp,

	primary key (ring, ns, key)
);
