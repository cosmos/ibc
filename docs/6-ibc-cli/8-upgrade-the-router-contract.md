---
title: "Upgrade an IBC contract"
description: "Upgrade an IBC Solidity contract using the router as an example."
---

This guide demonstrates a contract upgrade using the IBC router (`ICS26Router`) as an example. You will replace the implementation behind each router proxy on the tutorial's two chains while preserving each proxy's address and state.

## Before you begin

Complete the [Deploy IBC and send a token](2-tutorial-deploy-ibc-and-send-a-token.md) tutorial first. This guide starts from the state it leaves:

- IBC is deployed on chains 41001 and 41002.
- At least one token transfer has been delivered.
- The relayer is running in its own terminal.

You also need:

- [Foundry](https://getfoundry.sh/introduction/installation), for `forge` and `cast`.
- [Bun](https://bun.com/docs/installation), to install the dependencies the contracts import.
- The access manager's admin key. The tutorial's deployer key holds that role.

Run every command from the `cli` directory of your IBC clone.

## 1. Set your shell variables

Read the values the steps use:

```bash
AM_A=$(./bin/ibc deploy show 41001 | jq -r '.targetData.accessManager')
AM_B=$(./bin/ibc deploy show 41002 | jq -r '.targetData.accessManager')
ROUTER_A=$(./bin/ibc deploy show 41001 | jq -r '.core.router')
ROUTER_B=$(./bin/ibc deploy show 41002 | jq -r '.core.router')
IFT_A=$(./bin/ibc deploy show 41001 | jq -r '.tokens[0].address')
RELAYER=$(./bin/ibc keys show relayer | jq -r '.evmAddress')
DEPLOYER_KEY=$(./bin/ibc keys show deployer --private | jq -r '.privateKey')
```

The last line reads the deployer's raw private key, because Foundry cannot read the CLI's key files. Run `unset DEPLOYER_KEY` when you finish, so the key does not stay in your shell.

## 2. Get the router source

For a different target version, review the contracts repository's [upgradeability overview](https://github.com/cosmos/ibc-contracts/blob/main/UPGRADEABILITY.md#solidity-contracts) and [release policy](https://github.com/cosmos/ibc-contracts/blob/main/RELEASES.md#solidity-contracts).
The policy defines minor Solidity releases as requiring a new initializer for storage changes.
Patch releases require neither storage changes nor a new initializer.
Major releases introduce API-breaking changes.

Check the target release for required initialization and function-role changes.
See [Permissions and upgrades](../5-ibc-solidity-contracts/6-permissions-and-upgrades.md#proxies-and-upgrade-authority) for storage-layout and access-control considerations.

Clone the contracts and check out the commit pinned by the target CLI version.
For another release, replace the checkout commit below with that release's tag or commit.

1. Clone the repository next to your IBC clone:

```bash
git clone https://github.com/cosmos/ibc-contracts.git ../../ibc-contracts
```

2. Check out the contracts version used by the tutorial:

```bash
git -C ../../ibc-contracts checkout f564e6339cfd49498b043cc9fab770ed6f20ebaa
```

3. Install the contracts' dependencies:

```bash
bun install --cwd ../../ibc-contracts/ibc-solidity --frozen-lockfile
```

## 3. Upgrade the router on each chain

1. Build and deploy the new logic on 41001:

```bash
NEW_IMPL_A=$(forge create --root ../../ibc-contracts/ibc-solidity contracts/ICS26Router.sol:ICS26Router --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY" --broadcast --json | jq -r '.deployedTo') && echo "$NEW_IMPL_A"
```

<Warning>
When changing contract versions, verify storage compatibility and follow the release's initialization or migration requirements. The upgrade transaction does not check storage compatibility.
</Warning>

2. Switch the 41001 router to `NEW_IMPL_A`:

Use empty call data, `0x`, because this example redeploys the same contract version.
If your target release requires an initializer or migration function, encode that call and its arguments.
Pass the encoded data instead of `0x` in both upgrade calls.
With nonempty data, `upgradeToAndCall` executes that call in the same transaction.

```bash
cast send "$ROUTER_A" "upgradeToAndCall(address,bytes)" "$NEW_IMPL_A" 0x --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY"
```

The receipt it prints has a `status` line reading `1 (success)`.

3. Build and deploy the new logic on 41002:

```bash
NEW_IMPL_B=$(forge create --root ../../ibc-contracts/ibc-solidity contracts/ICS26Router.sol:ICS26Router --rpc-url http://localhost:8745 --private-key "$DEPLOYER_KEY" --broadcast --json | jq -r '.deployedTo') && echo "$NEW_IMPL_B"
```

4. Switch the 41002 router to `NEW_IMPL_B`:

```bash
cast send "$ROUTER_B" "upgradeToAndCall(address,bytes)" "$NEW_IMPL_B" 0x --rpc-url http://localhost:8745 --private-key "$DEPLOYER_KEY"
```

This example uses the same packet functions on both chains throughout the upgrade.

## 4. Verify the upgrade

1. Read which logic each router now runs:

```bash
cast implementation "$ROUTER_A" --rpc-url http://localhost:8545
```

```bash
cast implementation "$ROUTER_B" --rpc-url http://localhost:8745
```

They must match `NEW_IMPL_A` and `NEW_IMPL_B`.

2. Send 10 tokens across the upgraded routers:

```bash
TX=$(./bin/ibc tx ift send --chain 41001 --ift "$IFT_A" --client-id cli-41001-41002 --to deployer --from deployer --amount 10000000000000000000 | jq -r '.txHash') && echo "$TX"
```

3. Poll for `PACKET_STATE_SUCCEEDED`:

```bash
./bin/ibc relayer packets --chain-id 41001 --tx-hash "$TX" | jq -r '.packets[0].state'
```

Clear the admin key when finished:

```bash
unset DEPLOYER_KEY
```

<!-- Verified 2026-10-22 against CLI code from main
1e957fe1f95ca4ba1433451c8c190a82f186d9c2 and contracts
f564e6339cfd49498b043cc9fab770ed6f20ebaa. The commands were exercised on
isolated Besu 25.4.0 chains: both implementation addresses changed, deployment
checks passed, and a transfer increased the destination balance by ten tokens.
Deployment manifests remained unchanged. Upgrade and release-policy guidance
was checked against UPGRADEABILITY.md, RELEASES.md, and OpenZeppelin's
UUPSUpgradeable/ERC1967Utils. Independent source and readability reviews completed.
RPC ports and the CLI home were isolated; external tool installation was not tested. -->
