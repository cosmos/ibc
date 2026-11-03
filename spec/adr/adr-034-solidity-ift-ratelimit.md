<!-- SPDX-License-Identifier: Apache-2.0 -->

# ADR 034: Rate Limiting in IFT in Solidity

**Status**: Accepted
**Date**: Sep 10, 2026
**Last Updated**: Sep 24, 2026

## Context

IFT (Interchain Fungible Token) requires rate limiting to prevent funds from being drained quickly in the event of an IBC exploit. In this document, we document the reasons behind the rate limiting design.

The objective is to bound how quickly tokens can be minted or moved across chains, giving operators time to detect an exploit and respond. Rate limiting cannot establish that an incoming transfer is legitimate, and an attacker can continue using capacity as it refills until the affected route is disabled.

This ADR describes the proposed IFT policy. The existing [IFT base contract](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/utils/IFTBaseUpgradeable.sol) burns on send and mints on receive, but does not yet enforce this policy. The [ICS20 escrow rate limiter](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/utils/RateLimitUpgradeable.sol) uses a different, fixed-window design.

## Summary

|  **Category**   |    **Option**    |                                                                                                 **Meaning**                                                                                                  | **Decision** |                                                         **Reason**                                                         |
|:---------------:|:----------------:|:------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------:|:------------:|:--------------------------------------------------------------------------------------------------------------------------:|
|  **Algorithm**  |   Fixed Window   |                                                                    Usage resets to zero at a period boundary (e.g. 00:00 UTC or every 6h)                                                                    |      ❌       |                                                    Not supported by OZ.                                                    |
|  **Algorithm**  |  Sliding Window  |                                                    Every transfer is timestamped and the sum over the trailing window seconds must stay under the limit.                                                     |      ❌       |                                          Consumes more gas than refilling bucket.                                          |
|  **Algorithm**  | Refilling Bucket | A bucket of fixed capacity is drained by each transfer and refills continuously at `capacity / window` per second. Bursts up to the capacity are allowed, and at most `2 × capacity` can pass in any window. |      ✅       |                                                  Consumes the least gas.                                                   |
|   **Feature**   | Static Capacity  |                                                                             Limit is a fixed token amount set by the authority.                                                                              |      ✅       |       Easy to reason about in token or dollar terms. Simpler to implement: no supply snapshots or bootstrap floors.        |
|   **Feature**   |    % Capacity    |                                                                            Limit is a share of local supply / TVL (self-scaling).                                                                            |      ❌       |       Harder to reason about because local supply fluctuates. Requires safe supply snapshots and a bootstrap floor.        |
| **Improvement** |  Capacity Floor  |                                                                      Limit is the `max(floor, %capacity)` in case %capacity is too low.                                                                      |      ❌       |                                      Only needed with % capacity, which was rejected.                                      |
|   **Feature**   |     Inbound      |                                                                                        Mints on receive are limited.                                                                                         |      ✅       |                                         We want to be as restrictive as possible.                                          |
|   **Feature**   |     Outbound     |                                                                                          Burns on send are limited.                                                                                          |      ✅       |                                         We want to be as restrictive as possible.                                          |
|   **Feature**   | Rewinding Usage  |                                                       Flow in the opposite direction gives capacity back, so round trips cannot exhaust the allowance.                                                       |      ❌       | Rewinding usage means that an attacker who can mint tokens can send outbound transfers to gain some capacity back to mint. |
|   **Feature**   | Refund Handling  |                                                                    A timeout or error acknowledgement does not consume inbound allowance.                                                                    |      ❌       | An attacker who can forge timeout proofs could time out their own transfers and double spend the refunded tokens. |
|    **Scope**    |     Optional     |                                                                                   Whether or not rate limits are optional                                                                                    |      ❌       |    Rate limits MUST be set before the token can be used. OZ default. Historically, when optional, users don't set this.    |
|    **Scope**    | Per Token/Client |                                                              Whether or not rate limits are per token or per IBC light client (IBC connection)                                                               |      ✅       |         Rate limits will be per token to be as restrictive as possible. Does not preclude per-client limits later.         |

## Decisions

### Algorithm

**Decision: use a refilling bucket.** Each transfer consumes an amount of available capacity equal to its token amount. Capacity recovers with elapsed time, up to the configured maximum. The contract only needs to account for that recovery when it reads or updates the bucket; no periodic transaction is required.

For an unchanged capacity `C`, refill window `W`, available amount `A`, and elapsed time `dt`, the conceptual accounting is:

```text
available = min(C, A + C * dt / W)
require(amount <= available)
available = available - amount
```

The window is the time needed to refill an empty bucket, not a promise that only `C` tokens can pass in every trailing window. Over an interval of length `T`, the upper bound is `C + C * T / W`: the initial burst plus the refill. For example, a capacity of 1,000 tokens and a one-hour window permits 1,000 tokens immediately from a full bucket, followed by another 500 after half an hour. Up to 2,000 tokens can pass over an hour if the refill is fully consumed. This bound assumes capacity and refill parameters do not change during the interval.

An exact sliding window provides a stricter guarantee: no more than the limit can pass in any trailing window. However, recording transfers and expiring their contributions requires more storage and processing as activity grows. Coarser time buckets can reduce that cost, but introduce approximation and additional accounting choices. We choose the refilling bucket for predictable accounting cost and gradual recovery, accepting the weaker trailing-window bound.

### Capacity

