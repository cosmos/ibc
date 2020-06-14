## Call receiver

Essential to the functionality of the IBC handler is an interface to other modules
running on the same ledger, so that it can accept requests to send packets and can 
route incoming packets to modules. This interface should be as minimal as possible
in order to reduce implementation complexity and requirements imposed on host ledgers.

For this reason, the core IBC logic uses a receive-only call pattern that differs
slightly from the intuitive dataflow. As one might expect, modules call into the IBC handler to create
connections, channels, and send packets. However, instead of the IBC handler, upon receipt
of a packet from another ledger, selecting and calling into the appropriate module,
the module itself must call `recvPacket` on the IBC handler (likewise for accepting
channel creation handshakes). When `recvPacket` is called, the IBC handler will check
that the calling module is authorised to receive and process the packet (based on included proofs and 
known state of connections / channels), perform appropriate state updates (incrementing
sequence numbers to prevent replay), and return control to the module or throw on error.
The IBC handler never calls into modules directly.

Although a bit counterintuitive to reason about at first, this pattern has a few notable advantages:

- It minimises requirements of the host ledger, since the IBC handler need not understand how to call
  into other modules or store any references to them.
- It avoids the necessity of managing a module lookup table in the handler state.
- It avoids the necessity of dealing with module return data or failures. If a module does not want to  
  receive a packet (perhaps having implemented additional authorisation on top), it simply never calls
  `recvPacket`. If the routing logic were implemented in the IBC handler, the handler would need to deal
  with the failure of the module, which is tricky to interpret.

It also has one notable disadvantage: without an additional abstraction, the relayer logic becomes more complex, since off-ledger
relayer processes will need to track the state of multiple modules to determine when packets
can be submitted.

For this reason, ledgers may implement an additional IBC "routing module" which exposes a call dispatch interface.

## Call dispatch

For common relay patterns, an "IBC routing module" can be implemented which maintains a module dispatch table and simplifies the job of relayers.

In the call dispatch pattern, datagrams (contained within transaction types defined by the host ledger) are relayed directly
to the routing module, which then looks up the appropriate module (owning the channel and port to which the datagram was addressed)
and calls an appropriate function (which must have been previously registered with the routing module). This allows modules to
avoid handling datagrams directly, and makes it harder to accidentally screw-up the atomic state transition execution which must
happen in conjunction with sending or receiving a packet (since the module never handles packets directly, but rather exposes
functions which are called by the routing module upon receipt of a valid packet).

Additionally, the routing module can implement default logic for handshake datagram handling (accepting incoming handshakes
on behalf of modules), which is convenient for modules which do not need to implement their own custom logic.
