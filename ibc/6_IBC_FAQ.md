# 6: IBC Frequently-Asked Questions

**This is a set of frequently asked questions about the IBC protocol along with their answers.**

## Forks & unbonding periods

*What happens to all of the established IBC channels if a chain forks?*

This depends on the light client algorithm. Tendermint light clients, at the moment, will halt the channel completely if a fork is detected (since it looks like equivocation) - if the fork doesn't use any sort of replay protection (e.g. change the chain ID). If one fork keeps the chain ID and the other picks a new one, the one which keeps it would be followed by the light client. If both forks change the chain ID (or validator set), they would both need new light clients.

*What happens after the unbonding period passes without an IBC packet to renew the channel? Are the escrowed tokens un-recoverable without intervention?*

By default, the tokens are un-recoverable. Governance intervention could alter the light client associated with the channel (there is no way to automate this that is safe). That said, it's always possible to construct light clients with different validation rules or to add the ability for a government proposal to reset the light client to a trusted header if it was previously valid and used, and if it was frozen due to the unbonding period.

## Data flow & packet relay

*Does Blockchain A need to know the address of a trustworthy node for Blockchain B in order to send IBC packets?*

Blockchain A will know of the existence of Blockchain B after a kind of handshake takes place. This handshake is facilitated by a relayer. It is the responsibility of the relayer to access an available node of the corresponding blockchain to begin the handshake. The blockchains themselves need not know about nodes, just be able to access the transactions that are relayed between them.
