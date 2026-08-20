<!-- SPDX-License-Identifier: Apache-2.0 -->

# IBC Link Development Guide for AI Agents

- Use `make lint-fix` to auto-format and lint code before finishing work.
- Don't add verbose comments. Be concise.
- `gen/` at the repository root holds generated code only, and its layout is fixed by an ADR — don't
  move it or put anything handwritten there. Generator scripts belong in the root `scripts/`
  directory, not beside their output. The one exception is `gen/README.md`, which documents the tree.
- Repository-wide black-box e2e lives in `../e2e`, with its harness in
  `../e2e/internal/harness` as a separate Go module. The real Link relayer submits packets through
  ICS26Router with attestation proofs signed by harness-managed attestors. Keep
  `make -C e2e test` green when changing that transport contract.
