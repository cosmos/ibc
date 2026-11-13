#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Turns govulncheck JSON into a markdown report of the vulnerable dependencies, grouped by
# library, listing every advisory and its CVEs. Advisories the repository has accepted are
# reported separately from the ones that need a fix.
#
#   make vulncheck-json VULNCHECK_JSON=vuln.json
#   ./scripts/vulncheck-report.sh vuln.json                    # markdown to stdout
#   ./scripts/vulncheck-report.sh vuln.json --counts counts.env # also write counts
#   ./scripts/vulncheck-report.sh --allow-ghsas                 # accepted list as GHSAs
#
# Accepted advisories are read from cli/osv-scanner.toml, the one list of them. osv-scanner
# and Scorecard read it only from the directory of each manifest, so every go.mod has an
# identical copy next to it; a copy that is missing or differs is an error, so the copies
# cannot drift apart.
#
# Only advisories reachable from our code are reported: a finding qualifies when its call
# trace reaches a specific function, which is the same set `make vulncheck` details. CVE and
# GHSA identifiers come from the advisory aliases, which only the JSON output carries.
#
# With --counts, writes `key=value` lines (unexpected, accepted, total, libraries) suitable
# for sourcing or for appending to $GITHUB_OUTPUT.
#
# With --allow-ghsas, prints the accepted advisories as GitHub Advisory (GHSA) identifiers,
# comma-separated, the format actions/dependency-review-action's allow-ghsas input wants,
# and exits. GHSA entries are passed through. Go advisories are keyed differently -- GO-ID
# in the Go database, GHSA in GitHub's -- so they are mapped through the Go database's alias
# list. An entry with no GHSA alias exists only in the Go database, which means the action
# cannot report it and there is nothing to allow; that is reported on stderr and skipped. A
# lookup that fails is an error rather than a skip: silently dropping an entry would fail
# the dependency review on an advisory this repository has already accepted.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

REPORT=""
COUNTS=""
ALLOW_GHSAS=""
OSV_CONFIG="${REPO_ROOT}/cli/osv-scanner.toml"
GO_VULN_DB="https://vuln.go.dev"
# Our own module paths are shown relative to this, so "Affects" reads "cli, e2e".
OUR_PREFIX="github.com/lfdt-ibc/ibc/"

die() {
  echo "vulncheck-report: $*" >&2
  exit 2
}

# The header comment is the help text. Printing it up to the first non-comment line, rather
# than to a hardcoded line number, keeps the two from drifting apart as the comment is
# edited -- a stale range silently spills the script's own code into `--help`.
usage() {
  sed -n '3,${/^#/!q; s|^# \{0,1\}||; p;}' "${BASH_SOURCE[0]}"
}

# `shift 2` past the end of the arguments fails under `set -e`, which would exit 1 with no
# message at all, so a flag's value is checked before it is consumed.
need_value() {
  [[ $# -ge 2 && -n "$2" ]] || die "$1 requires a value"
}

while (($# > 0)); do
  case "$1" in
    --counts) need_value "$@"; COUNTS="$2"; shift 2 ;;
    --allow-ghsas) ALLOW_GHSAS=1; shift ;;
    -h | --help) usage; exit 0 ;;
    -*) die "unknown argument '$1'" ;;
    *) REPORT="$1"; shift ;;
  esac
done

command -v jq >/dev/null || die "jq is required"
[[ -f "${OSV_CONFIG}" ]] || die "'${OSV_CONFIG}' does not exist"

for mod in $(git -C "${REPO_ROOT}" ls-files '*go.mod'); do
  copy="${REPO_ROOT}/$(dirname "${mod}")/osv-scanner.toml"
  cmp -s "${OSV_CONFIG}" "${copy}" \
    || die "'${copy}' is missing or differs from cli/osv-scanner.toml; copy it over"
done

# Parsed with Python's standard TOML parser (3.11+); an entry without an id is an error.
ignored="$(python3 -I -c '
import sys, tomllib
with open(sys.argv[1], "rb") as f:
    print(" ".join(sorted({v["id"] for v in tomllib.load(f).get("IgnoredVulns", [])})))
' "${OSV_CONFIG}")" || die "could not read the ids in ${OSV_CONFIG}"

