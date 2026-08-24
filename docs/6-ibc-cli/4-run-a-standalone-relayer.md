---
title: "Run a standalone relayer"
description: "Run a relayer in its own process, querying attestors it does not host, including one that finishes packets a stopped relayer left behind."
---

This guide shows you how to run a relayer in its own process. It hosts no attestors and queries them over the network instead.

## Before you begin

Before proceeding, run the [tutorial](/ibc-cli/tutorial-deploy-ibc-and-send-a-token) and [standalone attestor](/ibc-cli/run-a-standalone-attestor) guides first, in order. You'll need to have an IBC deployment and standalone attestors on both chains to complete this guide.

You also need a key funded on every chain you submit to.

## 1. Relayer config

1. Create a new configuration file for the relayer:

```bash
./bin/ibc config new --home ~/.ibc-relayer2
```

2. Import a funded key. This is an account from the tutorial's setup script:

```bash
./bin/ibc keys import ecdsa relayer2 --home ~/.ibc-relayer2 --private-key 0xa42b90503f0d4a418e2b8b140a03901c89d76704c8af49c3ef82d6f3cd91f23d
```

3. Write its configuration, reading both router addresses and both attestor names from your deployment:

```bash
cat > ~/.ibc-relayer2/ibc.yml <<EOF
server:
  listenAddr: 0.0.0.0:3002
db:
  type: sqlite
  url: ibc.db

chains:
- chainId: "41001"
  evm:
    rpc: http://localhost:8545
    ics26Router: "$(./bin/ibc deploy show 41001 | jq -r '.core.router')"
- chainId: "41002"
  evm:
    rpc: http://localhost:8745
    ics26Router: "$(./bin/ibc deploy show 41002 | jq -r '.core.router')"

relayer:
  connections:
  - alias: 41001-41002
    clientA:
      chainId: "41001"
      clientId: link-41001-41002
      signer: relayer2
      type: attestation
    clientB:
      chainId: "41002"
      clientId: link-41001-41002
      signer: relayer2
      type: attestation

attestors:
- name: attestor-41002-$(./bin/ibc keys show attestor-41002 --home ~/.ibc-attestor | jq -r '.evmAddress')
  type: remote
  grpc: 127.0.0.1:3001
- name: attestor-41001-$(./bin/ibc keys show attestor-41001 --home ~/.ibc-attestor-41001 | jq -r '.evmAddress')
  type: remote
  grpc: 127.0.0.1:3003

signers:
- alias: relayer2
  type: local
  file: relayer2
EOF
```

- No chain names a deployer, because this relayer deploys nothing.
- The signer on each client end is the key that submits on that end's own chain, so one connection can use a different key per side.

## 2. Validate the config

```bash
./bin/ibc config validate --home ~/.ibc-relayer2 --live --strict
```

```json
{
  "status": "valid"
}
```

## 3. Relay a packet

1. Send a transfer and keep the hash:

```bash
TX=$(./bin/ibc tx ift send --chain 41001 --ift "$(./bin/ibc deploy show 41001 | jq -r '.tokens[0].address')" --client-id link-41001-41002 --to deployer --from deployer --amount 1000000000000000000 | jq -r '.txHash') && echo "$TX"
```

The tokens are burned on 41001 and nothing has delivered them yet.

2. Start your relayer in a new terminal, and leave it running:

```bash
./bin/ibc relayer run --home ~/.ibc-relayer2
```

```
level=INFO msg="Migrated database" migrations_applied=3
level=INFO msg="Starting relayer"
{"event":"ready","chainsConnected":["41001","41002"],"http":"[::]:3002"}
```

It creates its database on the way up.

3. Back in your first terminal, relay the packet:

```bash
./bin/ibc relayer relay --home ~/.ibc-relayer2 --chain-id 41001 --tx-hash "$TX"
```

```json
{
  "packets": [
    {
      "source_client_id": "link-41001-41002",
      "sequence_number": "2",
      "selection": "PACKET_SELECTION_SELECTED"
    }
  ]
}
```

4. Watch it settle:

```bash
./bin/ibc relayer packets --home ~/.ibc-relayer2 --chain-id 41001 --tx-hash "$TX"
```

It reads `PACKET_STATE_PENDING` until the receive and the acknowledgement have both landed, then `PACKET_STATE_SUCCEEDED` with all three transaction hashes.

5. Confirm the balance on the destination chain:

```bash
./bin/ibc query ift balance --chain 41002 --ift "$(./bin/ibc deploy show 41002 | jq -r '.tokens[0].address')" --address deployer
```

The balance should be one token higher, meaning the packet was delivered successfully.

## What can go wrong

**The relayer refuses to start on the attestor quorum.** It prints the addresses each client trusts next to the attestors it managed to reach, and stops when too few match the threshold. Check that your attestor addresses are reachable and that each remote entry's name matches the name its host serves it under.

**Two attestors share one signing address on the same chain.** That is a startup error, because they would be one signer counted twice.

**A packet stays pending.** The delivery is waiting on a finality gate, which asks the attestor quorum what height it can currently prove. A gate that has waited half an hour logs a warning that the chain may be lagging.

**A transaction is rejected before it is sent.** The relayer builds EIP-1559 transactions and refuses a chain that reports no base fee.

## Next steps

- [Overview](/ibc-cli/overview) covers what the three parts are made of and how they fit together.
