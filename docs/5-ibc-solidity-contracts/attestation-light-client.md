---
title: AttestationLightClient
description: Contract reference for the attestation light client: what it verifies, what the constructor fixes, its functions, wire formats, roles, and errors.
---

AttestationLightClient is an IBC light client that trusts a fixed set of attestor addresses, and it accepts a claim once a quorum of that set has signed it. Attestors claim two things about the counterparty chain: the timestamp of a block at a given height, and the values held at commitment paths at that height.

One instance is deployed per client and registered on the ICS26Router under a client ID. The router calls it to advance the client and to check the proof behind every packet operation.

Verification is self-contained. The client checks signatures against its own configuration and stores what it accepts, reaching outside itself for nothing.

Source: [AttestationLightClient.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/light-clients/attestation/AttestationLightClient.sol)

Interacts with: [ICS26Router](/ibc-solidity-contracts/ics26-router), which drives its updates and proof checks, and the [attestors](/light-clients/attestors) that produce its signatures. The trust model it implements is detailed on [the attestation light client page](/light-clients/attestation-light-client).

## Attestation and proof formats

An attestation always travels as one envelope: ABI-encoded attestation data plus an array of 65-byte ECDSA signatures. An update takes that envelope directly; a verify call carries it in its proof field.

```solidity
struct AttestationProof {
    bytes attestationData;
    bytes[] signatures;
}
```

The `attestationData` field holds `abi.encode(StateAttestation)` for an update, and `abi.encode(PacketAttestation)` for both verify calls.

```solidity
struct StateAttestation {
    uint64 height;
    uint64 timestamp;
}

struct PacketAttestation {
    uint64 height;
    PacketCompact[] packets;
}

struct PacketCompact {
    bytes32 path;
    bytes32 commitment;
}
```

- A zero commitment in a packet attestation claims the path is empty at that height. That positive claim is what a non-membership proof needs.

- The contract sets no limit on how many packets one attestation covers, so sizing a proof against the block gas limit should be considered by the submitter.

## How signatures are verified

When a proof arrives, the client rebuilds the digest the attestors signed and checks the signatures against it. The digest is `sha256(typeTag || sha256(attestationData))`, where the tag is `0x01` for a state attestation and `0x02` for a packet attestation.

The check is strict. At least `minRequiredSigs` signatures must be present, and no two may recover to the same address. Each one must be exactly 65 bytes and must recover to an address in the configured attestor set.

One bad or unknown signature reverts the whole call, so a submitter prunes signatures off chain rather than padding the array.

## Interface

This client implements the standard light client interface:

```solidity
interface ILightClient {
    function updateClient(bytes calldata updateMsg) external returns (ILightClientMsgs.UpdateResult);
    function verifyMembership(ILightClientMsgs.MsgVerifyMembership calldata msg_) external returns (uint256);
    function verifyNonMembership(ILightClientMsgs.MsgVerifyNonMembership calldata msg_) external returns (uint256);
    function misbehaviour(bytes calldata misbehaviourMsg) external;
    function getClientState() external view returns (bytes memory);
}
```

AttestationLightClient implements all five and adds two getters for its trust configuration, plus the public `PROOF_SUBMITTER_ROLE` identifier. It also inherits OpenZeppelin AccessControl, which brings its standard role functions and `supportsInterface`.

### Client updates

Of the five interface functions, only `updateClient` writes state. Its return value says which of three things happened.

| Function | What it does |
|---|---|
| `updateClient(bytes updateMsg) returns (UpdateResult)` | Verifies a signed state attestation and stores its timestamp for that height |
| `misbehaviour(bytes)` | Reverts `FeatureNotSupported` |

An update message is a proof envelope carrying a state attestation. The attestation needs a quorum over the state-tagged digest, a nonzero height, and a nonzero timestamp.

| Result | When it comes back |
|---|---|
| `Update` | The height had no stored timestamp, so the attested one is stored |
| `NoOp` | The height already holds the identical timestamp |
| `Misbehaviour` | The height holds a different timestamp, which also freezes the client |

