<!-- SPDX-License-Identifier: Apache-2.0 -->

# E2E tests

Read [README.md](README.md) for running tests, environment behavior, matrix commands, and cleanup.

- Acceptance tests use `internal/e2etest` environments and assert through route-bound application
  bindings. Hold the temporary IBC CLI driver and relayer explicitly. Transport-contract tests may
  call the concrete driver or relayer directly.
- Use the IBC CLI's public command transport types, generated RPC clients, and signer-keyfile package for
  transport and provisioning. Do not import `cli/internal` or invoke handlers in process.
- Declare test requirements and regenerate and check the matrix after changing requirements or topology.
- Preserve inner quotes in `E2E_FLAGS`, because Make expands it into a shell command:
  `make -C e2e test E2E_FLAGS="-run '^(TestA|TestB)' -count=1"`.
