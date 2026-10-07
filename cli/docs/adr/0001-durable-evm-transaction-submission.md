<!-- SPDX-License-Identifier: Apache-2.0 -->

# ADR 0001: Durable EVM transaction submission

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

Keep `internal/txsubmitter` as a durable broadcaster. The pipeline determines packet outcomes from chain state; the submitter tracks pending work and records confirmed transaction outcomes. Use upstream geth, the existing signers, and the existing SQLite/PostgreSQL Store.

### Ownership and API

One relayer instance owns each chain/signer account. Configured signers have unique addresses. Concurrent routes share that account's submitter and lock.

- `Submit(ctx, intent) error` synchronously selects a nonce, estimates fees and gas, signs, journals, and attempts broadcast. It returns no transaction hash or submission ID.
- `HasPending(ctx, operation) (bool, error)` queries unresolved journal entries for the packet and relay kind. Unsigned work is not reported. Repeated and overlapping submissions are accepted at distinct nonces.
- The pipeline checks chain state before retrying. Pending work waits; otherwise the pipeline rebuilds a transaction if the packet still needs work. Neither broadcast acceptance nor nonce consumption establishes packet completion.

There is no internal submission queue, operation deduplication, unsigned reservation table, or second in-memory transaction cache. An account mutex serializes submissions; polling runs independently. Cancellation is checked after acquiring the mutex, so a canceled caller can wait for another submission to finish before returning. Caller cancellation can abort preparation, but cannot revoke a journaled transaction. Shutdown cancels requests and waits for polling and the active submission before closing the RPC client.

### Journal

Save each signed attempt and its operation associations atomically before broadcast. Retain signed bytes, hash, nonce, creation time, last broadcast attempt time, and state. Attempts begin `PENDING`. A canonical receipt at the confirmation boundary identifies a `CONFIRMED` or `REVERTED` attempt; the other attempts at that nonce become `REPLACED` in the same update. If the nonce is confirmed consumed but all known receipts are missing, mark the attempts `CONSUMED` with an unknown execution outcome. Older signed attempts remain available for diagnosis; no separate winning hash or provisional receipt is persisted.

A failed journal write prevents broadcast. An uncertain commit is recovered by querying the journal before the next allocation. Broadcast errors retain signed work. Recording the last broadcast is best effort: if it fails, the next pass may resend the same bytes safely.

### Submission and nonce allocation

Under the account lock:

1. Query the mined account nonce and distinct pending journal nonces.
2. Select the first unowned nonce at or above the mined nonce. Both reads must succeed. Do not use the node's pending nonce or depend on a previous polling pass.
3. Bound the selected nonce to a window of 16 nonces starting at the mined nonce. This permits filling holes below later journal entries without an extra capacity exception. Mined but unconfirmed entries remain journaled without occupying the allocation window.
4. Estimate fees and gas, sign, and commit the attempt and packet links.
5. Attempt broadcast and return. After a successful journal commit, cancellation or a broadcast error leaves recovery to polling.

The window limits how far new submissions run ahead of mining, not the number of entries awaiting finality. Reorgs can leave existing entries outside the window; retain those entries and continue recovery. Alert if confirmation progress stalls and the unresolved journal grows.

### Recovery

Each polling pass loads pending attempts and reads mined and confirmed account nonces. The confirmed read uses the configured finality policy and checks the RPC-reported block hash before and after the nonce query. It does not maintain head history. RPC-reported hashes are required because geth's computed header hash differs from Besu QBFT hashes.

- **Nonce below the confirmed nonce:** check known attempts for a canonical receipt at or below the confirmation height and record the outcome. RPC errors or invalid receipts leave work pending for retry unless another attempt has a valid winning receipt. If all attempts return receipt-not-found, resolve to `CONSUMED`. Each resolved outcome permits the pipeline to recheck packet state.
- **Nonce below the mined nonce but not confirmed:** keep the journal entries and wait.
- **Unmined nonce:** rebroadcast the latest signed bytes when due. Reprice only the lowest blocking nonce after the repricing interval, preserving destination, value, calldata, gas limit, and nonce.

There is no need to persist receipt appearance/disappearance or classify every broadcast error. If a transaction is reorged out before confirmation, its retained journal entry becomes eligible for broadcast again. Recovery beyond the configured confirmation boundary is outside this guarantee.

Polling can miss newly inserted attempts until the next pass, and concurrent broadcasts can send the same bytes twice. If polling resolves a nonce between submission's mined-nonce read and journal query, submission may use that already-consumed nonce. Normal resolution releases the operation for packet-state reconciliation and retry. These races do not require another lock or persisted state.

Receipt lookups and outcome persistence share a bounded request context per nonce. A failed nonce lookup does not prevent processing later nonce groups. Gas telemetry uses the winning receipt after the outcome is saved. Gas accounting can undercount when receipts are unavailable or the process exits between persistence and telemetry; the journal is not a billing ledger.

### Policies and accepted tradeoffs

- Bound RPC and signer calls to 10 seconds. Poll every 3 seconds and reprice after 1 minute. These remain internal defaults.
- Increase both fee caps by at least 20% and one wei, using current estimates if higher. Enforce the maximum fee and gas limits. Cap exhaustion or insufficient funds leaves pending work visible for operator recovery.
- Preserve `confirmations` and `maxFeePerGas` as the only new YAML settings. Zero confirmations selects the finalized block tag; positive values select block depth. Optional submission pacing defaults to zero and is not needed for nonce correctness.
- Accept occasional redundant relays and their gas cost when concurrent checks race. Router packet semantics provide idempotency; the submitter does not promise exactly-once execution.
- New relay work may replace an external transaction at an unowned nonce. Dedicated keys avoid this conflict. Do not generate cancellation transactions or self-transfers. Without new work, an unowned gap requires operator recovery.
- Use the journal for restart recovery and packet records to rebuild unsigned work. Do not support legacy transaction records, multiple active processes sharing a signer, or recovery from lost journal data without operator reconciliation.

## Validation

Tests must cover atomic persistence, uncertain commits and broadcasts, concurrent nonce allocation, nonce gaps/window limits, restart, same-nonce repricing, fee-cap exhaustion, older attempts winning, reverts, missing receipts, pre-confirmation reorgs, and changing confirmation anchors. Pending checks must survive restart and distinguish relay kinds. Outcome resolution must never write packet transaction hashes.

Run journal tests against SQLite and PostgreSQL, race tests over submission and polling, and the real receive/acknowledgement/timeout recovery E2Es. Load/soak qualification and independent security review remain release readiness work.

Simulation-failure classification, individual-packet fallback, a general pipeline redesign, and deployment-tool reuse are deferred. See the [recovery runbook](../transaction-submission.md) for operator procedures and metrics.