An update at a lower height is accepted and its timestamp stored, but `latestHeight` advances only when the new height is higher.

Evidence of misbehaviour reaches this client one way only, through the conflicting timestamp in the table above.

### Membership proofs

Both verify functions re-check quorum over a packet attestation, then look the path up in the attested list.

| Function | What it does |
|---|---|
| `verifyMembership(MsgVerifyMembership) returns (uint256)` | Proves a commitment is attested at the proof height, and returns the counterparty's trusted timestamp there |
| `verifyNonMembership(MsgVerifyNonMembership) returns (uint256)` | Proves a path is attested as empty at the proof height, and returns the same timestamp |

Each call requires a stored consensus timestamp at the proof height, a quorum over the packet-tagged digest, and an attested height equal to the requested proof height.
Both are view functions, so neither writes anything.

The path is exactly one element, hashed with `keccak256` before the lookup. A membership call also takes a nonempty value that decodes as a `bytes32` commitment.

Membership then succeeds on an attested entry whose path hash and commitment both match. It reverts when no entry does.

Non-membership needs the path to appear in the attested list with an explicitly zero commitment. A path missing from the list reverts `NotMember`.

When the proof height is new, the client has to be updated before the verify call reaches it. The relayer's EVM builder does that by putting the `updateClient` call first in the router multicall, so a verify call later in that transaction finds a stored timestamp at the proof height.

### Views

Three getters expose the whole trust configuration and every stored timestamp, and they keep answering after a freeze.

| Function | What it returns |
|---|---|
| `getClientState() returns (bytes)` | The ABI-encoded client state: attestor addresses, `minRequiredSigs`, `latestHeight`, `isFrozen` |
| `getAttestationSet() returns (address[], uint8)` | The configured attestor addresses and the quorum threshold |
| `getConsensusTimestamp(uint64 revisionHeight) returns (uint64)` | The trusted timestamp in unix seconds for one height |

A zero from `getConsensusTimestamp` means no usable timestamp is stored for that height, and no proof at that height verifies while it reads zero.

## State and storage

The client's own storage is its trust configuration, the highest height it has seen, a frozen flag, and one timestamp per attested height. Role assignments live in the inherited AccessControl storage.

| What is stored | When it is written |
|---|---|
| The attestor addresses and `minRequiredSigs` | At construction |
| The attestor membership mapping | At construction |
| The trusted consensus timestamp for a height | At construction for the initial height, then on every update that brings a new height |
| `latestHeight` | At construction, then whenever an update brings a higher height |
| `isFrozen` | Once, when an update carries a conflicting timestamp for a known height |
| The role assignments | At construction, then on every grant or revoke |

The contract has no delete path, so every attested height stays queryable for the life of the deployment.

Progress is read through the getters rather than from events: `getClientState` for `latestHeight` and the frozen flag, `getConsensusTimestamp` for one height at a time. The only events it emits are the OpenZeppelin AccessControl events for granting and revoking a role.

## Freezing

One quorum-signed contradiction freezes the client, and the freeze is permanent.

The client freezes when a state attestation passes the quorum check but carries a timestamp different from the one already stored for its height. That call sets `isFrozen` and returns `Misbehaviour`.

A frozen client stops verifying. Calls to `updateClient`, `verifyMembership`, `verifyNonMembership`, and `misbehaviour` revert on the frozen guard, while the getters and role administration keep working.

The contract carries no unfreeze function, so recovery runs through a new deployment and a client migration, described in Deployment and replacement below.

## Errors

The table lists every error the client declares.

