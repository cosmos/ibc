<!-- SPDX-License-Identifier: Apache-2.0 -->

# ADR 0001: EVM submission with packet-driven recovery

- Status: Accepted
- Date: 2026-09-23
- IBC revision inspected: `972f1356999f64c2666135102683658cb43de085`

## Context

The relayer needs to recover from RPC failures, dropped transactions, restarts, and reorgs without losing ownership of pending nonces. Transaction handling currently spans the submitter, relay processors, and packet storage:

| Component | Current responsibility | Limitation |
| --- | --- | --- |
| [EVM submitter](../../internal/txsubmitter/evm/evm.go) | Nonce lookup, fee estimation, signing, broadcast, receipt-based retry decision | Queries the pending nonce for every submission; a mutex and a two-second default delay reduce conflicts but do not track ownership of outstanding nonces or retain signed attempts. |
| [Receive](../../internal/relay/processors/batch_recv_packet.go), [acknowledgement](../../internal/relay/processors/batch_ack_packet.go), and [timeout](../../internal/relay/processors/timeout_packet.go) processors | Build calls and save submission details | Broadcast precedes the database write. A lost RPC response or crash can leave an accepted transaction unrecorded. |
| Retry processors | Clear a transaction reference after a revert or missing receipt past the two-minute expiry | The next submission builds another transaction instead of managing replacements at the original nonce. The previous transaction may still execute. |

The packet database retains relay work but not signed bytes, transaction nonces, or replacement history. Existing IBC state checks remain useful for detecting completed packet operations, but do not repair these transaction ownership gaps.

## Reuse assessment

The assessment uses pinned versions rather than assuming the latest upstream behavior will remain unchanged:

- Optimism: [`op-service/txmgr` at `a6a088325dee`](https://github.com/ethereum-optimism/optimism/tree/a6a088325deebbe6cad74db395b9a079669580b6/op-service/txmgr).
- Chainlink: [EVM txmgr at `2a2b38e8b744`](https://github.com/smartcontractkit/chainlink-evm/tree/2a2b38e8b74479054db92ab911521d60278c3d78/pkg/txmgr), using [the shared transaction framework at `bb6a2de39bcb`](https://github.com/smartcontractkit/chainlink-framework/tree/bb6a2de39bcb/chains/txmgr). This assesses the established txmgr implementation, not txm/v2.

| Option | What fits | Additional work |
| --- | --- | --- |
| Import OP's manager | Concurrent nonce allocation, rebroadcasting, same-nonce fee replacement, receipt polling, confirmation depth, request deadlines, configurable signer | Its transaction lifecycle is in memory. The public API has no resume-from-signed-attempt operation. Importing this version also requires OP's geth fork. |
| Import Chainlink's manager | Durable transactions and attempts, persist-before-broadcast, restart recovery, bounded in-flight work, fee replacement and receipt reconciliation | Integrates with Chainlink's storage, service lifecycle, keystore, fee estimation, and head tracking. Adoption would require substantial adaptation to IBC's SQLite/PostgreSQL store and signer interfaces. This is a source assessment, not an integration test. |
| Extend IBC's submitter | Keeps current geth, signers, storage, and observability; can introduce durability alongside existing packet records | IBC owns the implementation and review burden. Use the upstream lifecycle patterns and failure cases as references; this does not inherit their testing or production history. |

### Compatibility spike

Importing OP's package in an isolated Go 1.26.4 module with IBC's `go-ethereum v1.17.5` fails to compile: OP's receipt code references `DepositNonce` and `DepositReceiptVersion`, which upstream geth does not expose. OP's own module replaces geth with `op-geth v1.101702.3-rc.7`; dependency-module replacement directives are not inherited by consumers.

This import check can be repeated outside the repository:

```sh
go mod init txmgr-spike
go mod edit -go=1.26.4
go get github.com/ethereum/go-ethereum@v1.17.5
go get github.com/ethereum-optimism/optimism/op-service/txmgr@a6a088325deebbe6cad74db395b9a079669580b6
go build github.com/ethereum-optimism/optimism/op-service/txmgr
```

The local Besu 25.4.0 QBFT chain exposes a base fee, `eth_maxPriorityFeePerGas`, and `eth_blobBaseFee`. OP's default estimator requests the blob fee even for ordinary transactions; other Besu configurations without that method would need its custom estimator hook. This is compatibility with the tested development chain, not validation across Besu versions or configurations.

Signing can use the existing digest-signing interface: hash with the transaction's chain-specific Ethereum signer, call `Signer.Sign(ctx, digest)`, then attach the signature with `WithSignature`. The adapter must verify the requested sender and bound the signing request.

With OP's geth replacement confined to the temporary module, the following tests passed:

| Check | Result |
| --- | --- |
| Actual IBC local signer and `RemoteSigner` against an in-process KMS gRPC server | Both adapters produced a transaction whose recovered sender matched the key. This did not test an external KMS/HSM deployment. |
| Ordinary EIP-1559 submission on local Besu chain 41001 | A self-transaction from a fresh funded test key confirmed successfully. |
| Broadcast accepted by Besu, with a simulated lost RPC response | OP recovered and confirmed the transaction without changing its nonce. |
| First two sends return injected underpricing errors | OP raised fees at the same nonce; the resulting transaction confirmed. Native Besu rejection messages and configurable replacement thresholds were not exhaustively tested. |
| Four concurrent submissions sharing one manager | All confirmed, using four distinct nonces. |
| Stalled signing through IBC's `RemoteSigner` | OP's two-second request deadline canceled the KMS call. The test stubbed transaction preflight and broadcast nothing. |

These are compatibility checks, not a restart, reorg, throughput, or IBC packet E2E qualification. No product dependencies or runtime code were changed by the spike.

## Decision

Use a synchronous, in-memory transaction submitter. The pipeline owns recovery by checking packet state and retrying unfinished operations. Remove the transaction journal, operation associations, `HasPending`, and the transaction reconciliation service.

### Responsibilities

- `TxSubmitter.Submit(ctx, intent)` selects a nonce, estimates fees and gas, signs, and attempts broadcast. An account mutex serializes submissions. It returns the attempted transaction's hash, submission time, and relayer address, including when broadcast returns an error.
- The pipeline saves that reference in the packet record and calls `ShouldRetry` on subsequent dispatches. A successful receipt permits progress, a failed receipt permits retry immediately, and a missing receipt permits retry after `resubmitAfter`. RPC errors leave the reference intact.
- The pipeline clears recorded transactions that disappear before finality. Clearing a receive also clears its synchronous write acknowledgement. Source send, destination write acknowledgement, and source settlement checks use the route's provable height before progressing or completing the packet.

Use one active relayer per chain/signer account, with distinct signer addresses. Signers must be dedicated to relaying: stalled-nonce recovery can replace external submissions made with the same key.

### Nonce selection

Keep the next allocated nonce, the lowest unmined nonce, when that nonce was first observed, and its latest attempted fee caps in memory.

For each submission, under the account mutex:

1. Read both mined and pending nonces. Failed reads prevent submission.
2. Advance the local allocation counter to at least both RPC values. When the mined nonce changes, reset the stalled-nonce timer and fee history.
3. If the lowest nonce has remained unmined for `resubmitAfter`, reuse it. Otherwise allocate from the local counter, within a window of 16 nonces starting at the mined nonce.
4. Advance the counter immediately before broadcast. An error or lost response can still mean the node accepted the transaction.

A retry can carry different packet work from the original transaction. Reusing a nonce only happens when new or retried work reaches `Submit`; there is no background rebroadcasting, cancellation transaction, or empty self-transfer. A nonce gap with no retryable work needs operator intervention.

### Fees and request bounds

Estimate gas against mined state. Pending state may already include the operation being replaced and make an idempotent relay look cheaper than its actual execution.

Use current fee estimates. When reusing the lowest nonce with known fees, increase both fee caps by at least 20% and one wei, taking the current estimate if higher. A node's underpriced response also causes an immediate increase and another attempt at the same nonce. Limit a submission to eight broadcast attempts and a ten-second context covering RPC, signing, and broadcast after acquiring the mutex. Enforce gas and fee caps before signing.

Retain attempted fees for the lowest nonce across calls so repeated underpriced responses can make progress. Also increase fees after a stall even if the node accepted the previous broadcast: acceptance does not guarantee inclusion. An accepted replacement restarts the stalled-nonce timer.

### Packet retries and restart

Save the attempted hash, submission time, and relayer address in the batch's packet records atomically after submission. Preparation failures return no reference and can retry on the next dispatch. A crash before saving leaves packet work retryable; chain checks recover any completed operation, and nonce recovery handles a broadcast transaction that remains stuck. The submitter does not persist signed bytes or packet associations.

An error from `Submit` does not establish whether broadcast succeeded. The next dispatch checks chain state before retrying. Concurrent checks can still produce redundant relays; router packet semantics provide idempotency.

After restart, packet records and saved submission references remain, but nonce allocation, observed age, signed bytes, and fee history do not. Initialize allocation from the node and begin a new grace period for pending work. Once packet retry is allowed, a rebuilt submission can reuse a nonce immediately if the node has forgotten its transaction. Underpriced responses recover unknown replacement fees through bounded increases.

### Configuration and tradeoffs

Expose `resubmitAfter` (15 minutes) and `maxFeePerGas` (500 gwei) under each chain's EVM override. The same interval controls packet resubmission and lowest-nonce reuse; their timers start at different observations, so recovery can take more than one interval. Existing fee multipliers remain supported. Maximum gas is 10 million and the in-flight nonce window is 16.

This design accepts:

- No durable transaction outcome history or exact recovery of original signed bytes.
- Restart resets nonce age and fee progress; frequent restarts can postpone recovery.
- Duplicate transactions and additional gas costs. A later retry can displace a different relay operation; that packet remains retryable.
- A fee cap, depleted balance, repeated RPC error, or permanently invalid rebuilt payload can require operator intervention. Expiry does not fix those causes.
- Trust in the configured RPC view. Pending state is node-local; lagging or inconsistent endpoints can cause unnecessary retries.
- No recovery from a violation of the route's finality assumption after terminal packet completion.

Existing wallet and gas metrics remain best-effort telemetry. `ShouldRetry` accounts for gas when it finds a receipt and forgets missing transactions when their retry interval expires. In-memory tracking is bounded and is not an accounting ledger.

## Validation

Cover concurrent allocation, stalled nonces at capacity, unknown gaps after restart, lost broadcast responses, accepted-but-unmined transactions, underpriced replacement retries, fee caps, signer deadlines, mined failures, and reorgs before finality. Packet submission references must persist in SQLite and PostgreSQL and clear when their transactions fail or expire.

Run real receive, acknowledgement, timeout, restart, crash-before-broadcast, and reorg recovery E2Es with a short retry interval. Keep load/soak qualification and independent security review as release readiness work. Simulation-failure classification and individual-packet fallback remain deferred.

See the [recovery runbook](../transaction-submission.md) for operator procedures.
