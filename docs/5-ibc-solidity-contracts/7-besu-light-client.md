---
title: "BesuLightClient"
description: "Contract reference for the Besu light client: what it verifies, its message formats, functions, events, errors, permissions, and what the constructor sets."
---

BesuLightClient lets this chain verify a counterparty chain that runs Besu with QBFT or IBFT 2.0 consensus. It accepts a Besu block header only after checking its signatures against a validator set it already trusts. It then checks packet commitments with Ethereum storage proofs against that header's state root.

One instance is deployed per client and registered on the ICS26Router under a client ID. The router calls it to update the client, and to check the proof for every packet it receives, acknowledges, or times out. The same contract serves both consensus protocols, and it does not record which one the counterparty runs.

Source: [BesuLightClient.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/light-clients/besu/BesuLightClient.sol)

Interacts with: [ICS26Router](2-ics26-router.md) on this chain, and the counterparty chain's ICS26Router, whose storage it proves. The [Besu light client page](../4-light-clients/3-besu-light-client.md) covers the trust model, deployment, and relaying.

## Update and proof formats

An update decodes as `MsgUpdateClient`, and `verifyMembership` and `verifyNonMembership` decode their proof field as `MembershipProof`. The client state holds the trust settings `trustingPeriod`, `maxClockDrift`, and the fraction `trustLevel`. It also records the counterparty router `ibcRouter`, the highest accepted height `latestHeight`, and the `isFrozen` flag. A consensus state holds one header's timestamp, state root, and validator list. The client state and consensus state structs are shown in [what the client trusts](../4-light-clients/3-besu-light-client.md#what-the-client-trusts).

```solidity
struct MsgUpdateClient {
    bytes headerRlp;
    bytes[] commitSeals;
    IICS02ClientMsgs.Height trustedHeight;
    ConsensusState consensusStatePreimage;
}

struct MembershipProof {
    ConsensusState consensusStatePreimage;
    bytes[] accountProofNodes;
    bytes[] proofNodes;
}
```

The fields hold the header, its signatures, the trusted state to check them against, and the two Merkle proofs.

| Field | What it holds |
|---|---|
| `headerRlp` | The RLP-encoded header in the form its validators signed, with the commit seals removed from `extraData`. That leaves four `extraData` items for IBFT 2.0, or five for QBFT, the last of which is an empty list |
| `commitSeals` | 65-byte signatures over `keccak256(headerRlp)` with recovery ID 27 or 28, in strictly ascending order of signer address |
| `trustedHeight`, `consensusStatePreimage` | An already trusted height, and the full consensus state stored there |
| `accountProofNodes` | The state-trie proof of the counterparty router's account. Empty when an earlier call in the same transaction proved that height |
| `proofNodes` | The storage-trie proof of the commitment's slot |

The client stores only a hash of each consensus state. Every update and proof therefore includes the full consensus state it relies on. The client checks that state against the stored hash.

## Header checks

An update is accepted only when the header passes every check below.

| Check | Why it is there |
|---|---|
| The header decodes in the signing form described above. Its ommers hash, difficulty, mix hash, and nonce hold Besu BFT's fixed values. Its height and timestamp are nonzero and fit in 64 bits | Rejects malformed input before any signature is recovered |
| The state root is neither zero nor the empty-trie root | No account proof can verify against an empty-trie root |
| The validator list is nonempty, strictly ascending, and free of the zero address | The overlap and quorum counts rely on sorted lists without duplicates |
| The header is above the trusted height | |
| The trusted consensus state matches its stored hash and is younger than `trustingPeriod` | Limits how long an old validator set can still sign headers the client accepts |
| Each commit seal is 65 bytes and recovers to a signer above the one before it | No signer is counted twice |
| Trusted overlap: at least `max(min(ceil(n * trustLevel), floor(2n / 3)), floor(n / 3) + 1)` of the `n` trusted validators signed | Validators the client never trusted cannot get a header accepted on their own |
| Quorum: every signer is in the header's validator list, and at least `ceil(2n / 3)` of its `n` validators signed | Fewer than two thirds of the header's own validators cannot get it accepted |

