# 5: IBC Design Patterns

**This is a discussion of design patterns used throughout the interblockchain communication protocol specification.**

**For an architectural overview, see [here](./1_IBC_ARCHITECTURE.md).**

**For a broad set of protocol design principles, see [here](./2_IBC_DESIGN_PRINCIPLES.md).**

**For definitions of terms used in IBC specifications, see [here](./3_IBC_TERMINOLOGY.md).**

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

## Call receiver instead of call dispatch
