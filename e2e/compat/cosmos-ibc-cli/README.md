<!-- SPDX-License-Identifier: Apache-2.0 -->

# Compatibility shim: `github.com/cosmos/ibc/cli`

This module exists only to break a dependency loop between this repository and
[`skip-mev/catalyst`](https://github.com/skip-mev/catalyst). It adds no
behaviour. Delete it when catalyst imports the renamed module (see
[Removing it](#removing-it)).

## The loop

The CLI module was renamed from `github.com/cosmos/ibc/cli` to
`github.com/lfdt-ibc/ibc/cli`. The e2e load tests depend on catalyst, and
catalyst depends back on a released version of this repository's CLI API under
the old path:

```text
lfdt-ibc/ibc/e2e ──► skip-mev/catalyst ──► cosmos/ibc/cli/api/v2/relayer   (old path, released)
        │
        └──────────────────────────────►  lfdt-ibc/ibc/cli                 (new path, ../cli)
```

This is not a Go import cycle. The problem is that both arrows end at the same
code, and Go cannot satisfy both:

- **Replacing the old path with `../cli` fails.** That directory already
  declares itself as `github.com/lfdt-ibc/ibc/cli`, and Go refuses to use one
  directory for two module paths.
- **Using the released `github.com/cosmos/ibc/cli` builds but panics at
  startup.** The e2e binary then links two copies of the generated relayer API.
  Both register `relayer.proto`, and protobuf panics on the duplicate:
  `proto: file "relayer.proto" is already registered`.

## How the shim breaks it

This directory is a module that declares the old path,
`github.com/cosmos/ibc/cli`, but holds no generated code. Its
`api/v2/relayer` package only aliases the identifiers catalyst uses to the
renamed package:

```go
type RelayRequest = relayer.RelayRequest // relayer = github.com/lfdt-ibc/ibc/cli/api/v2/relayer
```

`e2e/go.mod` points the old path here:

```text
replace github.com/cosmos/ibc/cli => ./compat/cosmos-ibc-cli
```

Catalyst compiles against the old import path, the types are the renamed
package's own types, and `relayer.proto` is registered once.

If a catalyst upgrade fails to build with an undefined identifier from
`github.com/cosmos/ibc/cli/api/v2/relayer`, add an alias for it in
[`api/v2/relayer/relayer.go`](api/v2/relayer/relayer.go).

## Removing it

Once a catalyst release imports `github.com/lfdt-ibc/ibc/cli`:

1. Bump `github.com/skip-mev/catalyst` in `e2e/go.mod` to that release.
2. Delete the `github.com/cosmos/ibc/cli => ./compat/cosmos-ibc-cli` replace
   line and this directory.
3. Run `go mod tidy` in `e2e/`, then `go build ./...` and
   `go test -count=1 -run '^$' ./...` to confirm nothing still imports the old
   path.
