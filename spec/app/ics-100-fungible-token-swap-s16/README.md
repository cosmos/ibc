---
ics: 100
title: Fungible Token Swap
stage: draft
category: IBC/APP
requires: 25, 26
kind: instantiation
author: Ping Liang <18786721@qq.com>
created: 2022-07-27
modified: 2022-07-27
---

## Synopsis

This standard document specifies packet data structure, state machine handling logic, and encoding details for the atomic swap of fungible tokens over an IBC channel between two modules on separate chains.

### Motivation

Users may wish to exchange tokens without transfering tokens away from its native chain. ICS-100 enabled chains can facilitate atomic swaps between users and their tokens located on the different chains. This is useful for exchanges between specific users at specific prices, and opens opportunities for new application designs.

### Definitions

`Atomic Swap`: An exchange of tokens from separate chains without transfering tokens away from its native chain.

`Order`: an offer to exchange quantity X of token A for quantity Y of token B. Tokens offered are sent to an escrow account (owned by the module)

`Maker`: A user that makes or initiates an order.

`Taker`: Is the counterparty who takes or responds to an order.

### Desired Properties

-   `Permissionless`: no need to whitelist connections, modules, or denominations.
-   `Gaurantee of exchange`: no occurence of a user receiving tokens without the equivalent promised exchange.
-   `Escrow enabled`: an account owned by the module will hold tokens and facilitate exchange.
-   `Refundable`: tokens are refunded by escrow when an orders is cancelled
-   `Basic orderbook`: a store of orders functioning as an orderbook system
-   `Partial filled orders`: allows takers to partially fill an order by a maker

## Technical Specification

### General Design

A user offers tokens for exchange by making an order. The order specifies the quantity and price of exchange, and sends the offered tokens to the chain's escrow account.

Any user on a different chain with the correct token denomination can accept the offer by taking the order. The taker sends the desired amount of tokens to the chain's escrow account.

The escrow account on each respective chain transfers the corresponding token amounts to each user's receiving address on each token's native chain, without requiring the usual ibc transfer.
