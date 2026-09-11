<!--
  ~ SPDX-License-Identifier: Apache-2.0
-->

# Observability

## Metric Modes

IBC supports two metric modes:

- `simple` exposes a Prometheus-compatible `/metrics` endpoint for direct scraping. It is
  intended for local development and straightforward Prometheus deployments.
- `otel` configures the OpenTelemetry SDK from a YAML file and exports metrics to an OTEL
  collector. It is intended for deployments that need configurable metric pipelines and
  observability backends.

Metrics are best-effort: failures while setting up the metrics pipeline are logged and
tolerated, so they do not prevent IBC from running.

## Available Metrics

Default OpenTelemetry Go runtime metrics and ConnectRPC RPC metrics from
 [otelconnect](https://github.com/connectrpc/otelconnect-go) are also exposed when observability is enabled.

### Signer metrics

Shared label: `{otel_scope_name="ibc.signer"}`


| metric               | type           | labels                                  | notes                                                                                                                  |
| -------------------- | -------------- | --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `signer_operation_*` | histogram (ms) | `signer`, `type`, `result`, `operation` | Sign latency. `type` is `remote`, `local_ecdsa`, or `local_eddsa`. `operation` is `sign`. `result` is `ok` or `error`. |

### Attestor metrics

Shared label: `{otel_scope_name="ibc.attestor"}`

| metric                 | type           | labels                                                  | notes                                                                                                                                     |
| ---------------------- | -------------- | ------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `attestor_operation_*` | histogram (ms) | `operation`, `chain_id`, `attestor`, `result`, `caller` | Latency. `operation`: `latest_height`, `state_attestation`, `packet_attestation`. `result`: `ok` / `error`. `caller`: `rpc` or `internal` |
| `latest_height`        | gauge          | `chain_id`, `attestor`, `caller`                        | Last successful height. Not written on error. Same `caller` values as above.                                                              |

### Prover metrics

Shared label: `{otel_scope_name="ibc.prover"}`

| metric                   | type           | labels                                                                                  | notes                                                                                                    |
| ------------------------ | -------------- | --------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `prover_operation_*`     | histogram (ms) | `operation`, `chain_id`, `client_id`, `type`, `result`; `proof_kind` on `packet_proofs` | `operation`: `latest_provable_height`, `state_proof`, `packet_proofs`. `type` is the client/prover type. |
| `latest_provable_height` | gauge          | `chain_id`, `client_id`                                                                 | Recorded only when `latest_provable_height` succeeds.                                                    |
| `packet_batch_size_*`    | histogram      | `chain_id`, `client_id`, `type`, `proof_kind`                                           | Packet count on PacketProofs. Default histogram buckets.                                                 |


`proof_kind`: `packet_commitment`, `acknowledgement`, `receipt_absence`, `unknown`.

### Relayer metrics

Shared label: `{otel_scope_name="ibc.relayer"}`

- `processor`: `recv`, `ack`, or `timeout`.
- Batch `chain_id`: destination for `recv`; source for `ack` and `timeout`.
- Transaction `chain_id`: chain receiving the transaction.
- Transaction `client_id`: client updated by the transaction; destination for `recv`, source for `ack` and `timeout`.
- Processor `type`: `send_to_recv`, `send_to_timeout`, or `recv_to_ack`.

In metrics that label both packet sides, `chain_id` / `client_id` refer to the packet's **source** side and
`dest_chain_id` / `dest_client_id` to its destination.

Confirmations count only successful receipts for transactions broadcast by this process and are deduplicated in memory by chain and tx hash.
Completions and durations are recorded only on success (`CompleteWithAck`, `CompleteWithTimeout`).
EVM wallet balances are queried concurrently at the latest block, at most once every 10 seconds per wallet.
Collections inside that threshold re-emit the last observed value. A failed query is logged and reported as `-1`.

| package     | metric                          | type               | labels                                                              | notes                                                          |
| ----------- | ------------------------------- | ------------------ | ------------------------------------------------------------------- | -------------------------------------------------------------- |
| pipeline    | `packets_total`                 | counter            | `chain_id`, `dest_chain_id`, `client_id`, `dest_client_id`, `state` | +1 per packet status transition                                |
| pipeline    | `batch_size_*`                  | histogram          | `chain_id`, `processor`, `result`                                   | Packet count in the batch; `batch_size_count` counts batches   |
| processors  | `relays_completed_total`        | counter            | `chain_id`, `dest_chain_id`, `client_id`, `dest_client_id`, `type`  | +1 per completed leg                                           |
| processors  | `relay_duration_*`              | histogram (s)      | `chain_id`, `dest_chain_id`, `client_id`, `dest_client_id`, `type`  | Wall time of that leg                                          |
| processors  | `transactions_submitted_total`  | counter            | `chain_id`, `client_id`                                             | +1 per successful broadcast                                    |
| processors  | `transactions_confirmed_total`  | counter            | `chain_id`, `client_id`                                             | +1 per successful receipt                                      |
| processors  | `transaction_retries_total`     | counter            | `chain_id`, `dest_chain_id`, `client_id`, `dest_client_id`, `type`  | +1 per submitted transaction retried                           |
| txsubmitter | `evm_gas_spent`                 | observable counter | `chain_id`, `wallet`                                                | Cumulative successful owned EVM tx cost in native-token units  |
| txsubmitter | `evm_gas_balance`               | observable gauge   | `chain_id`, `wallet`                                                | Latest EVM wallet balance in native-token units; `-1` on error |
| watcher     | `watcher_events_total`          | counter            | `chain_id`, `type` (`send_packet`)                                  | Observed send-packet events                                    |
| dispatch    | `packets_pending`               | gauge              | `chain_id`, `dest_chain_id`, `client_id`, `dest_client_id`          | Pending packets per route                                      |
| dispatch    | `excessive_relay_latency_total` | counter            | `chain_id`, `dest_chain_id`, `client_id`, `dest_client_id`, `type`  | +1 per poll while a packet is pending past its leg threshold   |

`excessive_relay_latency_total` counts packets still pending past a per-leg threshold, using the
same `type` values as processor metrics (`send_to_recv`, `send_to_timeout`, `recv_to_ack`). It is an
event counter, not a failure gauge: a packet that remains stuck increments on every dispatcher poll,
so alerts should be built on `rate()`. The threshold is hard-coded for now (60m pending; timeouts 5m
past the packet timeout, with a 15m source-finality guard) and can move to config in the future.

## Guide on creating new metrics

1. Ensure unique attribute keys are present in `otel/metrics.go` (eg `AttrChainID`, `AttrAttestor`). Add new if needed.
2. Create `metrics.go` based on the following reference example. Don't enforce units if not necessary.
3. Keep metric-related code in `<pkg>/metrics.go`, create small helpers to make metrics recording
  more concise for callers. See `latestHeight(...)` and `recordOperation(...)` in attestor's metrics.
4. Leave metrics unitless (except durations).

```go
package attestor

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/cosmos/ibc/cli/internal/otel"
)

type instrumentation struct {
	Operation    metric.Float64Histogram
	LatestHeight metric.Int64Gauge
}

var metrics = otel.RegisterMetrics("attestor", newInstrumentation)

func newInstrumentation(m metric.Meter) (*instrumentation, error) {
	operation, err := m.Float64Histogram("attestor_operation", otel.UnitMilliseconds())
	if err != nil {
		return nil, err
	}

	latestHeight, err := m.Int64Gauge("latest_height")
	if err != nil {
		return nil, err
	}

	return &instrumentation{
		Operation:    operation,
		LatestHeight: latestHeight,
	}, nil
}

func (m *instrumentation) recordOperation(
	ctx context.Context,
	operation, chainID, attestor string,
	err error,
	ts time.Time,
) {
	otel.RecordOperation(ctx, m.Operation, operation, ts,
		otel.AttrChainID.String(chainID),
		otel.AttrAttestor.String(attestor),
		otel.AttrResultError(err),
	)
}

func (m *instrumentation) latestHeight(ctx context.Context, attestor, chainID string, height uint64) {
	m.LatestHeight.Record(ctx, int64(height), otel.WithAttributes(
		otel.AttrChainID.String(chainID),
		otel.AttrAttestor.String(attestor),
	))
}
```