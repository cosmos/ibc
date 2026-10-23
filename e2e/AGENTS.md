<!-- SPDX-License-Identifier: Apache-2.0 -->

# E2E tests

Read the relevant [README.md](README.md) section first:
- [Running the acceptance tests](README.md#running-the-acceptance-tests) before running tests
- [Writing a test](README.md#writing-a-test) before adding one,
- [Provider and topology matrix](README.md#provider-and-topology-matrix) before changing requirements or topology
- [Debugging](README.md#debugging) for environment or cleanup behavior.

- Transport-contract tests may call the concrete driver or relayer directly; acceptance tests follow
  the sequence in "Writing a test".
- Application-effect wait budgets derive from the resolved Chain's `environment.Timing`, never from a
  literal tuned to instant Anvil.
- After changing requirements or topology, run `make -C e2e generate-matrix` and
  `make -C e2e check-matrix`; `make -C e2e test` does not check the matrix.
- Preserve inner quotes in `E2E_FLAGS`, because Make expands it into a shell command:
  `make -C e2e test E2E_FLAGS="-run '^(TestA|TestB)' -count=1"`.
