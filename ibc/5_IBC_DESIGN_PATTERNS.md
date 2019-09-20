# 5: IBC Design Patterns

**This is a discussion of design patterns used throughout the interblockchain communication protocol specification.**

**For definitions of terms used in IBC specifications, see [here](./1_IBC_TERMINOLOGY.md).**

**For an architectural overview, see [here](./2_IBC_ARCHITECTURE.md).**

**For a broad set of protocol design principles, see [here](./3_IBC_DESIGN_PRINCIPLES.md).**

**For a set of example use cases, see [here](./4_IBC_USECASES.md).**

## Verification instead of computation

Computation on distributed ledgers is expensive: any computations performed
in the IBC handler must be replicated across all full nodes. Therefore, when it
is possible to merely *verify* a computational result instead of performing the
computation, the IBC handler should elect to do so and require extra parameters as necessary.

In some cases, there is no cost difference - adding two numbers and checking that two numbers sum to
a particular value both require one addition, so the IBC handler should elect to do whatever is simpler.
However, in other cases, performing the computation may be much more expensive. For example, connection
and channel identifiers must be uniquely generated. This could be implemented by
the IBC handler hashing the genesis state plus a nonce when a new channel is created, to create
a pseudorandom identifier - but that requires computing a hash function on-chain, which is expensive.
Instead, the IBC handler should require that the random identifier generation be performed
off-chain and merely check that a new channel creation attempt doesn't use a previously
reserved identifier.

## Call receiver

Essential to the functionality of the IBC handler is an interface to other modules
running on the same machine, so that it can accept requests to send packets and can
route incoming packets to modules. This interface should be as minimal as possible
in order to reduce implementation complexity and requirements imposed on host state machines.

For this reason, the core IBC logic uses a receive-only call pattern that differs
slightly from the intuitive dataflow. As one might expect, modules call into the IBC handler to create
connections, channels, and send packets. However, instead of the IBC handler, upon receipt
of a packet from another chain, selecting and calling into the appropriate module,
the module itself must call `recvPacket` on the IBC handler (likewise for accepting
channel creation handshakes). When `recvPacket` is called, the IBC handler will check
that the calling module is authorised to receive and process the packet (based on included proofs and
known state of connections / channels), perform appropriate state updates (incrementing
sequence numbers to prevent replay), and return control to the module or throw on error.
The IBC handler never calls into modules directly.

Although a bit counterintuitive to reason about at first, this pattern has a few notable advantages:

- It minimises requirements of the host state machine, since the IBC handler need not understand how to call
  into other modules or store any references to them.
- It avoids the necessity of managing a module lookup table in the handler state.
- It avoids the necessity of dealing with module return data or failures. If a module does not want to 
  receive a packet (perhaps having implemented additional authorisation on top), it simply never calls
  `recvPacket`. If the routing logic were implemented in the IBC handler, the handler would need to deal
  with the failure of the module, which is tricky to interpret.

It also has one notable disadvantage:

- Without an additional abstraction, the relayer logic becomes more complex, since off-chain
  relayer processes will need to track the state of multiple modules to determine when packets
  can be submitted.

For this reason, there is an additional IBC "routing module" which exposes a call dispatch interface.

## Call dispatch

For common relay patterns, an "IBC routing module" can be implemented which maintains a module dispatch table and simplifies the job of relayers.

In the call dispatch pattern, datagrams (contained within transaction types defined by the host state machine) are relayed directly
to the routing module, which then looks up the appropriate module (owning the channel & port to which the datagram was addressed)
and calls an appropriate function (which must have been previously registered with the routing module). This allows modules to 
avoid handling datagrams directly, and makes it harder to accidentally screw-up the atomic state transition execution which must
happen in conjunction with sending or receiving a packet (since the module never handles packets directly, but rather exposes
functions which are called by the routing module upon receipt of a valid packet).

Additionally, the routing module can implement default logic for handshake datagram handling (accepting incoming handshakes
on behalf of modules), which is convenient for modules which do not need to implement their own custom logic.
