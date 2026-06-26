-- +migrate Up
create table if not exists relay_submissions (
    id              integer primary key,
    source_chain_id text    not null,
    source_tx_hash  text    not null,
    created_at      text    not null default (datetime('now')),

    constraint relay_submissions_source_tx_unique
        unique (source_chain_id, source_tx_hash)
);

-- +migrate Down
drop table if exists relay_submissions;
