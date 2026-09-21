---
title: "Run a standalone attestor"
description: "Run an attestor in its own process, separate from any relayer, so a relayer can query it and count its signatures toward a light client's quorum."
---

This guide runs an attestor as its own process, serving one chain. A relayer can then query it and count its signatures toward a light client's quorum. The last section optionally moves the signing key off the attestor host and into a remote signer.

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
./bin/ibc config validate attestor --config ibc-attestor-41002.yml
```

```json
{
  "attestor": "valid",
  "path": "/home/you/.ibc/ibc-attestor-41002.yml",
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

## 5. Move the signing key into a remote signer

Both attestors above sign with a `type: local` signer: a key file on the same host as the process. A `type: remote` signer holds the key material in [cosmos/kms](https://github.com/cosmos/kms) instead, and the attestor asks it for each signature over gRPC. The attestor host then holds no key at all.

This moves the keys you already generated, rather than making new ones. A client's attestation set is fixed on chain when the client is deployed, so a new key means a new address, and an address outside the attestation set has every attestation it signs rejected.

1. Build kms. It needs Go 1.25 or later:

```bash
git clone https://github.com/cosmos/kms.git ~/kms && make -C ~/kms build
```

The binary is at `~/kms/build/kms`.

2. Export both attestor keys from the IBC keystore into the files kms reads:

```bash
mkdir -p ~/.kms/keys
./bin/ibc keys show attestor-41001 --private | jq -r '.privateKey' > ~/.kms/keys/attestor-41001.hex
./bin/ibc keys show attestor-41002 --private | jq -r '.privateKey' > ~/.kms/keys/attestor-41002.hex
chmod 600 ~/.kms/keys/*.hex
```

`keys show` prints the key `0x`-prefixed, and kms strips the prefix as it loads the file.

> **Warning:** Do not run `keys show --private` on a shared or recorded terminal. The private key it prints controls every asset its address holds.

3. Write the kms configuration. A `grpc` block on its own runs kms in SignerService mode, with no validator signing alongside it:

```bash
cat > ~/.kms/kms.yaml <<EOF
grpc:
  listen: 127.0.0.1:9090

  keys:
    - id: attestor-41001
      backend: file
      algorithm: secp256k1eth
      key_file: keys/attestor-41001.hex

    - id: attestor-41002
      backend: file
      algorithm: secp256k1eth
      key_file: keys/attestor-41002.hex
EOF
```

Each `key_file` resolves against the kms home directory. The `id` is what an attestor names in `remoteKeyId`.

> **Warning:** The algorithm has to be `secp256k1eth`, not `secp256k1`. Only that scheme signs the pre-hashed 32-byte digest and returns the 65-byte recoverable signature an attestation light client recovers an address from. Any other scheme fails at attestor startup with `unsupported remote key scheme`.

4. Start kms in a new terminal, and leave it running:

```bash
~/kms/build/kms start --home ~/.kms
```

> **Warning:** kms performs no caller authentication or authorization. Any client that can reach the listener may sign with any key it holds. Binding to `127.0.0.1` is what keeps this local setup contained; a real deployment needs network controls in front of it.

5. Point each attestor at kms. In `~/.ibc/ibc-attestor-41002.yml`, replace the `signers:` block with:

```yaml
signers:
- alias: attestor-41002
  type: remote
  grpc: 127.0.0.1:9090
  remoteKeyId: attestor-41002
```

And in `~/.ibc/ibc-attestor-41001.yml`:

```yaml
signers:
- alias: attestor-41001
  type: remote
  grpc: 127.0.0.1:9090
  remoteKeyId: attestor-41001
```

The alias does not change, so the `attestors[].signer` line in each file still resolves. A `remote` signer sets `grpc` and `remoteKeyId` and drops `file`.

6. Restart both attestors:

```bash
./bin/ibc attestor run --config ibc-attestor-41002.yml
```

```bash
./bin/ibc attestor run --config ibc-attestor-41001.yml
```

An attestor fetches its key from kms while starting up, so kms has to be running first. If it is not, startup fails on the key lookup rather than on the first signature.

7. Confirm the address did not change:

```bash
./bin/ibc attestor info attestor-41002 --host 127.0.0.1:3001
```

```json
{
  "chainId": "41002",
  "address": "0xc7f148Da846781a9a1D9d22F699A7A88c592CCee"
}
```

This is the same address the attestor reported in step 3, and the same one in the client's attestation set — now recovered from a signature made inside kms. Ask for an attestation to prove the signing path end to end:

```bash
./bin/ibc attestor state-attestation attestor-41002 --height <height> --host 127.0.0.1:3001
```

Use a height from `attestor latest-height`, which returns one that is attestable under the configured `finalityOffset`. The `signature` that comes back was produced by kms.

8. Nothing reads the attestor key files from the keystore any more. Removing them from the attestor host is the point of the exercise:

```bash
rm ~/.ibc/keys/attestor-41001.json ~/.ibc/keys/attestor-41002.json
```

> **Warning:** The hex files under `~/.kms/keys/` then hold the only copy of these keys, and their addresses are fixed in an attestation set on chain. Losing them means redeploying the clients with a new set. The tutorial's own `~/.ibc/ibc.yml` also still names these keys as local signers, so leave its relayer stopped.

### What a remote signer cannot do yet

- **The connection is plaintext.** kms treats TLS as mandatory and its `grpc` block takes `tls_cert` and `tls_key`, but the CLI dials a remote signer with insecure transport credentials and offers no way to configure otherwise. A remote signer therefore has to serve plaintext to be reachable from the CLI, which makes network isolation the only control available today.
- **Deployment keys cannot be remote.** `ibc deploy` needs the raw private key and rejects a remote signer with `deployer signer "<alias>" must be a local key (deployment tooling needs the raw key)`. The deployer key has to stay a local file even when every other key is in kms.
- **`--attestors` needs an address, not an alias.** Deploying a client against an attestor whose key lives in kms fails with `cannot derive an address for remote signer "<alias>"`, because the CLI cannot derive an address offline for a key it does not hold. Pass the hex address instead, and read it from `attestor info` or from the key before you move it.

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
