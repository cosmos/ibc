# Rollup Integration Guide

## Context

The following is a guide for rollup frameworks seeking to integrate with IBC. A rollup is a decentralized application that relies on a third-party blockchain for data availability (DA) and optionally for settlement. The rollup consensus mechanism differs from sovereign blockchains in important ways. The consensus on the blocks and ordering of the rollup is defined by the order in which they are posted onto a third party ledger, the DA layer. Since this third party ledger is not itself executing transactions and constructing the rollup app state, rollups may additionally have a settlement mechanism. There are two types of rollup architectures: optimistic and Zero Knowledge (ZK). ZK rollups submit a proof that the reported app hash is correctly constructed from the included transactions in the block, thus a rollup block and header can be trusted as legitimate as soon as it is finalized on the DA layer. An optimistic rollup on the other hand, relies on third party watchers, that can post a proof to a settlement layer that the rollup did not post the correct app hash from the posted transactions. This requires the settlement layer to be able to execute the rollup state machine. The DA layer and settlement layer **may** be different blockchains or the same.

This guide is not intended to be a formal specification or Interchain Standard. As the architectures for rollups and their underlying data availability and settlement layers differ vastly: from ZK rollups to optimistic rollups with separate data availability and settlement layers to sovereign rollups; it is impossible to write a fully specified client to encompass all these cases. Thus this guide is intended to highlight the IBC client functions that are most affected by rollup specific features and explain what must be done in each one to take into account the unique properties of rollups. Rollup light client developers should use this document as a starting point when designing their light clients to ensure they are taking into account rollup-specific logic in the appropriate places.

## Definitions

**Execution Layer or Rollup**: This is the rollup blockchain itself. It executes the rollup application and derives its consensus and security from the underlying layers (e.g. DA layer). The rollup client is the light client that tracks the rollup blockchain.

**Sequencer**: This is the actor(s) that collects user transaction and creates new rollup blocks. The sequencer must post these blocks to the data availability layer. Since the rollup's security is backed by the data availability and settlement layers, the sequencer does not need to be as decentralized as a sovereign validator set, it may even be a single operator. Some rollup architectures may even be "sequencerless", in this case, any actor may post new blocks to the data availability layer.

**Data Availability Layer (DA layer)**: This is the ledger on which the rollup block producers must post their blocks. Any rollup user can thus download the rollup blockchain from the DA layer. Thus the Data Availability layer provides a guarantee of the availability of the rollup blocks and the included transactions. Since the Data Availability layer is a blockchain and thus has a definite ordering, the ordering of rollup blocks can be derived from their ordering on the data-availability layer. Thus, the rollup derives its consensus (i.e. the agreed upon ordering of included transactions) from the data availability layer. The DA client is the light client that tracks the data availability blockchain.

**Settlement Layer**: The settlement layer is where disputes on the correctness of the posted rollup state is resolved. In addition to the included transactions, the rollup block producer must also post the state hash that results from applying the newly included transactions to the previous rollup state. If the rollup block producer posts an incorrect app hash for the posted block, any observer may submit a fraud proof to the settlement layer to dispute the incorrect app hash. At this point, the settlement layer must verify the fraud proof; often through a fraud proving game that requires the block producer and fraud submitter to narrow down on a disputed execution result before the settlement layer can execute the relevant logic to determine which party is honest. If the fraud is valid, the settlement layer must mark the fraudulent block as invalid. This block and any subsequent blocks built on top of it are invalidated and removed from the blockchain history of the rollup. The settlement layer is OPTIONAL as some rollup architectures do not involve settlement. For example, Celestia rollups are "sovereign rollups" and thus full nodes and the rollup p2p network itself is responsible for executing blocks and propagating fraud proofs. Also, the settlement layer MAY be the same ledger as the DA layer OR it may be completely different ledgers. The settlement client is the light client that tracks the settlement layer blockchain.

