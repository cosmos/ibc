<!-- SPDX-License-Identifier: Apache-2.0 -->

# ADR 028: Solana IBC Storage Architecture

- **Status**: Implemented
- **Date**: Jun 17, 2025
- **Last Updated**: Mar 23, 2026
- **Scope**: `ibc-contracts/solana`
- **ICS**: ICS-24 (IBC v2), ICS-26 (IBC v2)

## Context

The Solana IBC implementation requires efficient storage mechanisms for both light client consensus states and packet lifecycle data. Unlike EVM chains where storage is managed within contract state, Solana's account model requires explicit decisions about data distribution across Program Derived Addresses (PDAs). Solana programs cannot use standard Rust collections like `BTreeMap` or `BTreeSet`, and the idiomatic approach is to use PDAs as a key-value store.

## Decision

We implement a PDA-based storage architecture with chunked storage for large data that exceeds Solana's transaction size limits.

### Key Design Principles

1. **PDA-Based Storage**: All state stored in Program Derived Addresses
2. **Chunked Uploads**: Large data (headers, proofs, payloads) split into ~900-byte chunks
3. **Rent Reclamation**: Temporary accounts (chunks, commitments) closed after use
4. **Access Control**: Centralized access-manager program for role-based permissions

## Storage Architecture

### 1. ICS07-Tendermint Light Client Storage

**Client State PDA:**

```text
Seeds: [b"client", chain_id.as_bytes()]
```

Stores:

- `chain_id`: String (max 64 bytes)
- `trust_level_numerator`, `trust_level_denominator`: u64
- `trusting_period`, `unbonding_period`, `max_clock_drift`: u64
- `frozen_height`, `latest_height`: `IbcHeight { revision_number, revision_height }`

**Consensus State PDAs:**

```text
Seeds: [b"consensus_state", client_state_pubkey.as_ref(), height.to_le_bytes()]
```

Stores:

- `height`: u64
- `consensus_state`: ConsensusState
  - `timestamp`: u64 (nanoseconds since Unix epoch)
  - `root`: [u8; 32] (commitment root)
  - `next_validators_hash`: [u8; 32]

**App State PDA:**

```text
Seeds: [b"app_state"]
```

Stores:

- `access_manager`: Pubkey
- `_reserved`: [u8; 256]

### 2. ICS26-Router Storage

**Router State PDA:**

```text
Seeds: [b"router_state"]
```

Stores:

- `version`: AccountVersion
- `access_manager`: Pubkey
- `_reserved`: [u8; 256]

**Client Registry PDA:**

```text
Seeds: [b"client", client_id.as_bytes()]
```

Stores:

- `version`: AccountVersion
- `client_id`: String (max 64 bytes)
- `client_program_id`: Pubkey (light client program)
- `counterparty_info`: `CounterpartyInfo { client_id, merkle_prefix }`
- `active`: bool
- `_reserved`: [u8; 256]

**IBC App Registry PDA:**

```text
Seeds: [b"ibc_app", port_id.as_bytes()]
```

Stores:

- `version`: AccountVersion
- `port_id`: String (max 128 bytes)
- `app_program_id`: Pubkey
- `authority`: Pubkey
- `_reserved`: [u8; 256]

### 3. Packet Commitment Storage

All packet commitments are 32-byte SHA256 hashes stored in Commitment PDAs.

**Packet Commitment (for sent packets):**

```text
Seeds: [b"packet_commitment", source_client.as_bytes(), sequence.to_le_bytes()]
```

**Packet Receipt (for received packets):**

```text
Seeds: [b"packet_receipt", dest_client.as_bytes(), sequence.to_le_bytes()]
```

**Packet Acknowledgement:**

```text
Seeds: [b"packet_ack", dest_client.as_bytes(), sequence.to_le_bytes()]
```

### 4. Chunked Storage Pattern

Large data exceeding transaction limits is split into 900-byte chunks uploaded in separate transactions, then assembled. Chunking is split across two programs:

- **ICS07-Tendermint**: Header chunks, misbehaviour chunks, and signature verification PDAs (light client data)
- **ICS26-Router**: Payload chunks and proof chunks (packet lifecycle data)

**Header Chunk (for client updates) — ICS07-Tendermint:**

```text
Seeds: [b"header_chunk", submitter.as_ref(), chain_id.as_bytes(), target_height.to_le_bytes(), chunk_index]
```

