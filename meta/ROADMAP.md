# Roadmap IBC specs

_Lastest update: April 6th, 2022_

This document endeavours to inform the wider IBC community about plans and priorities for the specification work of IBC. This roadmap should be read as a high-level guide, rather than a commitment to schedules and deliverables. The degree of specificity is inversely proportional to the timeline. We will update this document periodically to reflect the status and plans.
 
This roadmap reflects the major activities that the [standards committee](STANDARDS_COMMITTEE.md) is engaged with in the coming quarters. It is, by no means, a thorough reflection of all the specification work that is happening in the broad ecosystem, as many other parties work as well in specs that eventually end up in this repository.

## Q2 - 2022

- Work on general readability improvements and inconsistency fixes in some of the specs ([ICS02](https://github.com/cosmos/ibc/blob/master/spec/core/ics-002-client-semantics/README.md), [ICS06](https://github.com/cosmos/ibc/blob/master/spec/client/ics-006-solo-machine-client/README.md), [ICS07](https://github.com/cosmos/ibc/blob/master/spec/client/ics-007-tendermint-client/README.md)). This is a first step on the long-term plan to make the specs easier to understand to qualified developers.
- The [connection](https://github.com/cosmos/ibc/pull/621) and [channel](https://github.com/cosmos/ibc/pull/677) upgradability specs have been merged, but they need some small fixes. The spec team will also help with the planning of the implementation of channel upgradability in [ibc-go](https://github.com/cosmos/ibc-go).
- Finish writing the spec for [ordered channels that support timeouts](https://github.com/cosmos/ibc/pull/636).
- Start writing the spec to support state trees without absence proofs.
- The implementation of [ICS29](https://github.com/cosmos/ibc/tree/master/spec/app/ics-029-fee-payment) in ibc-go will be finished in Q2 and the spec might need some updates to reflect the latest status.
- Finish [ICS28](https://github.com/cosmos/ibc/pull/666) (Cross-chain validation) spec.
- Review and possibly merge [ICS721](https://github.com/cosmos/ibc/pull/615) spec for NFT transfers.
- Review and possibly merge the spec for [IBC queries](https://github.com/cosmos/ibc/pull/647).
- Write and add to the repository a high level overview of what IBC is. This can be used as an entry point for newcomers to IBC to understand its general principles.