**Sovereign Rollup:** Sovereign rollups post their blocks to a data availability layer, but do not rely on any other blockchain for correctness (ie settlement). Thus rollup nodes derive consensus and ordering from the data availability layer, but must execute the transactions themselves to verify correctness or obtain fraud proofs from the rollup p2p network.

**Optimistic Rollup**: Optimistic rollups post their blocks to a data availability layer and rely on a settlement layer that can adjudicate fraud proofs submitted by rollup observers. Thus, rollup blocks are accepted "optimistically" before correctness can be guaranteed but they are only considered safe and finalized once a fraud window time period has passed without any successful challenge being submitted to the settlement layer.

**ZK Rollup**: A ZK rollup has a Zero-Knowledge circuit that represents its state machine. Thus, a rollup block producer can submit a ZK-SNARK proof that the submitted app hash is indeed the correct result of applying the included transactions in the block. Thus, there is no need for a settlement layer or a fraud window. The block can be trusted and finalized as soon as the ZK proof is verified.

### `verifyClientMessage`

In order to verify a new header for the rollup, the rollup client must also be able to verify the header's (and associated block's) inclusion in the DA layer. Thus, the rollup client's update logic **must** have the ability to invoke verification of the associated DA client. After verifying the rollups own consensus mechanism (which itself may be non-existent for some rollup architectures), it verifies the header and blockdata in the data availability layer. Simply proving inclusion is not enough however, we must ensure that the data we are proving is valid; i.e. the data is not simply included but is included in the way that is expected by the rollup architecture. In the example below, we check that the blockdata hashes to the `txHash` in the header.

ZK rollups can verify correctness of the header upon submission since the rollup client can embed a proving circuit that can verify a ZK proof from the relayer that the submitted header is correct. Optimistic rollups on the other hand cannot immediately trust a header upon submission, as the header may later be proved fraudulent. Thus, the header can be stored but must wait for the fraud period to elapse without any successful challenges to the correctness of the header before it is finalized and used for proof verification.

```typescript
function verifyClientMessage(clientMessage: ClientMessage) {
  switch typeof(clientMessage) {
    case Header:
      verifyHeader(clientMessage)
    case Misbehaviour:
      // this is completely rollup specific so it is left unspecified here
      // misbehaviour verification specification for rollups
      // is instead described completely in checkForMisbehaviour
  }
}

function verifyHeader(clientMessage: ClientMessage) {
  clientState = provableStore.get("clients/{clientMessage.clientId}/clientState")
  header = Header(clientMessage)

  // note: unmarshalling logic omitted
  // verify the header against the rollups own consensus mechanism if it exists
  // e.g. verify sequencer signature
  verifySignatures(header, clientSequencers)

  // we must assert that the block data is correctly associated with the header
  // this is specific to the rollup header and block architecture
  // the following is merely an example of what might be verified
  assert(hash(header.blockData) === header.txHash)

  // In addition to the rollups own consensus mechanism verification, 
  // we must ensure that the header and associated block data is stored in the DA layer.
  // The expected path, the header and data stored are
  // rollup-specific so it is left as an unspecified function
  // in this document. Though the path should reference a unique
  // namespace for the rollup specified here with the chain ID
  // and a unique height for the rollup
  daClient = getClient(clientState.DALayer)
  verifyMembership(
    daClient,
    header.DAProofHeight,
    0,
    0,
    header.DAHeaderProof,
    DAHeaderPath(clientState.chainId, header.height),
    header)
  verifyMembership(
    daClient,
    header.DAProofHeight,
    0,
    0,
    header.DABlockDataProof,
    DABlockDataPath(clientState.chainID, header.height),
    header.blockData)

  // if the rollup is a ZK rollup, then we can verify the correctness immediately.
  // Otherwise, the correctness of the submitted rollup header is contingent on passing
  // the fraud period without a valid proof being submitted (see misbehaviour logic)
  prove(client.ZKProvingCircuit, header.zkProof)
}
```

### `updateState`