The overlap comparison is inclusive. The requirement is capped at `floor(2n / 3)`, so that one validator added or removed by a vote cannot stall updates. The `floor(n / 3) + 1` term takes precedence over the cap, so more than a third of the trusted validators must always sign. The [Besu light client page](../4-light-clients/3-besu-light-client.md#what-an-update-proves) explains why both rules exist.

An update also rejects a header more than `maxClockDrift` seconds ahead of this chain's block time. Misbehaviour evidence skips this check, so a header from the future still counts as evidence.

## Interface

This client implements the five functions of the standard light client interface, `ILightClient`: `updateClient`, `verifyMembership`, `verifyNonMembership`, `misbehaviour`, and `getClientState`. The [attestation light client page](5-attestation-light-client.md#interface) shows the interface.

BesuLightClient adds `getConsensusStateHash`. It also inherits the bound router and the `pause` and `unpause` functions from `ICS26ManagedLightClient`, described in [Permissions and pausing](#permissions-and-pausing).

### Updates and misbehaviour

Two `ILightClient` functions write the client's persistent storage: `updateClient` and `misbehaviour`.

| Function | What it does |
|---|---|
| `updateClient(bytes updateMsg) returns (UpdateResult)` | Verifies a header against a trusted consensus state, and stores the hash of the header's consensus state at its height |
| `misbehaviour(bytes misbehaviourMsg)` | Freezes the client if the evidence proves one of the conflicts in [Events](#events), and reverts `NoMisbehaviourDetected` when valid evidence shows no conflict |

An update returns one of three results.

| Result | When it is returned |
|---|---|
| `Update` | The header passed, and no consensus state was stored at its height. Its hash is now stored |
| `NoOp` | The header does not conflict with its trusted consensus state, and the header's consensus state is already stored at its height |
| `Misbehaviour` | The header conflicts with its trusted consensus state, or with a different one stored at its height. The client is now frozen |

An update may be for a height below `latestHeight`, as long as it is above its trusted height.

### Membership proofs

Both verify functions check a proof about one slot in the counterparty router's storage. Each returns the trusted timestamp at the proof height.

| Function | What it does |
|---|---|
| `verifyMembership(MsgVerifyMembership) returns (uint256)` | Proves the router stores the given 32-byte commitment at the path |
| `verifyNonMembership(MsgVerifyNonMembership) returns (uint256)` | Proves the path's slot is absent from the router's storage |

Each verify call checks its path, which must have exactly three elements.

| Path element | What the client requires |
|---|---|
| `path[0]` | The 20-byte address of the counterparty router, equal to the client state's `ibcRouter` |
| `path[1]` | The 32-byte constant `IBCSTORE_STORAGE_SLOT` |
| `path[2]` | The commitment path |

The client checks the counterparty router's account proof against the consensus state's state root. The proven account holds the router's storage root. It then checks the storage proof at the slot `keccak256(abi.encode(keccak256(path[2]), IBCSTORE_STORAGE_SLOT))`. That constant is the base slot of the router's commitment mapping in `IBCStoreUpgradeable`.

The proven storage root is cached for the rest of the transaction, keyed by router and height. A later call at that height may then send an empty account proof. Neither function is a view function, because both write that cache.

### Misbehaviour evidence

Anyone can submit evidence through the router's `submitMisbehaviour`, which forwards it to `misbehaviour`. The evidence is an ABI-encoded `MsgSubmitMisbehaviour`.

```solidity
struct MsgSubmitMisbehaviour {
    MisbehaviourType misbehaviourType;
    bytes misbehaviour;
}
```

The type selects one of two payloads, each ABI-encoded into the `misbehaviour` field.

```solidity
struct MsgConflictingConsensusStatesMisbehaviour {
    IICS02ClientMsgs.Height height1;
    IICS02ClientMsgs.Height height2;
    ConsensusState consensusStatePreimage1;
    ConsensusState consensusStatePreimage2;
}

struct MsgConflictingHeadersMisbehaviour {
    MsgUpdateClient update1;
    MsgUpdateClient update2;
}
```

| `misbehaviourType` | Payload | What the client checks |
|---|---|---|
| `ConflictingConsensusStates` | `MsgConflictingConsensusStatesMisbehaviour`: two heights and the consensus states stored there | The two stored states conflict. States past the trusting period are still accepted as evidence |
| `ConflictingHeaders` | `MsgConflictingHeadersMisbehaviour`: two `MsgUpdateClient` messages | If the first header conflicts with its trusted state or with the state stored at its height, the client freezes. Otherwise the second header is checked the same way, then the two headers against each other. Each checked header must pass the update checks except clock drift |

### Views

The getters expose the client state and stored hashes, and they still work after a freeze.

| Function | What it returns |
|---|---|
| `getClientState() returns (bytes)` | The ABI-encoded `ClientState` |
| `getConsensusStateHash(uint64 revisionHeight) returns (bytes32)` | The stored consensus state hash at one height. Reverts `ConsensusStateNotFound` when none is stored |
| `ICS26_ROUTER() returns (address)` | The router the client is bound to |
| `paused() returns (bool)` | Whether the client is paused |

The contract cannot return a full consensus state. A caller rebuilds it from the counterparty header at that height: its timestamp, state root, and validator list.

## State and storage

The client stores its client state, one consensus state hash per trusted height, and the pause flag. It also caches proven storage roots for the length of one transaction.

| What is stored | When it is written |
|---|---|
| The client state | At construction. An update to a higher height advances `latestHeight`, and a freeze sets `isFrozen` |
| A consensus state hash per height, `keccak256(abi.encode(ConsensusState))` | At construction for the first height, then on every update that returns `Update` |
| The router's proven storage root per height, in transient storage | On each verify call that sends an account proof. It is cleared when the transaction ends |
| The pause flag | On every `pause` or `unpause` |

The bound router is fixed in the contract's code as an immutable value. The contract has no delete path, so a stored hash stays for the life of the deployment.

## Events

The client emits one event each time it freezes, naming the conflict.

| Event | Conflict |
|---|---|
| `DoubleSign(uint64 indexed revisionHeight, bytes32 consensusStateHash1, bytes32 consensusStateHash2)` | Two different consensus states at one height |
| `TimeNonMonotonicity(uint64 indexed height2, uint64 height1, uint64 timestamp2, uint64 timestamp1)` | The lower height's timestamp is equal to or above the higher height's |
| `SubSecondBlockTime(uint64 indexed height2, uint64 height1, uint64 timestamp2, uint64 timestamp1)` | Two timestamps differ by fewer seconds than their heights differ in blocks |
| `InvalidValidatorSetChange(uint64 indexed height2, uint64 height1, uint256 changes)` | Two validator sets differ by more validators than their heights do |

The only other events the client emits are OpenZeppelin Pausable's `Paused` and `Unpaused`. The client emits no event for a successful update. The router emits `ICS02ClientUpdated` for every update it forwards.

## What freezes the client

The client freezes when an update or misbehaviour evidence shows two consensus states that conflict in one of the four ways listed in [Events](#events). The [Besu light client page](../4-light-clients/3-besu-light-client.md#misbehaviour) explains what each conflict means.

A frozen client rejects `updateClient`, both verify calls, and `misbehaviour` with `FrozenClientState`. The getters, `pause`, and `unpause` keep working. The contract has no unfreeze function, so replacing the client is the only recovery, as [Deployment, registration, and replacement](#deployment-registration-and-replacement) describes.

## Errors

The table lists every error the client declares, plus the three router-binding errors it inherits.

| Error | When it is thrown |
|---|---|
| `LightClientZeroICS26Router` | The constructor gets the zero address as its router |
| `InvalidIbcRouter` | The initial client state has a zero counterparty router |
| `InvalidTrustingPeriod(trustingPeriod)` | The trusting period is zero or above 30 days |
| `InvalidMaxClockDrift(maxClockDrift)` | The maximum clock drift is above 12 seconds |
| `InvalidTrustLevel(numerator, denominator)` | The trust level has a zero denominator, or lies outside one third to two thirds |
| `LightClientUnauthorizedCaller(caller)` | A gated function is called by an address other than the bound router |
| `LightClientUnauthorizedRole(caller, roleId)` | `pause` or `unpause` is called by an address that lacks the matching role in the router's access manager, or holds it with an execution delay |
| `FrozenClientState` | The client is frozen, or the initial client state is marked frozen |
| `InvalidRevisionNumber(providedRevisionNumber)` | A height has a nonzero revision number |
| `InvalidHeaderFormat(itemsLength)` | The header has fewer than 15 RLP items |
| `InvalidOmmersHash(actualOmmersHash)`, `InvalidDifficulty(actualDifficulty)`, `InvalidMixHash(actualMixHash)`, `InvalidNonce(actualNonce)` | A header field does not hold the fixed value Besu BFT requires |
| `InvalidExtraDataFormat(itemsLength)` | `extraData` has neither four nor five items |
| `NonEmptyCommitSealList` | The fifth `extraData` item is not an empty list |
| `InvalidHeaderHeight` | A header or the initial height is zero |
| `InvalidHeaderTimestamp` | A header or the initial consensus state has a zero timestamp |
| `InvalidStateRoot(stateRoot)` | A state root is zero or the empty-trie root |
| `EmptyValidatorSet` | A validator list is empty |
| `InvalidValidatorAddress(validator)` | A validator list starts with the zero address |
| `UnsortedValidatorSet(index)` | A validator list is not strictly ascending |
| `InvalidTrustedHeight(trustedHeight, headerHeight)` | The header is not above its trusted height |
| `ConsensusStateNotFound(revisionHeight)` | No consensus state is stored at the height |
| `ConsensusStatePreimageMismatch(expectedHash, actualHash)` | The submitted consensus state does not hash to the stored one |
| `ConsensusStateExpired(trustedTimestamp, currentTimestamp, trustingPeriod)` | A consensus state's age has reached the trusting period |
| `HeaderFromFuture(currentTimestamp, headerTimestamp, maxClockDrift)` | A header, or the initial consensus state, is more than `maxClockDrift` ahead of the block time |
| `EmptyCommitSeals` | An update, or a header in misbehaviour evidence, has no commit seals |
| `InvalidECDSASignatureLength(length)` | A commit seal is not 65 bytes |
| `InvalidCommitSeal` | Declared for a first seal that recovers no signer. OpenZeppelin's `ECDSA.recover` reverts `ECDSAInvalidSignature` first, so this error is never thrown |
| `UnsortedCommitSealSigners(index)` | The seals are not in strictly ascending order of signer |
| `InsufficientTrustedValidatorOverlap(actual, required)` | Too few trusted validators signed |
| `UnknownCommitSealSigner(signer)` | A signer is not in the header's validator list |
| `InsufficientValidatorQuorum(actual, required)` | Fewer than two thirds of the header's validators signed |
| `InvalidPathLength(expectedLength, actualLength)` | A verify call's path does not have exactly three elements |
| `InvalidPathElementLength(expectedLength, actualLength)` | A verify call's router element is not 20 bytes, or its slot element is not 32 bytes |
| `StorageSlotMismatch(expected, actual)` | A verify call's slot element is not `IBCSTORE_STORAGE_SLOT` |
| `CounterpartyRouterMismatch(expected, actual)` | A verify call's router element is not the client state's `ibcRouter` |
| `InvalidValueLength(expectedLength, actualLength)` | A membership value is not 32 bytes |
| `StorageRootNotInCache(revisionHeight)` | A verify call sends no account proof, and no earlier call in the transaction proved that height |
| `InvalidCommitmentValue(expectedValue, actualValue)` | The proven commitment differs from the expected one |
| `InvalidExclusionProof` | A non-membership proof does not show the slot is absent |
| `NoMisbehaviourDetected` | The evidence shows no conflict |

Some reverts come from code the client inherits or calls.

| Error | When it is thrown |
|---|---|
| `EnforcedPause` | An update or verify call is made on a paused client, or `pause` is called while paused |
| `ExpectedPause` | `unpause` is called while not paused |
| `ECDSAInvalidSignature`, `ECDSAInvalidSignatureS` | A commit seal fails ECDSA recovery, including one with a recovery ID other than 27 or 28 |
| `TrieProofTraversalError(err)` | An account proof, or a membership storage proof, does not resolve under its root |
| `SafeCastOverflowedUintDowncast(bits, value)` | A header's height or timestamp does not fit in 64 bits |

A call with malformed ABI or RLP input reverts while that input is decoded.

## Permissions and pausing

The client takes no role configuration of its own. It is bound at construction to one [router](2-ics26-router.md), and that router's [access manager](6-permissions-and-upgrades.md#the-access-manager-and-its-roles) decides who may pause it.

| Function | Who may call it |
|---|---|
| `updateClient`, `verifyMembership`, `verifyNonMembership`, `misbehaviour` | Only the bound router, `ICS26_ROUTER` |
| `pause` | A holder of `PAUSER_ROLE` in the router's access manager, with no execution delay |
| `unpause` | A holder of `UNPAUSER_ROLE` in the router's access manager, with no execution delay |

A paused client rejects `updateClient`, `verifyMembership`, and `verifyNonMembership`, so the router cannot receive, acknowledge, or time out packets through it. The `misbehaviour` function has no pause check, so a paused client can still be frozen. [Permissions on a light client](6-permissions-and-upgrades.md#permissions-on-a-light-client) explains how the role lookup works.

## Constructor and configuration

The constructor sets the trust settings and the first trusted consensus state, and checks each against its limits.

| Parameter | Type | What it sets | What the constructor checks |
|---|---|---|---|
| `initialClientState` | `ClientState memory` | The counterparty router `ibcRouter`, the first trusted height, `trustingPeriod`, `maxClockDrift`, and `trustLevel` | `ibcRouter` is nonzero. The height has revision number zero and is nonzero. `isFrozen` is false. `trustingPeriod` is above zero and at most 30 days. `maxClockDrift` is at most 12 seconds. `trustLevel` has a nonzero denominator and lies from one third to two thirds |
| `initialConsensusState` | `ConsensusState memory` | The trusted timestamp, state root, and validator list at that height | The timestamp, state root, and validator list pass the same checks as a header's. The timestamp is within the trusting period and at most `maxClockDrift` ahead of the block time |
| `ics26Router` | `address` | The only router that may call the client's update, verify, and misbehaviour functions. Its access manager decides who may pause the client | Nonzero |

A failed check reverts with its own error from [Errors](#errors). The `trustLevel` field is a `TrustThreshold` fraction:

```solidity
struct TrustThreshold {
    uint8 numerator;
    uint8 denominator;
}
```

The constructor checks no signatures, so the deployer is trusted to supply a correct first consensus state. [Deploying](../4-light-clients/3-besu-light-client.md#deploying) describes how `ibc deploy client besu` reads it from a recent counterparty header.

No function changes the counterparty router, the trust settings, or the bound router after deployment.

## Deployment, registration, and replacement

The client is deployed directly, with no proxy and no upgrade function. [Permissions and upgrades](6-permissions-and-upgrades.md) lists every proxy and who may upgrade it.

<Warning>
Deploy this client only on a chain that has activated the Cancun upgrade. The contracts are compiled for the Cancun EVM version, so the compiled code may use any opcode up to Cancun. For example, the client caches proven storage roots in transient storage, which Cancun adds. On a chain without Cancun, client updates and packet proofs revert, so no packet can be relayed.
</Warning>

The router uses a deployed client once `addClient` registers it under a client ID. That call also records the counterparty's client ID and Merkle prefix. The router's `prefixedPath` appends the commitment path to the last element of the Merkle prefix. The client expects the prefix `[ibcRouter, IBCSTORE_STORAGE_SLOT, ""]`, which gives the three-element path in [Membership proofs](#membership-proofs). The IBC CLI registers that prefix for every Besu client.

Replacing a client is a router operation: the admin-gated `migrateClient` replaces the contract registered under an existing client ID. That is the only recovery from a freeze, or from a client whose stored consensus states are all past the trusting period.

Migration overwrites the client address and the counterparty information. It copies no state from the old contract, so the replacement starts from its own first consensus state.

<!-- Verified 2026-10-28 against cosmos/ibc-contracts 770b6e02 (ibc-solidity/contracts/light-clients/besu/, light-clients/utils/, interfaces/ILightClient.sol, utils/ICS02ClientUpgradeable.sol, utils/ICS24Host.sol, utils/IBCStoreUpgradeable.sol, utils/IBCRolesLib.sol, errors/IICS26ManagedLightClientErrors.sol, msgs/IICS02ClientMsgs.sol, ICS26Router.sol, ibc-solidity/foundry.toml, with OpenZeppelin Contracts 5.7.0) and this repository's cli/, by five rounds of blind fact-checks that did not write the page. The last round was clean. A final readability pass then changed wording only: it split sentences, named fields, and moved the Events section ahead of What freezes the client, without adding a claim. Re-verified after the bump from dfba365f2760 to 770b6e02, which changed the proof path checks, three errors, the trust level bounds, and the cap on the trusted overlap requirement. A sixth blind round checked the whole page against 770b6e02. It found one wrong word in the body and two incomplete lists in this footer, all fixed. The bump from 770b6e02 to daaed552 changed only utils/ICS27Account.sol, interfaces/IICS27Account.sol, the ICS27Account Go binding, the contracts README, and tests, none of the files above, and the cli/ changes merged with it touched only logging and metrics, so no claim on the page moved; checked by diff, without another blind round. daaed552 is an untagged commit, 127 commits after proof-api-v0.8.1, not a release. Outside what the tree can settle: whether a given chain has activated Cancun, that transient storage clears when a transaction ends, and that ecrecover returns zero for a recovery ID other than 27 or 28. -->
