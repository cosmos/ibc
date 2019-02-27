---
ics: 4
title: IBC Channel Semantics
status: Proposal
category: IBC
author: Juwoon Yun <joon@tendermint.com>
created: 2019-02-27
---

## Abstract 

`Channel` is n-pair of `Queue` where each queue has different role. `Queue` is a data structure that is essentially a list with additional `top`. In the initial protocol, `Channel` will be defined as 2-pair of `Queue`, each named `PacketQueue` and `ReceiptQueue`. The behaviour of the queues are determined, and extending `Channel` requires update on the protocol.

## Channel

`Channel` is `(p Queue, r Queue)` where `p` is queue for packets and `r` is queue for receipts. `Channel` satisfies the followings:

1. 