| Error | Raised when |
|---|---|
| `NoAttestors` | The constructor gets an empty attestor set |
| `BadQuorum(minRequired, attestationCount)` | The threshold is zero, or larger than the attestor set |
| `DuplicateSigner(signer)` | An address appears twice in the constructor's set, or two signatures recover to the same signer |
| `EmptySignatures` | A proof carries no signatures |
| `ThresholdNotMet(validSigners, minRequired)` | A proof carries fewer signatures than the threshold |
| `InvalidSignatureLength(signature)` | A signature is not 65 bytes |
| `SignatureInvalid(signature)` | Never raised. ECDSA recovery reverts before this check runs |
| `UnknownSigner(signer)` | A signature recovers to an address outside the attestor set |
| `FrozenClientState` | A gated function is called on a frozen client |
| `ConsensusTimestampNotFound(height)` | The proof height has no stored timestamp |
| `HeightMismatch(expected, provided)` | The attested height differs from the requested proof height |
| `InvalidPathLength(expectedLength, providedLength)` | The path is not exactly one element |
| `EmptyValue` | A membership call passes an empty value |
| `EmptyPackets` | The attestation carries no packet entries |
| `NotMember` | No attested entry matches the path and commitment, or a non-membership path is absent from the list |
| `NotNonMember` | A non-membership path is attested with a nonzero commitment |
| `InvalidState(height, timestamp)` | An update carries a zero height or a zero timestamp |
| `FeatureNotSupported` | A call reaches `misbehaviour` |

A call can also revert on the OpenZeppelin code the client inherits. A caller lacking the role a function requires gets `AccessControlUnauthorizedAccount`. The compiled ABI lists the inherited errors in full.

A proof envelope that fails to decode reverts with no named error.

## Roles and permissions

One role gates all four proof entry points, and the deployment decides whether that means everyone or a named holder.

| Role | What it gates | Typical holder |
|---|---|---|
| `PROOF_SUBMITTER_ROLE` | `updateClient`, `verifyMembership`, `verifyNonMembership`, `misbehaviour` | The ICS26Router, when the client backs an IBC connection |
| `DEFAULT_ADMIN_ROLE` | Granting and revoking roles on this client | The `roleManager` passed to the constructor |

Holders of `PROOF_SUBMITTER_ROLE` may call the four gated functions. When the zero address holds that role, any caller passes the check. Otherwise the caller must hold the role itself.

The interface says to grant the role to the ICS26Router when the client is used with IBC.

The `roleManager` argument decides between those two cases. A nonzero address receives the AccessControl default admin role along with the submitter role, so whether the submitter role can change later depends on what that address is. `ibc deploy client` passes the [router](/ibc-solidity-contracts/ics26-router), which exposes no way to call `grantRole`, so on a client IBC Link deployed those roles cannot be moved without upgrading the router. A zero address leaves submission open to anyone.

## Constructor and configuration

The constructor fixes what the client trusts: the attestor set, the quorum threshold, a first trusted height with its timestamp, and the address that manages roles.

| Parameter | Type | What it sets |
|---|---|---|
| `attestorAddresses` | `address[] memory` | The attestor addresses whose signatures this client accepts |
| `minRequiredSigs` | `uint8` | How many distinct attestor signatures a claim needs |
| `initialHeight` | `uint64` | The first trusted height |
| `initialTimestampSeconds` | `uint64` | The trusted timestamp for that height |
| `roleManager` | `address` | Who administers roles and may submit proofs. Zero opens submission to anyone |

The initial height becomes `latestHeight`, and its timestamp is stored as the trusted timestamp for that height. A nonzero timestamp there lets a proof at the deployment height verify with no prior update. Deployment reverts on an empty attestor set, a zero threshold, a threshold larger than the set, or a duplicate attestor address.

No function adds, removes, or replaces an attestor.

The signed bytes carry no chain, client, or contract identifier, so an attestation is valid on every client that trusts the same attestor addresses.

<Warning>
Do not reuse attestor addresses across clients. A signature gathered for one client verifies on every other client that trusts the same address.
</Warning>

## Deployment and replacement

The client is a plain deployment. It sits behind no proxy and has no upgrade function, because the constructor sets everything it needs.

The [Permissions and upgrades](/ibc-solidity-contracts/permissions-and-upgrades) page carries the full proxy and upgrade-authority map.

A deployed client becomes usable once `addClient` registers it on the router under a client ID. That same call sets its counterparty information.

Replacing a client is a router operation: the admin-gated `migrateClient` swaps a new client contract in behind an existing client ID.
That is also the only recovery from a freeze, since the contract itself cannot be unfrozen.