if [[ -n "${ALLOW_GHSAS}" ]]; then
  ghsas=""

  # Two Go advisories can share a GHSA alias, so each GHSA is added once.
  add() {
    case ", ${ghsas}, " in
      *", $1, "*) ;;
      *) ghsas="${ghsas:+${ghsas}, }$1" ;;
    esac
  }

  for id in ${ignored}; do
    if [[ "${id}" =~ ^GHSA-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}$ ]]; then
      add "${id}"
      continue
    fi

    # Checked before it reaches a URL: a typo should be a clear error, not an odd request.
    [[ "${id}" =~ ^GO-[0-9]{4}-[0-9]+$ ]] \
      || die "'${id}' in ${OSV_CONFIG} is neither a GHSA nor a Go advisory ID"

    json="$(curl -fsS --retry 3 --retry-delay 2 "${GO_VULN_DB}/ID/${id}.json")" \
      || die "could not read ${id} from ${GO_VULN_DB}"

    mapped="$(printf '%s' "${json}" | jq -r '(.aliases // [])[] | select(startswith("GHSA-"))')"

    if [[ -z "${mapped}" ]]; then
      echo "${id}: no GHSA alias, so GitHub's advisory database cannot report it" >&2
      continue
    fi

    for ghsa in ${mapped}; do
      add "${ghsa}"
    done
  done

  printf '%s\n' "${ghsas}"
  exit 0
fi

[[ -n "${REPORT}" ]] || die "usage: vulncheck-report.sh <govulncheck-json> [--counts <file>] | --allow-ghsas"
[[ -f "${REPORT}" ]] || die "report '${REPORT}' does not exist"

