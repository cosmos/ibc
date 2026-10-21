<!-- SPDX-License-Identifier: Apache-2.0 -->

# Updating the reference pages

Use this procedure when a change affects the CLI commands or flags, configuration surface, public
API under `proto/cli/`, code cited by the reference prose, or build inputs represented on the pages.
Internal changes that cannot affect those surfaces do not require regeneration.

The generated pages are:

| Page | Derived from |
|------|--------------|
| [5-configuration.md](../5-configuration.md) | config structs and `cli/internal/config/testdata/sample.yml` |
| [6-cli-commands.md](../6-cli-commands.md) | the built command tree |
| [7-api.md](../7-api.md) | `proto/cli/*.proto` |

Run the commands below from the repository root. They require Python, Go, and `buf`.

Never edit between a `<!-- GEN:... START -->` marker and its `END`. To change the generated example
configuration, edit `cli/internal/config/testdata/sample.yml`.

## 1. Find what moved

```sh
python3 docs/6-ibc-cli/tools/refgen.py all --check
python3 docs/6-ibc-cli/tools/refgen.py all --plan
```

Both commands run the CLI, building it when needed. Resolve `--check` errors as well as plan entries:
`--plan` and `--report` omit notice-placement and prose-citation checks. Read
[REFERENCE.md](REFERENCE.md) for the reported kinds and exit codes.

## 2. Resolve decisions

Fix missing or stale information at its source when possible. Do not guess facts the source does not
establish, use `SKIP_FIELDS` or `CLI_EXCLUDED` merely to silence a refusal, or bypass validation with
`REFGEN_NO_REQUIRED_FLAGS`, `REFGEN_NO_REQUIRED_KEYS`, or `REFGEN_NO_BUILD`. Report an unresolved
decision as a blocker using the hand-back format in [REFERENCE.md](REFERENCE.md).

## 3. Regenerate

```sh
python3 docs/6-ibc-cli/tools/refgen.py all
```

## 4. Update prose

Read each changed region against its source and correct the surrounding prose. Also update the
hand-written pages in `docs/6-ibc-cli/` when the change affects behavior, examples, defaults, output,
or configuration that they describe; the generator cannot detect those inconsistencies.

## 5. Verify

```sh
python3 docs/6-ibc-cli/tools/refgen.py all --check
```

If `refgen.py` changed, also run:

```sh
python3 docs/6-ibc-cli/tools/test-refgen.py
python3 docs/6-ibc-cli/tools/test-refgen-e2e.py
```

When preparing a pull request, generate its documentation report with
`python3 docs/6-ibc-cli/tools/refgen.py all --report` and identify any prose written by hand or work
handed back. The documentation work is complete when affected prose has been reviewed and `--check`
is clean. If unresolved blockers remain, report the work as incomplete and list them using the
hand-back format in [REFERENCE.md](REFERENCE.md).

Read [AUDIT.md](AUDIT.md) only when the task is to establish whether the generated pages are true,
rather than whether they are current.
