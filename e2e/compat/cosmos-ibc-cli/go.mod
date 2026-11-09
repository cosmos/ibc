// SPDX-License-Identifier: Apache-2.0

// Compatibility shim that breaks the dependency loop
// e2e -> skip-mev/catalyst -> github.com/cosmos/ibc/cli (pre-rename path).
// It declares the old path and aliases it to github.com/lfdt-ibc/ibc/cli, so
// the generated protobuf types are linked once. See README.md; delete this
// module once catalyst imports github.com/lfdt-ibc/ibc/cli.
module github.com/cosmos/ibc/cli

go 1.26.10

require github.com/lfdt-ibc/ibc/cli v0.0.0

require (
	connectrpc.com/connect v1.21.1 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

// Lets the module build on its own (IDE, go vet). Ignored when e2e uses it:
// only the main module's replace directives apply.
replace github.com/lfdt-ibc/ibc/cli => ../../../cli
