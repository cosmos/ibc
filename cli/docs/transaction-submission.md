<!-- SPDX-License-Identifier: Apache-2.0 -->

# EVM transaction recovery

The relayer saves each signed transaction and its packet associations before attempting broadcast. `Submit` runs synchronously under an account mutex that serializes submissions. A transaction reconciler independently polls chain state and records mined outcomes and reorgs. The pipeline rebuilds and resubmits unfinished work after abandonment. A canceled caller waits to acquire the mutex before returning. Cancellation can stop preparation; once the journal commit succeeds, the attempt and its operation associations survive cancellation or a lost broadcast response.

`HasPending` reports pending journaled work for a packet operation. The pipeline determines packet completion from chain state. It may retry after the journal entry is resolved or abandoned if the packet still needs work. Concurrent observations can cause redundant relays; the submitter does not deduplicate operations.

Use distinct signer addresses and run one relayer instance per signer on each chain. Every submission reads the mined nonce and journal, then uses the first unowned nonce. New work can replace external pending transactions; use dedicated relayer keys.

## Recovery behavior

Attempts remain `PENDING` until their nonce is consumed in the latest mined state or the abandonment window expires. Reconciliation records these states:

- `CONFIRMED`: a canonical mined receipt reports successful execution; this does not imply finality.
- `FAILED`: a canonical receipt reports failed execution.
- `REPLACED`: another journaled attempt at the same nonce has a canonical mined receipt.
- `CONSUMED`: the mined nonce has advanced, but all known hashes return receipt-not-found. The execution outcome is unknown.
- `ABANDONED`: unmined attempts exceeded the abandonment window. They stop reserving their nonce and packet operations.

Receipt RPC errors, invalid receipts, and noncanonical receipts leave work pending for retry. Historical signed bytes and resolved statuses are retained, together with the block number and hash where nonce consumption was observed. The mined attempt is identifiable by its status; no separate winning hash or provisional receipt is stored.

The reconciler does not broadcast or reprice. A retry builds a fresh transaction from current packet state, estimates gas against mined state, and increases both fee caps above prior attempts at the selected nonce. Its payload and batch composition can differ from the original.

Each pending attempt expires after `abandonAfter` (15 minutes by default), measured from when it was saved. The reconciler applies expiry before querying chain state; restarts do not reset the deadline. The pipeline rechecks packet state and rebuilds unfinished work, which can fill an earlier nonce gap. Abandonment does not cancel a transaction: retained attempts may still mine and are reconciled when the mined nonce advances. Each retry is saved with its own operation associations and abandonment window. Saved but unbroadcast attempts also wait for expiry before the pipeline retries.

Polling reads the account nonce at a specific canonical block hash. A resolved attempt at or above that nonce is reopened when the selected block reaches or exceeds the saved resolution height. A lower nonce at that height establishes a reorg; an older block observation leaves the attempt resolved. Reopening atomically clears the associated packet operation’s recorded transaction and makes the packet dispatchable again. Clearing a receive also clears its synchronous write acknowledgement. Recovery also runs after restart when no attempts are pending. Allocation reserves all non-abandoned journaled nonces at or above the mined nonce, including resolved attempts that polling has not yet reopened.

Polling may miss a new submission until the next pass.

## Configuration

| Setting under `relayer.chainOverrides[].evm` | Default | Purpose |
| --- | --- | --- |
| `maxFeePerGas` | `500000000000` | Maximum fee per gas, in wei (500 gwei). |
| `abandonAfter` | `15m` | How long an unfinished attempt reserves its nonce and operations before allowing a retry. |

Internal defaults are 10-second request deadlines, 3-second polling, a 16-nonce allocation window, and 10 million gas per transaction. The window starts at the mined nonce. It bounds new unmined submissions while allowing gaps to be filled. Mined entries awaiting receipt reconciliation may exceed this count. Reduce `packetBatchSize` if a batch exceeds the gas limit.

Retries at a previously used nonce increase both fee caps by at least 20% and one wei, using current estimates if higher. This includes attempts whose broadcast failed. If the required increase exceeds the configured cap, submission fails before signing; abandonment does not reset the fee history. Such a nonce can remain blocked even if network fees fall.

Reorg recovery detects nonces that become unmined again after their resolution block is replaced. A shorter replacement chain must reach the saved resolution height before recovery can resume. It does not revise resolved outcomes when another transaction has already consumed the same nonce on the new chain. Packet reconciliation and proof finality remain the responsibility of the relay and attestation layers.

## Operator procedures

- **Restart:** retain the database and signer keys. Recovery starts in the background immediately; journal read errors are logged and retried. Unfinished packet records supply work that was never signed. There is no automatic journal pruning.
- **Signer changes:** drain journaled work before removing a signer. Keep the old key available until its work is resolved.
- **Fee cap or insufficient funds:** restore funding or adjust the configured cap and restart. Pending work keeps its nonce until abandonment. Do not clear packet hashes or journal rows to force retry.
- **Nonce gaps:** new relay work fills unowned nonces starting at the mined nonce. Underpriced attempts wait for abandonment and a pipeline retry with higher fees. Abandonment releases work blocked at higher nonces so the pipeline can rebuild it at the gap. Without retryable packet work, an unowned gap requires manual recovery. The submitter never creates cancellation transactions.
- **Repeated broadcast rejection:** abandonment permits rebuilding the transaction, but does not fix an unchanged payload that violates gas limits or provider policy. Address that cause if retries keep failing.
- **RPC or database outage:** retain unresolved work and retry after service recovery. Failed reads prevent new allocation. Receipt lookup errors delay outcome resolution but do not block new submissions. Receipt-not-found responses after mined nonce consumption resolve to `CONSUMED`.
- **Database backup/restore:** back up the entire database consistently, including transaction and association tables. Account for SQLite WAL files. An old snapshot or lost journal requires reconciling account nonces and packet state before resuming.

Legacy transaction records without signed bytes are not supported.

## Monitoring

Transaction lifecycle metrics are deferred. Use logs and the journal to inspect pending attempts, outcomes, broadcast errors, and blocked replacements. Existing gas-spent and wallet-balance metrics use `chain_id` and `wallet`; packet completion and relay-duration metrics remain in the pipeline.

Gas-spent metrics use the receipt obtained during outcome resolution. Missing receipts or a restart between saving the outcome and emitting telemetry can undercount gas. Reorg recovery can count gas again when an attempt is mined again. These metrics are not an authoritative accounting ledger.
