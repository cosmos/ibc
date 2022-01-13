# Roadmap IBC specs

_Lastest update: November 18th, 2021_

This document endeavours to inform the wider IBC community about plans and priorities for the specification work of IBC. This roadmap should be read as a high-level guide, rather than a commitment to schedules and deliverables. The degree of specificity is inversely proportional to the timeline. We will update this document periodically to reflect the status and plans.
 
This roadmap reflects the major activities that the [standards committee](STANDARDS_COMMITTEE.md) is engaged with in the coming quarters. It is, by no means, a thorough reflection of all the specification work that is happening in the broad ecosystem, as many other parties work as well in specs that eventually end up in this repository.

## Q4 - 2021

- Update the Interchain Accounts ([ICS27](https://github.com/cosmos/ibc/blob/master/spec/app/ics-027-interchain-accounts/README.md)) and Relayer Incentivisation ([ICS29](https://github.com/cosmos/ibc/tree/master/spec/app/ics-029-fee-payment)) specifications to align them with the ibc-go implementation.
- The function `NegotiateAppVersion` has been [added to the app module interface in ibc-go](https://github.com/cosmos/ibc-go/pull/384) as part of the Interchain Accounts work. This change is not reflected yet in the [ICS05](https://github.com/cosmos/ibc/blob/master/spec/core/ics-005-port-allocation/README.md) specification. An update is needed to describe its semantics beyond the Interchain Accounts use case.
- Review [IRISnet](https://www.irisnet.org)'s [ICS721](https://github.com/cosmos/ibc/pull/615) specification proposal for NFT transfers.
- Rough draft of channel upgrade process (potentially connection as well).

## Q1 - 2022

- Possibly finalize the review and merge of [ICS721](https://github.com/cosmos/ibc/pull/615) (NFT tranfers) specification.
- Continue review and advisory work on [Informal Systems](https://informal.systems)' [specification proposal for cross-chain validation](https://github.com/cosmos/ibc/pull/563).
- Begin a re-write of IBC specifications to make them easier to understand to qualified developers trying to implement IBC in other ecosystems. This will most likely be a multi-quarter effort.