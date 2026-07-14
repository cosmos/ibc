-- +migrate Up

create table if not exists packet_tx_submissions (
    packet_id   bigint NOT NULL references packets (id),
    submission_id bigint NOT NULL references relayer_tx_submissions (id),

    primary key (packet_id, submission_id)
);

-- +migrate Down
drop table if exists packet_tx_submissions;
