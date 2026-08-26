---
title: "Run a standalone attestor"
description: "Run an attestor in its own process, separate from any relayer, so a relayer can query it and count its signatures toward a light client's quorum."
---

<!-- SPDX-License-Identifier: Apache-2.0 -->

This guide runs an attestor as its own process, serving one chain. A relayer can then query it and count its signatures toward a light client's quorum.

In order to run a standalone attestor, your signing address must already be in the light client's attestation set, which is fixed when the client is deployed and read from the chain.

## Before you begin

You need:

- **A chain with IBC deployed, and a light client tracking it.** This guide continues from the [tutorial](2-tutorial-deploy-ibc-and-send-a-token.md).
- **Your signing address already in that client's attestation set.**
- **Your signing key.**
- **The chain's RPC endpoint and its router address.** Configuration validation treats the router address as optional, but the process cannot start without it.
- [Go](https://go.dev/doc/install) 1.26.4 or later and a [build of the binary](2-tutorial-deploy-ibc-and-send-a-token.md).

The commands below continue from the [tutorial](2-tutorial-deploy-ibc-and-send-a-token.md). They move the attestor keys it generated into processes of their own, reading values from your own deployment rather than asking you to copy one.

> **Warning:** Stop the tutorial's relayer before you start, with `Ctrl+C` in its terminal. It hosts both attestors inside its own process, and this guide gives those same attestors processes of their own.

## 1. Attestor config

Create a second configuration file alongside the tutorial's:

```bash
./bin/ibc config new --config ibc-attestor-41002.yml
```

It shares the tutorial's keystore, so the attestor key generated there is already available. That key's address matches the one in the client's attestation set.

> **Warning:** An attestor address must never appear in more than one client's attestation set. The signed attestation carries no domain separation, so a signature made for one client can be replayed against another.

## 2. Write its configuration

Write the attestor's configuration file, reading the router address and your own signing address from the deployment:

```bash
cat > ~/.ibc/ibc-attestor-41002.yml <<EOF
server:
  listenAddr: 0.0.0.0:3001

chains:
- chainId: "41002"
  evm:
    rpc: http://localhost:8745
    ics26Router: "$(./bin/ibc deploy show 41002 | jq -r '.core.router')"

attestors:
- name: attestor-41002
  type: local
  chainId: "41002"
  signer: attestor-41002
  finalityOffset: 1

signers:
- alias: attestor-41002
  type: local
  file: attestor-41002
EOF
```

An attestor's name is `attestor-<chain it watches>`.

> **Warning:** The attestor's name has to match the name the relayer uses for it. A relayer sends that name in every query, and the process serves its attestors by name, so a mismatch makes every lookup fail.

A finality offset of `1` signs one block behind the chain head. Zero waits for the chain's own finalized block instead.

## 3. Start it and verify

1. Validate the configuration:

```bash
./bin/ibc config validate --config ibc-attestor-41002.yml --strict
```

```json
{
  "status": "valid"
}
```

2. Start the process in a new terminal, and leave it running:

```bash
./bin/ibc attestor run --config ibc-attestor-41002.yml
```

```
level=INFO msg="Starting attestor" module=bootstrap
level=INFO msg=Readiness module=bootstrap readiness="{Event:ready HTTP:[::]:3001}"
```

That readiness line names the address it bound.

3. Back in your first terminal, ask the process what its address is:

```bash
./bin/ibc attestor info attestor-41002 --host 127.0.0.1:3001
```

```json
{
  "chainId": "41002",
  "address": "0xc7f148Da846781a9a1D9d22F699A7A88c592CCee"
}
```

This should be the address of the attestor.

4. Ask how far it can attest:

```bash
./bin/ibc attestor latest-height attestor-41002 --host 127.0.0.1:3001
```

```json
{
  "height": "25509"
}
```

This shows the attestor's latest height.

## 4. Run the second attestor

The [tutorial](2-tutorial-deploy-ibc-and-send-a-token.md) this guide follows generated one attestor per chain. The following steps repeat the process to create an attestor for chain 41001, in its own configuration file on port 3003. Port 3002 is left free for the relayer in the next guide.

1. Create a third configuration file:

```bash
./bin/ibc config new --config ibc-attestor-41001.yml
```

2. Write its configuration:

```bash
cat > ~/.ibc/ibc-attestor-41001.yml <<EOF
server:
  listenAddr: 0.0.0.0:3003

chains:
- chainId: "41001"
  evm:
    rpc: http://localhost:8545
    ics26Router: "$(./bin/ibc deploy show 41001 | jq -r '.core.router')"

attestors:
- name: attestor-41001
  type: local
  chainId: "41001"
  signer: attestor-41001
  finalityOffset: 1

signers:
- alias: attestor-41001
  type: local
  file: attestor-41001
EOF
```

3. Start it in another terminal:

```bash
./bin/ibc attestor run --config ibc-attestor-41001.yml
```

```
level=INFO msg="Starting attestor" module=bootstrap
level=INFO msg=Readiness module=bootstrap readiness="{Event:ready HTTP:[::]:3003}"
```

Both attestors now run in processes of their own, and no relayer is running.

## Connect a relayer

A relayer references your attestor by name and network address:

```yaml
attestors:
- name: attestor-41002
  type: remote
  grpc: 127.0.0.1:3001
```

## Next steps

- [Run a standalone relayer](4-run-a-standalone-relayer.md) brings up a relayer that queries these attestors instead of hosting its own.
