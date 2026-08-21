---
title: "API"
description: "The two gRPC services a running relayer and attestor serve, and how to call them."
---

A running `ibc` process serves gRPC on one address, `server.listenAddr`, defaulting to `0.0.0.0:3000`. A relayer serves `ibc.v2.relayer.RelayerApiService`, an attestor serves `ibc.v2.attestor.AttestationService`, and a process running both serves both on that one address.

The server speaks HTTP/2 without TLS, so a plaintext client connects without extra configuration. <!-- [server.go:L38-L43](link/internal/server/server.go#L38-L43) --> Reflection is registered, so a client works without the proto files. <!-- [server.go:L103-L104](link/internal/server/server.go#L103-L104) -->

```bash
grpcurl -plaintext localhost:3000 list
```

```
ibc.v2.relayer.RelayerApiService
```

An attestor-only process lists the attestation service instead, and a dual process lists both.

## Relayer service

`ibc.v2.relayer.RelayerApiService`

<!-- GEN:api:relayer:rpcs START -->

| RPC | Request | Response | What it does |
|---|---|---|---|
| `Relay` | `RelayRequest` | `RelayResponse` | Tracks the packets emitted by a source transaction and submits the transactions required to complete them. |
| `Status` | `StatusRequest` | `StatusResponse` | Returns per-packet relay status for a transaction previously submitted via Relay. |

<!-- [relayer.proto:L9](proto/link/relayer.proto#L9) -->

<!-- GEN:api:relayer:rpcs END -->

Nothing watches the chains, so `Relay` is what starts a delivery. `ibc relayer relay` and `ibc relayer status` are thin wrappers over these two calls.

### `Relay`

Asks the relayer to deliver the packets one transaction emitted.

<!-- GEN:api:msg:RelayRequest START -->

| Field | Type | Description |
|---|---|---|
| `tx_hash` | `string` |  |
| `source_chain_id` | `string` |  |
| `selection` | oneof: `all_packets` or `selected_packets` | Required and controls only this relayer instance; IBC relaying remains permissionless. |

<!-- [relayer.proto:L19](proto/link/relayer.proto#L19) -->

<!-- GEN:api:msg:RelayRequest END -->

The relayer reads the packet events out of that transaction, so the request names a transaction rather than a packet. Chain state holds only a commitment, and the packet's contents live in the event the send emitted.

The two selection options differ in what happens to a packet this relayer cannot route.

`all_packets` takes every packet for which this relayer has a configured client and route. Packets without one are skipped, and the request succeeds even when that leaves nothing to relay. <!-- [relayer.proto:L30-L33](proto/link/relayer.proto#L30-L33) -->

`selected_packets` takes exactly the packets listed, each named by a client and a sequence number.

<!-- GEN:api:msg:SelectedPackets START -->

| Field | Type | Description |
|---|---|---|
| `packets` | `repeated PacketSelector` |  |

<!-- [relayer.proto:L38](proto/link/relayer.proto#L38) -->

<!-- GEN:api:msg:SelectedPackets END -->

<!-- GEN:api:msg:PacketSelector START -->

| Field | Type | Description |
|---|---|---|
| `source_client_id` | `string` |  |
| `sequence_number` | `uint64` |  |

<!-- [relayer.proto:L42](proto/link/relayer.proto#L42) -->

<!-- GEN:api:msg:PacketSelector END -->

A `selected_packets` request fails if any packet it names is absent or has no configured route. Naming a packet that is already selected, in flight, or finished succeeds and changes nothing. <!-- [relayer.proto:L35-L37](proto/link/relayer.proto#L35-L37) -->

`RelayResponse` is empty. The call returns once the request is recorded, and the delivery happens afterwards. A successful response means the relayer accepted the work, not that it finished.

```bash
grpcurl -plaintext -d '{"txHash":"0xSendTxHash","sourceChainId":"41001","allPackets":{}}' \
  localhost:3000 ibc.v2.relayer.RelayerApiService/Relay
```

### `Status`

Reports what happened to each packet in a transaction the relayer was asked about.

<!-- GEN:api:msg:StatusRequest START -->

| Field | Type | Description |
|---|---|---|
| `tx_hash` | `string` |  |
| `source_chain_id` | `string` |  |

<!-- [relayer.proto:L49](proto/link/relayer.proto#L49) -->

<!-- GEN:api:msg:StatusRequest END -->

<!-- GEN:api:msg:StatusResponse START -->

| Field | Type | Description |
|---|---|---|
| `packet_statuses` | `repeated PacketStatus` |  |

<!-- [relayer.proto:L54](proto/link/relayer.proto#L54) -->

<!-- GEN:api:msg:StatusResponse END -->

<!-- GEN:api:msg:PacketStatus START -->

| Field | Type | Description |
|---|---|---|
| `state` | `PacketState` |  |
| `sequence_number` | `uint64` | Together with `source_client_id`, identifies the packet. |
| `source_client_id` | `string` | Together with `sequence_number`, identifies the packet. |
| `send_tx` | `TransactionInfo` | The source-chain SendPacket transaction. Always present. |
| `recv_tx` | `TransactionInfo` | The destination-chain receive transaction, once submitted or discovered. |
| `ack_tx` | `TransactionInfo` | The source-chain acknowledgement transaction. Present for succeeded and rejected packets, and may be present while pending. |
| `timeout_tx` | `TransactionInfo` | The source-chain timeout transaction. Present for timed-out packets, and may be present while pending. |

<!-- [relayer.proto:L81](proto/link/relayer.proto#L81) -->

<!-- GEN:api:msg:PacketStatus END -->

<!-- GEN:api:msg:TransactionInfo START -->

| Field | Type | Description |
|---|---|---|
| `tx_hash` | `string` |  |
| `chain_id` | `string` |  |

<!-- [relayer.proto:L76](proto/link/relayer.proto#L76) -->

<!-- GEN:api:msg:TransactionInfo END -->

`NOT_SELECTED` and `PENDING` are still open. The other four are final.

<!-- GEN:api:enum:PacketState START -->

| Value | Meaning |
|---|---|
| `PACKET_STATE_NOT_SELECTED` | The packet was discovered but has not been selected by this relayer instance. Another relayer may still deliver it. |
| `PACKET_STATE_PENDING` | The relayer is still processing the packet. |
| `PACKET_STATE_SUCCEEDED` | The packet completed with a successful acknowledgement on the source chain. |
| `PACKET_STATE_TIMED_OUT` | The packet completed with a timeout refund on the source chain. |
| `PACKET_STATE_REJECTED` | The packet completed with an error acknowledgement on the source chain. |
| `PACKET_STATE_RELAY_FAILED` | A permanent error prevents the relayer from processing the packet. |

<!-- [relayer.proto:L58](proto/link/relayer.proto#L58) -->

<!-- GEN:api:enum:PacketState END -->

`REJECTED` means the packet arrived and the application refused it, which is a completed relay and a failed application call. `RELAY_FAILED` means the relayer could not deliver it at all.

```bash
grpcurl -plaintext -d '{"txHash":"0xa3222ab810c72019802aeb1e0c53d1b7cd914318e3f77a24b9c69a2c9810b45f","sourceChainId":"41001"}' \
  localhost:3000 ibc.v2.relayer.RelayerApiService/Status
```

```json
{
  "packetStatuses": [
    {
      "state": "PACKET_STATE_SUCCEEDED",
      "sequenceNumber": "3",
      "sourceClientId": "link-41001-41002",
      "sendTx": {
        "txHash": "0xa3222ab810c72019802aeb1e0c53d1b7cd914318e3f77a24b9c69a2c9810b45f",
        "chainId": "41001"
      },
      "recvTx": {
        "txHash": "0x665fef2903364258e99e204eec917e47d3bb478a88772ba1509c4aeded80973c",
        "chainId": "41002"
      },
      "ackTx": {
        "txHash": "0x2bc07edaea5302b66ecec06163be3bb11924af9402cd0a8a55f5ca9426080557",
        "chainId": "41001"
      }
    }
  ]
}
```

A field with no transaction yet is absent, so a pending packet carries `sendTx` alone.

## Attestation service

`ibc.v2.attestor.AttestationService`

<!-- GEN:api:attestor:rpcs START -->

| RPC | Request | Response | What it does |
|---|---|---|---|
| `StateAttestation` | `StateAttestationRequest` | `StateAttestationResponse` | Retrieves an attestation for a state at a given height. |
| `PacketAttestation` | `PacketAttestationRequest` | `PacketAttestationResponse` | Retrieves an attestation for a set of packets. |
| `LatestHeight` | `LatestHeightRequest` | `LatestHeightResponse` | Returns the latest height the attestor will generate attestations for. |
| `Info` | `InfoRequest` | `InfoResponse` | Returns identity information about a configured attestor. |

<!-- [attestor.proto:L10](proto/link/attestor.proto#L10) -->

<!-- GEN:api:attestor:rpcs END -->

Every request names an attestor, because one process can run several. The name is the `name` key of a `local` entry in its `attestors` block, and a name the process does not serve returns `NotFound`.

### `Info` and `LatestHeight`

<!-- GEN:api:msg:InfoRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` |  |

<!-- [attestor.proto:L59](proto/link/attestor.proto#L59) -->

<!-- GEN:api:msg:InfoRequest END -->

<!-- GEN:api:msg:InfoResponse START -->

| Field | Type | Description |
|---|---|---|
| `chain_id` | `string` | The chain this attestor watches. |
| `address` | `string` | The attestor's signing address. |

<!-- [attestor.proto:L61](proto/link/attestor.proto#L61) -->

<!-- GEN:api:msg:InfoResponse END -->

`address` is what a light client checks signatures against. So this call is how an operator confirms that the running attestor is the one an on-chain client expects.

```bash
grpcurl -plaintext -d '{"attestor":"attestor-41002"}' \
  localhost:3000 ibc.v2.attestor.AttestationService/Info
```

```json
{
  "chainId": "41002",
  "address": "0xc7f148Da846781a9a1D9d22F699A7A88c592CCee"
}
```

`LatestHeight` returns the highest height this attestor will attest to, which is where its finality offset shows up. An offset of zero tracks the chain's own finalized tag, and a larger offset holds further back.

<!-- GEN:api:msg:LatestHeightRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` |  |

<!-- [attestor.proto:L55](proto/link/attestor.proto#L55) -->

<!-- GEN:api:msg:LatestHeightRequest END -->

<!-- GEN:api:msg:LatestHeightResponse START -->

| Field | Type | Description |
|---|---|---|
| `height` | `uint64` |  |

<!-- [attestor.proto:L57](proto/link/attestor.proto#L57) -->

<!-- GEN:api:msg:LatestHeightResponse END -->

### `StateAttestation`

Signs over a chain's state at one height. A relayer calls this to build the proof a light client verifies.

<!-- GEN:api:msg:StateAttestationRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` |  |
| `height` | `uint64` |  |

<!-- [attestor.proto:L24](proto/link/attestor.proto#L24) -->

<!-- GEN:api:msg:StateAttestationRequest END -->

<!-- GEN:api:msg:StateAttestationResponse START -->

| Field | Type | Description |
|---|---|---|
| `attestation` | `Attestation` |  |

<!-- [attestor.proto:L29](proto/link/attestor.proto#L29) -->

<!-- GEN:api:msg:StateAttestationResponse END -->

A height above what `LatestHeight` reports is refused, so ask for the height first.

### `PacketAttestation`

Signs over a set of packet commitments at one height.

<!-- GEN:api:msg:PacketAttestationRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` |  |
| `packets` | `repeated bytes` | The packets to attest to |
| `height` | `uint64` | The height to attest to the `packets` at |
| `commitment_type` | `CommitmentType` | The type of commitment to attest (packet or acknowledgment) Defaults to COMMITMENT_TYPE_PACKET if not specified (for backward compatibility) |

<!-- [attestor.proto:L35](proto/link/attestor.proto#L35) -->

<!-- GEN:api:msg:PacketAttestationRequest END -->

<!-- GEN:api:msg:PacketAttestationResponse START -->

| Field | Type | Description |
|---|---|---|
| `attestation` | `Attestation` |  |

<!-- [attestor.proto:L51](proto/link/attestor.proto#L51) -->

<!-- GEN:api:msg:PacketAttestationResponse END -->

`commitment_type` selects what is being proven, which differs by direction. A delivery proves a packet commitment, an acknowledgement proves an acknowledgement commitment, and a timeout proves that no receipt exists.

<!-- GEN:api:enum:CommitmentType START -->

| Value | Meaning |
|---|---|
| `COMMITMENT_TYPE_PACKET` | Packet commitment (for SendPacket events) |
| `COMMITMENT_TYPE_ACK` | Acknowledgment commitment (for WriteAcknowledgement events) |
| `COMMITMENT_TYPE_RECEIPT` | Receipt commitment (for Timeout events - non-membership proof) |

<!-- [attestor.proto:L69](proto/link/attestor.proto#L69) -->

<!-- GEN:api:enum:CommitmentType END -->

One request carries at most 100 packets, each at most 128 KB, and a request over either limit is refused. <!-- [service.go:L72-L76](link/internal/service/attestor/service.go#L72-L76) --> <!-- [service.go:L169-L180](link/internal/service/attestor/service.go#L169-L180) -->

### `Attestation`

Both attestation calls return the same shape.

<!-- GEN:api:msg:Attestation START -->

| Field | Type | Description |
|---|---|---|
| `height` | `uint64` | The height of the attestation |
| `timestamp` | `uint64, optional` | The timestamp of the block |
| `attested_data` | `bytes` | The attested data |
| `signature` | `bytes` | The attestation signature |

<!-- [attestor.proto:L80](proto/link/attestor.proto#L80) -->

<!-- GEN:api:msg:Attestation END -->

`attested_data` is what was signed and `signature` is the attestor's signature over it. A light client accepts the attestation when enough attestors in its set sign the same data, up to its threshold.

## Next steps

- [Configuration](/ibc-cli/configuration) for `server.listenAddr` and the `attestors` block these calls read.
- [CLI commands](/ibc-cli/cli-commands) for the commands that make these calls.
