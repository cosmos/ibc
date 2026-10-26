---
title: "Upgrade the router contract"
description: "Switch the router proxies on the tutorial's two chains to freshly deployed ICS26Router logic, then confirm the router runs it and packets still flow."
---

This guide upgrades the router on a running two-chain IBC deployment to new logic (the implementation contract behind the router's proxy). The new logic is built from the same source the tutorial deployed, so the steps show the upgrade itself rather than a change in behavior. Each router keeps its address and its state, so the clients, the applications, and the running relayer need no change.

## Before you begin

Complete the [Deploy IBC and send a token](2-tutorial-deploy-ibc-and-send-a-token.md) tutorial first. This guide starts from the state it leaves:

- IBC is deployed on chains 41001 and 41002.
- At least one token transfer has been delivered.
- The relayer is running in its own terminal.

You also need:

- [Foundry](https://getfoundry.sh/introduction/installation), for `forge` and `cast`.
- [Bun](https://bun.com/docs/installation), to install the dependencies the contracts import.
- The access manager's admin key. The tutorial's deployer key holds that role with no [execution delay](../5-ibc-solidity-contracts/6-permissions-and-upgrades.md#the-access-manager-and-its-roles), so each upgrade call runs immediately.

Run every command from the `cli` directory of your IBC clone.

## 1. Set your shell variables

Read the values the steps use:

```bash
ROUTER_A=$(./bin/ibc deploy show 41001 | jq -r '.core.router')
ROUTER_B=$(./bin/ibc deploy show 41002 | jq -r '.core.router')
IFT_A=$(./bin/ibc deploy show 41001 | jq -r '.tokens[0].address')
IFT_B=$(./bin/ibc deploy show 41002 | jq -r '.tokens[0].address')
DEPLOYER_KEY=$(./bin/ibc keys show deployer --private | jq -r '.privateKey')
```

The last line reads the deployer's raw private key, because Foundry cannot read the CLI's key files. Run `unset DEPLOYER_KEY` when you finish, so the key does not stay in your shell.

## 2. Get the router source

This guide builds the new logic from the [ibc-contracts](https://github.com/cosmos/ibc-contracts) commit the IBC CLI pins, which is the source of the logic `ibc deploy core` deployed.

For your own upgrade, check out the release you are shipping. Compare its `IBCRolesLib` and `DeployAccessManagerWithRoles` with your deployment's assignments. If the release newly marks a function `restricted`, assign its selector before you call `upgradeToAndCall`. The old logic does not consult the access manager for that function, so the assignment changes nothing until the upgrade, and callers the release admits never lose access.

1. Read the commit the IBC CLI pins:

```bash
CONTRACTS_COMMIT=$(go list -m -f '{{.Version}}' github.com/cosmos/ibc-contracts/packages/go-abigen | cut -d- -f3) && echo "$CONTRACTS_COMMIT"
```

The IBC CLI pins the module at a pseudo-version, which ends in a 12-character commit hash. The command prints that hash.

2. Clone the repository next to your IBC clone:

```bash
git clone https://github.com/cosmos/ibc-contracts.git ../../ibc-contracts
```

3. Check out the commit:

```bash
git -C ../../ibc-contracts checkout "$CONTRACTS_COMMIT"
```

4. Install the contracts' dependencies:

```bash
bun install --cwd ../../ibc-contracts/ibc-solidity --frozen-lockfile
```

## 3. Check the router before the upgrade

Read which logic the router proxy on 41001 runs:

```bash
cast implementation "$ROUTER_A" --rpc-url http://localhost:8545
```

It prints the `targetData.ics26RouterImplementation` address from `./bin/ibc deploy show 41001`, in lowercase.

## 4. Upgrade the router on each chain

1. Build and deploy the new logic on 41001:

```bash
NEW_IMPL_A=$(forge create --root ../../ibc-contracts/ibc-solidity contracts/ICS26Router.sol:ICS26Router --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY" --broadcast --json | jq -r '.deployedTo') && echo "$NEW_IMPL_A"
```

<Warning>
Upgrade only to logic written for the state the proxy already holds. The proxy keeps its storage and runs the new code against it. No on-chain check compares the two storage layouts. Logic that lays out storage differently reads the wrong values for the router's clients, applications, and packet commitments.
</Warning>

The new logic is built from the same source as the logic the tutorial deployed, so its storage layout is the same.

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

Packets in flight while only one chain is upgraded are still delivered, because both logic contracts are built from the same source.

## 5. Verify the upgrade

1. Read which logic each router now runs:

```bash
cast implementation "$ROUTER_A" --rpc-url http://localhost:8545
```

```bash
cast implementation "$ROUTER_B" --rpc-url http://localhost:8745
```

They print the addresses in `NEW_IMPL_A` and `NEW_IMPL_B`, in lowercase, so each proxy now runs the new logic. The record from `./bin/ibc deploy show` still lists the original logic, so read the proxy for the current address.

2. Check that both routers still hold their clients and applications:

```bash
./bin/ibc deploy status | jq '[.[].checks[].status] | unique'
```

```json
[
  "ok"
]
```

3. Send a token across the upgraded routers:

```bash
TX=$(./bin/ibc tx ift send --chain 41001 --ift "$IFT_A" --client-id cli-41001-41002 --to deployer --from deployer --amount 10000000000000000000 | jq -r '.txHash') && echo "$TX"
```

4. Check the transfer's state:

```bash
./bin/ibc relayer packets --chain-id 41001 --tx-hash "$TX" | jq -r '.packets[0] | .sequenceNumber + " " + .state'
```

It reads `PACKET_STATE_PENDING` for up to a minute, then:

```
2 PACKET_STATE_SUCCEEDED
```

Sequence 2 is the first packet both upgraded routers handled. These values assume the tutorial's transfer is the only one so far. Each earlier transfer, such as the one in [Grant, change, and revoke a role](9-grant-change-and-revoke-a-role.md), raises the sequence by one and the balance below by its amount.

5. Confirm the tokens arrived on 41002:

```bash
./bin/ibc query ift balance --chain 41002 --ift "$IFT_B" --address deployer | jq -r '.balance'
```

```
20000000000000000000
```

That is the tutorial's 10 tokens plus this transfer's 10.
