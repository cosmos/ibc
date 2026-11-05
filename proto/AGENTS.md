<!-- SPDX-License-Identifier: Apache-2.0 -->

# CLI API

When changing schemas under `proto/cli/` or their generation configuration, run
`make -C cli codegen-proto` from the repository root and include the updated
bindings in `cli/api/v2/`.

The reference pages under `docs/ibc/05-ibc-cli/04-reference/` carry tables
generated from this directory. When changing the public API under `proto/cli/`,
**the documentation must be regenerated before the pull request is opened**,
and the regenerated pages belong in the same pull request as the code.

Follow [`docs/scripts/AGENTS.md`](../docs/scripts/AGENTS.md).
It covers what to run, what the generator refuses and why, and what you may
write yourself.

Never hand-edit between a `{/* GEN:... START */}` marker and its `END`: the
next run overwrites it.
