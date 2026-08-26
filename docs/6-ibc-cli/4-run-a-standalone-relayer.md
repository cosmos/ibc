---
title: "Run a standalone relayer"
description: "Run a relayer in its own process, querying attestors it does not host."
---

This guide shows you how to run a relayer in its own process. It hosts no attestors and queries them over the network instead.

## Before you begin

Before proceeding, run the [tutorial](2-tutorial-deploy-ibc-and-send-a-token.md) and [standalone attestor](3-run-a-standalone-attestor.md) guides first, in order. You'll need to have an IBC deployment and standalone attestors on both chains to complete this guide.

The tutorial's keystore already holds a funded key, so this guide reuses it rather than importing another.

## 1. Relayer config

1. Create a relayer configuration file:

```bash
./bin/ibc config new --config ibc-relayer2.yml
```

2. Write its configuration, reading both router addresses from your deployment:

```bash
cat > ~/.ibc/ibc-relayer2.yml <<EOF
server:
  listenAddr: 0.0.0.0:3002
db:
  type: sqlite
  url: relayer2.db

chains:
- chainId: "41001"
  evm:
    rpc: http://localhost:8545
    ws: ws://localhost:8546
    ics26Router: "$(./bin/ibc deploy show 41001 | jq -r '.core.router')"
- chainId: "41002"
  evm:
    rpc: http://localhost:8745
    ws: ws://localhost:8746
    ics26Router: "$(./bin/ibc deploy show 41002 | jq -r '.core.router')"

relayer:
  connections:
  - alias: 41001-41002
    clientA:
      chainId: "41001"
      clientId: cli-41001-41002
      signer: relayer
      type: attestation
      autoRelay:
        enabled: true
    clientB:
      chainId: "41002"
      clientId: cli-41001-41002
      signer: relayer
      type: attestation
      autoRelay:
        enabled: true

attestors:
- name: attestor-41002
  type: remote
  grpc: 127.0.0.1:3001
- name: attestor-41001
  type: remote
  grpc: 127.0.0.1:3003

signers:
- alias: relayer
  type: local
  file: relayer
EOF
```

- No chain names a deployer, because this relayer deploys nothing.
- The signer on each client end is the key that submits on that end's own chain, so one connection can use a different key per side.
- `autoRelay` is set on each client end and governs packets leaving that end's chain. It requires the chain's `ws` endpoint.
- The listen address and the database are this relayer's own, so neither collides with the tutorial's.

## 2. Validate the config

```bash
./bin/ibc config validate --config ibc-relayer2.yml --live --strict
```

```json
{
  "status": "valid"
}
```

## 3. Relay a packet

1. Start your relayer in a new terminal, and leave it running:

```bash
./bin/ibc relayer run --config ibc-relayer2.yml
```

```
level=INFO msg="Migrated database" module=bootstrap migrations_applied=3
level=INFO msg="Starting relayer" module=bootstrap
level=INFO msg="Subscribed to send packets" module=bootstrap module=watcher chainID=41001 clientIDs=[cli-41001-41002]
level=INFO msg=Readiness module=bootstrap readiness="{Event:ready ChainsConnected:[41001 41002] HTTP:[::]:3002}"
level=INFO msg="Subscribed to send packets" module=bootstrap module=watcher chainID=41002 clientIDs=[cli-41001-41002]
```

The relayer is running and waiting for packets.

2. Back in your first terminal, send a transfer and keep the hash:

```bash
TX=$(./bin/ibc tx ift send --chain 41001 --ift "$(./bin/ibc deploy show 41001 | jq -r '.tokens[0].address')" --client-id cli-41001-41002 --to deployer --from deployer --amount 1000000000000000000 | jq -r '.txHash') && echo "$TX"
```

The relayer will pick up the packet automatically and relay it to the destination.

3. Watch it settle:

```bash
./bin/ibc relayer packets --config ibc-relayer2.yml --chain-id 41001 --tx-hash "$TX"
```

It reads `PACKET_STATE_PENDING` until the receive and the acknowledgement have both landed, then `PACKET_STATE_SUCCEEDED` with all three transaction hashes.

4. Confirm the balance on the destination chain:

```bash
./bin/ibc query ift balance --chain 41002 --ift "$(./bin/ibc deploy show 41002 | jq -r '.tokens[0].address')" --address deployer
```

The balance should be one token higher, meaning the packet was delivered successfully.
