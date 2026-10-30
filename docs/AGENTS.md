# AGENTS.md — the docs tree

The Mintlify site lives here. `docs/` is the site root: Mintlify resolves every
page path relative to `docs/docs.json`, and nothing above `docs/` is reachable
from the site.

## Layout

| Path | What it is |
|---|---|
| `docs.json` | Navigation and theme. **Cannot move** — this file defines the site root. |

Navigation is five `tabs`, which Mintlify renders as a horizontal bar:
**Learn** (14 — the protocol, its applications and light clients, plus the site's
landing page), **IBC CLI** (7 — get started, run the services, reference),
**IBC-Solidity** (6 — the contracts), **IBC-Go** (`latest` and `v10.1.x`, via a
`versions` block nested inside the tab) and **Specification** (42 generated
pages, grouped by section).

The split is by subject, not by task: each tab answers "what am I reading
about?". The Learn → Build journey is carried by the cards on `index.mdx`, which
is what a landing page is for. Mintlify's own docs only show `versions`
containing `tabs`; the reverse also validates and is what this site uses, so a
version switcher appears on the ibc-go tab alone.

| `ibc/` | This monorepo's docs. Unversioned, tracks `main`. |
| `ibc-go/latest/` | ibc-go docs. Edit here. |
| `ibc-go/v10.1.x/` | Frozen. Do not edit except to fix a critical error. |
| `ibc/spec/` | **Generated** from the repo's `spec/`. Never edit by hand. |
| `scripts/` | Generators that read repo source code. |
| `mintlify/` | Site machinery: stylesheet, images, brand assets, and versioning scripts. |
| `README.md` | What this directory is, for a human arriving from the repo root. |
| `package.json` | Pins the Mintlify CLI and defines `dev` and `broken-links`. |
| `.mintignore` | Paths Mintlify must not publish. Everything here that is not a page needs an entry, or it becomes a live URL. |

## Rules

- **No Solidity in `ibc-go/`.** It documents the Go implementation. Link to
  `ibc/06-ibc-solidity-contracts/` instead.
- **Never hand-edit `ibc/spec/`.** Edit `spec/` at the repo root and regenerate.
- **Numbered prefixes** order the hand-authored pages under `ibc/`: directories
  `01-introduction/`, `02-how-ibc-works/` …, files `01-`, `02-` within each.
  The numbers are a sort key for reading the repo on GitHub — Mintlify takes
  navigation order from `docs.json`, not from filenames.

  **To insert a page, append a letter to the one before it**: a page between
  `02-` and `03-` is `02a-`; between `02a-` and `03-` is `02b-`; between `02a-`
  and `02b-` is `02aa-`. Nothing else is renamed.

  **Never renumber to tidy up.** Mintlify has no slug field, so the number is
  part of the URL — `/ibc/02-how-ibc-works/05-relayer`. Renumbering changes
  URLs and needs a redirect for each one. A directory reading `01, 02, 02a, 03,
  07` is healthy; the numbers are a sort key, not a count.

  A letter rather than `02.1-` or `02-1-` is deliberate. Both of those sort
  wrongly: `02.10` lands before `02.2`, and a digit after the hyphen sorts
  before the slug's first letter, so `02-1-prereq` jumps ahead of `02-overview`.
  Letters need no zero-padding and keep the URL free of dots.

  `ibc/spec/` is generated and `ibc-go/` is imported, so neither is numbered —
  numbering them would mean changing the canonical `spec/` or diverging from
  the upstream the importer re-runs against.

  After any rename: update `docs.json`, add a `redirects` entry, fix inbound
  links, then run `node docs/mintlify/scripts/check-nav.js` and
  `npx --yes mint@4 broken-links`.
- There is no `next/` tier. Document unreleased features in `latest/` with a
  version callout.
