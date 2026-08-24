---
title: "API"
description: "The two gRPC services a running relayer and attestor serve, and how to call them."
---

The IBC CLI exposes two APIs: a relayer API and an attestation API.

Packets are moved by calling the relayer's API, which has two main parts:

- `Relay` takes the transaction that sent the packets.
- `Status` reports how far each of them got.

The attestor's API serves the attestations a light client verifies. A relayer is its usual caller, gathering proofs for a packet it is delivering. An operator calls it directly to check which attestor is answering and how far behind the head it will sign.

Both services listen on `server.listenAddr`, defaulting to `0.0.0.0:3000`.

Connections are plaintext, HTTP/1.1 or HTTP/2 without TLS, and reflection is registered, so a client needs no TLS setup and no proto files. <!-- [server.go:L38-L43](link/internal/server/server.go#L38-L43) --> <!-- [server.go:L103-L113](link/internal/server/server.go#L103-L113) -->

To list running services:

```bash
grpcurl -plaintext localhost:3000 list
```

```
ibc.v2.relayer.RelayerApiService
```

## Relayer service

`ibc.v2.relayer.RelayerApiService`. `ibc relayer relay` and `ibc relayer status` call these two.

### `Relay`

<!-- GEN:api:rpc:Relay START -->

Tracks the packets emitted by a source transaction and submits the transactions required to complete them.

<!-- [relayer.proto:L12](proto/link/relayer.proto#L12) -->

<!-- GEN:api:rpc:Relay END -->

#### `RelayRequest`

<!-- GEN:api:msg:RelayRequest START -->

| Field | Type | Description |
|---|---|---|
| `tx_hash` | `string` | The transaction that sent the packets, on the source chain. |
| `source_chain_id` | `string` | The chain that transaction was sent on. |
| `selection` | oneof: `all_packets` or `selected_packets` | Required and controls only this relayer instance; IBC relaying remains permissionless. |

<!-- [relayer.proto:L19](proto/link/relayer.proto#L19) -->

<!-- GEN:api:msg:RelayRequest END -->

Field names in these tables are the schema's. The JSON encoding uses lowerCamelCase, so `tx_hash` is sent as `txHash`.

`all_packets` takes every packet for which this relayer has a configured client and route. Packets without one are skipped, and the request succeeds even when that leaves nothing to relay. <!-- [relayer.proto:L30-L33](proto/link/relayer.proto#L30-L33) -->

#### `SelectedPackets`

<!-- GEN:api:msg:SelectedPackets START -->

| Field | Type | Description |
|---|---|---|
| `packets` | `PacketSelector[]` | The packets to relay. At least one. |

<!-- [relayer.proto:L38](proto/link/relayer.proto#L38) -->

<!-- GEN:api:msg:SelectedPackets END -->

#### `PacketSelector`

<!-- GEN:api:msg:PacketSelector START -->

| Field | Type | Description |
|---|---|---|
| `source_client_id` | `string` | The client the packet was sent on. |
| `sequence_number` | `uint64` | The packet's number on that client. |

<!-- [relayer.proto:L42](proto/link/relayer.proto#L42) -->

<!-- GEN:api:msg:PacketSelector END -->

A `selected_packets` request fails if any packet it names is absent or has no configured route. Naming a packet that is already selected, in flight, or finished succeeds and changes nothing. <!-- [relayer.proto:L35-L37](proto/link/relayer.proto#L35-L37) -->

`Relay` returns nothing. A successful call means the relayer recorded the request, and `Status` reports what happens after that.

```bash
grpcurl -plaintext -d '{"txHash":"0xSendTxHash","sourceChainId":"41001","allPackets":{}}' \
  localhost:3000 ibc.v2.relayer.RelayerApiService/Relay
```

### `Status`

<!-- GEN:api:rpc:Status START -->

Returns per-packet relay status for a transaction previously submitted via Relay.

<!-- [relayer.proto:L16](proto/link/relayer.proto#L16) -->

<!-- GEN:api:rpc:Status END -->

#### `StatusRequest`

<!-- GEN:api:msg:StatusRequest START -->

| Field | Type | Description |
|---|---|---|
| `tx_hash` | `string` | The transaction whose packets to report on. |
| `source_chain_id` | `string` | The chain that transaction was sent on. |

<!-- [relayer.proto:L49](proto/link/relayer.proto#L49) -->

<!-- GEN:api:msg:StatusRequest END -->

#### `StatusResponse`

<!-- GEN:api:msg:StatusResponse START -->

| Field | Type | Description |
|---|---|---|
| `packet_statuses` | `PacketStatus[]` | One entry per packet this relayer recorded for the transaction. |

<!-- [relayer.proto:L54](proto/link/relayer.proto#L54) -->

<!-- GEN:api:msg:StatusResponse END -->

#### `PacketStatus`

<!-- GEN:api:msg:PacketStatus START -->

| Field | Type | Description |
|---|---|---|
| `state` | `PacketState` | Where the packet got to. See the states below. |
| `sequence_number` | `uint64` | Together with `source_client_id`, identifies the packet. |
| `source_client_id` | `string` | Together with `sequence_number`, identifies the packet. |
| `send_tx` | `TransactionInfo` | The source-chain SendPacket transaction. Always present. |
| `recv_tx` | `TransactionInfo` | The destination-chain receive transaction, once submitted or discovered. |
| `ack_tx` | `TransactionInfo` | The source-chain acknowledgement transaction. Present for succeeded and rejected packets, and may be present while pending. |
| `timeout_tx` | `TransactionInfo` | The source-chain timeout transaction. Present for timed-out packets, and may be present while pending. |

<!-- [relayer.proto:L81](proto/link/relayer.proto#L81) -->

<!-- GEN:api:msg:PacketStatus END -->

#### `TransactionInfo`

<!-- GEN:api:msg:TransactionInfo START -->

| Field | Type | Description |
|---|---|---|
| `tx_hash` | `string` | The transaction's hash. |
| `chain_id` | `string` | The chain it was submitted to. |

<!-- [relayer.proto:L76](proto/link/relayer.proto#L76) -->

<!-- GEN:api:msg:TransactionInfo END -->

#### `PacketState`

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

`REJECTED` means the packet arrived and the application refused it, which is a completed relay and a failed application call. `RELAY_FAILED` means a permanent error stopped the relayer, wherever the packet had got to.

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

A transaction with no entry yet is absent from the response.

## Attestation service

`ibc.v2.attestor.AttestationService`. One process can run several attestors, so every request names one.

### `Attestation`

Both attestation calls return this shape.

<!-- GEN:api:msg:Attestation START -->

| Field | Type | Description |
|---|---|---|
| `height` | `uint64` | The height of the attestation |
| `timestamp` | `uint64` (optional) | The timestamp of the block |
| `attested_data` | `bytes` | The attested data |
| `signature` | `bytes` | The attestation signature |

<!-- [attestor.proto:L80](proto/link/attestor.proto#L80) -->

<!-- GEN:api:msg:Attestation END -->

`attested_data` is what was signed. A light client accepts the attestation when its threshold of attestors or more sign the same data. <!-- [quorum.go:L73-L77](link/internal/relay/proofgen/attestation/quorum.go#L73-L77) -->

### `StateAttestation`

<!-- GEN:api:rpc:StateAttestation START -->

Retrieves an attestation for a state at a given height.

<!-- [attestor.proto:L12](proto/link/attestor.proto#L12) -->

<!-- GEN:api:rpc:StateAttestation END -->

#### `StateAttestationRequest`

<!-- GEN:api:msg:StateAttestationRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` | Which attestor to ask, by its `name` in the `attestors` block. |
| `height` | `uint64` | The height to attest to. |

<!-- [attestor.proto:L24](proto/link/attestor.proto#L24) -->

<!-- GEN:api:msg:StateAttestationRequest END -->

#### `StateAttestationResponse`

<!-- GEN:api:msg:StateAttestationResponse START -->

| Field | Type | Description |
|---|---|---|
| `attestation` | `Attestation` | The signed attestation. See below. |

<!-- [attestor.proto:L29](proto/link/attestor.proto#L29) -->

<!-- GEN:api:msg:StateAttestationResponse END -->

A height above what `LatestHeight` reports is refused, so ask for the height first. <!-- [local.go:L111-L117](link/internal/service/attestor/local.go#L111-L117) -->

### `PacketAttestation`

<!-- GEN:api:rpc:PacketAttestation START -->

Retrieves an attestation for a set of packets.

<!-- [attestor.proto:L15](proto/link/attestor.proto#L15) -->

<!-- GEN:api:rpc:PacketAttestation END -->

#### `PacketAttestationRequest`

<!-- GEN:api:msg:PacketAttestationRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` | Which attestor to ask, by its `name` in the `attestors` block. |
| `packets` | `bytes[]` | The packets to attest to |
| `height` | `uint64` | The height to attest to the `packets` at |
| `commitment_type` | `CommitmentType` | The type of commitment to attest (packet or acknowledgment) Defaults to COMMITMENT_TYPE_PACKET if not specified (for backward compatibility) |

<!-- [attestor.proto:L35](proto/link/attestor.proto#L35) -->

<!-- GEN:api:msg:PacketAttestationRequest END -->

#### `CommitmentType`

`commitment_type` selects what is being proven, which differs by direction. A delivery proves a packet commitment, an acknowledgement proves an acknowledgement commitment, and a timeout proves that no receipt exists.

<!-- GEN:api:enum:CommitmentType START -->

| Value | Meaning |
|---|---|
| `COMMITMENT_TYPE_PACKET` | Packet commitment (for SendPacket events) |
| `COMMITMENT_TYPE_ACK` | Acknowledgment commitment (for WriteAcknowledgement events) |
| `COMMITMENT_TYPE_RECEIPT` | Receipt commitment (for Timeout events - non-membership proof) |

<!-- [attestor.proto:L69](proto/link/attestor.proto#L69) -->

<!-- GEN:api:enum:CommitmentType END -->

#### `PacketAttestationResponse`

<!-- GEN:api:msg:PacketAttestationResponse START -->

| Field | Type | Description |
|---|---|---|
| `attestation` | `Attestation` | The signed attestation. See below. |

<!-- [attestor.proto:L51](proto/link/attestor.proto#L51) -->

<!-- GEN:api:msg:PacketAttestationResponse END -->

One request carries at most 100 packets, each at most 128 KB, and a request over either limit is refused. <!-- [service.go:L72-L76](link/internal/service/attestor/service.go#L72-L76) --> <!-- [service.go:L169-L180](link/internal/service/attestor/service.go#L169-L180) -->

### `LatestHeight`

<!-- GEN:api:rpc:LatestHeight START -->

Returns the latest height the attestor will generate attestations for.

<!-- [attestor.proto:L18](proto/link/attestor.proto#L18) -->

<!-- GEN:api:rpc:LatestHeight END -->

#### `LatestHeightRequest`

<!-- GEN:api:msg:LatestHeightRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` | Which attestor to ask, by its `name` in the `attestors` block. |

<!-- [attestor.proto:L55](proto/link/attestor.proto#L55) -->

<!-- GEN:api:msg:LatestHeightRequest END -->

#### `LatestHeightResponse`

<!-- GEN:api:msg:LatestHeightResponse START -->

| Field | Type | Description |
|---|---|---|
| `height` | `uint64` | The highest height this attestor will attest to. |

<!-- [attestor.proto:L57](proto/link/attestor.proto#L57) -->

<!-- GEN:api:msg:LatestHeightResponse END -->

Offset zero attests up to the chain's `finalized` tag. Above zero, the attestor reads `latest` and subtracts, which sits closer to the head. <!-- [local.go:L77-L104](link/internal/service/attestor/local.go#L77-L104) -->

### `Info`

<!-- GEN:api:rpc:Info START -->

Returns identity information about a configured attestor.

<!-- [attestor.proto:L21](proto/link/attestor.proto#L21) -->

<!-- GEN:api:rpc:Info END -->

#### `InfoRequest`

<!-- GEN:api:msg:InfoRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` | Which attestor to ask, by its `name` in the `attestors` block. |

<!-- [attestor.proto:L59](proto/link/attestor.proto#L59) -->

<!-- GEN:api:msg:InfoRequest END -->

#### `InfoResponse`

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

## Next steps

- [Configuration](/ibc-cli/configuration) for `server.listenAddr` and the `attestors` block these calls read.
- [CLI commands](/ibc-cli/cli-commands) for the commands that make these calls.
