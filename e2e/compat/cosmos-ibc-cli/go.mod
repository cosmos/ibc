// SPDX-License-Identifier: Apache-2.0

// Compatibility shim for dependencies that still import the CLI API under its
// pre-rename path. e2e replaces github.com/cosmos/ibc/cli with this module so
// that only one copy of the generated protobuf types is linked; two copies
// panic at init on duplicate registration. Delete it once those dependencies
// (github.com/skip-mev/catalyst) import github.com/lfdt-ibc/ibc/cli.
module github.com/cosmos/ibc/cli

go 1.26.10

require github.com/lfdt-ibc/ibc/cli v0.0.0
