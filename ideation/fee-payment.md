Notes from IBC ecosystem call:

1. Class zero: user = relayer
    - User relays their own transactions
    - Zero trust
    - Could run multiple light clients on a phone, auto-balance funds between fee paying accounts
1. Class one-half: wallet pays fees
    - Wallet operator charges extra initial fees, sends all the transactions
    - Limited trust in the operator
1. Class one: in-protocol, relayer market
    - Payment on original chain to relayers
    - Payment routed through multi-hop IBC transaction, little bits split off for each tx
    - Zero trust, probably
1. Class two: out-of-band, specific relayers
    - ILP payments to relayers which pay fees
    - Varying amounts of trust
1. Class three: no fees
    - DEX accepting incoming liquidity
    - Zero trust

Formats
1. Wrap datagram with fee payment
    1. Fee payment executed, then datagram
1. Bare datagram, no fees, external account req’d
    1. Datagram executed as normal

Further ideas from discussion in second ecosystem call:

- Use another chain to coordinate the relayers
- Slash if a leader fails to relay (after some time period)
- Prior fee payment sent to relayer chain
- Relayers maintain balance everywhere, payment channels
- Latency mechanism for relayer accountability, slashing on failure to meet latency obligations
- Chains negotiate on relayer payments with the relayer zone
- “IBC relayer fee negotiation protocol”
