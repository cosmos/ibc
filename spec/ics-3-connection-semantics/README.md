# ICS 3: Connection Semantics

// stub for handshake, merge it to main ICS3 spec later

`Connection`s are categorized into multiple categories, each defined with own state machine for handshaking and expected behaviour. In the initial protocol, there will be two kinds of connection, unidirectional and bidirectional. 

The following definitions omits parts irrelevant to the handshaking state machine.

### Broadcasting

A connection can be opened without setting the counterparty. The packet pushed on the channels on this connection will persist, or be pruned after predefined amount of time. There is no guarantee about the packets.

### Unsafe Connection

// XXX: should we rename `ChainID` to `ConnID`?
A counterparty information can be set on a broadcasting connection. Counterpaty information `CI` is defined as `(ChainID, ROT)` where the `ChainID` is the id of the connection on the counterparty that this connection will listen on, and `ROT` is the root-of-trust `Block` of the chain that this connection will listen on.

There is no guarantee about the packets those are sent from this chain, however this chain can now receive packets from the counterparty, without the ability of responding with receipts or timeout. Also the connection cannot ensure that the incoming packets are intended to arrive on them.

Pair of (Broadcasting Connection^, Unsafe Connection^) forms an unidirectional chain, where the packet is sent from the first to the second.

### Safe Connection

An unsafe connection can check its counterparty unsafe connection's counterparty information. If the `CI.ChainID` is same with this connection's and `CI.ROT` is one of the blocks of this chain, it means that the counterparty is correctly pointing this connection. 

There is guarantee that the packet sent from this connection will arrive at the registered counterparty, and any packet coming from the counterparty is intended to arrive on this connection.

Pair of (Safe Connection, Safe Connection) forms an bidirectional chain.

### Attack Vector

In permissionless connection registeration, an attacker can register two connections on chain `A` both pointing connection `ChainID_A` on `B`, while it is referring only one of the connections on `A`. This is possible since both connection can pass the handshaking process as the `ROT` stored in `ChainID_A` on `B` is pointing chain `A`. One way to solve this is, in any given time, make there is only one valid `ROT` for a chain. For example, 
