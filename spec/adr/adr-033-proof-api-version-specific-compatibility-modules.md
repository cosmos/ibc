<!-- SPDX-License-Identifier: Apache-2.0 -->

# ADR 033: Version-Specific Proof API Compatibility Modules

**Status**: Implemented
**Date**: Jun 17, 2026

## Context

Proof API modules generate chain-specific transactions from a stable gRPC API. Most proof API changes should be handled by evolving the current module implementation. Some changes, however, are incompatible with already-deployed on-chain light clients or already-released proof API logic.

The `eth_to_cosmos_compat` module is the first use of this pattern. It restores compatibility with the legacy Ethereum wasm light client v1.3.0 while keeping the current `eth_to_cosmos` module as the default implementation for current clients.

The compatibility problem has four important properties:

- The incompatible behavior is tied to a specific deployed light client version.
- The deployed client can be identified deterministically, such as by wasm checksum.
- The current public proof API request and response types are still usable, even if the implementation must delegate to older internal logic.
- The legacy behavior already exists in a historical repository revision and should be imported rather than reimplemented.

## Decision

Use a separate proof API module when backwards compatibility requires version-specific behavior that should not be embedded in the current module.

A version-specific compatibility module must import the exact historical proof API implementation that produced the deployed behavior. The compatibility module should be a thin router around two implementations:

- The current proof API module for normal behavior.
- The imported legacy proof API implementation for the specific deployed version.

For Ethereum to Cosmos v1.3.0 compatibility, the module is named `eth_to_cosmos_compat`. It delegates to the current `eth_to_cosmos` module by default and falls back to the legacy v1.3 implementation imported from git revision `035aa5eb171b6608614a9bc4e76e79d61c190e39` only when the destination client is the v1.3 wasm light client.

The v1.3.0 source revision predates the current `proof-api` crate layout, so this compatibility module imports the historical `relayer` crates that provided the equivalent service API at that revision. For future compatibility modules, import the legacy `proof-api` module crate directly when the target revision contains one.

## When To Use This Pattern

Use a version-specific compatibility module when all of the following are true:

- A released or deployed client depends on older proof generation behavior.
- The affected client version can be identified from request data or chain state.
- The legacy proof API implementation can be located at a specific repository revision, tag, or release commit.
- The compatibility logic is expected to be temporary, narrow, or limited to a small set of legacy versions.
- Keeping the logic in the current module would make the current implementation harder to reason about or test.
- Operators need an explicit configuration choice to opt into compatibility behavior.

Do not use this pattern for ordinary feature evolution, internal refactors, or changes where the current module can support old and new behavior cleanly without version-specific branching.

## Implementation Pattern

### 1. Find The Legacy Source Revision

Before writing compatibility code, find the exact git revision that produced the deployed behavior.

Use the deployed artifact version, release tag, checksum, or historical deployment metadata to identify the source revision. The selected revision must be stable enough to use as a Cargo `rev` dependency and should be recorded in the compatibility module dependencies and in this ADR or a follow-up ADR.

For Ethereum wasm light client v1.3.0, the imported revision is:

```text
035aa5eb171b6608614a9bc4e76e79d61c190e39
```

This revision contains the legacy Ethereum to Cosmos behavior needed by clients using checksum `af84cccca3e746d9c4ea980c6d1b4511de0fa962ed5003dee8cb44eda10e4568`.

Do not infer compatibility from the latest commit on a branch. Import the exact revision that matches the deployed client behavior.

### 2. Import The Legacy Proof API Module

The compatibility module must depend on the legacy implementation from the selected revision.

Use dependency aliases that include the legacy version so imports are explicit:

```toml
proof-api-eth-to-cosmos-vX_Y = { package = "proof-api-eth-to-cosmos", git = "https://github.com/cosmos/ibc-contracts", rev = "<exact-legacy-rev>", default-features = false }
```

If the target revision predates the current `proof-api` crate layout, import the legacy crate that exposed the equivalent service boundary. The v1.3.0 compatibility module uses the historical relayer crates:

