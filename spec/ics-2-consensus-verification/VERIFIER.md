# Verifier

Full nodes are procedures which process a list of messages, `[Message]`, according to the
`Consensus` algorithm. The lightclient `Verifier` does not process all messages, but
it uses artifacts produced by execution of the `Consensus` algorithm, along with well-forumated security assumptions, to verify parts of a consensus execution transcript
process. The `Verifier` MUST work identically to a full node, given that the
security assumptions of `Consensus` are preserved. This means that if and only
if a full node accepts the new `Header` given a `ConsensusState` and
`[Message]`, will the `Verifier` then also accept it.

## Definitions

### Consensus

`Consensus` is a `Header` generating function which takes the previous
`ConsensusState` with the messages and returns the result.

```go
type Consensus func(ConsensusState, [Message]) Header
```

### Blockchain

Defined as blockchain consensus algorithm which generates valid `Header`s.
It generates a unique list of headers starting from a genesis `ConsensusState` with arbitrary
messages.

`Blockchain` is defined as
```go
type Blockchain struct {
  Genesis ConsensusState
  Consensus Consensus
}
```
where
  * `Genesis` is the genesis `ConsensusState`
  * `Consensus` is the header generating function

The headers generated from the `Blockchain` are expected to satisfy the
followings:

1. The `Header`s MUST NOT have more than one direct child

* Satisfied if: deterministic safety
* Possible violation scenario: validator double signing, chain reorganization (Nakamoto consensus)

2. The `Header`s MUST eventually have at least one direct child

* Satisfied if: liveness, light-client verifier continuity
* Possible violation scenario: synchronised halt, incompatible hard fork

3. The `Header`s MUST be generated from the `Consensus`, which ensures valid transition of the state

* Satisfied if: correct block generation & state machine
* Possible violation scenario: invariant break, supermajor validator cartel

If the blockchain does not satisfy any of the above then the IBC protocol
may not work as intended; the chain can receive multiple conflicting
packets, the chain cannot recover from the timeout event, the chain can
steal the user's asset, etc.

## Verifier

The validity of `Verifier` is dependent on the security model of the
`Consensus`. For example, the `Consensus` can be a proof of authority with
a trusted operator without legal binding, or the a proof of stake but with
insufficient value of stake. In such cases, it is possible that the
security assumptions break, the correspondence between `Consensus` and
`Verifier` no longer exists, and the behaviour of `Verifier` becomes
undefined. Also, the `Blockchain` may not longer satisfy
the requirements above, which leads the chain to be incompatible with IBC
protocol. The equivocation proof have to be generated and submitted to the
chain storing the `LightClient`, as defined in
[ICS ?](https://github.com/cosmos/ics/issues/53), to safely disconnect the
light client.
