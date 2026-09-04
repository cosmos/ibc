<!-- SPDX-License-Identifier: Apache-2.0 -->

# IBC CLI Development Guide for AI Agents

- Use `make lint-fix` to auto-format and lint code before finishing work.
- Don't add verbose comments. Be concise.
- `gen/` at the repository root holds generated code only, and its layout is fixed by an ADR — don't
  move it or put anything handwritten there. Generator scripts belong in the root `scripts/`
  directory, not beside their output. The one exception is `gen/README.md`, which documents the tree.
- Repository-wide black-box e2e lives in `../e2e`, with its harness in
  `../e2e/internal/harness` as a separate Go module. The real relayer submits packets through
  ICS26Router with attestation proofs signed by harness-managed attestors. Keep
  `make -C e2e test` green when changing that transport contract.

## Documentation generated from this code

Three reference pages under `docs/6-ibc-cli/` are generated from this repository:

* `5-configuration.md` — from the config structs
* `6-cli-commands.md` — from the command tree and the built binary's `--help`
* `7-api.md` — from the protos

The tables are generated, the prose around them is not. Never hand-edit between a
`<!-- GEN:... START -->` marker and its `END`.

If you change a config struct, a command, a flag, or a proto:

```sh
python3 docs/6-ibc-cli/tools/refgen.py all --check   # is anything stale?
python3 docs/6-ibc-cli/tools/refgen.py all           # update the tables
```

* Exit 1 — a page is stale, and regenerating fixes it
* Exit 2 — the generator refused, and the message says what it could not read

`docs/6-ibc-cli/tools/README.md` covers the rest, including keys with no doc
comment and descriptions whose fingerprint has moved.
