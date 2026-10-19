---
title: "Upgrade the router contract"
description: "Deploy new ICS26Router logic behind the existing router proxies on the tutorial's two chains, then confirm the router runs it and packets still flow."
---

This guide upgrades the router on a running two-chain IBC deployment to new logic (the implementation contract behind the router's proxy). Each router keeps its address and its state, so the clients, the applications, and the running relayer need no change.

## Before you begin

Complete the [Deploy IBC and send a token](2-tutorial-deploy-ibc-and-send-a-token.md) tutorial first. This guide starts from the state it leaves:

- IBC is deployed on chains 41001 and 41002.
- One token transfer has been delivered.
- The relayer is running in its own terminal.

You also need:

- [Foundry](https://getfoundry.sh/introduction/installation), for `forge` and `cast`.
- [Bun](https://bun.com/docs/installation), to install the dependencies the contracts import.
- The access manager's admin key. The tutorial's deployer key holds that role with no [execution delay](../5-ibc-solidity-contracts/6-permissions-and-upgrades.md#the-access-manager-and-its-roles), so each upgrade call runs immediately.

Run every command from the `cli` directory of your IBC clone.

## 1. Set your shell variables

Read the values the steps use:

```bash
AM_A=$(./bin/ibc deploy show 41001 | jq -r '.targetData.accessManager')
AM_B=$(./bin/ibc deploy show 41002 | jq -r '.targetData.accessManager')
ROUTER_A=$(./bin/ibc deploy show 41001 | jq -r '.core.router')
ROUTER_B=$(./bin/ibc deploy show 41002 | jq -r '.core.router')
IFT_A=$(./bin/ibc deploy show 41001 | jq -r '.tokens[0].address')
IFT_B=$(./bin/ibc deploy show 41002 | jq -r '.tokens[0].address')
RELAYER=$(./bin/ibc keys show relayer | jq -r '.evmAddress')
DEPLOYER_KEY=$(./bin/ibc keys show deployer --private | jq -r '.privateKey')
```

The last line reads the deployer's raw private key, because Foundry cannot read the CLI's key files.

## 2. Get the new router source

This guide builds the new logic from commit `a99cb151` of [ibc-contracts](https://github.com/cosmos/ibc-contracts). At that commit, two more router functions are `restricted`: `addIBCApp(address)`, and the `addClient` overload in which the router picks the client ID. The release keeps both public: its `DeployAccessManagerWithRoles` script assigns them to `PUBLIC_ROLE`. The CLI's deployment assigns them no role, so after the upgrade only the admin can call them until step 6 assigns them to `PUBLIC_ROLE`. For your own upgrade, check out the release you are shipping. Compare its `IBCRolesLib` and `DeployAccessManagerWithRoles` with your deployment's assignments, and assign any selector the release newly restricts.

1. Clone the repository next to your IBC clone:

```bash
git clone https://github.com/cosmos/ibc-contracts.git ../../ibc-contracts
```

2. Check out the commit:

```bash
git -C ../../ibc-contracts checkout a99cb151aee41fc768d726d47ef82f61bf46310e
```

3. Install the contracts' dependencies:

```bash
bun install --cwd ../../ibc-contracts/ibc-solidity --frozen-lockfile
```

## 3. Check the router before the upgrade

1. Read which logic the router proxy on 41001 runs:

```bash
cast implementation "$ROUTER_A" --rpc-url http://localhost:8545
```

It prints the `targetData.ics26RouterImplementation` address from `./bin/ibc deploy show 41001`, in lowercase.

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
Upgrade only to logic written for the state the proxy already holds. The proxy keeps its storage and runs the new code against it. No on-chain check compares the two storage layouts. Logic that lays out storage differently reads the wrong values for the router's clients, applications, and packet commitments.
</Warning>

The router at `a99cb151` differs from the tutorial's only by those two access checks, so its storage layout is the same.

2. Switch the 41001 router to `NEW_IMPL_A`:

```bash
cast send "$ROUTER_A" "upgradeToAndCall(address,bytes)" "$NEW_IMPL_A" 0x --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY"
```

The receipt it prints has a `status` line reading `1 (success)`. The final `0x` is empty call data, because this upgrade runs no migration.

3. Build and deploy the new logic on 41002:

```bash
NEW_IMPL_B=$(forge create --root ../../ibc-contracts/ibc-solidity contracts/ICS26Router.sol:ICS26Router --rpc-url http://localhost:8745 --private-key "$DEPLOYER_KEY" --broadcast --json | jq -r '.deployedTo') && echo "$NEW_IMPL_B"
```

4. Switch the 41002 router to `NEW_IMPL_B`:

```bash
cast send "$ROUTER_B" "upgradeToAndCall(address,bytes)" "$NEW_IMPL_B" 0x --rpc-url http://localhost:8745 --private-key "$DEPLOYER_KEY"
```

Packets in flight while only one chain is upgraded are still delivered, because the router's packet functions are the same in both versions.

## 5. Verify the upgrade

1. Read which logic each router now runs:

```bash
cast implementation "$ROUTER_A" --rpc-url http://localhost:8545
```

```bash
cast implementation "$ROUTER_B" --rpc-url http://localhost:8745
```

They print the addresses in `NEW_IMPL_A` and `NEW_IMPL_B`, in lowercase. The record from `./bin/ibc deploy show` still lists the original logic, so read the proxy for the current address.

2. Repeat the simulated call from step 3:

```bash
cast call "$ROUTER_A" "addIBCApp(address)" "$RELAYER" --from "$RELAYER" --rpc-url http://localhost:8545
```

```
Error: server returned an error response: error code -32000: Execution reverted, data: "0x068ca9d8<...>"
```

The data starts with `0x068ca9d8`, the selector of `AccessManagedUnauthorized(address)`, followed by the relayer's address. The call that succeeded before the upgrade now reverts, so the router runs the new logic. Step 6 makes it public again, as the release intends.

The CLI's `deploy client` and `deploy gmp` commands use other overloads of these functions. Those were already admin-only, so the commands still succeed.

3. Check that both routers still hold their clients and applications:

```bash
./bin/ibc deploy status | jq '[.[].checks[].status] | unique'
```

```json
[
  "ok"
]
```

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

Sequence 2 is the first packet both upgraded routers handled. These values assume the tutorial's transfer is the only one so far. Each earlier transfer, such as the one in [Grant, change, and revoke a role](9-grant-change-and-revoke-a-role.md), raises the sequence by one and the balance below by its amount.

6. Confirm the tokens arrived on 41002:

```bash
./bin/ibc query ift balance --chain 41002 --ift "$IFT_B" --address deployer | jq -r '.balance'
```

```
20000000000000000000
```

That is the tutorial's 10 tokens plus this transfer's 10.

## 6. Assign the newly restricted functions to the public role

The release's role map keeps `addIBCApp(address)` and the auto-ID `addClient` open to every address. Assign both selectors to `PUBLIC_ROLE` on each chain, as its `DeployAccessManagerWithRoles` script does.

1. Compute the two selectors:

```bash
PUBLIC_FNS="[$(cast sig 'addIBCApp(address)'),$(cast sig 'addClient((string,bytes[]),address)')]"
```

2. Assign them to `PUBLIC_ROLE` on 41001:

```bash
cast send "$AM_A" "setTargetFunctionRole(address,bytes4[],uint64)" "$ROUTER_A" "$PUBLIC_FNS" 18446744073709551615 --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY"
```

3. Do the same on 41002:

```bash
cast send "$AM_B" "setTargetFunctionRole(address,bytes4[],uint64)" "$ROUTER_B" "$PUBLIC_FNS" 18446744073709551615 --rpc-url http://localhost:8745 --private-key "$DEPLOYER_KEY"
```

4. Repeat the simulated call from step 3:

```bash
cast call "$ROUTER_A" "addIBCApp(address)" "$RELAYER" --from "$RELAYER" --rpc-url http://localhost:8545
```

```
0x
```

The call succeeds again. The router still runs the new logic, and the access manager now lets any address call the function.
