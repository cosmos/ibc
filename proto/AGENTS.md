<!-- SPDX-License-Identifier: Apache-2.0 -->

# CLI API

When changing schemas under `proto/cli/` or their generation configuration, run
`make -C cli codegen-proto` from the repository root and include the updated bindings in `cli/api/v2/`.

When changing the public API under `proto/cli/`, follow
[`docs/6-ibc-cli/tools/AGENTS.md`](../docs/6-ibc-cli/tools/AGENTS.md).