Stores:

- `submitter`: Pubkey
- `chunk_data`: `Vec<u8>` (max 900 bytes)

**Payload Chunk (for large packet payloads) — ICS26-Router:**

```text
Seeds: [b"payload_chunk", payer.as_ref(), client_id.as_bytes(), sequence.to_le_bytes(), payload_index, chunk_index]
```

Stores:

- `client_id`: String
- `sequence`: u64
- `payload_index`: u8 (for multi-payload packets)
- `chunk_index`: u8
- `chunk_data`: `Vec<u8>` (max 900 bytes)

**Proof Chunk (for membership/verification proofs) — ICS26-Router:**

```text
Seeds: [b"proof_chunk", payer.as_ref(), client_id.as_bytes(), sequence.to_le_bytes(), chunk_index]
```

Stores:

- `client_id`: String
- `sequence`: u64
- `chunk_index`: u8
- `chunk_data`: `Vec<u8>` (max 900 bytes)

**Misbehaviour Chunk (for misbehaviour reports) — ICS07-Tendermint:**

```text
Seeds: [b"misbehaviour_chunk", submitter.as_ref(), client_id.as_bytes(), chunk_index]
```

Stores:

- `chunk_data`: `Vec<u8>` (max 900 bytes)

**Signature Verification (cached verification results) — ICS07-Tendermint:**

```text
Seeds: [b"sig_verify", signature_hash]
```

Stores:

- `submitter`: Pubkey
- `is_valid`: bool

**Inline vs Chunked Mode (ICS26-Router payloads and proofs):**

Payloads and proofs can be sent in two modes:

- **Inline**: Data fits in a single transaction and is included directly in the instruction data (`packet.payloads` non-empty). No chunk accounts needed.
- **Chunked**: Data exceeds transaction limits and is uploaded via separate chunk transactions first, then assembled during `recv_packet`/`ack_packet`/`timeout_packet` (`packet.payloads` empty, metadata with `total_chunks > 0` provided instead).

The router's `validate_and_reconstruct_packet` handles both modes transparently. Inline and chunked modes are mutually exclusive per packet — providing both is an error.

Note: Header and misbehaviour uploads in ICS07-Tendermint are always chunked (Tendermint headers always exceed transaction size limits).

### 5. Chunked Upload Workflow

```text
1. Upload Phase:
   - Relayer uploads chunks in separate transactions
   - Each chunk stored in deterministic PDA
   - Per-submitter PDAs enable cleanup of failed uploads

2. Assembly Phase:
   - Single transaction reads all chunk accounts
   - Assembles full data (header/proof/payload)
   - Triggers verification/processing

3. Cleanup Phase:
   - Chunk accounts closed after successful assembly
   - Rent (~0.01 SOL per chunk) reclaimed to payer
```

### 6. Access Control Integration

The `access-manager` program provides centralized role-based access control.

**Access Manager PDA:**

```text
Seeds: [b"access_manager"]
```

Stores:

- `roles`: `Vec<RoleData>` (max 16 entries)
  - Each `RoleData`: `{ role_id: u64, members: Vec<Pubkey> }`

**Upgrade Authority PDA:**

```text
Seeds: [b"upgrade_authority", target_program.as_ref()]
```

Programs reference `access_manager` pubkey in their state to validate permissions via CPI.

## Storage Lifecycle

### Packet Lifecycle

```text
1. Send:
   - Create PacketCommitment PDA (caller-chosen sequence; create_account fails on duplicates)
   - Emit event

2. Receive:
   - Create PacketReceipt PDA (prevents replay)
   - Execute app callback
   - Create PacketAck PDA

3. Acknowledge:
   - Verify ack proof against commitment
   - Zero PacketCommitment PDA (PDA persists to prevent sequence reuse)

4. Timeout:
   - Verify non-receipt on destination
   - Zero PacketCommitment PDA (PDA persists to prevent sequence reuse)
```

### Client Update Lifecycle

```text
1. Upload header chunks (multiple transactions)
2. Assemble and verify header (single transaction)
3. Create/update ConsensusState PDA
4. Close header chunk accounts
5. Reclaim rent
```

## Cost Analysis

**Account Rent Costs (approximate):**

```text
Per account rent: ~0.01 SOL (refundable when account closed)
```

