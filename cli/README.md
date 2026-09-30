<!-- SPDX-License-Identifier: Apache-2.0 -->

# IBC CLI

IBC CLI deploys IBC onto a chain and runs the relayer and attestor
processes that carry packets across it.

See [`docs/6-ibc-cli/`](../docs/6-ibc-cli) for guides on operating IBC CLI.
Start with [Deploy IBC and send a token](../docs/6-ibc-cli/2-tutorial-deploy-ibc-and-send-a-token.md),
which brings up two chains, deploys the stack on both, and moves a token
between them.

## Prerequisites

- [Go](https://go.dev/doc/install) 1.26.6 or later

## Support

| | Supported |
| --- | --- |
| Chain types | `evm` |
| Light client types | `attestation` |

## CLI Commands

The `ibc` binary covers the whole lifecycle: creating a configuration and keys,
deploying IBC and its applications onto chains, running the relayer and attestor, and sending transactions and queries against what you
deployed. See [CLI commands](../docs/6-ibc-cli/6-cli-commands.md) for the full
reference.

```bash
# build the CLI
make build

# create a new config at ~/.ibc/ibc.yml
./bin/ibc config new

# migrate the database (skip if running migrations on relayer startup.
# `relayer run` migrates on startup automatically unless passed --no-migrate)
./bin/ibc migrate up

# generate a new local signing key, or import an existing private key
./bin/ibc keys new ecdsa <name>
./bin/ibc keys import ecdsa <name> --private-key <hex>

# deploy IBC between two configured chains (idempotent; a rerun continues a partial deployment)
./bin/ibc deploy core --chain <chainA> --yes
./bin/ibc deploy core --chain <chainB> --yes
./bin/ibc deploy client --chain <chainA> --counterparty-chain <chainB> --yes
./bin/ibc deploy client --chain <chainB> --counterparty-chain <chainA> --yes

# deploy the GMP app on each chain, so contracts can be called across the connection
./bin/ibc deploy gmp --chain <chainA> --yes
./bin/ibc deploy gmp --chain <chainB> --yes

# deploy an IFT token on each chain, then register the bridge between the two
./bin/ibc deploy ift --name <name> --symbol <symbol> --chain <chainA> --yes
./bin/ibc deploy ift --name <name> --symbol <symbol> --chain <chainB> --yes
./bin/ibc deploy ift-bridge --chain-a <chainA> --ift-a <addressA> --chain-b <chainB> --ift-b <addressB> --yes

# mint the token, then send it across a connection
./bin/ibc tx ift mint --chain <chainA> --ift <addressA> --to <recipient> --amount <baseUnits> --from <signer>
./bin/ibc tx ift send --chain <chainA> --ift <addressA> --client-id <clientID> --to <receiver> --amount <baseUnits> --from <signer>

# run the relayer and/or attestor after populating config
./bin/ibc relayer run
./bin/ibc attestor run
```

## Configuration

See [Configuration](../docs/6-ibc-cli/5-configuration.md) for the full 
configuration reference, or [`internal/config/ibc.yml`](internal/config/ibc.yml) 
for a worked example.

## E2E

Repository-wide black-box tests live in [`../e2e/`](../e2e/README.md), with the
harness in `../e2e/internal/harness` as a separate Go module. From the
repository root, `make -C e2e doctor && make -C e2e test` runs the suite.

Outbound TLS and mTLS through the real CLI are covered there too, in
[`TestOutboundTLS`](../e2e/outbound_tls_test.go). Unlike most of that suite it
needs no Docker or blockchain nodes, running against local service fixtures
instead; focus it on its own with:

```sh
make test-e2e
```

## Outbound TLS

Remote signers and attestors enable TLS when a `tls:` block is present;
`tls: {}` uses system roots. Remote provers require an `http://` or `https://`
URL, whose scheme enables TLS. A prover's optional `tls:` block requires HTTPS.
Prover URLs may include a path prefix (such as `/grpc`), but must not contain
a query string or fragment. Chain RPC supports HTTP(S), WS(S), IPC paths
(absolute or relative, such as `/tmp/geth.ipc` or `geth.ipc`), and `stdio:`.
IPC paths are resolved relative to the CLI home directory and checked for
reachability when connecting, not during static validation. Network URLs must
include a host and a valid port when specified; authentication query strings
are allowed for chain endpoints. The separate chain `ws` field still requires
a `ws://` or `wss://` URL.
All three support the same options:

```yaml
tls:
  caFile: /tls/ca.crt
  certFile: /tls/client.crt
  keyFile: /tls/client.key
  serverName: service.example.com
  minVersion: "1.2"
```

Omit `certFile` and `keyFile` for one-way TLS; supply both for mTLS. Certificate
and CA files are parsed during config validation, so a bad PEM or a mismatched
cert/key pair fails at load rather than at first connection. `caFile` replaces
the system roots rather than adding to them. Client certificates reload on
each handshake, but CA changes require a restart. A reload that fails — for
example, because it raced a separate cert/key update — falls back to the last
certificate that loaded successfully, so the handshake doesn't fail over a
transient, self-correcting read. `insecureSkipVerify: true` disables server
verification, logs a warning, and cannot be combined with `caFile`.

Remote prover probes warn on failure during relayer startup, but fail
`ibc config validate --live`. Each probe has a five-second timeout during
startup or the normal one-minute RPC timeout under `--live`; probes run
concurrently across client ends, so an unresponsive endpoint adds roughly one
probe's worth of delay rather than multiplying by how many are unresponsive.

Plaintext remote attestors now use HTTP/2 (h2c), matching remote provers. An
HTTP/1-only proxy in front of an attestor must support h2c or be reconfigured.
