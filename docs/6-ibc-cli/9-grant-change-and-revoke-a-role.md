---
title: "Grant, change, and revoke a role"
description: "Grant the relayer role on the tutorial's two chains, restrict packet relaying to it, confirm the change took effect, then revoke it."
---

This guide grants the relayer role on a running two-chain IBC deployment and restricts packet relaying to that role. It then revokes the role and reopens relaying. Every change is a call to the access manager (the OpenZeppelin contract that decides who may call the router's functions), so the router itself needs no change.

## Before you begin

Complete the [Deploy IBC and send a token](2-tutorial-deploy-ibc-and-send-a-token.md) tutorial first. This guide starts from the state it leaves:

- IBC is deployed on chains 41001 and 41002.
- One token transfer has been delivered.
- The relayer is running in its own terminal.

You also need:

- [Foundry](https://getfoundry.sh/introduction/installation), for `cast`. The IBC CLI has no role commands.
- The access manager's admin key. The tutorial's deployer key holds the [admin role](../5-ibc-solidity-contracts/6-permissions-and-upgrades.md#the-access-manager-and-its-roles), the only role that may grant, revoke, or reassign roles.

Run every command from the `cli` directory of your IBC clone.

## 1. Set your shell variables

1. Read the values the steps use:

```bash
AM_A=$(./bin/ibc deploy show 41001 | jq -r '.targetData.accessManager')
AM_B=$(./bin/ibc deploy show 41002 | jq -r '.targetData.accessManager')
ROUTER_A=$(./bin/ibc deploy show 41001 | jq -r '.core.router')
ROUTER_B=$(./bin/ibc deploy show 41002 | jq -r '.core.router')
IFT_A=$(./bin/ibc deploy show 41001 | jq -r '.tokens[0].address')
DEPLOYER=$(./bin/ibc keys show deployer | jq -r '.evmAddress')
RELAYER=$(./bin/ibc keys show relayer | jq -r '.evmAddress')
DEPLOYER_KEY=$(./bin/ibc keys show deployer --private | jq -r '.privateKey')
```

The last line reads the deployer's raw private key, because Foundry cannot read the CLI's key files.

2. Compute the selectors of the router's four relaying functions:

```bash
RECV=$(cast sig 'recvPacket(((uint64,string,string,uint64,(string,string,string,string,bytes)[]),bytes,(uint64,uint64)))')
ACK=$(cast sig 'ackPacket(((uint64,string,string,uint64,(string,string,string,string,bytes)[]),bytes,bytes,(uint64,uint64)))')
TIMEOUT=$(cast sig 'timeoutPacket(((uint64,string,string,uint64,(string,string,string,string,bytes)[]),bytes,(uint64,uint64)))')
UPDATE=$(cast sig 'updateClient(string,bytes)')
RELAY_FNS="[$RECV,$ACK,$TIMEOUT,$UPDATE]"
```

A selector is the 4-byte ID that `cast sig` computes from a function's signature. The access manager assigns roles to functions by selector.

## 2. Check the roles before the change

1. Confirm the deployer holds the admin role, role `0`:

```bash
cast call "$AM_A" "hasRole(uint64,address)(bool,uint32)" 0 "$DEPLOYER" --rpc-url http://localhost:8545
```

```
true
0
```

The second line is the deployer's execution delay, the wait before its calls may run.

2. Read the role assigned to `recvPacket`:

```bash
cast call "$AM_A" "getTargetFunctionRole(address,bytes4)(uint64)" "$ROUTER_A" "$RECV" --rpc-url http://localhost:8545
```

```
18446744073709551615 [1.844e19]
```

That number is `PUBLIC_ROLE`, which every address holds. `ibc deploy core` assigns the four relaying functions to it, so any address can relay. Only the admin may call the router's other restricted functions.

3. Confirm the relayer does not hold the relayer role, role `1`:

```bash
cast call "$AM_A" "hasRole(uint64,address)(bool,uint32)" 1 "$RELAYER" --rpc-url http://localhost:8545
```

```
false
0
```

## 3. Grant the relayer role on each chain

1. Grant role `1` to the relayer on 41001:

```bash
cast send "$AM_A" "grantRole(uint64,address,uint32)" 1 "$RELAYER" 0 --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY"
```

The receipt it prints has a `status` line reading `1 (success)`. The final `0` is the relayer's execution delay in seconds. A nonzero delay would make the relayer schedule each call and wait, and the IBC CLI relayer does not schedule calls.

2. Grant the same role on 41002:

```bash
cast send "$AM_B" "grantRole(uint64,address,uint32)" 1 "$RELAYER" 0 --rpc-url http://localhost:8745 --private-key "$DEPLOYER_KEY"
```

3. Confirm the grant on 41002:

```bash
cast call "$AM_B" "hasRole(uint64,address)(bool,uint32)" 1 "$RELAYER" --rpc-url http://localhost:8745
```

```
true
0
```

The grant takes effect immediately. Any address can still relay until the next step.

## 4. Restrict relaying on each chain

Each relaying function is `restricted`. Before it runs, the router asks the access manager whether the caller holds the role assigned to that function. A caller without it gets `AccessManagedUnauthorized`, and the call reverts.

<Warning>
Grant the role before you restrict the functions. If no address holds the role, every delivery reverts and packets stay undelivered on both chains.
</Warning>

1. Assign the four relaying functions to role `1` on 41001:

```bash
cast send "$AM_A" "setTargetFunctionRole(address,bytes4[],uint64)" "$ROUTER_A" "$RELAY_FNS" 1 --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY"
```

2. Do the same on 41002:

```bash
cast send "$AM_B" "setTargetFunctionRole(address,bytes4[],uint64)" "$ROUTER_B" "$RELAY_FNS" 1 --rpc-url http://localhost:8745 --private-key "$DEPLOYER_KEY"
```

The relayer sends its deliveries in one `multicall`, which any address may call. The router checks the role for each delivery inside it, so the relayer needs no other role.

## 5. Verify the change

1. Read the role now assigned to `recvPacket`:

```bash
cast call "$AM_A" "getTargetFunctionRole(address,bytes4)(uint64)" "$ROUTER_A" "$RECV" --rpc-url http://localhost:8545
```

```
1
```

2. Ask the access manager whether the relayer may call it:

```bash
cast call "$AM_A" "canCall(address,address,bytes4)(bool,uint32)" "$RELAYER" "$ROUTER_A" "$RECV" --rpc-url http://localhost:8545
```

```
true
0
```

3. Simulate a call to `updateClient` from the deployer's account:

```bash
cast call "$ROUTER_A" "updateClient(string,bytes)" cli-41001-41002 0x --from "$DEPLOYER" --rpc-url http://localhost:8545
```

```
Error: server returned an error response: error code -32000: Execution reverted, data: "0x068ca9d8<...>"
```

The data starts with `0x068ca9d8`, the selector of `AccessManagedUnauthorized(address)`, followed by the deployer's address. The admin role lets the deployer grant roles, but calling a function requires that function's role.

4. Send a token across:

```bash
TX=$(./bin/ibc tx ift send --chain 41001 --ift "$IFT_A" --client-id cli-41001-41002 --to deployer --from deployer --amount 1000000000000000000 | jq -r '.txHash') && echo "$TX"
```

5. Check the transfer's state:

```bash
./bin/ibc relayer packets --chain-id 41001 --tx-hash "$TX" | jq -r '.packets[0].state'
```

It reads `PACKET_STATE_PENDING` for up to a minute, then `PACKET_STATE_SUCCEEDED`. The relayer delivered the packet while holding role `1`.

## 6. Revoke the role and reopen relaying

These steps return the deployment to the tutorial's state. Skip them to keep relaying restricted. To replace a relayer instead, grant the new address the role before you revoke the old one.

<Warning>
Revoking the only relayer's role stops delivery. Its deliveries revert with `AccessManagedUnauthorized` until another address holds the role or the functions reopen.
</Warning>

1. Revoke role `1` from the relayer on 41001:

```bash
cast send "$AM_A" "revokeRole(uint64,address)" 1 "$RELAYER" --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY"
```

2. Revoke it on 41002:

```bash
cast send "$AM_B" "revokeRole(uint64,address)" 1 "$RELAYER" --rpc-url http://localhost:8745 --private-key "$DEPLOYER_KEY"
```

3. Confirm the relayer no longer holds it:

```bash
cast call "$AM_A" "hasRole(uint64,address)(bool,uint32)" 1 "$RELAYER" --rpc-url http://localhost:8545
```

```
false
0
```

4. Reassign the four relaying functions to `PUBLIC_ROLE` on 41001:

```bash
cast send "$AM_A" "setTargetFunctionRole(address,bytes4[],uint64)" "$ROUTER_A" "$RELAY_FNS" 18446744073709551615 --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY"
```

5. Do the same on 41002:

```bash
cast send "$AM_B" "setTargetFunctionRole(address,bytes4[],uint64)" "$ROUTER_B" "$RELAY_FNS" 18446744073709551615 --rpc-url http://localhost:8745 --private-key "$DEPLOYER_KEY"
```

6. Confirm `recvPacket` is open again:

```bash
cast call "$AM_A" "getTargetFunctionRole(address,bytes4)(uint64)" "$ROUTER_A" "$RECV" --rpc-url http://localhost:8545
```

```
18446744073709551615 [1.844e19]
```

The relayer retries a failed delivery, so any packet that waited is now delivered.