The `updateState` function for rollups works the same as typical clients, though it is critical that the optimistic rollup client stores the submit time for when the consensus state was created so that we can verify that the fraud period has passed.

```typescript
function updateState(clientMessage: ClientMessage) {
  // marshalling logic omitted
  header = Header(clientMessage)
  consensusState = ConsensusState{header.timestamp, header.appHash}

  provableStore.set("clients/{clientMessage.clientId}/consensusStates/{header.GetHeight()}", consensusState)

  // create mapping between consensus state and the current time for fraud proof waiting period
  provableStore.set("clients/{clientMessage.clientId}/processedTimes/{header.GetHeight()}", currentTimestamp())
}
```

### `checkForMisbehaviour`

Misbehaviour verification has a different purpose for rollup architectures than it does in traditional consensus mechanisms.

Typical consensus mechanisms, like proof-of-stake, are self-reliant on ordering. Thus, we must have mechanisms to detect when the consensus set is violating the ordering rules. For example, in tendermint, the misbehaviour verification checks that header times are monotonically increasing and that there exists only one valid header for each height.

However, with rollups the ordering is derived from the data availability layer. Thus, even if there is a consensus violation in the rollup consensus, it can be resolved by the DA layer and the consensus rules of the rollup. E.g. even if the sequencer signs multiple blocks at the same height, the canonical block is the first block submitted to the DA layer.

Thus, so long as the verification method encodes the consensus rules of the rollup architecture correctly (for instance, ensuring the header submitted is the earliest one for the given height), then there is no need to verify misbehaviour of the rollup consensus. The consensus is derived from the DA layer, and so if the DA client is frozen due to misbehaviour, this should halt proof verification in the rollup client as well.

Instead, the misbehaviour most relevant for rollups is in the application layer, as the transactions are executed by the sequencer but not by the underlying data availability layer. For ZK rollups, the application is already proven correct so there is no need for application misbehaviour verification. However, optimistic rollups must provide the ability for off-chain processes to submit a proof that the application hash submitted in the header was the result of an incorrect computation of transaction(s) in the block i.e. a fraud proof.

The optimistic fraud proof verifier, or proving circuit, should be implemented as a smart contract, since the fraud prover depends not on the consensus mechanism, but on the application state machine itself. Thus each rollup instance needs its own fraud prover. Having each fraud prover encoded directly in the client requires a different implementation for each rollup instance. Instead, calling out to a separate smart contract allows the client to be reused for all instances, and for new fraud provers to be uploaded for a new rollup application.

```typescript
// optimistic rollup fraud proof
// the misbehaviour must be associated with a height on the rollup
function checkForMisbehaviour(clientMessage: ClientMessage) {
  // unmarshalling logic omitted
  misbehaviour = Misbehaviour(clientMessage)
  clientId = clientMessage.clientId
  clientState = provableStore.get("clients/{clientMessage.clientId}/clientState")

  // if the rollup has a settlement layer, we can delegate the fraud proof game to the settlement layer
  // and simply verify with the settlement client that fraud has been proven for the given misbehaviour
  if clientState.settlementLayer == nil {
    // fraud prover here is a contract so the same rollup client implementation may
    // be initiated with different fraud prover contracts for each
    // different state machine
    fraudProverContract = getFraudProver(clientId)
    fraudProverContract.verifyFraudProof(misbehaviour)
  } else {
    // in order to use a settlement client some sentinel value signifying submitted misbehaviour
    // must be stored at a specific path for the given rollup and height
    // so that the client can prove that the settlement client did in fact successfully prove misbehaviour
    // for the given rollup at the given height
    misbehavingHeight = getHeight(misbehaviour)
    settlementClient = getClient(clientState.settlementLayer)
    misbehaviourPath = getMisbehaviourPath(clientId, misbehavingHeight)
    settlementClient.verifyMembership(misbehaviour.proofHeight, 0, 0, misbehaviour.proof, misbehaviourPath, MISBEHAVIOUR_SUCCESS_VALUE)
  }
}
```