**Real-World Client Update Costs (Measured on Mainnet):**

| Chain        | Validators | 2/3 Threshold | Prep Txs    | Total CUs | Total Cost (SOL) | USD Cost      | Latency |
| ------------ | ---------- | ------------- | ----------- | --------- | ---------------- | ------------- | ------- |
| **Noble**    | 20         | ~14 sigs      | 19 parallel | ~548k     | 0.000166         | ~$0.025-0.033 | ~18s    |
| **Celestia** | 100        | ~67 sigs      | 83 parallel | ~2.16M    | 0.000721         | ~$0.11-0.14   | ~22s    |

*Costs at $150-200/SOL. Latency depends on RPC throttling.*

**Cost Breakdown:**

- Transaction base fees: 5,000 lamports/tx
- Priority fees: variable (market-driven)
- Chunk rent: ~0.01 SOL per chunk (refunded after assembly)
- Consensus state rent: ~0.01 SOL (permanent)

**Per-Packet Cost:**

```text
- Commitment creation: ~0.002 SOL (permanently locked — PDA persists after zeroing for replay protection)
- Transaction fees: ~0.000005 SOL
- Net cost per packet: ~0.002 SOL
```

**Key Insights:**

- Cost scales roughly linearly with validator count (~5x validators = ~4x cost)
- Chunk rent is fully refundable; commitment rent is permanently locked (replay protection trade-off)
- Relayers must call cleanup instructions to reclaim chunk rent

## Security Considerations

1. **PDA Determinism**: Consistent seeds prevent duplicate accounts
2. **Authority Checks**: Only authorized parties can modify state
3. **Chunk Ownership**: Per-submitter PDAs prevent interference
4. **Access Control**: Role-based permissions via access-manager
5. **Commitment Integrity**: Only router can create commitment PDAs; after ack/timeout the commitment is zeroed but the PDA persists, preventing sequence reuse via Anchor's `init` constraint

## Byte Encoding and Sequence Calculation

### Caller-Chosen Sequence

Callers provide a `u64` sequence in `MsgSendPacket.sequence`. There is no on-chain sequence counter — uniqueness is enforced by `create_account` failing if the `PacketCommitment` PDA already exists.

**PDA Seeds:**

```text
PacketCommitment: [b"packet_commitment", client_id, sequence]
```

- `client_id` — IBC client identifier
- `sequence` — caller-chosen u64 (little-endian)

**Why caller-chosen?** A shared on-chain counter is a write-lock bottleneck. Caller-chosen u64 sequences eliminate contention with negligible collision probability.

**Collision handling**: If a caller picks a sequence that already has a commitment PDA, `create_account` fails and the transaction reverts. Callers should use unique u64 values to avoid this.

### PDA Seed Encoding (Little-Endian)

All numeric values in PDA seeds use **little-endian** encoding (`to_le_bytes()`).

**Example**: sequence `1` → `[0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]`

### IBC Commitment Path Encoding (Big-Endian)

IBC commitment paths (for cross-chain proofs) use **big-endian** per IBC spec:

```text
// Packet commitment path: sourceClient + 0x01 + sequence (big-endian)
// Receipt path: destClient + 0x02 + sequence (big-endian)
// Ack path: destClient + 0x03 + sequence (big-endian)
```

**Example**: sequence `1` → `[0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01]`

### Sequence Management

- Callers provide a u64 in `MsgSendPacket.sequence`; uniqueness enforced by PDA creation

## Configuration Constants

- **CHUNK_DATA_SIZE**: 900 bytes (maximum chunk size for uploads)
- **MAX_CLIENT_ID_LENGTH**: 64 bytes
- **MAX_PORT_ID_LENGTH**: 128 bytes

## Future Considerations

The following features are not currently implemented but may be added:

1. **Consensus State Pruning**: Rolling window to limit storage growth
2. **Archival System**: Off-chain storage for historical packet data
3. **Batch Operations**: Batching PDA closures for efficiency

## References

- [IBC Packet Lifecycle](https://github.com/cosmos/ibc/tree/main/spec/ics/core/ics-004-channel-and-packet-semantics)
- [Solana Account Model](https://solana.com/docs/core/accounts)
- [Solana PDAs](https://solana.com/docs/core/pda)
- [Solana Rent Economics](https://solana.com/docs/core/fees)