# One row per advisory: library, version in use, advisory, CVEs, fixed version, summary and
# the modules of ours it reaches. Grouping happens in awk below, where the ordering and the
# accepted/unexpected split are decided.
rows="$(jq -rs --arg prefix "${OUR_PREFIX}" '
  (reduce (.[] | select(.osv) | .osv) as $o ({}; .[$o.id] = $o)) as $osvs
  | [ .[] | select(.finding) | .finding | select(.trace[0].function) ]
  | map({
      id: .osv,
      # trace[0] is the vulnerable function, so its module is the library at fault.
      library: .trace[0].module,
      version: (.trace[0].version // ""),
      fixed: (.fixed_version // ""),
      # The trace ends in our code, which is how a finding is attributed to our modules.
      ours: ((.trace[-1].module // "") | if startswith($prefix) then .[($prefix | length):] else . end),
      cves: (($osvs[.osv].aliases // []) | map(select(startswith("CVE-"))) | join(" ")),
      summary: (($osvs[.osv].summary // "") | gsub("[|\n]"; " "))
    })
  | group_by(.id)
  # One row per advisory, but our modules are separate go.mod files: the same advisory can
  # be reached at a different version from each, so versions are unioned rather than taken
  # from an arbitrary finding in the group.
  | map(.[0] + {
      versions: (map(.version) | map(select(. != "")) | unique | join(", ")),
      ours: (map(.ours) | unique | join(", "))
    })
  | .[]
  | [.library, .versions, .id, .cves, .fixed, .ours, .summary]
  | @tsv
' "${REPORT}")"

printf '%s\n' "${rows}" | awk -F'\t' -v ignored="${ignored}" -v counts_file="${COUNTS}" '
function is_ignored(id,    i, n, parts) {
  n = split(ignored, parts, " ")
  for (i = 1; i <= n; i++)
    if (id == parts[i]) return 1
  return 0
}

# The standard library is reported as module "stdlib" with a "v" version; it upgrades with
# the toolchain rather than with a dependency bump, so it is labelled and sorted apart.
function display_library(lib) { return lib == "stdlib" ? "Go standard library" : lib }
# gsub on a local copy: gensub is a GNU extension and this has to run under mawk too.
function commas(s) { gsub(/ /, ", ", s); return s }

# Group metadata is unioned, never assigned: our modules are separate go.mod files, so one
# library can legitimately be in use at several versions -- across two advisories or within
# a single one -- and each advisory reaches a different set of our modules.
function add_unique(cur, item,    n, parts, i) {
  if (item == "") return cur
  if (cur == "") return item
  n = split(cur, parts, ", ")
  for (i = 1; i <= n; i++)
    if (parts[i] == item) return cur
  return cur ", " item
}
function merge_list(cur, list,    n, parts, i) {
  n = split(list, parts, ", ")
  for (i = 1; i <= n; i++)
    cur = add_unique(cur, parts[i])
  return cur
}
function display_version(lib, v) {
  if (lib != "stdlib") return v
  sub(/^v/, "", v)
  return "go" v
}
# The version column is a list, one entry per version of the library an advisory was reached
# at, so each entry is labelled and quoted before being unioned into the heading.
function display_versions(lib, list,    n, parts, i, out) {
  n = split(list, parts, ", ")
  for (i = 1; i <= n; i++)
    if (parts[i] != "") out = add_unique(out, "`" display_version(lib, parts[i]) "`")
  return out
}

NF >= 3 {
  lib = $1; vers = $2; id = $3; cves = $4; fixed = $5; ours = $6; summary = $7
  tier = is_ignored(id) ? "accepted" : "unexpected"
  # stdlib sorts after third-party libraries: it is the least likely to be a supply-chain
  # problem and is fixed in one place.
  key = tier SUBSEP (lib == "stdlib" ? 1 : 0) SUBSEP lib
  if (!(key in seen_key)) { seen_key[key] = 1; keys[++n_keys] = key }
  n_rows[key]++
  rows[key, n_rows[key]] = id "\t" cves "\t" fixed "\t" summary
  version[key] = merge_list(version[key], display_versions(lib, vers))
  affects[key] = merge_list(affects[key], ours)
  n_tier[tier]++
  if (!(tier SUBSEP lib in seen_lib)) { seen_lib[tier SUBSEP lib] = 1; n_libs[tier]++ }
  total++
}

END {
  print "<!-- dependency-review:govulncheck -->"
  print "## Vulnerable dependencies\n"

  if (total == 0) {
    print ":white_check_mark: govulncheck reports no advisories reachable from our code."
  } else {
    printf "%d %s across %d %s need attention, %d accepted.\n",
      n_tier["unexpected"] + 0, (n_tier["unexpected"] == 1 ? "advisory" : "advisories"),
      n_libs["unexpected"] + 0, (n_libs["unexpected"] == 1 ? "library" : "libraries"),
      n_tier["accepted"] + 0
  }

  emit("unexpected", "### :rotating_light: Needs attention")
  # Printed before the accepted section: this advice is about the advisories that still need
  # a fix, and under a later heading it would read as applying to the accepted ones.
  if (n_tier["unexpected"] > 0) {
    print "Run `make vulncheck` for the call paths. Either upgrade the library, or add the"
    print "advisory to `cli/osv-scanner.toml` with a reason if no fix exists yet."
  }
  emit("accepted", "### Accepted")

  if (counts_file != "") {
    printf "unexpected=%d\n", n_tier["unexpected"] + 0 > counts_file
    printf "accepted=%d\n",   n_tier["accepted"] + 0 > counts_file
    printf "total=%d\n",      total + 0 > counts_file
    printf "libraries=%d\n",  n_libs["unexpected"] + 0 > counts_file
  }
}

# Sections are emitted per library so every advisory for one dependency is read together.
function emit(tier, heading,    i, j, key, parts, lib, r, cells) {
  if (n_tier[tier] + 0 == 0) return
  print ""
  print heading "\n"
  if (tier == "accepted") print "Listed in `cli/osv-scanner.toml` with a reason.\n"
  # keys were appended in first-seen order, but the sort key puts stdlib last.
  for (pass = 0; pass <= 1; pass++) {
    for (i = 1; i <= n_keys; i++) {
      key = keys[i]
      split(key, parts, SUBSEP)
      if (parts[1] != tier || parts[2] + 0 != pass) continue
      lib = parts[3]
      printf "#### `%s`\n\n", display_library(lib)
      printf "In use: %s", version[key]
      if (affects[key] != "") printf " — reached from %s", affects[key]
      printf "\n\n"
      print "| Advisory | CVE | Fixed in | Summary |"
      print "|---|---|---|---|"
      for (j = 1; j <= n_rows[key]; j++) {
        split(rows[key, j], cells, "\t")
        printf "| [%s](https://pkg.go.dev/vuln/%s) | %s | %s | %s |\n",
          cells[1], cells[1],
          (cells[2] == "" ? "—" : commas(cells[2])),
          (cells[3] == "" ? "**no fix yet**" : "`" display_version(lib, cells[3]) "`"),
          cells[4]
      }
      print ""
    }
  }
}
'
