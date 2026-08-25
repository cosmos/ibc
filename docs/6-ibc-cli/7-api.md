---
title: "API"
description: "The two gRPC services a running relayer and attestor serve, and how to call them."
---

The IBC CLI exposes two APIs: a relayer API and an attestation API.

The relayer's API has two main parts:

- `Relay` takes the transaction that sent the packets.
- `Packets` reports how far each of them got.

The attestor's API serves the attestations a light client verifies. A relayer is its usual caller, gathering proofs for a packet it is delivering. An operator calls it directly to check which attestor is answering and how far behind the head it will sign.

Both services listen on `server.listenAddr`, defaulting to `0.0.0.0:3000`.

To list running services:

```bash
grpcurl -plaintext localhost:3000 list
```

```
ibc.v2.relayer.RelayerApiService
```

## Relayer service

`ibc.v2.relayer.RelayerApiService`. `ibc relayer relay` and `ibc relayer packets` call these two.

### `Relay`

<!-- GEN:api:rpc:Relay START -->

Tracks the packets emitted by a source transaction and submits the transactions required to complete them.

<!-- [relayer.proto:L12](proto/cli/relayer.proto#L12) -->

<!-- GEN:api:rpc:Relay END -->

#### `RelayRequest`

<!-- GEN:api:msg:RelayRequest START -->

| Field | Type | Description |
|---|---|---|
| `tx_hash` | `string` | The transaction that sent the packets, on the source chain. |
| `source_chain_id` | `string` | The chain that transaction was sent on. |
| `selection` | oneof: `all_packets` or `selected_packets` | Required and controls only this relayer instance; IBC relaying remains permissionless. |

<!-- [relayer.proto:L18](proto/cli/relayer.proto#L18) -->

<!-- GEN:api:msg:RelayRequest END -->

Field names in these tables are the schema's. The JSON encoding uses lowerCamelCase, so `tx_hash` is sent as `txHash`.

`all_packets` takes every packet for which this relayer has a configured client and route. Packets without one are skipped, and the request succeeds even when that leaves nothing to relay. <!-- [relayer.proto:L30-L33](proto/link/relayer.proto#L30-L33) -->

#### `SelectedPackets`

<!-- GEN:api:msg:SelectedPackets START -->

| Field | Type | Description |
|---|---|---|
| `packets` | `PacketSelector[]` | The packets to relay. At least one. |

<!-- [relayer.proto:L37](proto/cli/relayer.proto#L37) -->

<!-- GEN:api:msg:SelectedPackets END -->

#### `PacketSelector`

<!-- GEN:api:msg:PacketSelector START -->

| Field | Type | Description |
|---|---|---|
| `source_client_id` | `string` | The client the packet was sent on. |
| `sequence_number` | `uint64` | The packet's number on that client. |

<!-- [relayer.proto:L41](proto/cli/relayer.proto#L41) -->

<!-- GEN:api:msg:PacketSelector END -->

A `selected_packets` request fails if any packet it names is absent or has no configured route. Naming a packet that is already selected, in flight, or finished succeeds and changes nothing. <!-- [relayer.proto:L35-L37](proto/link/relayer.proto#L35-L37) -->

`Relay` answers with every send packet in the transaction, including the ones it will not carry.

#### `RelayResponse`

<!-- GEN:api:msg:RelayResponse START -->

| Field | Type | Description |
|---|---|---|
| `packets` | `ObservedPacket[]` | Every send packet in the transaction, including those this relayer will not deliver. |

<!-- [relayer.proto:L46](proto/cli/relayer.proto#L46) -->

<!-- GEN:api:msg:RelayResponse END -->

#### `ObservedPacket`

<!-- GEN:api:msg:ObservedPacket START -->

| Field | Type | Description |
|---|---|---|
| `source_client_id` | `string` | The client the packet was sent on. |
| `sequence_number` | `uint64` | The packet's number on that client. |
| `selection` | `PacketSelection` | Whether this relayer took the packet. See the values below. |

<!-- [relayer.proto:L52](proto/cli/relayer.proto#L52) -->

<!-- GEN:api:msg:ObservedPacket END -->

#### `PacketSelection`

<!-- GEN:api:enum:PacketSelection START -->

| Value | Meaning |
|---|---|
| `PACKET_SELECTION_SELECTED` | Recorded for delivery by this relayer. Delivery happens afterwards and can still fail; query the packet's state to follow it. |
| `PACKET_SELECTION_NOT_SELECTED` | Configured and routed, but this request did not select it. |
| `PACKET_SELECTION_UNCONFIGURED` | No configured client or route, so this relayer skips it. |

<!-- [relayer.proto:L58](proto/cli/relayer.proto#L58) -->

<!-- GEN:api:enum:PacketSelection END -->

A successful call means the relayer recorded the request. `Packets` reports what happens after that.

```bash
grpcurl -plaintext -d '{"txHash":"0xSendTxHash","sourceChainId":"41001","allPackets":{}}' \
  localhost:3000 ibc.v2.relayer.RelayerApiService/Relay
```

### `Packets`

<!-- GEN:api:rpc:Packets START -->

Lists the packets this relayer is aware of, most recent first.

<!-- [relayer.proto:L15](proto/cli/relayer.proto#L15) -->

<!-- GEN:api:rpc:Packets END -->

#### `PacketsRequest`

<!-- GEN:api:msg:PacketsRequest START -->

| Field | Type | Description |
|---|---|---|
| `filter` | `PacketFilter` | Narrows the results. Every field is optional. |
| `limit` | `uint32` | Zero applies the default of 100; values above 1000 are capped. |
| `cursor` | `string` | Opaque next_cursor from a previous response. Empty starts at the newest packet. |

<!-- [relayer.proto:L69](proto/cli/relayer.proto#L69) -->

<!-- GEN:api:msg:PacketsRequest END -->

#### `PacketFilter`

Every field is optional, and a request with none returns everything the relayer has recorded.

<!-- GEN:api:msg:PacketFilter START -->

| Field | Type | Description |
|---|---|---|
| `source_chain_id` | `string` (optional) | Only packets sent from this chain. |
| `destination_chain_id` | `string` (optional) | Only packets bound for this chain. |
| `source_client_id` | `string` (optional) | Only packets sent on this client. |
| `destination_client_id` | `string` (optional) | Only packets received on this client. |
| `state` | `PacketState` (optional) | Only packets in this state. |
| `source_tx_hash` | `string` (optional) | Only packets sent by this transaction. |
| `sequence_number` | `uint64` (optional) | Only packets with this sequence number. |

<!-- [relayer.proto:L78](proto/cli/relayer.proto#L78) -->

<!-- GEN:api:msg:PacketFilter END -->

#### `PacketsResponse`

<!-- GEN:api:msg:PacketsResponse START -->

| Field | Type | Description |
|---|---|---|
| `packets` | `PacketStatus[]` | One entry per matching packet on this page, newest first. |
| `has_more` | `bool` | More `packets` match beyond this page. |
| `next_cursor` | `string` | Cursor for the next page, set only when `has_more`. |

<!-- [relayer.proto:L88](proto/cli/relayer.proto#L88) -->

<!-- GEN:api:msg:PacketsResponse END -->

Results are paged. Ask again with `next_cursor` while `has_more` is set.

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

<!-- [relayer.proto:L119](proto/cli/relayer.proto#L119) -->

<!-- GEN:api:msg:PacketStatus END -->

#### `TransactionInfo`

<!-- GEN:api:msg:TransactionInfo START -->

| Field | Type | Description |
|---|---|---|
| `tx_hash` | `string` | The transaction's hash. |
| `chain_id` | `string` | The chain it was submitted to. |

<!-- [relayer.proto:L114](proto/cli/relayer.proto#L114) -->

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

<!-- [relayer.proto:L96](proto/cli/relayer.proto#L96) -->

<!-- GEN:api:enum:PacketState END -->

`REJECTED` means the packet arrived and the application refused it, which is a completed relay and a failed application call. `RELAY_FAILED` means a permanent error stopped the relayer, wherever the packet had got to.

```bash
grpcurl -plaintext -d '{"filter":{"sourceChainId":"41001"},"limit":20}' \
  localhost:3000 ibc.v2.relayer.RelayerApiService/Packets
```

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

<!-- [attestor.proto:L80](proto/cli/attestor.proto#L80) -->

<!-- GEN:api:msg:Attestation END -->

`attested_data` is what was signed. A light client accepts the attestation when its threshold of attestors or more sign the same data. <!-- [resolve.go:L37-L60](link/internal/relay/proofgen/attestation/resolve.go#L37-L60) -->

### `StateAttestation`

<!-- GEN:api:rpc:StateAttestation START -->

Retrieves an attestation for a state at a given height.

<!-- [attestor.proto:L12](proto/cli/attestor.proto#L12) -->

<!-- GEN:api:rpc:StateAttestation END -->

#### `StateAttestationRequest`

<!-- GEN:api:msg:StateAttestationRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` | Which attestor to ask, by its `name` in the `attestors` block. |
| `height` | `uint64` | The height to attest to. |

<!-- [attestor.proto:L24](proto/cli/attestor.proto#L24) -->

<!-- GEN:api:msg:StateAttestationRequest END -->

#### `StateAttestationResponse`

<!-- GEN:api:msg:StateAttestationResponse START -->

| Field | Type | Description |
|---|---|---|
| `attestation` | `Attestation` | The signed attestation. See below. |

<!-- [attestor.proto:L29](proto/cli/attestor.proto#L29) -->

<!-- GEN:api:msg:StateAttestationResponse END -->

A height above what `LatestHeight` reports is refused, so ask for the height first. <!-- [local.go:L111-L117](link/internal/service/attestor/local.go#L111-L117) -->

### `PacketAttestation`

<!-- GEN:api:rpc:PacketAttestation START -->

Retrieves an attestation for a set of packets.

<!-- [attestor.proto:L15](proto/cli/attestor.proto#L15) -->

<!-- GEN:api:rpc:PacketAttestation END -->

#### `PacketAttestationRequest`

<!-- GEN:api:msg:PacketAttestationRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` | Which attestor to ask, by its `name` in the `attestors` block. |
| `packets` | `bytes[]` | The packets to attest to |
| `height` | `uint64` | The height to attest to the `packets` at |
| `commitment_type` | `CommitmentType` | The type of commitment to attest (packet or acknowledgment) Defaults to COMMITMENT_TYPE_PACKET if not specified (for backward compatibility) |

<!-- [attestor.proto:L35](proto/cli/attestor.proto#L35) -->

<!-- GEN:api:msg:PacketAttestationRequest END -->

#### `CommitmentType`

`commitment_type` selects what is being proven, which differs by direction. A delivery proves a packet commitment, an acknowledgement proves an acknowledgement commitment, and a timeout proves that no receipt exists.

<!-- GEN:api:enum:CommitmentType START -->

| Value | Meaning |
|---|---|
| `COMMITMENT_TYPE_PACKET` | Packet commitment (for SendPacket events) |
| `COMMITMENT_TYPE_ACK` | Acknowledgment commitment (for WriteAcknowledgement events) |
| `COMMITMENT_TYPE_RECEIPT` | Receipt commitment (for Timeout events - non-membership proof) |

<!-- [attestor.proto:L69](proto/cli/attestor.proto#L69) -->

<!-- GEN:api:enum:CommitmentType END -->

#### `PacketAttestationResponse`

<!-- GEN:api:msg:PacketAttestationResponse START -->

| Field | Type | Description |
|---|---|---|
| `attestation` | `Attestation` | The signed attestation. See below. |

<!-- [attestor.proto:L51](proto/cli/attestor.proto#L51) -->

<!-- GEN:api:msg:PacketAttestationResponse END -->

One request carries at most 100 packets, each at most 128 KB, and a request over either limit is refused. <!-- [service.go:L72-L76](link/internal/service/attestor/service.go#L72-L76) --> <!-- [service.go:L169-L180](link/internal/service/attestor/service.go#L169-L180) -->

### `LatestHeight`

<!-- GEN:api:rpc:LatestHeight START -->

Returns the latest height the attestor will generate attestations for.

<!-- [attestor.proto:L18](proto/cli/attestor.proto#L18) -->

<!-- GEN:api:rpc:LatestHeight END -->

#### `LatestHeightRequest`

<!-- GEN:api:msg:LatestHeightRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` | Which attestor to ask, by its `name` in the `attestors` block. |

<!-- [attestor.proto:L55](proto/cli/attestor.proto#L55) -->

<!-- GEN:api:msg:LatestHeightRequest END -->

#### `LatestHeightResponse`

<!-- GEN:api:msg:LatestHeightResponse START -->

| Field | Type | Description |
|---|---|---|
| `height` | `uint64` | The highest height this attestor will attest to. |

<!-- [attestor.proto:L57](proto/cli/attestor.proto#L57) -->

<!-- GEN:api:msg:LatestHeightResponse END -->

Offset zero attests up to the chain's `finalized` tag. Above zero, the attestor reads `latest` and subtracts, which sits closer to the head. <!-- [local.go:L77-L104](link/internal/service/attestor/local.go#L77-L104) -->

### `Info`

<!-- GEN:api:rpc:Info START -->

Returns identity information about a configured attestor.

<!-- [attestor.proto:L21](proto/cli/attestor.proto#L21) -->

<!-- GEN:api:rpc:Info END -->

#### `InfoRequest`

<!-- GEN:api:msg:InfoRequest START -->

| Field | Type | Description |
|---|---|---|
| `attestor` | `string` | Which attestor to ask, by its `name` in the `attestors` block. |

<!-- [attestor.proto:L59](proto/cli/attestor.proto#L59) -->

<!-- GEN:api:msg:InfoRequest END -->

#### `InfoResponse`

<!-- GEN:api:msg:InfoResponse START -->

| Field | Type | Description |
|---|---|---|
| `chain_id` | `string` | The chain this attestor watches. |
| `address` | `string` | The attestor's signing address. |

<!-- [attestor.proto:L61](proto/cli/attestor.proto#L61) -->

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

- [Configuration](5-configuration.md) for `server.listenAddr` and the `attestors` block these calls read.
- [CLI commands](6-cli-commands.md) for the commands that make these calls.
