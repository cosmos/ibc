<!-- SPDX-License-Identifier: Apache-2.0 -->

# Generated code

This directory contains committed, shared code generated from canonical schemas and contract artifacts.
Generated packages must not be edited by hand.

## Solidity Go bindings

`go/solidity-abi` contains Go bindings generated from the Foundry artifacts built in
`e2e/internal/harness/environment/solidityibc/contracts`. Run `just e2e::test-apps` from the repository root
to regenerate them. `just e2e::check-stale` rebuilds the artifacts and fails when the committed bindings are
stale.

Bindings for contracts imported from `solidity-ibc-eureka` remain owned by that dependency until its Solidity
sources migrate into this monorepo. Bindings generated here cover repository-owned test contracts and deployable
artifacts that the upstream Go binding module does not publish.
