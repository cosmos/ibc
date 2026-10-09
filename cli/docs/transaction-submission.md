<!-- SPDX-License-Identifier: Apache-2.0 -->

# EVM transaction recovery

The relayer saves each signed transaction and its packet associations before attempting broadcast. `Submit` runs synchronously under an account mutex that serializes submissions. A transaction reconciler independently polls chain state, rebroadcasts pending attempts, and reprices stuck transactions. A canceled caller waits to acquire the mutex before returning. Cancellation can stop preparation; once the journal commit succeeds, the reconciler owns recovery even if the caller cancels or the broadcast response is lost.

`HasPending` reports unresolved journaled work for a packet operation. The pipeline determines packet completion from chain state. It may retry after the journal entry is resolved if the packet still needs work. Concurrent observations can cause redundant relays; the submitter does not deduplicate operations.

Use distinct signer addresses and run one relayer instance per signer on each chain. Every submission reads the mined nonce and journal, then uses the first unowned nonce. New work can replace external pending transactions; use dedicated relayer keys.

## Recovery behavior

Attempts remain `PENDING` until their nonce is consumed in the latest mined state. Receipt checks then resolve all attempts at that nonce atomically:

- `CONFIRMED`: a canonical mined receipt reports successful execution; this does not imply finality.
- `REVERTED`: a canonical receipt reports failed execution.
- `REPLACED`: another journaled attempt at the same nonce has a canonical mined receipt.
- `CONSUMED`: the mined nonce has advanced, but all known hashes return receipt-not-found. The execution outcome is unknown.

Receipt RPC errors, invalid receipts, and noncanonical receipts leave work pending for retry. Historical signed bytes and resolved statuses are retained, together with the block number and hash where nonce consumption was observed. The mined attempt is identifiable by its status; no separate winning hash or provisional receipt is stored.

Unmined work is rebroadcast using the latest signed bytes and repriced at the same nonce when needed. Repricing preserves the call and only increases fees. An attempt without a recorded broadcast is sent unchanged before repricing.

Polling reads the account nonce at a specific canonical block hash. A resolved attempt at or above that nonce is reopened only if the saved resolution block has been replaced. If the node is behind that height, still reports the saved block as canonical, or cannot provide the block, the attempt remains resolved. Recovery also runs after restart when no attempts are pending. Allocation reserves all journaled nonces at or above the mined nonce, including resolved attempts that polling has not yet reopened.

Polling may miss a new submission until the next pass or duplicate a broadcast in progress.

## Configuration

| Setting under `relayer.chainOverrides[].evm` | Default | Purpose |
| --- | --- | --- |
| `maxFeePerGas` | `500000000000` | Maximum fee per gas, in wei (500 gwei). |

Internal defaults are 10-second request deadlines, 3-second polling, 1-minute repricing, a 16-nonce allocation window, and 10 million gas per transaction. The window starts at the mined nonce. It bounds new unmined submissions while allowing gaps to be filled. Mined entries awaiting receipt reconciliation may exceed this count. Reduce `packetBatchSize` if a batch exceeds the gas limit.

Replacement fees increase both caps by at least 20% and one wei, using current estimates if higher.

Reorg recovery detects nonces that become unmined again after their resolution block is replaced. A shorter replacement chain must reach the saved resolution height before recovery can resume. It does not revise resolved outcomes when another transaction has already consumed the same nonce on the new chain. Packet reconciliation and proof finality remain the responsibility of the relay and attestation layers.

## Operator procedures

- **Restart:** retain the database and signer keys. Recovery starts in the background immediately; journal read errors are logged and retried. Unfinished packet records supply work that was never signed. There is no automatic journal pruning.
- **Signer changes:** drain journaled work before removing a signer. Keep the old key available until its work is resolved.
- **Fee cap or insufficient funds:** restore funding or adjust the configured cap and restart. Pending work keeps its nonce. Do not clear packet hashes or journal rows to force retry.
- **Nonce gaps:** new relay work fills unowned nonces starting at the mined nonce. Underpriced attempts use normal repricing. Without new work, an unowned gap requires manual recovery. The submitter never creates cancellation transactions.
- **RPC or database outage:** retain unresolved work and retry after service recovery. Failed reads prevent new allocation. Receipt lookup errors delay outcome resolution but do not block new submissions. Receipt-not-found responses after mined nonce consumption resolve to `CONSUMED`.
- **Database backup/restore:** back up the entire database consistently, including transaction and association tables. Account for SQLite WAL files. An old snapshot or lost journal requires reconciling account nonces and packet state before resuming.

Legacy transaction records without signed bytes are not supported.

## Monitoring

Transaction lifecycle metrics are deferred. Use logs and the journal to inspect pending attempts, outcomes, broadcast errors, and blocked replacements. Existing gas-spent and wallet-balance metrics use `chain_id` and `wallet`; packet completion and relay-duration metrics remain in the pipeline.

Gas-spent metrics use the receipt obtained during outcome resolution. Missing receipts or a restart between saving the outcome and emitting telemetry can undercount gas. Reorg recovery can count gas again when an attempt is mined again. These metrics are not an authoritative accounting ledger.
