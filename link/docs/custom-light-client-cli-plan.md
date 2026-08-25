<!-- SPDX-License-Identifier: Apache-2.0 -->

# Custom light clients in a custom-compiled CLI

## Goal

Allow a downstream Go module to compile an `ibc` binary with additional light-client proof generators and run them through the existing operator interface:

```sh
ibc relayer run
```

The initial extension point is compile-time registration. Runtime plugin loading and embedding the relayer as a library are out of scope.

## Decisions

- Expose the extension domain as `link/lightclient`; keep proof generation as
  the runtime capability within that package.
- Pass custom factories explicitly when constructing the CLI; do not use package-level registration or `init` side effects.
- Make the CLI reusable by a downstream `main` package while keeping bootstrap, services, config structs, and relayer lifecycle internal.
- Preserve the standard binary's behavior by constructing it with no custom factories.
- Keep built-in attestation resolution internal, reject attempts to override
  its type name, and validate custom parameters before creating generators.
- Remove `link/app`; an embeddable relayer lifecycle is not required for the initial use case.

## Target API

The public API should be small enough to support a downstream entrypoint like this:

```go
package main

import (
	"os"

	"github.com/cosmos/ibc/link/cli"
	"github.com/cosmos/ibc/link/lightclient"
	"example.com/acme/myclient"
)

func main() {
	registry := lightclient.NewRegistry()
	if err := registry.Register("my-client", myclient.Factory{}); err != nil {
		panic(err)
	}

	root := cli.NewRootCmd(cli.Options{
		Relayer: cli.RelayerOptions{LightClients: registry},
	})
	os.Exit(cli.Execute(root))
}
```

`cli.Options` should expose only extension inputs that the custom binary needs.
Initially that is the light-client registry, nested under `RelayerOptions` because
it affects only `ibc relayer`. Configuration paths, logging, signals, and command
flags remain owned by the CLI.

## Implementation

### 1. Rename and tighten the light-client API

- Rename `link/lightclient` to `link/lightclient`.
- Keep `ProofGenerator`, `Factory`, `Deps`, `ClientEnd`, `RawParams`, and `Registry` public under `link/lightclient`.
- Review every exported field and method for what a third-party implementation actually needs.
- Keep counterparty chain access narrow. Add methods only when required by a real custom generator.
- Document that registration is complete before command execution and registry reads begin.
- Test duplicate names, nil factories, parameter decoding, and deterministic registered-type reporting.

### 2. Make CLI construction importable

- Move root command construction and execution from `link/cmd/ibc` into an importable `link/cli` package.
- Make `cli.NewRootCmd` the primary composition API and keep execution separate.
- Introduce `cli.Options{Relayer: cli.RelayerOptions{...}}` and thread
  `RelayerOptions.LightClients` into the relayer command constructor.
- Keep command flags local to command constructors and retain a single place that assembles the command tree.
- Make `link/cmd/ibc/main.go` a thin standard entrypoint that calls `cli.Run(cli.Options{})`.
- Preserve command names, flags, help text, environment behavior, signal handling, output, and exit codes.

If moving the entire command tree at once creates excessive churn, first extract an importable root constructor plus the relayer command family. Do not ship a custom binary that silently omits the other existing `ibc` commands.

### 3. Inject the registry into `ibc relayer run`

- Replace the hard-coded `bootstrap.BuildRelayer(cfg, nil)` call with the registry supplied through `cli.Options`.
- Restore attestation's original internal `ResolveGenerator` path; do not adapt
  it to the public factory API.
- Dispatch only non-attestation client types through the custom registry.
- Keep the built-in `attestation` name reserved.
- Ensure both the standard and custom binaries use the same relayer startup, migration, readiness, graceful-shutdown, and error paths.

### 4. Remove the broader embedding API

- Delete `link/app/app.go` and its public lifecycle types.
- Move any useful lifecycle tests to the CLI or bootstrap layer.
- Update package documentation so it directs custom-client authors to `link/lightclient` and `link/cli`.
- Confirm no production or test package depends on `link/app` before removal.

### 5. Adapt the end-to-end fixture

- Replace the current in-process `link/app` test host with a test custom binary built through the public CLI API.
- Give the fixture a custom factory and a distinct configured client type.
- Start it with `ibc relayer run`, wait for the normal readiness event, submit traffic through the normal relayer API, and stop it through the normal signal path.
- Assert that the custom factory is selected, receives decoded parameters and
  relayer-provided dependencies, and allows the ordinary relayer command to
  reach readiness. Full proof correctness remains the custom implementation's
  responsibility and belongs in that implementation's own tests.
- Retain a negative test showing that the standard binary rejects the same unregistered client type.

### 6. Document downstream builds

- Add a minimal custom-client module example with its `Factory`, `ProofGenerator`, and `main` packages.
- Document version pinning to the IBC CLI module and the compatibility expectations of the public API.
- Show how to build, name, and invoke the resulting binary.
- Make clear that the official binary cannot load arbitrary custom Go implementations at runtime.

## Verification

- Unit-test registry injection and collision handling.
- Exercise command construction through both the standard and custom binaries.
- Run existing CLI tests to demonstrate command compatibility.
- Run IBC CLI unit tests and linting.
- Run the custom-binary end-to-end startup test through `ibc relayer run`.
- Compare `ibc --help` and `ibc relayer --help` before and after the extraction for unintended changes.

## Compatibility and release boundary

Treat `link/lightclient` and the minimal constructor surface in `link/cli` as public APIs. Everything below them remains internal and may evolve without supporting downstream imports. Because custom clients are compiled into the binary, downstream implementations must rebuild when upgrading to an incompatible IBC CLI release.

## Deferred work

- Loading custom implementations into an existing official binary.
- Go shared-object plugins.
- Subprocess or RPC proof-generator protocols.
- A general-purpose embeddable relayer service.
- Public configuration or bootstrap types beyond what proof-generator factories require.
