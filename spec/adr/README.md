<!-- SPDX-License-Identifier: Apache-2.0 -->

# Architecture Decision Records

An architecture decision record (ADR) documents one design decision: the context, the decision, and its consequences. This directory is the single home for the design decisions behind the Interchain Standards and behind the implementations in [cosmos/ibc](https://github.com/cosmos/ibc), [cosmos/ibc-go](https://github.com/cosmos/ibc-go) and [cosmos/ibc-contracts](https://github.com/cosmos/ibc-contracts).

ADRs are not part of the Interchain Standards and are not normative. An ADR about an implementation, such as a storage layout or a contract hook, describes that implementation, not a requirement on others. Where an ADR and an ICS disagree, the ICS applies.

## When to write one

Write an ADR for a change to the architecture, a public interface, or the behaviour an implementation exposes to its users and counterparties. Bug fixes, internal refactors and documentation changes do not need one.

An ADR is optional for a change to a standard. The ICS and its History section remain the record of what changed; write an ADR when the reasoning behind a normative change needs more room than the pull request gives it. A new standard starts from the [ICS template](../ics-template.md) and the [standardisation process](../PROCESS.md) instead.

## Numbering

ADRs are numbered like the standards, in one series across every repository: `adr-NNN-short-title.md` in this directory, with a three-digit, zero-padded number. Take the next number after the highest one in the [index](#index) when you open the pull request. If another ADR merges with that number first, take the next free one before yours merges. Once an ADR has merged, its number never changes and is never reused. In text, refer to an ADR as `ADR NNN`, for example ADR 008.

ADRs 001 to 027 were written in ibc-go and keep their numbers; the numbers missing from that range are not assigned. ADRs 028 to 037 were moved here from other locations and renumbered by repository, ibc-contracts (028 to 035) and then this repository's end-to-end test harness (036 and 037), each in the order of its first commit (see [Moved ADRs](#moved-adrs)).

## Header

An ADR opens with its title and a list of four required header lines:

- `**Status**`: one of the statuses below.
- `**Date**`: the date the ADR was proposed, and optionally `**Last Updated**`.
- `**Scope**`: what the decision applies to, from the table below. List every scope it applies to.
- `**ICS**`: the standards the decision changes, implements or depends on, or empty if there are none. IBC v1 and IBC v2 reuse some ICS numbers for different documents (ICS-26 is the routing module in IBC v1 and application callbacks in IBC v2), so give each standard as `ICS-<number> (IBC v1)` for a document under `spec/core`, `spec/client`, `spec/app` or `spec/relayer`, or `ICS-<number> (IBC v2)` for a document under `spec/IBC_V2`, for example `ICS-26 (IBC v2)`. The qualifier says which tree the document lives in, not which protocol an implementation runs: an IBC v2 application that implements ICS-20 still cites `ICS-20 (IBC v1)`, because that is where ICS-20 is specified. Where a standard has more than one version, also give the version from the document's `version` field, for example `ICS-20 v1 (IBC v1)`.

| Scope | Covers |
| --- | --- |
| `spec` | The Interchain Standards in this repository |
| `ibc-go` | [cosmos/ibc-go](https://github.com/cosmos/ibc-go) |
| `ibc-contracts/solidity` | The Solidity contracts in [cosmos/ibc-contracts](https://github.com/cosmos/ibc-contracts) |
| `ibc-contracts/solana` | The Solana programs in cosmos/ibc-contracts |
| `ibc-contracts/proof-api` | The proof API in cosmos/ibc-contracts |
| `ibc/<component>` | A component of this repository, for example `ibc/e2e` or `ibc/cli` |

| Status | Meaning |
| --- | --- |
| Proposed | Under discussion; not yet decided |
| Accepted | Decided, not yet fully implemented |
| Rejected | Decided against; kept so the reasoning stays on record |
| Implemented | Decided and implemented |
| Deprecated | No longer applies, and nothing replaces it |
| Superseded | Replaced by a later ADR, named in a `**Superseded by**` line |

### Headers of moved ADRs

ADRs 001 to 037 were written before this directory existed and keep their original headers. Their scope and standards are recorded only in the [index](#index), and their status there is the status in the file mapped to the table above; the file's own wording, such as "Accepted and applied in v7", is authoritative. Several of them were merged while `Proposed`. The process below applies to ADRs added from 038 onwards.

## Process

1. Copy the [ADR template](adr-template.md) to `adr-NNN-short-title.md`, set the status to `Proposed`, and add a row to the [index](#index). Run `python3 docs/scripts/specgen.py` and add the new page to the "Architecture Decision Records" group in `docs/docs.json`. Open a pull request in this repository titled `ADR NNN: short title`.
2. For a decision scoped to `spec`, the changes to the standards follow in their own pull requests, one per standard as [CONTRIBUTING.md](../CONTRIBUTING.md#pull-requests) requires, each linking the ADR. For a decision about an implementation, the pull requests that implement it link the ADR.
3. Every ADR pull request needs the approvals that [CONTRIBUTING.md](../CONTRIBUTING.md#process-for-reviewing-prs) requires for any pull request to this repository, and also an approving review from a maintainer (someone with write access) of every repository in its scope other than this one. Whoever merges the pull request checks that these reviews are there.
4. Set the status to `Accepted` before the pull request merges. If the decision is not taken, either close the pull request or merge the ADR as `Rejected`.
5. Whoever merges the implementation opens a pull request here to set the status to `Implemented`, in the ADR and in the index. A later ADR that reverses or replaces a decision says so in its context, and its pull request sets the earlier ADR to `Superseded` and adds a `**Superseded by**` line. An ADR that stops applying without a replacement is set to `Deprecated`.

## Index

| ADR | Title | Scope | ICS | Status |
| --- | --- | --- | --- | --- |
| [001](adr-001-coin-source-tracing.md) | Coin Source Tracing | `ibc-go` | ICS-20 v1 (IBC v1) | Implemented |
| [002](adr-002-go-module-versioning.md) | Go module versioning | `ibc-go` | | Accepted |
| [003](adr-003-ics27-acknowledgement.md) | ICS27 Acknowledgement Format | `ibc-go` | ICS-27 (IBC v1) | Accepted |
| [004](adr-004-ics29-lock-fee-module.md) | Lock fee module upon escrow out of balance | `ibc-go` | ICS-29 (IBC v1) | Accepted |
| [005](adr-005-consensus-height-events.md) | UpdateClient Events - ClientState Consensus Heights | `ibc-go` | ICS-2 (IBC v1) | Accepted |
| [006](adr-006-02-client-refactor.md) | ICS-02 client refactor | `ibc-go` | ICS-2 (IBC v1) | Implemented |
| [007](adr-007-solomachine-signbytes.md) | Solo machine sign bytes | `ibc-go` | ICS-6 (IBC v1) | Implemented |
| [008](adr-008-app-caller-cbs.md) | Callback to IBC Actors | `ibc-go` | | Implemented |
| [009](adr-009-v6-ics27-msgserver.md) | ICS27 message server addition | `ibc-go` | ICS-27 (IBC v1) | Implemented |
| [010](adr-010-light-clients-as-sdk-modules.md) | IBC light clients as SDK modules | `ibc-go` | ICS-2 (IBC v1) | Proposed |
| [011](adr-011-transfer-total-escrow-state-entry.md) | ICS-20 transfer state entry for total amount of tokens in escrow | `ibc-go` | ICS-20 v1 (IBC v1) | Implemented |
| [015](adr-015-ibc-packet-receiver.md) | IBC Packet Receiver | `ibc-go` | ICS-26 (IBC v1) | Proposed |
| [025](adr-025-ibc-passive-channels.md) | IBC Passive Channels | `ibc-go` | ICS-4 (IBC v1) | Deprecated |
| [026](adr-026-ibc-client-recovery-mechanisms.md) | IBC Client Recovery Mechanisms | `ibc-go` | ICS-2 (IBC v1) | Accepted |
| [027](adr-027-ibc-wasm.md) | Add support for Wasm based light client | `ibc-go` | ICS-8 (IBC v1) | Implemented |
| [028](adr-028-solana-storage-architecture.md) | Solana IBC Storage Architecture | `ibc-contracts/solana` | ICS-24 (IBC v2), ICS-26 (IBC v2) | Implemented |
| [029](adr-029-solana-upgradability.md) | Solana IBC Contract Upgradability | `ibc-contracts/solana` | | Proposed |
| [030](adr-030-solana-ics27-gmp-architecture.md) | ICS27 General Message Passing (GMP) Architecture for Solana | `ibc-contracts/solana` | ICS-27 (IBC v2) | Implemented |
| [031](adr-031-solana-ift-architecture.md) | Interchain Fungible Token (IFT) Architecture for Solana | `ibc-contracts/solana` | ICS-27 (IBC v2) | Implemented |
| [032](adr-032-solana-attestation-light-client.md) | Solana Attestation Light Client | `ibc-contracts/solana` | ICS-2 (IBC v2) | Implemented |
| [033](adr-033-proof-api-version-specific-compatibility-modules.md) | Version-Specific Proof API Compatibility Modules | `ibc-contracts/proof-api` | | Implemented |
| [034](adr-034-solidity-ift-ratelimit.md) | Rate Limiting in IFT in Solidity | `ibc-contracts/solidity` | ICS-27 (IBC v2) | Accepted |
| [035](adr-035-solidity-pausability.md) | Pausing and Freezing in IBC-Solidity | `ibc-contracts/solidity` | ICS-26 (IBC v2) | Accepted |
| [036](adr-036-e2e-environments-as-owned-resource-graphs.md) | Model E2E environments as owned resource graphs | `ibc/e2e` | | Proposed |
| [037](adr-037-e2e-providers-from-test-requirements.md) | Select E2E providers from test requirements | `ibc/e2e` | | Proposed |

## Moved ADRs

These ADRs were moved here from the repository they were written in. Their history before the move is in that repository.

| ADR | Previously |
| --- | --- |
| 001 to 027 | cosmos/ibc-go `docs/architecture/`, same file names |
| 028 | cosmos/ibc-contracts `docs/adr/solana/solana-storage-architecture.md` |
| 029 | cosmos/ibc-contracts `docs/adr/solana/solana-upgradability.md` |
| 030 | cosmos/ibc-contracts `docs/adr/solana/solana-ics27-gmp-architecture.md` |
| 031 | cosmos/ibc-contracts `docs/adr/solana/solana-ift-architecture.md` |
| 032 | cosmos/ibc-contracts `docs/adr/solana/solana-attestation-light-client.md` |
| 033 | cosmos/ibc-contracts `docs/adr/proof-api/version-specific-compatibility-modules.md` |
| 034 | cosmos/ibc-contracts `docs/adr/solidity/ift-ratelimit.md` |
| 035 | cosmos/ibc-contracts `docs/adr/solidity/pausability.md` |
| 036 | cosmos/ibc `e2e/docs/adr/0001-model-e2e-environments-as-owned-resource-graphs.md` |
| 037 | cosmos/ibc `e2e/docs/adr/0002-select-e2e-providers-from-test-requirements.md` |
