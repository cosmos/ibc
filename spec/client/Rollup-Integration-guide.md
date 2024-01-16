# Rollup Integration Guide

### Background context

The following is a guide for rollup frameworks seeking to integrate with IBC. A rollup is a decentralized application that relies on a third-party blockchain for data availability and optionally for settlement. The rollup consensus mechanism differs from sovereign blockchains in important ways. The consensus on the blocks and ordering of the rollup is defined by the order in which they are posted onto a third party ledger, the DA layer. Since this third party ledger is not itself executing transactions and constructing the rollup app state, rollups may additionally have a settlement mechanism. There are two types of rollup architectures: optimistic and ZK. ZK rollups submit a proof that the reported app hash is correctly constructed from the included transactions in the block, thus a rollup block and header can be trusted as legitimate as soon as it is finalized on the DA layer. An optimistic rollup on the other hand, relies on third party watchers, that can post a proof to a settlement layer that the rollup did not post the correct app hash from the posted transactions. This requires the settlement layer to be able to execute the rollup state machine. The DA layer and settlement layer **may** be different blockchains or the same.

### VerifyClientMessage

In order to verify a new header for the rollup; the rollup client must also be able to verify the header's (and associated block's) inclusion in the DA layer. Thus, the rollup client's update logic **must** have the ability to execute verification of the DA client.

```typescript
function verifyClientMessage(clientMsg: ClientMessage) {
    // verify the header against the rollups own consensus mechanism if it exists
    // e.g. verify sequencer signature
    verifySignatures(clientMessage.header, clientSequencers)

    // in addition to the rollups own consensus mechanism
    // we must additionally ensure that the header and associated block data is stored in the DA layer
    verifyMembership(clientMessage.header)
    verifyMembership(clientMessage.blockData)
    // we must also assert that the blockData is correctly associated with the header
    // this is specific to the rollup header and block architecture
}
```
