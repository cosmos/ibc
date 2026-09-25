#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Turns `capslock -output=compare` output into a review-ordered markdown report, so that a
# dependency newly gaining EXEC, NETWORK or FILES is not buried among analysis noise.
#
#   ./scripts/capslock-classify.sh <report.txt>                     # markdown to stdout
#   ./scripts/capslock-classify.sh <report.txt> --counts counts.env # also write counts
#
# Additions are sorted into three tiers:
#
#   high   a capability class a compromised release typically gains. Read the call path.
#   moved  the same capability disappeared from a like-named package in this diff, which
#          is what a vendored or renamed package looks like, not new privilege.
#   low    capabilities that say more about Capslock's analysis than about privilege
#          (UNANALYZED, REFLECT, UNSAFE_POINTER, CGO, RUNTIME).
#
# The tiers order the report; nothing is dropped. `moved` requires the added and removed
# packages to share a final path element, so an unrelated removal elsewhere in the diff
# cannot explain away a real finding.
#
# A high-signal capability is demoted to `moved` only on an exact pairing: the two import
# paths are equal once version elements are dropped, as in semconv/v1.41.0/otelconv ->
# semconv/v1.43.0/otelconv or a /v2 major bump. That is a package the compiler resolves
# differently, not a package that gained privilege, and it recurs on every dependency
# bump that crosses a version directory. A merely like-named pairing still stays in
# `high`, named but not acted on: `high` is the count the CI gate reads, and a rename is
# the shape an attacker would pick to disarm it.
#
# With --counts, writes `key=value` lines (high, moved, low, added, removed) suitable for
# sourcing or for appending to $GITHUB_OUTPUT.

set -euo pipefail

# Capability classes that justify blocking a dependency update until someone reads the
# call path. MODIFY_SYSTEM_STATE matches its subcategories too (/ENV, /SIGNALS, ...).
HIGH_SIGNAL="EXEC NETWORK FILES ARBITRARY_EXECUTION SYSTEM_CALLS MODIFY_SYSTEM_STATE"

REPORT=""
COUNTS=""

die() {
  echo "capslock-classify: $*" >&2
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
    --high-signal) need_value "$@"; HIGH_SIGNAL="$2"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    -*) die "unknown argument '$1'" ;;
    *) REPORT="$1"; shift ;;
  esac
done

[[ -n "${REPORT}" ]] || die "usage: capslock-classify.sh <report.txt> [--counts <file>]"
[[ -f "${REPORT}" ]] || die "report '${REPORT}' does not exist"

# The report is read twice: once to learn which capabilities disappeared (so additions can
# be recognised as moves), then again to emit findings with their call paths.
awk -v high_signal="${HIGH_SIGNAL}" -v counts_file="${COUNTS}" '
function basename(path,    n, parts) {
  n = split(path, parts, "/")
  return parts[n]
}

# Two import paths address the same package when they differ only in version elements:
# semconv/v1.41.0/otelconv and semconv/v1.43.0/otelconv, or foo/bar and foo/v2/bar.
# Dropping those elements is what makes a pairing exact rather than merely plausible.
function canonical(path,    i, n, parts, out) {
  n = split(path, parts, "/")
  for (i = 1; i <= n; i++) {
    if (parts[i] ~ /^v[0-9]+(\.[0-9]+)*$/) continue
    out = (out == "" ? parts[i] : out "/" parts[i])
  }
  return out
}

# A capability that left another package only explains this addition if the two packages
# plausibly are the same package under a new path, which vendoring and renames produce.
# Requiring a matching final path element keeps an unrelated removal elsewhere in the diff
# from explaining away a genuinely new capability. An exact pairing is preferred over a
# like-named one, because only an exact pairing can demote a high-signal capability.
function find_move(cap, pkg,    i, n, parts, base, canon, fallback) {
  if (!(cap in removed_list)) return ""
  base = basename(pkg)
  canon = canonical(pkg)
  n = split(removed_list[cap], parts, " ")
  for (i = 1; i <= n; i++) {
    if (parts[i] == pkg) continue
    if (canonical(parts[i]) == canon) return parts[i]
    if (fallback == "" && basename(parts[i]) == base) fallback = parts[i]
  }
  return fallback
}

function is_high(cap,    i, n, parts) {
  n = split(high_signal, parts, " ")
  for (i = 1; i <= n; i++) {
    if (cap == parts[i]) return 1
    # MODIFY_SYSTEM_STATE also covers MODIFY_SYSTEM_STATE/ENV and friends.
    if (index(cap, parts[i] "/") == 1) return 1
  }
  return 0
}

# Both header forms are "Package <pkg> <phrase> <cap> <suffix>".
function parse(line, phrase, suffix,    n, rest) {
  sub(/^Package /, "", line)
  n = index(line, phrase)
  if (n == 0) return 0
  cur_pkg = substr(line, 1, n - 1)
  rest = substr(line, n + length(phrase))
  sub(suffix, "", rest)
  cur_cap = rest
  return 1
}

