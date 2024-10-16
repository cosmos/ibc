# Celestia Data Availability Client

## Synopsis

The Celestia Data Availability client is a minor modification to the standard Tendermint client that allows rollup clients to use the data availability client as part of its header verification logic. Celestia commits to the headers and block data of rollups in the DataHash of its header. Relayers must provide a valid [ShareProof](https://docs.celestia.org/developers/blobstream-proof-queries#converting-the-proofs-to-be-usable-in-the-daverifier-contract) to the rollup client. The rollup client will validate the namespace and the key/value pair before passing the proof to the underlying Celestia DA client to be verified.

### updateState

The first change that must be made to the Tendermint client to enable Celestia DA verification is to store the `DataHash` of the Tendermint Header instead of the `AppHash`. Thus, we update the consensus state:

```typescript
interface ConsensusState {
    timestamp: uint64
    nextValidatorsHash: []byte
    dataAvailabilityRoot: []byte
}
```

And the associated updateState function:

```typescript
function updateState(clientMsg: clientMessage) {
    clientState = provableStore.get("clients/{clientMsg.identifier}/clientState")
    header = Header(clientMessage)
    // only update the clientstate if the header height is higher
    // than clientState latest height
    if clientState.height < header.GetHeight() {
        // update latest height
        clientState.latestHeight = header.GetHeight()

        // save the client
        provableStore.set("clients/{clientMsg.identifier}/clientState", clientState)
    }

    // create recorded consensus state, save it
    consensusState = ConsensusState{header.timestamp, header.nextValidatorsHash, header.dataHash} // key change
    provableStore.set("clients/{clientMsg.identifier}/consensusStates/{header.GetHeight()}", consensusState)


    // these may be stored as private metadata within the client in order to verify
    // that the delay period has passed in proof verification
    provableStore.set("clients/{clientMsg.identifier}/processedTimes/{header.GetHeight()}", currentTimestamp())
    provableStore.set("clients/{clientMsg.identifier}/processedHeights/{header.GetHeight()}", currentHeight())
}
```

### verifyMembership

The verify proof functions `verifyMembership` and `verifyNonMembership` must be modified from verifying ICS23 proofs against an apphash in the consensus state to verifying Celestia ShareProofs against the datahash.

```typescript
function verifyMembership(
  clientState: ClientState,
  height: Height,
  delayTimePeriod: uint64,
  delayBlockPeriod: uint64,
  proof: CommitmentProof,
  path: CommitmentPath,
  value: []byte
): Error {
  // check that the client is at a sufficient height
  assert(clientState.latestHeight >= height)
  // check that the client is unfrozen or frozen at a higher height
  assert(clientState.frozenHeight === null || clientState.frozenHeight > height)
  // assert that enough time has elapsed
  assert(currentTimestamp() >= processedTime + delayPeriodTime)
  // assert that enough blocks have elapsed
  assert(currentHeight() >= processedHeight + delayPeriodBlocks)
  // fetch the previously verified dataAvailability root & verify membership
  // Implementations may choose how to pass in the identifier
  // ibc-go provides the identifier-prefixed store to this method
  // so that all state reads are for the client in question
  consensusState = provableStore.get("clients/{clientIdentifier}/consensusStates/{height}")
  shareProof = convertToShareProof(proof)
  shares = convertBytesToShares(value)
  assert(shares == shareProof.data)
  // verify that shares are stored in dataAvailability root
  if !proof.Validate(consensusState.dataAvailabilityRoot) {
    return error
  }
  return nil
}
```