```toml
ibc-eureka-relayer-core-v1_3 = { package = "ibc-eureka-relayer-core", git = "https://github.com/cosmos/ibc-contracts", rev = "035aa5eb171b6608614a9bc4e76e79d61c190e39", default-features = false }
ibc-eureka-relayer-eth-to-cosmos-v1_3 = { package = "ibc-eureka-relayer-eth-to-cosmos", git = "https://github.com/cosmos/ibc-contracts", rev = "035aa5eb171b6608614a9bc4e76e79d61c190e39", default-features = false }
```

Avoid floating branches, broad version ranges, or dependencies whose behavior can change without a deliberate compatibility update.

### 3. Create A Separate Module Crate

Add a new proof API module crate under `packages/proof-api/modules`.

The crate should implement `ProofApiModule` and expose a distinct module name, for example:

```rust
impl ProofApiModule for EthToCosmosCompatProofApiModule {
    fn name(&self) -> &'static str {
        "eth_to_cosmos_compat"
    }
}
```

The module name is an operator-facing compatibility contract. Use a name that makes compatibility behavior explicit.

### 4. Register Both Implementations

Register the compatibility module in the proof API binary alongside the current module:

```rust
proof_api_builder.add_module(EthToCosmosProofApiModule);
proof_api_builder.add_module(EthToCosmosCompatProofApiModule);
```

This keeps the current module available for normal deployments and requires operators or tests to opt into the compatibility module by configuration.

### 5. Identify The Legacy Version Deterministically

Route to legacy behavior only after checking a deterministic version signal.

For wasm light clients, prefer the wasm checksum. For example, Ethereum wasm light client v1.3.0 is identified by its checksum:

```rust
const V1_3_CHECKSUM: &[u8] = &[/* 32 bytes */];
```

For `create_client`, the checksum can come from request parameters. For operations against an existing client, query the destination chain client state and decode the checksum from that state.

The compatibility module should default to the current implementation when the version signal does not match a legacy version.

### 6. Translate At The Boundary

Keep translation between current and legacy API types at the compatibility boundary.

The compatibility module should convert current proof API requests into legacy service requests, call the legacy service, and convert the legacy response back into the current proof API response.

This keeps compatibility concerns out of the current module and prevents legacy types from leaking into callers.

### 7. Map Configuration Explicitly

If legacy configuration differs from current configuration, map fields explicitly and reject unsupported modes.

For example, `eth_to_cosmos_compat` maps current transaction builder modes to the legacy `mock` flag:

```rust
let mock = match &eth_to_cosmos_config.mode {
    TxBuilderMode::Real => false,
    TxBuilderMode::Mock => true,
    TxBuilderMode::Attested(_) => {
        anyhow::bail!("eth_to_cosmos_compat does not support attested mode")
    }
};
```

Do not silently approximate unsupported behavior. Failing during service creation is preferable to producing invalid transactions later.

## What To Avoid

Avoid adding legacy branches to the current module unless the compatibility behavior is tiny and clearly permanent.

Avoid reimplementing the legacy proof generation behavior from memory or by copying selected code into the current tree. Import the historical module from the exact revision and write only the adapter code needed to connect it to the current proof API boundary.

Avoid making compatibility automatic at the proof API server level. Operators should select the compatibility module intentionally through module configuration.

Avoid guessing the client version from chain names, test names, or environment variables. Use on-chain state, request parameters, checksums, code IDs, or another deterministic version signal.

Avoid changing public request or response schemas only to support a legacy implementation. Prefer boundary translation inside the compatibility module.

Avoid supporting incompatible modes by best effort. Reject unsupported modes early with clear errors.

Avoid using a compatibility module as a long-term fork. Once legacy deployments are retired, remove the compatibility module and its pinned dependencies.

## Consequences

This pattern keeps current proof API modules simple while allowing deployed clients to keep working. It makes compatibility opt-in and auditable through configuration and exact legacy dependency pins.

The tradeoff is that compatibility modules add dependency weight and maintenance surface. Each module should therefore be narrowly scoped to a known legacy version or version family and should contain clear version checks.

## References

- `packages/proof-api/modules/eth-to-cosmos-compat/src/lib.rs`
- `programs/proof-api/src/bin/proof-api.rs`
- `programs/proof-api/config.example.json`
