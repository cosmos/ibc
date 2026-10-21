<!-- SPDX-License-Identifier: Apache-2.0 -->

# E2E tests

Read the relevant [README.md](README.md) section first:
[Running the acceptance tests](README.md#running-the-acceptance-tests) before running tests,
[Writing a test](README.md#writing-a-test) before adding one,
[Provider and topology matrix](README.md#provider-and-topology-matrix) before changing requirements
or topology, and [Debugging](README.md#debugging) for environment or cleanup behavior.

- Acceptance tests follow the explicit setup sequence in "Writing a test": `e2etest.Start`,
  `e2etest.Deploy` (which returns the driver), app constructors such as `e2etest.NewTransfer`, and
  `e2etest.StartRelayer`, asserting through the app values. Transport-contract tests may call the
  concrete driver or relayer directly.
- Application-effect wait budgets derive from the resolved Chain's `environment.Timing`, never from a
  literal tuned to instant Anvil. Test-application effect and stability observation lives with the
  e2e-only bindings that interpret those effects.
- Exercise CLI commands through the binary's public input/output contracts. Use generated RPC clients
  and `cli/keyfile` for RPC and signer provisioning.
- Declare test requirements in `e2etest.EVMRequirements`, naming a provider only when the test
  depends on it. After changing requirements or topology, run `make -C e2e generate-matrix` and
  `make -C e2e check-matrix`; `make -C e2e test` does not check the matrix.
- Preserve inner quotes in `E2E_FLAGS`, because Make expands it into a shell command:
  `make -C e2e test E2E_FLAGS="-run '^(TestA|TestB)' -count=1"`.