FNR == NR {
  if (index($0, "Package ") == 1 && index($0, " no longer has capability ") > 0) {
    if (parse($0, " no longer has capability ", " which was in the baseline\\.$")) {
      removed_list[cur_cap] = removed_list[cur_cap] " " cur_pkg
    }
  }
  next
}

# Second pass. A finding is a header line plus the indented call path that follows it.
{
  if (index($0, "Package ") == 1 && index($0, " has new capability ") > 0) {
    if (parse($0, " has new capability ", " compared to the baseline\\.$")) {
      n_add++
      add_pkg[n_add] = cur_pkg
      add_cap[n_add] = cur_cap
      current = "add"
      idx = n_add
      next
    }
  }
  if (index($0, "Package ") == 1 && index($0, " no longer has capability ") > 0) {
    if (parse($0, " no longer has capability ", " which was in the baseline\\.$")) {
      n_rem++
      rem_pkg[n_rem] = cur_pkg
      rem_cap[n_rem] = cur_cap
      current = "rem"
      idx = n_rem
      next
    }
  }
  # Blank lines separate findings; anything else belongs to the current call path.
  if ($0 ~ /^[[:space:]]*$/) { current = ""; next }
  if (current == "add") add_path[idx] = add_path[idx] $0 "\n"
  else if (current == "rem") rem_path[idx] = rem_path[idx] $0 "\n"
}

END {
  for (i = 1; i <= n_add; i++) {
    cap = add_cap[i]
    moved_from[i] = find_move(cap, add_pkg[i])
    # Only an exact pairing demotes a high-signal capability. Pairing on a shared final
    # path element alone is a guess, and `high` is the count CI blocks on, so letting an
    # unrelated removal produce a pairing would silently disarm the gate; those are
    # reported in `high` with the pairing named, for a reviewer to dismiss.
    exact[i] = (moved_from[i] != "" && canonical(moved_from[i]) == canonical(add_pkg[i]))
    if (is_high(cap) && !exact[i]) tier[i] = "high"
    else if (moved_from[i] != "") tier[i] = "moved"
    else tier[i] = "low"
    n_tier[tier[i]]++
  }

  if (n_tier["high"] > 0) {
    printf "### :rotating_light: Review closely: %d new high-signal capability use(s)\n\n", n_tier["high"]
    print "A dependency gaining one of these without a matching change in what it does is the"
    print "signature of a compromised release. Read each call path before merging.\n"
    for (i = 1; i <= n_add; i++)
      if (tier[i] == "high") {
        printf "- `%s` gained **%s**", add_pkg[i], add_cap[i]
        # Noted, not acted on: the same capability leaving a like-named package often means
        # a move, but only a reviewer can confirm the two packages are the same code.
        if (moved_from[i] != "")
          printf " (the same capability left `%s` in this diff, so this may be a package move -- confirm the pairing)", moved_from[i]
        printf "\n\n```\n%s```\n\n", add_path[i]
      }
  }

  if (n_tier["moved"] > 0) {
    printf "### Likely a package move: %d capability use(s)\n\n", n_tier["moved"]
    print "The same capability disappeared from another package in this diff, which is what a"
    print "vendored or renamed package looks like rather than new privilege. Heuristic, so"
    print "confirm the pairing makes sense. Call paths are in the raw Capslock report.\n"
    for (i = 1; i <= n_add; i++)
      if (tier[i] == "moved") {
        printf "- `%s` gained %s, which left `%s`", add_pkg[i], add_cap[i], moved_from[i]
        # Worth calling out which of these would otherwise have blocked the merge.
        if (is_high(add_cap[i])) printf " (high-signal, demoted: the paths differ only in a version element)"
        printf "\n"
      }
    print ""
  }

  if (n_tier["low"] > 0) {
    printf "### Lower signal: %d capability use(s)\n\n", n_tier["low"]
    print "These describe the Capslock analysis more than they describe privilege.\n"
    for (i = 1; i <= n_add; i++)
      if (tier[i] == "low")
        printf "- `%s` gained %s\n", add_pkg[i], add_cap[i]
    print ""
  }

  if (n_rem > 0) {
    printf "### No longer present: %d capability use(s)\n\n", n_rem
    print "Reported for completeness; removing code legitimately drops capabilities.\n"
    for (i = 1; i <= n_rem; i++)
      printf "- `%s` no longer uses %s\n", rem_pkg[i], rem_cap[i]
    print ""
  }

  if (n_add == 0 && n_rem == 0)
    print ":white_check_mark: No capability changes compared to the baseline."

  if (counts_file != "") {
    printf "high=%d\n",    n_tier["high"] + 0 > counts_file
    printf "moved=%d\n",   n_tier["moved"] + 0 > counts_file
    printf "low=%d\n",     n_tier["low"] + 0 > counts_file
    printf "added=%d\n",   n_add + 0 > counts_file
    printf "removed=%d\n", n_rem + 0 > counts_file
  }
}
' "${REPORT}" "${REPORT}"
