# Localhost Support in 04-channel

## Rationale

IBC provide the ICS-26 interface for two mutually-untrusted applications to negotiate a communication channel and start sending messages to each other. This interface is now widely adopted by applications (both smart contracts and static modules) so that they can speak to applications on remote chains. However, there is no way to reuse this interface to communicate with applications on the same chain. So applications that wish to communicate with counterparties on remote and local state machines must for the moment have two separate implementations to handle both cases. This is a dev-UX issue that we can solve by providing localhost functionality (analogous to localhost for the internet protocol).

One approach is to implement this feature at the client layer. However this leads to a number of inefficiencies. Firstly, it requires the creation of a connection (handshake messages mediated by relayers) to be sent from the state machine back to itself in order to connect. It then requires that state that exists at the channel layer be routed all the way into the client so that it can then be inspected for verification purposes. Lastly, it does not immediately lend itself to future improvements that might make localhost packet flows atomic.

The state that applications wish to verify about their counterparties are all stored in the channel store (i.e. `channelEnd`, `packet_commitment`, `receipt`, `acknowledgement`). Thus, the simplest approach is to add the special localhost handling here at the channel layer, rather than propogating it all the way down the stack. This provides the uniform interface that applications are looking for with minimal changes to the core handlers.

## Technical Specification

Implementations will reserve a special connectionID: `connection-localhost` which applications can build their channels on in order to communicate with a counterparty on the same state machine.

In all channel verification methods, the channel logic must first check that the connectionID is the sentinel localhost connectionID: `connection-localhost`. If it is, then we simply introspect our own channel store to check if the expected state is stored at the expected key path. If the connection is not a localhost, then we delegate verification to the connection layer and ultimately the counterparty client so it can verify that the expected state is stored at the expected key path on the remote chain.

Here is an example of how this would be implemented for the `recvPacket` handler:

```typescript
function recvPacket(packet: OpaquePacket,
  proof: CommitmentProof,
  proofHeight: Height,
  relayer: string): Packet {
    // pre-verification logic

    // get the underlying connectionID for this channel
    channel = provableStore.get(channelPath(packet.destPort, packet.destChannel))
    connectionID = channel.connecti

    //construct the expectedState and keypath
    expectedState = commitPacket(packet)
    keyPath = channelPath(packet.sourcePort, packet.sourceChannel, packet.Sequence)


    verifyPacketCommitment(connectionID, keyPath, expectedState, proof, proofHeight)
}

// private function created to abstract the verification logic of localhost and remote chains from the rest of channel logic
//
// NOTE: It currently takes in a packet so we can call the connection verify function appropriately
// however, a future refactor that abstracts verification at the connection layer
// may remove the need for this additional argument.
function verifyPacketCommitment(connectionID: string, packet: Packet, keyPath: bytes, expectedState: bytes, proof: CommitmentProof, proofHeight: Height) {
    if connectionID == LocalHostConnection {
        // verify packet by checking if the expected state 
        // exists in our own state at the given keyPath
        // NOTE: proof and proofHeight are ignored in this case
        actualState = provableStore.get(keyPath)
        abortTransactionUnless(expectedState == actualState)
    } else {
        abortTransactionUnless(connection !== null)
        abortTransactionUnless(connection.state === OPEN)

        abortTransactionUnless(connection.verifyPacketData(
            proofHeight,
            proof,
            packet.sourcePort,
            packet.sourceChannel,
            packet.sequence,
            expectedState,
        ))
    }
}
```

## Relayer Messages

Relayers supporting localhost packet flow must be adapted to submit messages from sending applications back to the originating chain.

This would require first checking the underlying connectionID on any channel-level messages. If the underlying connectionID is `connection-localhost`, then the relayer must construct the message with an empty proof and proofHeight and submit the message back to the originating chain.

Implementations **may** choose to implement localhost such that the next message in the handshake or packet flow is automatically called without relayer-driven transactions. However, implementors must take care to ensure that automatic message execution does not cause gas consumption issues.