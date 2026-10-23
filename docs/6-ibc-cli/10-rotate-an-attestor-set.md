---
title: "Rotate an attestor set"
description: "Replace an attestor in the attestor set."
---

Replace the tutorial's attestor for chain 41002 by migrating to a replacement client.

## Before you begin

Complete [Deploy IBC and send a token](2-tutorial-deploy-ibc-and-send-a-token.md).
Keep its relayer running, including the local attestor for 41002.
Run every command from the `cli` directory of your IBC clone.
You need [Foundry](https://getfoundry.sh/introduction/installation), [Bun](https://bun.com/docs/installation), and the tutorial's deployer admin key.

This guide replaces only the client on 41001 that verifies chain 41002.
The replacement client accepts only the replacement attestor, with a threshold of one signature.

## 1. Create the replacement attestor key

Read the tutorial's values in your working terminal:

```bash
CLIENT_ID=cli-41001-41002
ROUTER_A=$(./bin/ibc deploy show 41001 | jq -er '.core.router')
IFT_B=$(./bin/ibc deploy show 41002 | jq -er '.tokens[0].address')
DEPLOYER_KEY=$(./bin/ibc keys show deployer --private | jq -er '.privateKey')
COUNTERPARTY=$(cast call "$ROUTER_A" 'getCounterparty(string)((string,bytes[]))' "$CLIENT_ID" --rpc-url http://localhost:8545)
```

Create a distinct replacement key:

```bash
./bin/ibc keys new ecdsa attestor-41002-replacement --populate-config
```

```bash
NEW_ATTESTOR=$(./bin/ibc keys show attestor-41002-replacement | jq -er '.evmAddress')
```

## 2. Deploy the replacement client

Clone the contracts and check out the commit pinned by the target CLI version:

```bash
git clone https://github.com/cosmos/ibc-contracts.git ../../ibc-contracts-attestor-rotation
```

```bash
git -C ../../ibc-contracts-attestor-rotation checkout f564e6339cfd49498b043cc9fab770ed6f20ebaa
```

```bash
bun install --cwd ../../ibc-contracts-attestor-rotation/ibc-solidity --frozen-lockfile
```

Read chain 41002's latest block height:

```bash
HEIGHT=$(cast block-number --rpc-url http://localhost:8745)
```

Read chain 41002's block timestamp at `HEIGHT`:

```bash
TIMESTAMP=$(cast block "$HEIGHT" --field timestamp --rpc-url http://localhost:8745)
```

Deploy a client that accepts only the replacement attestor:

```bash
REPLACEMENT_CLIENT=$(forge create --root ../../ibc-contracts-attestor-rotation/ibc-solidity contracts/light-clients/attestation/AttestationLightClient.sol:AttestationLightClient --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY" --broadcast --json --constructor-args "[$NEW_ATTESTOR]" 1 "$HEIGHT" "$TIMESTAMP" "$ROUTER_A" | jq -er '.deployedTo')
```

The constructor authorizes `ROUTER_A` to submit proofs.

## 3. Migrate to the replacement client

The router's [`migrateClient`](../5-ibc-solidity-contracts/2-ics26-router.md#client-and-counterparty-registry) function replaces the contract address registered under an existing client ID.
It also overwrites the counterparty information, so reuse `COUNTERPARTY` from step 1.
Migration does not copy the old client's stored timestamps.

Stop the relayer with `Ctrl-C` in its terminal.
Wait for it to exit.

Submit the migration:

```bash
cast send "$ROUTER_A" 'migrateClient(string,(string,bytes[]),address)' "$CLIENT_ID" "$COUNTERPARTY" "$REPLACEMENT_CLIENT" --rpc-url http://localhost:8545 --private-key "$DEPLOYER_KEY"
```

## 4. Switch the relayer to the replacement attestor

In `~/.ibc/ibc.yml`, replace the `attestors` section with:

```yaml
attestors:
- name: attestor-41002-replacement
  type: local
  chainId: "41002"
  signer: attestor-41002-replacement
  finalityOffset: 1
```

Validate the configuration against the migrated client:

```bash
./bin/ibc config validate relayer --live
```

Restart the relayer in its terminal:

```bash
./bin/ibc relayer run
```

## 5. Verify the rotation

In your working terminal, read the active client's address:

```bash
ACTIVE_CLIENT=$(cast call "$ROUTER_A" 'getClient(string)(address)' "$CLIENT_ID" --rpc-url http://localhost:8545)
```

Check its attestor set:

```bash
cast call "$ACTIVE_CLIENT" 'getAttestationSet()(address[],uint8)' --rpc-url http://localhost:8545
```

The set must contain only `NEW_ATTESTOR`, with threshold `1`.
Send one token from 41002 to 41001:

```bash
TX=$(./bin/ibc tx ift send --chain 41002 --ift "$IFT_B" --client-id "$CLIENT_ID" --to deployer --from deployer --amount 1000000000000000000 | jq -er '.txHash')
```

Poll for `PACKET_STATE_SUCCEEDED`:

```bash
./bin/ibc relayer packets --chain-id 41002 --tx-hash "$TX" | jq -r '.packets[0].state'
```

## Additional considerations

### Minimize downtime

To minimize downtime, first migrate to an intermediate attestation light client that accepts both old and new attestors with a threshold of one signature.
This preserves compatibility with the old and new attestor while you update the relayer configuration and restart it with the new attestor.
Verify a transfer.
Then migrate to the client that accepts only the replacement.
For larger sets, keep enough eligible signers to meet each active threshold.

### Manifest drift

The CLI does not currently have first-class support for attestor set migration.
This guide migrates the client directly on chain, leaving the manifest with the original client address and attestor set.
As a result, `ibc deploy status` reports a client-address mismatch.
Rerunning `ibc deploy client attestation` does not refresh the recorded set.
The `ibc deploy render-config` command still derives attestors from those stale records.

<!-- Mixed-client changes verified 2026-10-23 against CLI
ae06619851c69673e8bc8f4075fb0284ed93ef1f and contracts
f564e6339cfd49498b043cc9fab770ed6f20ebaa.
The mixed-client changes received two independent source reviews. The direct
migration ran on isolated Besu 25.4.0 chains. The replacement client accepted
only the replacement attestor with threshold one. A transfer from 41002 to
41001 succeeded using the replacement attestor for delivery and Besu proofs
for acknowledgement. RPC ports and the CLI home were isolated. A matching
contracts checkout was reused; external tool installation and the optional
intermediate-client procedure were not tested in this run. -->
