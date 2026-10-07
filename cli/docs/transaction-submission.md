<!-- SPDX-License-Identifier: Apache-2.0 -->

# EVM transaction recovery

The relayer saves each signed transaction and its packet associations before attempting broadcast. `Submit` runs synchronously under an account mutex that serializes submissions. Polling runs independently. A canceled caller waits to acquire the mutex before returning. Cancellation can stop preparation; once the journal commit succeeds, polling owns recovery even if the caller cancels or the broadcast response is lost.

`HasPending` reports unresolved journaled work for a packet operation. The pipeline determines packet completion from chain state. It may retry after the journal entry is resolved if the packet still needs work. Concurrent observations can cause redundant relays; the submitter does not deduplicate operations.

Use distinct signer addresses and run one relayer instance per signer on each chain. Every submission reads the mined nonce and journal, then uses the first unowned nonce. New work can replace external pending transactions; use dedicated relayer keys.

## Recovery behavior

Attempts remain `PENDING` until their nonce is consumed at the configured confirmation boundary. Receipt checks then resolve all attempts at that nonce atomically:

- `CONFIRMED`: a canonical receipt reports successful execution.
- `REVERTED`: a canonical receipt reports failed execution.
- `REPLACED`: another journaled attempt at the same nonce has a confirmed receipt.
- `CONSUMED`: the nonce is confirmed consumed, but all known hashes return receipt-not-found. The execution outcome is unknown.

Receipt RPC errors, invalid receipts, and receipts outside the confirmed canonical chain leave work pending for retry. Historical signed bytes and terminal statuses are retained. The mined attempt is identifiable by its status; no separate winning hash or provisional receipt is stored.

Mined but unconfirmed attempts remain journaled. If their inclusion is reorged out, polling resumes broadcasting them. Unmined work is rebroadcast using the latest signed bytes and repriced at the same nonce when needed. Repricing preserves the call and only increases fees. An attempt without a recorded broadcast is sent unchanged before repricing.

Polling may miss a new submission until the next pass or duplicate a broadcast in progress. If resolution races with allocation, a stale nonce can produce a rejected transaction; polling resolves it and the pipeline rechecks packet state before retrying.

## Configuration

| Setting under `relayer.chainOverrides[].evm` | Default | Purpose |
| --- | --- | --- |
| `confirmations` | `0` | Wait for `finalized`. A positive value selects block depth; `1` accepts inclusion. |
| `maxFeePerGas` | `500000000000` | Maximum fee per gas, in wei (500 gwei). |

Internal defaults are 10-second request deadlines, 3-second polling, 1-minute repricing, a 16-nonce allocation window, and 10 million gas per transaction. The window starts at the mined nonce. It bounds new unmined submissions while allowing gaps to be filled. Journal entries awaiting finality may exceed this count; monitor stalled confirmation progress. Reduce `packetBatchSize` if a batch exceeds the gas limit.

`chainOverrides[].txSubmissionDelay` defaults to zero and provides optional pacing. Nonce correctness does not depend on sleeps. Replacement fees increase both caps by at least 20% and one wei, using current estimates if higher.

Choose confirmation policy for the actual chain. Recovery retains transactions until that boundary; a deeper reorg requires operator reconciliation. The E2E harness uses one-block inclusion on disposable development chains, not as a production recommendation.

## Operator procedures

- **Restart:** retain the database and signer keys. The submitter recovers journaled work; unfinished packet records supply work that was never signed. There is no automatic journal pruning.
- **Signer changes:** drain journaled work before removing a signer. Keep the old key available until its work is resolved.
- **Fee cap or insufficient funds:** restore funding or adjust the configured cap and restart. Pending work keeps its nonce. Do not clear packet hashes or journal rows to force retry.
- **Nonce gaps:** new relay work fills unowned nonces starting at the mined nonce. Underpriced attempts use normal repricing. Without new work, an unowned gap requires manual recovery. The submitter never creates cancellation transactions.
- **RPC or database outage:** retain unresolved work and retry after service recovery. Failed reads prevent new allocation. Receipt lookup errors delay outcome resolution but do not block new submissions. Receipt-not-found responses after confirmed nonce consumption resolve to `CONSUMED`.
- **Database backup/restore:** back up the entire database consistently, including transaction and association tables. Account for SQLite WAL files. An old snapshot or lost journal requires reconciling account nonces and packet state before resuming.

Legacy transaction records without signed bytes are not supported.

## Monitoring

Metrics use `chain_id` and `wallet`. Hashes remain in logs and the journal.

| Metric | Use |
| --- | --- |
| `evm_tx_pending` | Journaled nonces awaiting resolution. |
| `evm_tx_oldest_pending_seconds` | Age of the oldest unresolved nonce. |
| `evm_tx_events` | Submissions, fee replacements, broadcast errors, replacement blockage, and per-attempt outcomes (`confirmed`, `reverted`, `replaced`, `consumed`), split by `event`. |
| `evm_tx_resolution_seconds` | Time from the first signed attempt to recorded nonce resolution. This does not imply packet success. |

Alert on sustained pending age, growing pending count, repeated `replacement_blocked` events, and RPC/database failures. Packet completion and relay-duration metrics remain in the pipeline.

Gas-spent metrics use the receipt obtained during outcome resolution. Missing receipts or a restart between saving the outcome and emitting telemetry can undercount gas. These metrics are not an authoritative accounting ledger.
