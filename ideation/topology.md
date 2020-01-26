Q: Why might one expect "hubs" (central sinks of trust) to emerge in the network, and what purpose might they serve?

A:

1. Economic nodes like exchanges, custodians find upgrades expensive and intrusive.
2. Counterparties do not like protocol upgrades for intermediate chains. Upgrades enforce high global costs.
3. No one wants to reason about the security of a global system. It’s much easier to reason about a small number of chains where you directly do business.

We believe the natural architecture of an IBC system is as follows.

Hubs are relatively simple blockchains that implement very little other than IBC protocols and slashing conditions. We believe that native token of a Hub will be incentivized to collect fees from the assets that it collateralizes the security of.

Zones are complex blockchains with complex business logic and frequent upgrade cycles. They will generally invent their own incentive models for validators.
