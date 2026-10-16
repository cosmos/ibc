---
title: "Upgrade the router contract"
description: "Deploy new ICS26Router logic behind the existing router proxies on the tutorial's two chains, then confirm the router runs it and packets still flow."
---

This guide upgrades the router on both chains of the [tutorial](2-tutorial-deploy-ibc-and-send-a-token.md)'s deployment to new logic (the implementation contract behind the router's proxy). Each router keeps its address and its state, so the clients, the applications, and the running relayer need no change.

## Before you begin

This guide starts where the tutorial ends:

- IBC is deployed on chains 41001 and 41002.
- One token transfer has been delivered.
- The relayer is running in its own terminal.

You need:

- The Deploy IBC and send a token tutorial, completed.
- [Foundry](https://getfoundry.sh/introduction/installation), for `forge` and `cast`.
- [Bun](https://bun.com/docs/installation), to install the dependencies the contracts import.
- The access manager's admin key. The tutorial's deployer key holds that role with no execution delay, so each upgrade call runs immediately.

An admin granted the role with an execution delay must schedule each call first, as [the access manager and its roles](../5-ibc-solidity-contracts/6-permissions-and-upgrades.md#the-access-manager-and-its-roles) describes.

Run every command from the `cli` directory of your IBC clone.

## 1. Set your shell variables

Read the addresses and the deployer key that the steps use:

```bash
ROUTER_A=$(./bin/ibc deploy show 41001 | jq -r '.core.router')
ROUTER_B=$(./bin/ibc deploy show 41002 | jq -r '.core.router')
IFT_A=$(./bin/ibc deploy show 41001 | jq -r '.tokens[0].address')
IFT_B=$(./bin/ibc deploy show 41002 | jq -r '.tokens[0].address')
RELAYER=$(./bin/ibc keys show relayer | jq -r '.evmAddress')
DEPLOYER_KEY=$(./bin/ibc keys show deployer --private | jq -r '.privateKey')
```

The last line prints the deployer's private key, because `forge` and `cast` sign with a raw key.

## 2. Get the new router source

The new logic comes from the [ibc-contracts](https://github.com/cosmos/ibc-contracts) repository. This guide builds the router from the latest `main` branch. On `main`, the router restricts two functions the tutorial leaves open to every address. They are `addIBCApp(address)` and the `addClient` form that assigns the client ID itself. For your own upgrade, check out the release you are shipping.

1. Clone the repository next to your IBC clone. The clone checks out `main`:

```bash
git clone https://github.com/cosmos/ibc-contracts.git ../../ibc-contracts
```

2. Install the contracts' dependencies from the repository's lockfile:

```bash
bun install --cwd ../../ibc-contracts/ibc-solidity --frozen-lockfile
```

## 3. Check the current behavior

1. Read which logic the router proxy on 41001 runs:

```bash
cast implementation "$ROUTER_A" --rpc-url http://localhost:8545
```

It prints the `ics26RouterImplementation` address from `./bin/ibc deploy show 41001`, in lowercase.

2. Simulate a call to `addIBCApp(address)` from the relayer's account, which is not an admin:

```bash
cast call "$ROUTER_A" "addIBCApp(address)" "$RELAYER" --from "$RELAYER" --rpc-url http://localhost:8545
```

```
0x
```

The call succeeds. The simulation sends no transaction, so nothing is registered.

## 4. Upgrade the router on each chain

1. Build and deploy the new logic on 41001:

```bash
NEW_IMPL_A=$(forge create --root ../../ibc-contracts/ibc-solidity contracts/ICS26Router.sol:ICS26Router --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY" --broadcast --json | jq -r '.deployedTo') && echo "$NEW_IMPL_A"
```

<Warning>
Upgrade only to logic written for the state the proxy already holds. The proxy keeps its storage and runs the new code against it. No on-chain check compares the two storage layouts. Logic that lays out storage differently misreads the router's clients, applications, and packet commitments.
</Warning>

Between the router the tutorial deploys and `main`, the router gains only access checks, so its storage layout stays the same.

2. Point the 41001 router at it:

```bash
cast send "$ROUTER_A" "upgradeToAndCall(address,bytes)" "$NEW_IMPL_A" 0x --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY"
```

The receipt it prints has a `status` line reading `1 (success)`. The `0x` argument is the call data the proxy runs against the new logic after switching. Empty call data runs nothing, and this upgrade needs no migration call.

3. Build and deploy the new logic on 41002:

```bash
NEW_IMPL_B=$(forge create --root ../../ibc-contracts/ibc-solidity contracts/ICS26Router.sol:ICS26Router --rpc-url http://localhost:8745 --private-key "$DEPLOYER_KEY" --broadcast --json | jq -r '.deployedTo') && echo "$NEW_IMPL_B"
```

4. Point the 41002 router at it:

```bash
cast send "$ROUTER_B" "upgradeToAndCall(address,bytes)" "$NEW_IMPL_B" 0x --rpc-url http://localhost:8745 --private-key "$DEPLOYER_KEY"
```

The relayer keeps running throughout. It calls each router at its proxy address, which the upgrade leaves unchanged. Packets in flight while only one chain is upgraded are unaffected, because the router's packet functions are the same in both versions.

## 5. Verify the upgrade

1. Read which logic each router now runs:

```bash
cast implementation "$ROUTER_A" --rpc-url http://localhost:8545
```

```bash
cast implementation "$ROUTER_B" --rpc-url http://localhost:8745
```

They print the addresses in `NEW_IMPL_A` and `NEW_IMPL_B`, in lowercase. The record from `./bin/ibc deploy show` still lists the original logic under `targetData.ics26RouterImplementation`, so read the proxy for the current address.

2. Repeat the simulated call from step 3:

```bash
cast call "$ROUTER_A" "addIBCApp(address)" "$RELAYER" --from "$RELAYER" --rpc-url http://localhost:8545
```

```
Error: server returned an error response: error code -32000: Execution reverted, data: "0x068ca9d8<...>"
```

The data starts with `0x068ca9d8`, the selector of `AccessManagedUnauthorized(address)`, followed by the relayer's address. The call that succeeded before the upgrade now reverts, so the router runs the new logic.

From here on, only the admin can call these two functions on these chains. The access manager admits only the admin to a `restricted` function with no assigned role. The CLI's `deploy client` and `deploy gmp` commands register through other forms of these functions, which were already admin-only, so they work as before.

3. Check that both routers still hold their clients and applications:

```bash
./bin/ibc deploy status | jq '[.[].checks[].status] | unique'
```

```json
[
  "ok"
]
```

It checks every recorded client, application, and token bridge on both chains. If one fails, the command exits with an error.

4. Send a token across the upgraded routers:

```bash
TX=$(./bin/ibc tx ift send --chain 41001 --ift "$IFT_A" --client-id cli-41001-41002 --to deployer --from deployer --amount 10000000000000000000 | jq -r '.txHash') && echo "$TX"
```

5. Check the transfer's state:

```bash
./bin/ibc relayer packets --chain-id 41001 --tx-hash "$TX" | jq -r '.packets[0] | .sequenceNumber + " " + .state'
```

It reads `PACKET_STATE_PENDING` for up to a minute, then:

```
2 PACKET_STATE_SUCCEEDED
```

Sequence 2 is the first packet both upgraded routers handled.

6. Confirm the tokens arrived on 41002:

```bash
./bin/ibc query ift balance --chain 41002 --ift "$IFT_B" --address deployer | jq -r '.balance'
```

```
20000000000000000000
```

That is the tutorial's 10 tokens plus this transfer's 10.

## Next steps

[Permissions and upgrades](../5-ibc-solidity-contracts/6-permissions-and-upgrades.md#proxies-and-upgrade-authority) lists who may upgrade the other proxied contracts.