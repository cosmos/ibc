<!-- SPDX-License-Identifier: Apache-2.0 -->

# Reference for the generator

The catalogue behind [AGENTS.md](AGENTS.md). That file is the procedure and
holds the rules; this one holds the detail you look up while following it.

## Plan entries

`--plan` classifies every gap. Each entry carries the file and line to go to.

| Kind | Meaning | What you do |
|------|---------|-------------|
| `stale` | a table no longer matches the source | nothing; step 3 heals it |
| `missing_marker` | the source has something the page cannot hold. Carries the rendered table, a suggested heading, and where to insert it | step 2 |
| `orphaned_marker` | a marker for something the source no longer has | step 2 |
| `curation` | a choice the source cannot express | step 2 |

Exit codes: `0` nothing to do, `1` stale or missing, `2` the tool refused. A
refusal outranks staleness, so `--plan` and `--report` still exit 2 when a page
could not be read, and name it.

`curation` holds the specific kinds below, and a dozen more. **A kind not named
here is not an exception**: each carries a `message` saying what would resolve
it, and the same three options apply — fix it at the source, ask where the code
does not say, hand it back.

## Resolving the common kinds

**Nobody has described it** (`missing_description` for a config key,
`missing_field_description` for a proto field). Add a doc comment to the
declaration, or a comment above the proto field, at the line the plan names.
The page then documents itself. Only when the declaration genuinely cannot be
edited, add an entry in `refgen.py`: `FALLBACK_DOCS` for a config key, keyed on
the **yaml key** rather than the Go field name, with the fingerprint the plan
gives you; `FIELD_DOCS` for a proto field, which carries no fingerprint and is
worth avoiding — edit the `.proto` comment instead.

**Its source changed underneath it** (`fingerprint_mismatch`). Re-read the
sentence against the code at the named line, correct it if it is now wrong, and
record the new fingerprint. This fires on a reworded validation message too,
which changes what the required column says.

**The page has nowhere to put it** (`missing_marker`). Add the heading at the
suggested point, then its `START`/`END` pair, then the prose introducing it.

**Its subject is gone** (`orphaned_marker`). Delete the marker pair and the
prose that existed only to introduce it. This is the one case where deleting a
marker is correct: the subject is gone, not merely unreadable.

**The parser could not read it** (`unreadable_declaration`, `unreadable_member`,
`unreadable_default`). The source grew a construct this tool does not
understand, and it refused rather than drop rows — a short table reads exactly
like a complete one. You may teach it the construct. The bar: the refusal is
gone, both suites pass, and the pages are unchanged except where you meant.
Otherwise hand it back with a proposed patch — a parser change that reads the
source wrongly fails silently, and no Go reviewer will catch it.

## The probe fixtures

`probe-config.yml` and `probe-config-otel.yml` are working configs the binary
loads. They are not published and no reader sees them: they exist so the
**Default or required** column can be asked rather than inferred. For each key,
the generator removes exactly that key from one of them and runs
`ibc config validate`. The key is required when the binary objects to the path
that was removed — a path, not a phrase, so rewording a validation message
changes nothing.

Two files because `observability.type` cannot be `simple` and `otel` at once.

Three things follow, and they are the only maintenance these carry:

- **Adding a config key costs you nothing.** A key absent from a config that
  loads is proof the program runs without it, so it reads `optional` without
  anyone touching a fixture.
- **Making a key required stops the page**, naming the key, because the fixture
  is now a config the CLI rejects. Put a value for it in both fixtures and the
  page generates again.

  Take the value from the source, the way every other fact here is taken: the
  key's doc comment, what its validation accepts, an existing example, or a
  value the package's own tests use. Copy the shape the fixture already uses
  for its neighbours -- these are two-chain configs with `example.com` hosts,
  not real deployments.

  If nothing in the tree says what a valid value looks like, that is a
  hand-back and not a guess: a fixture value invented to get past a refusal can
  make a required key read as optional, or make the whole page refuse for a
  reason nobody can trace. Say which key, and that the declaration needs to say
  what it accepts.
- **Removing or renaming a key heals itself.** The binary calls the old key
  unknown, and it is dropped from the fixture for that run.

A fixture that fails to load for any other reason refuses
(`stale_probe_fixture`) and quotes what the binary stopped at.

## Choices the source cannot make

These live in `refgen.py`:

| To change | Edit |
|-----------|------|
| the order command groups appear in | `CLI_SECTION_ORDER`. Membership is discovered; a new group missing from the list refuses |
| commands left off the page entirely | `CLI_EXCLUDED`, currently `completion` and `help` |
| where a pointer field's default comes from | `DEFAULT_CONSTS`, which names the constant rather than repeating its value |
| a pointer field with no named default | `NO_NAMED_DEFAULT`, so a new one cannot quietly read as optional |
| a config field to leave out | `SKIP_FIELDS` |
| which services the API page covers | `SERVICES` |
| which program the pages document | `CLI_PACKAGE` |

## Telling your own suite failures from the expected ones

A case asserts the *kind* of refusal it expects, so an edit that makes the
generator stop refusing surfaces as a failure naming the very refusal you were
told to expect. Get the list from a tree without your change and compare:

```sh
git worktree add /tmp/refgen-clean HEAD
python3 /tmp/refgen-clean/docs/6-ibc-cli/tools/test-refgen.py
git worktree remove /tmp/refgen-clean
```

A worktree, not `git stash` — the stash stack is shared, and popping it can
take changes that are not yours. Any failure in yours and not there is yours.

## The hand-back format

One line per item, naming what is blocked and what you would propose:

```
### Handed back

- `ChainConfig` declares an inline embedded struct the parser does not read,
  so 5-configuration.md cannot regenerate. Proposed: teach the parser to
  follow an inline embed, or tag the field `yaml:"-"` if readers never write it.
```