### `updateStateOnMisbehaviour`

The misbehaviour update is also dependent on the rollup architecture. In sovereign proof-of-stake chains, if the consensus rules are violated, there is often no fallback mechanism as the trust in the chain is completely destroyed without out-of-protocol social consensus restarting the chain with a new validator set. Thus, for sovereign chains, a client should simply be disabled upon receiving valid misbehaviour.

Rollups on the other hand do have a fallback layer in the data availability and settlement layers. For example, the settlement layer can verify a block is invalid and simply remove it thus enforcing that blocks can keep proceeding with valid states as the settlement layer can continue removing invalid blocks from the chain history. Similarly, it's possible that the settlement layer has a mechanism to switch the sequencer if a block is proven invalid.

Thus, `updateStateOnMisbehaviour` can be less strict for rollups and simply remove the fraudulent consensus state and wait for the resolution as specified by the rollup's consensus rules.

```typescript
function updateStateOnMisbehaviour(clientMessage: ClientMessage) {
  // unmarshalling logic omitted
  misbehaviour = Misbehaviour(clientMessage)
  misbehavingHeight = getHeight(misbehaviour)

  // delete the fraudulent consensus state
  deleteConsensusState(clientMessage.clientId, misbehavingHeight)

  // its possible for the rollup client to do additional logic here
  // e.g. verify the next sequencer chosen from settlement layer
  // however this is highly specific to rollup architectures
  // and is not necessary for all rollup architectures
  // so it will not be modelled here.
}
```

### Membership Verification Methods

The parts of the client that rely on the data availability are encapsulated in verifying new rollup blocks. Thus, once they are already added to the client, they can be used for proof verification without reference to the underlying data availability layer. For optimistic rollups, the consensus state must exist in the client for the full fraud period before it can be used for proof verification.

Since the rollup client is dependent on underlying clients: data availability client and settlement client, these must also not be frozen by misbehaviour in order for proof verification to proceed.

```typescript
function verifyMembership(
  clientState: ClientState,
  height: Height,
  delayPeriodTime: uint64, // disabled
  delayPeriodBlocks: uint64, // disabled
  proof: CommitmentProof,
  path: CommitmentPath,
  value: bytes
): Error {
  // check conditional clients are still valid
  daClient = getClient(clientState.DALayer)
  settlementClient = getClient(clientState.settlementLayer) // may not exist for all rollups

  assert(isActive(clientState))
  assert(isActive(daClient))
  assert(isActive(settlementClient))

  consensusState = provableStore.get("clients/{clientState.clientId}/consensusStates/{height}")
  processedTime = provableStore.set("clients/{clientState.clientId}/processedTimes/{height}")

  // must ensure fraud proof period has passed
  assert(processedTime + clientState.fraudPeriod > currentTimestamp())

  if !verifyMembership(consensusState.commitmentRoot, proof, path, value) {
    return error
  }
  return nil
}

function verifyNonMembership(
  clientState: ClientState,
  height: Height,
  delayPeriodTime: uint64, // disabled
  delayPeriodBlocks: uint64, // disabled
  proof: CommitmentProof,
  path: CommitmentPath,
): Error {
  // check conditional clients are still valid
  daClient = getClient(clientState.DALayer)
  settlementClient = getClient(clientState.settlementLayer) // may not exist for all rollups

  assert(isActive(clientState))
  assert(isActive(daClient))
  assert(isActive(settlementClient))

  consensusState = provableStore.get("clients/{clientState.clientId}/consensusStates/{height}")
  processedTime = provableStore.set("clients/{clientState.clientId}/processedTimes/{height}")

  // must ensure fraud proof period has passed
  assert(processedTime + clientState.fraudPeriod > currentTimestamp())

  if !verifyNonMembership(consensusState.commitmentRoot, proof, path) {
    return error
  }
  return nil
}
```
