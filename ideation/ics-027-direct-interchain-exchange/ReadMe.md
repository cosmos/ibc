# Cosmos Direct Interchain Exchange Protocol (DIEP)

#### Core Idea:

Asynchronous liquidity available with IBC that is resistant to theft by misbehaving validators. This eliminates the need to concentrate liquidity (limit orders) in a third party blockchain/venue. Price discovery and execution becomes native between Cosmos zones (e.g. N^2 IBC connections for N zones). There is a cost in terms of higher latency from this architecture.

Exchange consists of three core components.

1. Commitment of Funds for an order
2. Order matching
3. Execution and delivery

A DEX chain like Binance combines these 3 functions into a single synchronous environment. This comes with risks including asset peg risks, and trusting the validator set of faithfully executing your order.	

We propose an alternative model.  In this model, each of the 3 phases is asynchronously executed on independent blockchains.

#### Cast of Characters

Alice has Foo tokens on FooChain.

Bob has Bar tokens on BarChain.

FooChain and BarChain are connected via IBC.

There exists a Hub chain.  FooChain and BarChain are also connected via IBC to the HubChain.

Alice perceives the security of FooChain and Hub chain to be *strong*. 
Alice perceive the security of BarChain to be /moderate/.
Or, Alice perceives the security of both FooChain and BarChain to be *strong*.

Bob perceives the security of Bar Chain and Hub chain to be *strong*.
Bob perceive the security of Foo Chain to be /moderate/.
Or, Bob perceives the security of both FooChain and BarChain to be *strong*.

#### Protocol

For simplicity, we will describe only one scenario, where Alice wishes to submit a limit order to purchase Bar tokens with her Foo tokens, where the desired price is lower than market (e.g. the limit order will not be matched immediately).

Alice submits to FooChain a limit order transaction on FooChain: {type:"limit", pair:"bar/foo", sell:"20foo", price:"0.9"}

FooChain sees that the price of "0.9" is lower than the current price of "1", so it locks Alice's 20 Foo tokens, and sends this order to BarChain via IBC.

This order persists on FooChain (in the form of locked tokens) and BarChain (in the form of an open limit order) until one of two things happen.

Case A: Alice cancels the order by submitting a cancellation order on BarChain: {type:"limit-cancel", pair:"bar/foo", sell:"20foo", account:"Alice"}.
Case B: Bob (say) partially or fully matches the order by submitting a limit order on BarChain: {type:"limit", pair:"bar/foo", sell:"100bar", price:"0.8"}.

There are ways to design the protocol such that Alice can cancel the order without having to submit an order to BarChain, but due to the asynchrony of IBC it cannot be immediate for both Alice and Bob simultaneously.  Such extensions to the interchain exchange protocol are not described here.

For both case A and case B, a corresponding opposite IBC message is created from BarChain to FooChain.  In case A, the 20 Foo tokens are released back to Alice.  In case B, the 20 Foo tokens are released back to Bob, and in this example (since Bob's order volume was greater than Alice's), a new partial limit order would be created on FooChain on behalf of Bob.

#### Delivery

While the delivery of matched orders may be routed anywhere, by default we propose that the delivery happens on the Hub as IBC pegged tokens.
It simplifies custody for Alice and Bob because of common custodial protocols on the Hub.
It creates a substantial social pressure toward the economic finality of the exchange of tokens by having them delivered to Hub accounts.
Only delivered tokens need to be on the Hub, not the tokens behind pending limit orders, so as long as users move their funds out of the Hub soon after execution, the security requirement on the hub is minimal.

#### Hub Collateralization

With N^2 IBC connections between N zones for this Direct Interchain Exchange Protocol (DIEP), it becomes difficult for traders and market makers to make sense of the complexity arising from chain security.
Additionally, by forcing a chain to fail with a double spend attack (a consensus fork), it may become profitable to induce failure, especially with more complex scenarios that require consistency across the interchain.  For example, interchain pair trading may become profitable to attack.

To mitigate such attacks and guarantee more quantifiable skin in the game for the validators, the Hub should play the role of interchain collateralization.  All zones in the DIEP network should connect via IBC to the Hub, and the Hub should be modified to allow staking on behalf of these zones.  ATOM tokens staked on behalf of other zones should not be subject to the full inflationary ATOM tax (e.g. these staked zones may earn a proportionate amount of inflationary ATOMs, less any fees).  And the Hub becomes the arbiter for deciding whether a zone is consistent, or whether it had been double-spend attacked.  Any slashed ATOMs may be used to compensate the victims of an attack, although some ATOMs should probably always be burned.

#### Disadvantages

Alice wants to simultaneously place LimitOrders on FooChain to be taken on BarChain and take orders on Foochain. This requires 2x the value to be locked up by Alice because of asynchrony.


#### Prior Art

Agoric’s electronic rights transfer protocol and ZOE exchange protocol bear a pretty strong resemblance to this protocol based on informal descriptions. The Zoe protocol has been informally described by the Agoric team but there isn’t any public documentation I am aware of.