- **Diagrams under `ibc/` are PNGs with their mermaid source kept beside them.**
  The pages render an exported image from `mintlify/images/ibc/`, and the
  mermaid that image was drawn from sits directly below it, commented out:

  ```mdx
  ![alt text](/mintlify/images/ibc/ibc-core-router.png)

  {/* Diagram source. Changed the page? Update this and re-export the PNG. */}
  {/*
  ```mermaid
  flowchart LR
    ...
  ```
  */}
  ```

  **The mermaid is the source of truth, and the two drift silently.** Change
  what a diagram shows and you must edit the mermaid *and* re-export the PNG;
  nothing checks that they agree, and a commented block is invisible to
  `mint validate`, so a broken diagram source will not fail the build.

  Use `{/* */}`, never `<!-- -->` — MDX parses an HTML comment as JSX and the
  build fails.

  Five diagrams in `ibc/06-ibc-solidity-contracts/` have no exported image yet
  and still render as live mermaid. Leave them that way until one exists.
  `ibc/spec/` and `ibc-go/` are generated and imported, so their mermaid is not
  converted.

## Links

Internal links are absolute from the docs root, with no `.mdx`:

```text
ok:    /ibc/02-how-ibc-works/03-core-router-and-store
wrong: ./03-core-router-and-store.mdx
wrong: 03-core-router-and-store
```

`mint broken-links` does not resolve relative targets, so a wrong one fails
silently. The 144 relative links in `ibc/spec/` come from the ICS sources and
are not rewritten.

A nav entry pointing off-site is an `.mdx` file with a `url`, registered in
`docs.json` like any page — see `ibc/repos/`:

```mdx
---
title: "cosmos/ibc"
url: "https://github.com/cosmos/ibc"
icon: "github"
---
```

Redirects go in the top-level `redirects` array in `docs.json`: root-relative
`source` and `destination`, no `.mdx`, no `#` or `?`, no cycles. Wildcards work
(`/ibc/old/:slug*`).

### Anchors

Measured against this build, not Mintlify's docs. Lowercased, spaces to `-`,
adjacent separators collapsed.

| In the heading | In the anchor | Example |
|---|---|---|
| `.` | `-` | `v0.53.x` → `#v0-53-x` |
| `? ! ( ) :` | dropped | `Rate limit (beta)` → `#rate-limit-beta` |
| `& + = @ # $ % /` | kept | `Gas & Fees` → `#gas-&-fees` |
| `—` | kept | `Em — dash` → `#em-—-dash` |
| backticks, `*` | stripped as markup first | `` Back`tick` `` → `#backtick-here` |

`/` stays literal in the id; percent-encode it in a link (`#x%2Fparams`).
Copy anchors from a render rather than deriving them.

## Generators

| Script | Reads | Writes |
|---|---|---|
| `scripts/refgen.py` | `cli/`, `proto/`, `gen/` | CLI reference tables in `ibc/05-ibc-cli/` |
| `scripts/specgen.py` | `spec/` | `ibc/spec/` |

Both have a `--check` mode. Nothing in CI runs it, so after changing `cli/`,
`proto/` or `gen/`, follow `scripts/AGENTS.md` and run it yourself.

Site tooling — version freezing, link checking — is in `mintlify/scripts/`.

## Skills

The skills for working on these docs live in
[cosmos/docs-skills](https://github.com/cosmos/docs-skills), not in this repo.
Three of them cover this site specifically, and their paths are written relative
to this repository's root:

| Skill | Use it when |
|---|---|
| `release-version` | freezing `ibc-go/latest/` as a `vX.Y.x` archive, or running `mintlify/scripts/version-manager.js`. |
| `update-stale-refs` | `mintlify/scripts/check-github-refs.js` flagged a GitHub link, or a page links to upstream code that has since moved. |
| `review-generated-prose` | `refgen.py` or `specgen.py` regenerated a page and the output needs reading as English, which `--check` cannot do. |

`documentation`, `house-style`, `verify-facts`, `readability` and
`docs-generator` live there too and are not specific to this site. Copy the
directory you want into `.claude/skills/`, or clone the repo somewhere Claude
Code can see it.

## Running it

```bash
cd docs && npm install && npx mint@4 dev
```
