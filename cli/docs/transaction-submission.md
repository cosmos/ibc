<!-- SPDX-License-Identifier: Apache-2.0 -->

# EVM transaction recovery

The submitter signs and broadcasts synchronously under an account mutex. It keeps nonce allocation and fee progress in memory, without a transaction journal or background recovery loop. The pipeline saves the attempted hash and submission time in each packet record and uses `ShouldRetry` to check its receipt. A broadcast error may still mean the node accepted the transaction.

Run one relayer instance per chain/signer account and use dedicated signer keys. Multiple instances sharing a key are unsupported. Recovery can replace external transactions submitted with that key.

## Configuration

| Setting under `relayer.chainOverrides[].evm` | Default | Purpose |
| --- | --- | --- |
| `maxFeePerGas` | `500000000000` | Maximum fee per gas, in wei (500 gwei). |
| `resubmitAfter` | `15m` | Missing-transaction retry interval and delay before reusing the lowest stalled nonce. |

Submissions have a ten-second timeout after acquiring the account mutex, at most eight broadcast attempts, a 16-nonce window, and a 10-million gas limit. Reduce `packetBatchSize` if a batch exceeds the gas limit.

## Recovery behavior

New work normally uses the highest of the local next nonce and the node's mined and pending nonces. If the lowest unmined nonce remains unchanged for `resubmitAfter`, the next submission reuses it, even when the in-flight window is full. An accepted replacement restarts that timer. There is no cancellation or self-transfer when no packet work is available.

At a reused nonce, raise both fee caps above the remembered attempt, using current estimates if higher. An underpriced RPC response also triggers a bounded fee increase and retry. This covers both rejected replacements and transactions that the node accepts but does not mine. Fee increases cannot exceed the configured cap.

Packet retries use current packet state and can change payload or batch composition. A successful receipt allows the pipeline to proceed; a failed receipt makes the packet retryable immediately. A missing receipt allows retry after `resubmitAfter` from submission. RPC errors retain the reference for a later check. Preparation failures leave no transaction reference and can retry on the next dispatch. The packet retry timer and nonce timer start separately; recovering an unknown gap can take more than one interval.

The pipeline waits for the route's provable height before relaying a send or write acknowledgement and before completing the final acknowledgement or timeout. Recorded receive/settlement transactions that disappear before finality are cleared so work can be rebuilt. Clearing a receive also clears its write acknowledgement. Violations of the configured finality assumption after completion are outside this recovery model.

## Operator procedures

- **Restart:** preserve the packet database and signer keys. Saved transaction hashes and submission times survive; nonce age and fee history reset. The nonce timer gets a fresh grace period. Frequent restarts can delay recovery. A crash before saving a hash leaves the packet available for reconstruction from chain state.
- **Signer changes:** let outstanding packets settle before switching keys. Keep the old key available to resolve any remaining transactions.
- **Fee cap or insufficient funds:** restore funding or review the configured cap. Higher fees cannot fix every broadcast rejection. Investigate gas limits, RPC policy, and failing calls if retries keep failing.
- **Nonce gaps:** leave retryable work running long enough for the lowest nonce to expire. Without unfinished packet work, unblock the account manually. The relayer will not generate an empty transaction.
- **RPC outage:** restore a consistent RPC endpoint. Failed nonce queries prevent submission; ambiguous broadcast responses are handled by checking packet state before a later retry.
- **Database backup/restore:** back up packet records consistently, including SQLite WAL files. Restoring an older snapshot can cause redundant relay attempts. Lost packet records require packet recovery or re-ingestion; the submitter has no journal from which to reconstruct them.

## Monitoring

Use packet backlog, completion and relay-duration metrics, wallet balances, and broadcast errors to identify stalled work. Existing gas-spent metrics use receipts checked by `ShouldRetry` and forget missing transactions when their retry interval expires. Tracking has bounded memory. Restarts, lost transaction references, unavailable receipts, and reorgs can cause missing or inaccurate gas accounting. These metrics are not a billing ledger.