**Decision: use a static capacity.** The authority sets the limit as a fixed token amount together with its refill window. The same capacity and window apply to both directions (see [Inbound and Outbound](#inbound-and-outbound)). Operators typically know what their token is worth, so they can express a limit directly as the maximum amount that may cross the bridge over a window. A limit expressed as a percentage of local supply is harder to reason about, since local supply differs per chain and changes over time.

We considered deriving capacity from a percentage of local token supply so that the limit scales with the token without operator intervention. That design needs a snapshot of local supply, because reading live `totalSupply()` would let fraudulent mints raise their own capacity. It also needs an absolute floor, because a newly deployed token with zero local supply would otherwise have zero inbound capacity and could never receive its first transfer. Both mechanisms add implementation complexity and still require operators to pick static amounts for the floor. A static capacity avoids these problems and keeps the bound in the algorithm section simple to apply.

The trade-off is that a static limit does not track growth of the token and may require adjustment as usage changes. Operators may also be tempted to set a very large limit to avoid engaging with rate limiting. Since limits are mandatory, the authority must consciously choose a value, and it can adjust the capacity later.

### Inbound and Outbound

**Decision: limit both inbound mints and outbound burns, with a common capacity and window.** The authority configures a single capacity and refill window that applies to both directions. Each direction still tracks its own usage, shared across all clients for that token. An inbound transfer consumes inbound allowance; an outbound transfer consumes outbound allowance. Receiving tokens does not spend the allowance needed to send tokens, and vice versa.

An outbound transfer that exceeds the limit reverts, so the packet is never sent. An inbound transfer that exceeds the limit fails the receive and results in an error acknowledgement rather than a reverted transaction. The packet is not left pending for a later retry. The error acknowledgement refunds the sender on the source chain, and that refund is subject to the source chain's inbound limit (see [Refund Handling](#refund-handling)).

Sharing the settings keeps configuration to a single call and a single value to reason about. The trade-off is that operators cannot tighten one direction without tightening the other.

### Rewinding Usage

**Decision: do not restore allowance in response to transfers in the opposite direction.** The limiter tracks gross activity in each direction rather than net flow. With unchanged parameters, allowance recovers through time-based refill only.

Rewinding guards against round trips that would otherwise consume allowance in both directions. For example, without rewinding, if a user sends 100 tokens back and forth, then they can continue to do so until the entire allowance is consumed. Note that with refilling buckets, the allowance continuously recovers.

However, rewinding usage creates a security risk. An attacker who can cause fraudulent mints could mint until they hit the rate limit, then send them to another chain, and recover allowance to continue minting. The local burn can result in a mint elsewhere, so the attacker has moved the tokens while regaining permission to mint locally.

### Refund Handling

**Decision: refunds consume inbound allowance like any other mint.** A timeout or error acknowledgement mints the sender's tokens back locally. The contract only refunds amounts it recorded as pending when they were burned, so a refund can never mint more than previously left this chain, and that amount was already charged to the outbound limit. At first glance this makes exempting refunds from the inbound limit look safe, and it would spare users from having a full inbound bucket delay their refund.

However, an exemption gives an attacker more freedom in how they consume the inbound and outbound limits. Consider an attacker who can forge timeout proofs against the local light client. They send tokens to the counterparty chain, where the transfer completes and the tokens are minted. They then present a forged timeout for the same packets on the original chain and receive a refund. The refunds themselves are bounded by the outbound limit, since each one matches an earlier burn, but the attacker now holds the tokens on both chains. Finally, they send the counterparty tokens back through a regular IFT transfer, which mints on the original chain a second time. The result is a double spend on the original chain, and with exempt refunds only the final transfer would have been subject to the inbound limit, not the refunds.

Charging refunds to the inbound bucket makes both mints count, so the damage is bounded in the same way as any other fraudulent mint. A refund that does not fit in the bucket reverts and the pending transfer stays recorded, so the relayer can retry it once the bucket has refilled. Consistent with the no-rewinding decision, a refund does not restore outbound allowance.

We charge refunds to be as restrictive as possible. If this proves too inconvenient for users, the policy can be relaxed: even when refunds are charged, an attacker who can forge timeouts still holds the tokens on both chains and can double spend on the counterparty chain by sending the refunded tokens out again. Rate limits only bound the damage in either case.

### Per Token vs Per Client

**Decision: share each token's rate limits across all of its registered IBC clients on a given chain.** Here, “per token” means per local IFT contract. Separate tokens have separate budgets, and deployments on other chains enforce their own limits; this does not create a synchronized global bucket.

Given that the IFT contract is the authority for its own supply, it is reasonable to treat all clients as a single source of demand. This avoids multiplying the token's aggregate allowance by the number of clients.

On the other hand, per client limits would allow implementers to configure different limits on fast paths versus slow and more secure paths. Operators may also trust some counterparty chains more than others and want tighter limits on inflows from the less trusted ones. However, the added complexity of per-client accounting is not justified until we have such use cases.

Choosing per token limits now does not prevent us from adding per client limits in the future. Per client buckets can be layered on top of the per token bucket, so that a transfer must fit within both. Note that because rate limits are mandatory, a per client limit would make a newly registered client unusable until the authority configures its limit. This is a further reason to start with per token limits.

## Implementation Notes

The policy is implemented in [`IFTRateLimitUpgradeable`](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/utils/IFTRateLimitUpgradeable.sol) on top of OpenZeppelin's `RateLimiter.RefillingBucket`. A single limiter holds the shared capacity and window, and each direction is a separate entry in it, keyed by the direction. An unset limiter has zero capacity, which is what makes the limits mandatory.

- **Capacity updates:** Before the authority changes the capacity or window, both directions are synced: the refill accrued under the old rate is applied and the usage timestamp is moved to now. Consumed usage is therefore preserved and the new rate only applies going forward. Lowering the capacity below a direction's current usage leaves that direction empty until its usage drains at the new rate.
