-- +migrate Up
create table if not exists relay_submissions (
    id              integer                  id serial primary key,
    source_chain_id text                     NOT NULL,
    source_tx_hash  text                     NOT NULL,
    created_at      timestamp with time zone NOT NULL DEFAULT (now() at time zone 'utc'),

    constraint relay_submissions_source_tx_unique
        unique (source_chain_id, source_tx_hash)
);

-- +migrate Down
drop table if exists relay_submissions;
