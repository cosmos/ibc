---
title: "Deploy IBC and send a token"
description: "Bring up two chains, deploy IBC on both, and move an Interchain Fungible Token from one to the other."
---

This tutorial walks you through all the steps to deploy and run IBC on two EVM chains using IBC CLI.

By the end, you'll have the following:

- IBC CLI installed and configured
- IBC contracts deployed on two local Besu chains
- IBC connections established between the chains
- A relayer running
- Attestors running 
- An Interchain Fungible Token that can be transferred between the chains

## Prerequisites

- [Docker](https://docs.docker.com/get-started/get-docker/) installed and running
- [Go](https://go.dev/doc/install) v1.26.4 or later
- [jq](https://jqlang.org/download/) installed
- [Git](https://git-scm.com/downloads) installed

## 1. Set up your environment

Start by installing IBC CLI and running two local Besu chains.

1. Clone the IBC repository and build the binary:

```bash
git clone https://github.com/cosmos/ibc.git && cd ibc/link && make build
```

The binary is located at `bin/ibc`.

2. This tutorial uses two local [Besu](https://docs.besu-eth.org/) chains, which are EVM-based. Start both chains:

```bash
../examples/besu-to-besu/setup.sh
```

The script initializes two single-validator Besu chains, starts them, and waits until both answer RPC calls. It finishes by printing something similar to the following:

```
Chain status:
  besu-a (chain-id 41001) RPC=http://localhost:8545 WS=ws://localhost:8546 block=1
  besu-b (chain-id 41002) RPC=http://localhost:8745 WS=ws://localhost:8746 block=1
Chains are live and producing blocks.
```

Now there are two live chains, each producing blocks.

3. You can use the following command to look at the funded accounts included in the setup script:

```bash
../examples/besu-to-besu/setup.sh accounts
```

Five accounts are derived from one mnemonic and funded on both chains, so the same key works on either side. The next step uses these. 

## 2. Create a config and keys

IBC CLI uses one config file for all its operations.

1. Start by creating the config file:

```bash
./bin/ibc config new
```

The config is stored in `~/.ibc/ibc.yml` by default.

This is the home directory for everything that follows: the config, the keystore, the deployment records, and the relayer's database.

2. Import a deployer key from the accounts created during the chain setup. This key becomes the access manager's admin, which governs the IBC router and the GMP app. It is also the default owner of any token you deploy. 

Passing `--populate-config` adds each key to the config as a named signer. Later commands then name a key by alias instead of by path.

```bash
./bin/ibc keys import ecdsa deployer --private-key 0x33fa40f84e854b941c2b0436dd4a256e1df1cb41b9c1c0ccc8446408c19b8bf9 --populate-config
```

```json
{
  "evmAddress": "0x58A57ed9d8d624cBD12e2C467D34787555bB1b25",
  "path": "/Users/you/.ibc/keys/deployer.json",
  "publicKey": "0x03a70d1ef368ad99e90d509496e9888ee7404e4f4d360376bf521d769cf0c4de46",
  "type": "ecdsa"
}
```

3. Import a relayer key, which signs packet delivery and pays for the relayer's gas:

```bash
./bin/ibc keys import ecdsa relayer --private-key 0xd46507376b3a8a4af4f1f934375df25213a09857a3ed1086ba284c82d387904b --populate-config
```

4. Generate an attestor key for each chain. These sign attestations and never need funding. Never reuse an attestor key across clients, so each chain gets its own: see [Attestors](../4-light-clients/2-attestors.md#attestor-keys).

```bash
./bin/ibc keys new ecdsa attestor-41001 --populate-config
```

```bash
./bin/ibc keys new ecdsa attestor-41002 --populate-config
```

5. Next, register the first chain's details in the config. This passes the chain ID, RPC and websocket endpoints, and deployer key.

```bash
./bin/ibc config add-chain --chain-id 41001 --rpc http://localhost:8545 --ws ws://localhost:8546 --deployer deployer
```

6. Register the second chain's details:

```bash
./bin/ibc config add-chain --chain-id 41002 --rpc http://localhost:8745 --ws ws://localhost:8746 --deployer deployer
```

The `--deployer` flag names the key that signs deployments on that chain, so you won't need to pass it again.

## 3. Deploy IBC on both chains

With both chains registered, deploy the IBC contracts on each. 

1. Deploy the IBC core on each chain. This deploys the ICS26 router and the access manager:

```bash
./bin/ibc deploy core --chain 41001 --yes
```

```bash
./bin/ibc deploy core --chain 41002 --yes
```

Each run sends four transactions: an access manager, the router implementation, the router behind a proxy, and one call that opens the packet-delivery methods to any caller. Your deployer key is the access manager's admin.

```
level=INFO msg="transaction mined" label="deploy AccessManager" tx=0xa1c9fe97... block=38 chain=41001
level=INFO msg="transaction mined" label="deploy ICS26Router implementation" tx=0x23988e80... block=39 chain=41001
level=INFO msg="transaction mined" label="deploy ICS26Router proxy" tx=0x064d03a3... block=40 chain=41001
level=INFO msg="transaction mined" label=setTargetFunctionRole tx=0x2fb39d8a... block=41 chain=41001
[
  {
    "name": "core stack on chain 41001",
    "action": "executed"
  }
]
```

2. Deploy an attestation light client on each chain. Each deployment tracks the state of the other chain:

```bash
./bin/ibc deploy client --chain 41001 --counterparty-chain 41002 --attestors attestor-41002 --threshold 1 --yes
```

```bash
./bin/ibc deploy client --chain 41002 --counterparty-chain 41001 --attestors attestor-41001 --threshold 1 --yes
```

The `--attestors` flag takes the aliases from step 4. Each resolves to that key's address, and those addresses become the client's attestation set on chain.

The `--threshold` flag is how many of them must sign for the client to accept a proof. Both are fixed on chain when the client is deployed.

Note the crossed pairing. A client verifies the state of the chain it tracks, so the client on 41001 tracks 41002 and trusts the attestor watching 41002.

Both sides share one client identifier, derived from the two chain IDs in sorted order, so it reads the same on either chain. Here it is `link-41001-41002`, which will be used later to identify the connection.

## 4. Deploy the application

The next steps deploy the General Message Passing (GMP) app and the Interchain Fungible Token (IFT) contracts on each chain. IFT uses GMP to send and receive messages between the chains and facilitate token transfers.

1. Deploy the GMP app on each chain. This allows cross-chain contract calls between the chains:

```bash
./bin/ibc deploy gmp --chain 41001 --yes
```

```bash
./bin/ibc deploy gmp --chain 41002 --yes
```

2. Deploy an IFT contract on each chain. This is the token that will be transferred between the chains:

```bash
./bin/ibc deploy ift --name "Demo Token" --symbol DEMO --chain 41001 --yes
```

```bash
./bin/ibc deploy ift --name "Demo Token" --symbol DEMO --chain 41002 --yes
```

```
level=INFO msg="ift token deployed" chain=41001 symbol=DEMO address=0x6bbcD66DA283F6cD4e630b9fc6A91C37D5B2C7A8
```

3. Use the following commands to read both token addresses out of the deployment records. These will be used in later steps:

```bash
IFT_A=$(./bin/ibc deploy show 41001 | jq -r '.tokens[0].address')
IFT_B=$(./bin/ibc deploy show 41002 | jq -r '.tokens[0].address')
```

If you need to view these addresses later, the `deploy show` command prints a chain's deployment record as JSON.

4. The next step is to link the two token contracts together. Registering a bridge tells each token to accept mints from its counterpart on the other chain. A transfer then burns tokens on the source chain and mints them on the destination.

```bash
./bin/ibc deploy ift-bridge --chain-a 41001 --ift-a "$IFT_A" --chain-b 41002 --ift-b "$IFT_B" --yes
```

This command registers both sides. It ties each token to the client pointing at the other chain, which is how a transfer knows where to land.

## 5. Configure and start the relayer

Next, you'll need to configure the relayer to start sending packets between the two chains.

1. Generate the relayer's configuration.

```bash
./bin/ibc deploy render-config 41001 41002 --signer-a relayer --signer-b relayer
```

This prints the three sections relaying needs: the chains with their router addresses, the connection, and the attestors. Each is already filled in with the addresses your deploy commands recorded.

The two signer flags name the key that submits relay transactions on each chain. Both are required, and each is checked against your configured signers. You imported `relayer` in step 3.

The attestors section declares both of your attestor keys as `type: local`. This means the relayer will run the attestors in the same process.

2. Add the `render-config` output to your config manually or use the following command to merge the generated sections into your config:

```bash
{ sed -n '1,/^chains:/p' ~/.ibc/ibc.yml | sed '$d'
  ./bin/ibc deploy render-config 41001 41002 --signer-a relayer --signer-b relayer
  sed -n '/^signers:/,$p' ~/.ibc/ibc.yml
} > /tmp/ibc.yml.merged && mv /tmp/ibc.yml.merged ~/.ibc/ibc.yml
```

This keeps your `server`, `db`, and `signers` blocks, and replaces the three the deploy tool generated.

3. Use the validate command to check the result against both chains before starting anything:

```bash
./bin/ibc config validate --live --strict
```

```json
{
  "status": "valid"
}
```

4. Now you'll need to open a new terminal to start the relayer and attestors. Leave your first terminal open. You'll come back to it in the next step.

```bash
# open a new terminal and start the relayer
./bin/ibc relayer run
```

```
level=INFO msg="Attestor config provided, running in dual mode: relayer with attestor" module=bootstrap
level=INFO msg="Migrated database" module=bootstrap migrations_applied=3
level=INFO msg=Readiness module=bootstrap readiness="{Event:ready ChainsConnected:[41001 41002] HTTP:[::]:3000}"
```

The readiness line is how you know it is up. "Dual mode" means the attestors are running inside this process, because the rendered configuration declared them local.

Leave it running, and go back to your first terminal for the rest of the tutorial.

## 6. Transfer a token

The next steps will mint the token and send it to the other chain.

1. Mint an initial supply of the token into the deployer account.

```bash
./bin/ibc tx ift mint --chain 41001 --ift "$IFT_A" --to deployer --from deployer --amount 100000000000000000000
```

The `--from` key must be the token's owner, which is the deployer. Amounts are in base units, so this is 100 tokens at 18 decimals.

2. Send 10 tokens from one chain to the other, keeping the transaction hash. `$IFT_A` is the token address you stored earlier.

> **Note:** The `--timeout` flag sets how long the packet has to be delivered, counted from when you send. It defaults to 15 minutes and cannot exceed one day. A packet that misses that window can only be timed out and refunded, and the deadline cannot be changed after the send.

```bash
TX=$(./bin/ibc tx ift send --chain 41001 --ift "$IFT_A" --client-id link-41001-41002 --to deployer --from deployer --amount 10000000000000000000 | jq -r '.txHash') && echo "$TX"
```

The send prints `{"txHash": "0x..."}`. That hash is stored in `$TX`, which the next command uses:

```
0xf1fa599e3e4d048113a9ffd1cbe4749ae55492d2911b09d10a98fd6d3567f5f0
```

Once this transaction is mined, the tokens are burned on the source chain. They are minted on the destination only when the packet is delivered. If delivery fails, or the packet times out, the tokens are minted back to the sender once a relayer carries that outcome home.

3. The relayer will automatically detect that the IBC packet was created and relay it.

4. Now you can check the status of the transfer:

```bash
./bin/ibc relayer packets --chain-id 41001 --tx-hash "$TX"
```

It will read `PACKET_STATE_PENDING` for up to a minute, then it should read `PACKET_STATE_SUCCEEDED`:

```json
{
  "packets":  [
    {
      "state":  "PACKET_STATE_SUCCEEDED",
      "sequenceNumber":  "1",
      "sourceClientId":  "link-41001-41002",
      "sendTx":  {"txHash":  "0xf1fa599e...", "chainId":  "41001"},
      "recvTx":  {"txHash":  "0xf8281f04...", "chainId":  "41002"},
      "ackTx":  {"txHash":  "0x0f56d3f0...", "chainId":  "41001"},
      "timeoutTx":  null
    }
  ],
  "hasMore":  false,
  "nextCursor":  ""
}
```

This shows three transactions across two chains: the send you made, the receive on 41002, and the acknowledgement carried back to 41001.

5. Confirm the tokens arrived on the destination chain:

```bash
./bin/ibc query ift balance --chain 41002 --ift "$IFT_B" --address deployer
```

```json
{
  "address": "0x58A57ed9d8d624cBD12e2C467D34787555bB1b25",
  "balance": "10000000000000000000",
  "symbol": "DEMO"
}
```

Then check that the source chain was debited:

```bash
./bin/ibc query ift balance --chain 41001 --ift "$IFT_A" --address deployer
```

```json
{
  "address": "0x58A57ed9d8d624cBD12e2C467D34787555bB1b25",
  "balance": "90000000000000000000",
  "symbol": "DEMO"
}
```

Congratulations! You've deployed IBC and transferred a token between two chains.

## Next steps

- [Overview](1-overview.md) explains what the three parts are made of and how they fit together.
- [Run a standalone attestor](3-run-a-standalone-attestor.md) moves an attestor out of the relayer's process and into its own.
- [Run a standalone relayer](4-run-a-standalone-relayer.md) brings up a relayer against a deployment, including one that replaces a relayer that stopped.
