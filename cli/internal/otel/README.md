# Observability

## Metric Modes

`TODO`

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

| metric                 | type           | labels                                        | notes                                                                                                       |
| ---------------------- | -------------- | --------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `attestor_operation_*` | histogram (ms) | `operation`, `chain_id`, `attestor`, `result` | Latency. `operation`: `latest_height`, `state_attestation`, `packet_attestation`. `result`: `ok` / `error`. |
| `latest_height`        | gauge          | `chain_id`, `attestor`                        | Last successful height. Not written on error.                                                               |

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

`processor` is `recv` / `ack` / `timeout`. Batch `chain_id` is dest for `recv`, source for `ack`/`timeout`. Completions and durations only on success (`CompleteWithAck`, `CompleteWithTimeout`). Processor `type`: `send_to_recv`, `send_to_timeout`, `recv_to_ack`. Removed/reorg watcher events are not counted. Routes absent from the latest snapshot reset to 0.

| package    | metric                      | type          | labels                                                              | notes                           |
| ---------- | --------------------------- | ------------- | ------------------------------------------------------------------- | ------------------------------- |
| pipeline   | `packets_total`             | counter       | `chain_id`, `dest_chain_id`, `client_id`, `dest_client_id`, `state` | +1 per packet status transition |
| pipeline   | `batches_total`             | counter       | `chain_id`, `processor`, `result`                                   | +1 per batch submit attempt     |
| pipeline   | `batch_size_*`              | histogram     | `chain_id`, `processor`                                             | Packet count in the batch       |
| processors | `relays_completed_total`    | counter       | `chain_id`, `dest_chain_id`, `client_id`, `dest_client_id`, `type`  | +1 per completed leg            |
| processors | `relay_duration_*`          | histogram (s) | same                                                                | Wall time of that leg           |
| processors | `transaction_retries_total` | counter       | same                                                                | +1 per tx retry                 |
| watcher    | `watcher_events_total`      | counter       | `chain_id`, `type` (`send_packet`, `write_ack`)                     | Observed events                 |
| dispatch   | `packets_pending`           | gauge         | `chain_id`, `dest_chain_id`, `client_id`, `dest_client_id`          | Pending packets per route       |

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