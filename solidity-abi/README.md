<!-- SPDX-License-Identifier: Apache-2.0 -->

# Solidity Go bindings

This module contains committed Go bindings generated from Solidity contract artifacts, shared by the other
modules in this repository. The generated packages must not be edited by hand.

The bindings are generated from the Foundry artifacts built in
`e2e/internal/harness/environment/solidityibc/contracts`. Run `make test-apps` from the repository root to
regenerate them. `make check-test-apps` rebuilds the artifacts and fails when the committed bindings are stale.

Bindings for contracts imported from `solidity-ibc-eureka` remain owned by that dependency until its Solidity
sources migrate into this monorepo. Bindings generated here cover repository-owned test contracts and deployable
artifacts that the upstream Go binding module does not publish.
