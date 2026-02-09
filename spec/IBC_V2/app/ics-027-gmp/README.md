---
ics: 27
title: General Message Passing (GMP)
stage: Draft
category: IBC/APP
kind: instantiation
requires: 25, 26
author: IBC GMP contributors <ibc@interchain.io>
created: 2026-01-26
modified: 2026-01-26
---

## Synopsis

This specification defines ICS27-GMP (also known as ICS27-2), a general message passing protocol over IBC v2 that enables deterministic cross-chain contract execution. ICS27-GMP standardizes how a sender requests execution on a destination chain, how the destination derives the caller account, and how the call result is acknowledged back to the sender.

### Motivation

Interchain accounts (ICS-27) enabled cross-chain control of accounts but required a per-account channel handshake and channel lifecycle management. ICS27-GMP streamlines cross-chain execution by removing per-account handshakes and moving to deterministic account derivation over a single shared IBC port. It is intended for **smart contract calls** across heterogeneous execution environments (Cosmos SDK, EVM, Solana).

### Definitions

- **GMP account**: A deterministic account on the destination chain derived from `(client_id, sender, salt)`.
- **Caller account**: The GMP account that is authorized to execute the destination call.
- **Payload**: Opaque bytes interpreted by the destination chain runtime (e.g. protobuf-encoded SDK messages, ABI-encoded EVM call data).
- **Encoding**: A content type label describing how `GMPPacketData` and acknowledgements are serialized.

The IBC handler interface & IBC routing module interface are as defined in [ICS-25](../../core/ics-025-handler-interface) and [ICS-26](../../core/ics-026-routing-module), respectively.

### Desired Properties

- **Deterministic addressing**: Destination caller accounts are pre-computable from packet metadata without handshake.
- **Single-port operation**: All GMP packets are sent over a shared `gmpport` on each chain.
- **IBC v2 semantics**: No channel timeouts or channel closures are required for GMP.
- **Chain-agnostic payloads**: Payload bytes are opaque to the protocol and interpreted by destination runtimes.
- **Explicit acknowledgements**: Destination chains return a result blob for each call.

## Technical Specification

### Overview

ICS27-GMP defines a single IBC application module that can act as a sender and/or receiver of general message passing packets. A sender constructs `MsgSendCall`, which produces an IBC v2 packet containing `GMPPacketData`. The destination chain derives a caller account deterministically and executes the payload using its native execution environment. The destination then returns an acknowledgement containing the execution result.

### Identifiers

- **Port ID**: `gmpport` (fixed on all GMP-enabled chains).
- **Version**: `ics27-2`.

### Data Structures

#### `MsgSendCall`

```proto
message MsgSendCall {
  string source_client = 1;
  string sender = 2;
  string receiver = 3;
  bytes  salt = 4;
  bytes  payload = 5;
  uint64 timeout_timestamp = 6;
  string memo = 7;
  string encoding = 8;
}
```

- `source_client`: local client identifier used to send the packet.
- `sender`: address on the source chain (signer of the message).
- `receiver`: destination-specific receiver identifier. This may be an address (EVM), a program ID (Solana), or ignored if the destination interprets payload differently.
- `salt`: arbitrary bytes used to differentiate accounts for the same sender/client pair.
- `payload`: opaque call data interpreted by the destination runtime.
- `timeout_timestamp`: absolute nanoseconds since Unix epoch (IBC v2).
- `memo`: optional metadata; may include callback instructions for middleware.
- `encoding`: content type label for serialization (see Encodings).

#### `GMPPacketData`

```proto
message GMPPacketData {
  string sender = 1;
  string receiver = 2;
  bytes  salt = 3;
  bytes  payload = 4;
  string memo = 5;
}
```

#### `Acknowledgement`

```proto
message Acknowledgement {
  bytes result = 1;
}
```

The acknowledgement payload is opaque to the protocol; destination runtimes define the encoding of `result`.

### Encodings

ICS27-GMP supports multiple serialization encodings for `GMPPacketData` and acknowledgements. Payload encoding follows the per-packet `encoding` field defined by IBC v2 (see `spec/IBC_V2/core/ics-004-packet-semantics/PACKET.md`).

- `application/x-protobuf` (default)
- `application/json`
- `application/x-solidity-abi` (for EVM-compatible ABI encoding)

Implementations must validate that the requested encoding is supported before accepting a message.

### Deterministic Account Derivation

Each destination chain derives a GMP account deterministically from the triplet `(client_id, sender, salt)` using a chain-specific address derivation function. The account derivation must be collision resistant for distinct inputs and must produce a unique account for each unique triplet.

#### Account Identifier

```proto
message AccountIdentifier {
  string client_id = 1;
  string sender = 2;
  bytes  salt = 3;
}
```

Implementations must produce the same derived account for identical inputs and must reject invalid `client_id` values according to local rules. Because `client_id` is the client identifier of the chain deriving the account.

### Sender State Machine

#### `MsgSendCall`

1. Validate `source_client` and `sender`.
2. Validate size limits for `payload`, `memo`, `salt`, and `receiver` according to chain-specific constraints.
3. Validate `timeout_timestamp`.
4. Marshal `GMPPacketData` using the requested `encoding`.
5. Construct an IBC v2 packet with:
   - `source_client = MsgSendCall.source_client`
   - `payload.source_port = gmpport`
   - `payload.dest_port = gmpport`
   - `payload.version = ics27-2`
   - `payload.encoding = MsgSendCall.encoding`
   - `payload.value = Marshal(GMPPacketData)`
6. Send the packet using the v2 packet handler.

### Receiver State Machine

When a GMP packet is received on `gmpport`:

1. Unmarshal `GMPPacketData` according to `payload.encoding`.
2. Validate `GMPPacketData` size constraints.
3. Derive the GMP account from `(packet.dest_client, packet.sender, packet.salt)`, where `packet.dest_client` is the destination chain's client identifier tracking the source chain.
4. Execute the payload using the chain’s execution environment, attributing the call to the derived GMP account.
5. If call succeeds, return an acknowledgement containing the execution result.

### Acknowledgements

- Acknowledgements are mandatory and always contain a `result` byte array.
- The interpretation of `result` is chain-specific (e.g. ABI-encoded return data, protobuf-encoded transaction result).

### Middleware Interaction

ICS27-GMP is compatible with ICS-30 middleware. Implementations may expose packet metadata in `memo` for use by callbacks middleware. Source callback registration may be auto-derived from `sender` by GMP implementations.

### Packet Ordering

IBC v2 packet semantics apply; GMP does not require ordered channels and does not require per-account channels.

## Backwards Compatibility

ICS27-GMP is not backwards compatible with ICS-27 interchain accounts and does not reuse ICS-27 channel handshake or account registration flows. It is intended as a distinct version (`ics27-2`) for use with IBC v2.

## Forwards Compatibility

The protocol reserves the `encoding` field for future additions and allows destination runtimes to extend the `payload` schema without changes to the core GMP packet format.

## Example Implementations

- ibc-go GMP module: https://github.com/cosmos/ibc-go
- solidity-ibc-eureka GMP contracts: https://github.com/cosmos/solidity-ibc-eureka
- solana-ibc-eureka GMP program: https://github.com/cosmos/solidity-ibc-eureka

## Open Questions

- Confirm author list and maintainers for the specification.
- Confirm whether the default encoding should be `application/x-protobuf` or `application/x-solidity-abi` across implementations.

## History

- 2024-2025: Proof-of-concept implementations across Cosmos SDK, EVM, and Solana
- 2026-01-26: Initial draft of ICS27-GMP specification

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
