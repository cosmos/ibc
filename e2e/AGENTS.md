<!-- SPDX-License-Identifier: Apache-2.0 -->

# E2E tests

Read the relevant sections of [README.md](README.md) before running tests, changing requirements or
topology, or diagnosing environment or cleanup behavior.

- Acceptance tests use `internal/e2etest` environments and assert through route-bound application
  bindings. Hold the temporary IBC CLI driver and relayer explicitly. Transport-contract tests may
  call the concrete driver or relayer directly.
- Application-effect wait budgets derive from the resolved Chain's `environment.Timing`, never from a
  literal tuned to instant Anvil. Test-application effect and stability observation lives with the
  e2e-only bindings that interpret those effects.
- Exercise CLI commands through the binary's public input/output contracts. Use generated RPC clients
  and `cli/keyfile` for RPC and signer provisioning. Do not import `cli/internal` or invoke handlers
  in process.
- Declare test requirements and regenerate and check the matrix after changing requirements or topology.
- Preserve inner quotes in `E2E_FLAGS`, because Make expands it into a shell command:
  `make -C e2e test E2E_FLAGS="-run '^(TestA|TestB)' -count=1"`.
