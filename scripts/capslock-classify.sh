#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Turns `capslock -output=compare` output into a short, review-ordered markdown report, so
# that a dependency newly gaining EXEC, NETWORK or FILES is not buried among analysis noise.
#
#   ./scripts/capslock-classify.sh <report.txt>                       # markdown to stdout
#   ./scripts/capslock-classify.sh <report.txt> --counts counts.env   # also write counts
#   ./scripts/capslock-classify.sh <report.txt> --local example.com/  # first-party prefixes
#
# Additions are sorted into four tiers:
#
#   high   a capability class a compromised release typically gains. One line per finding,
#          grouped by capability, naming the ends of the call path.
#   moved  the same capability disappeared from a like-named package in this diff, which
#          is what a vendored or renamed package looks like, not new privilege. One line
#          per package pair, naming both sides, because the pairing is a guess.
#   low    capabilities that say more about Capslock's analysis than about privilege
#          (UNANALYZED, REFLECT, UNSAFE_POINTER, CGO, RUNTIME). One line per capability.
#   local  the package whose capability set changed is this repository's own, not a
#          dependency. Counted, not listed, and excluded from `high` so it cannot gate.
#
# The point of the gate is a dependency that gained a capability without a matching change
# in what it does -- the signature of a compromised release. First-party code gaining a
# capability is just the pull request doing its job, and is reviewed as part of the diff, so
# `--local` takes the import path prefixes that identify this repository's own packages
# (repeat the flag or separate them with spaces) and drops those findings.
#
# A finding is judged local by the package Capslock attributes it to, which is the package
# whose capability set changed. Judging by the call path origin instead -- the frame that
# makes the capability call -- was tried and does not work: that frame is inside a
# dependency almost every time, because the code that finally reaches os/exec or
# golang.org/x/sys/unix is a library even when first-party code is what newly reaches it.
# On a feature branch that added five first-party packages it filtered 1 finding of 36.
#
# Suppressing first-party findings is only sound while the dependency set is unchanged. A
# dependency update can make a first-party package newly reach a high-signal capability and
# be attributed only to that package, and dropping it would let the exact event this gate
# exists for pass unreviewed. Deciding that needs the pull request's file list, which this
# script does not have, so the decision belongs to the caller: pass --local only when no
# dependency manifest changed. Both callers in this repository do that -- see the paths
# filter in .github/workflows/capslock-diff.yml and the manifest check in capslock-diff.sh.
#
# The report is a triage aid, not an archive: the untouched Capslock report is kept next to
# it (job log plus artifact in CI, a file path locally), so every finding here is one line
# and the full call paths are read there. `moved` is the exception to one line per finding:
# it collapses to one line per package pair, since what a reviewer checks is whether the two
# packages are the same code, which is answered once however many capabilities moved with
# it. A count alone would not be checkable without opening the raw report.
#
# `moved` requires the added and removed packages to share a final path element, so an
# unrelated removal elsewhere in the diff cannot explain away a real finding. A high-signal
# capability stays in `high` even when a move pairing is found: the pairing is a heuristic
# and `high` is the count the CI gate reads, so the entry notes the package the capability
# left and a reviewer decides.
#
# With --counts, writes `key=value` lines (high, moved, low, local, added, removed) suitable
# for sourcing or for appending to $GITHUB_OUTPUT. All of them except `local` count only
# changes in dependencies, so `high` is what the CI gate can block on.

set -euo pipefail

# Capability classes that justify blocking a dependency update until someone reads the
# call path. MODIFY_SYSTEM_STATE matches its subcategories too (/ENV, /SIGNALS, ...).
HIGH_SIGNAL="EXEC NETWORK FILES ARBITRARY_EXECUTION SYSTEM_CALLS MODIFY_SYSTEM_STATE"

# How many packages to name per capability in the one-line tiers before summarising the
# rest as "+N more". Enough to recognise the change, short enough to stay one line.
PKG_LIMIT=6

# Import path prefixes belonging to this repository. Empty means every finding is treated as
# a dependency's, which is the safe default for a caller that does not know the module paths.
LOCAL_PREFIXES=""

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
    --packages) need_value "$@"; PKG_LIMIT="$2"; shift 2 ;;
    --local) need_value "$@"; LOCAL_PREFIXES="${LOCAL_PREFIXES:+${LOCAL_PREFIXES} }$2"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    -*) die "unknown argument '$1'" ;;
    *) REPORT="$1"; shift ;;
  esac
done

[[ -n "${REPORT}" ]] || die "usage: capslock-classify.sh <report.txt> [--counts <file>]"
[[ -f "${REPORT}" ]] || die "report '${REPORT}' does not exist"
[[ "${PKG_LIMIT}" =~ ^[0-9]+$ ]] || die "--packages must be a number"

# The report is read twice: once to learn which capabilities disappeared (so additions can
# be recognised as moves), then again to emit findings.
awk -v high_signal="${HIGH_SIGNAL}" -v pkg_limit="${PKG_LIMIT}" \
    -v local_prefixes="${LOCAL_PREFIXES}" -v counts_file="${COUNTS}" '
function basename(path,    n, parts) {
  n = split(path, parts, "/")
  return parts[n]
}

# A capability that left another package only explains this addition if the two packages
# plausibly are the same package under a new path, which vendoring and renames produce.
# Requiring a matching final path element keeps an unrelated removal elsewhere in the diff
# from explaining away a genuinely new capability.
function find_move(cap, pkg,    i, n, parts, base) {
  if (!(cap in removed_list)) return ""
  base = basename(pkg)
  n = split(removed_list[cap], parts, " ")
  for (i = 1; i <= n; i++)
    if (parts[i] != pkg && basename(parts[i]) == base) return parts[i]
  return ""
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

function trim(s) {
  gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
  return s
}

# A call path line is "<file>:<line>:<col><padding><symbol>", except the first, which is
# the entry point and carries no location. The location only helps while reading the path
# in full, which happens in the Capslock report, so the summary keeps just the symbol.
function raw_symbol(line) {
  line = trim(line)
  sub(/^[^[:space:]]+:[0-9]+:[0-9]+[[:space:]]+/, "", line)
  return trim(line)
}

function frame_symbol(line) {
  line = raw_symbol(line)
  # Display only: the package column already carries the fully qualified import path.
  gsub(/github\.com\//, "", line)
  return line
}


# Prefixes are expected to end in "/" so that "github.com/cosmos/ibc/" does not also match
# "github.com/cosmos/ibc-go/v11", which is a dependency.
function is_local(pkg,    i, n, parts) {
  if (local_prefixes == "") return 0
  n = split(local_prefixes, parts, " ")
  for (i = 1; i <= n; i++)
    if (index(pkg, parts[i]) == 1) return 1
  return 0
}

# The ends of a path are what a reviewer triages on: which of our own entry points reaches
# the dependency, and where the capability actually lands. The frames between them are in
# the Capslock report.
function path_summary(i,    n, from, to) {
  n = path_n[i]
  if (n == 0) return "no call path reported"
  from = frame_symbol(path[i, 1])
  if (n == 1) return sprintf("`%s`", from)
  to = frame_symbol(path[i, n])
  if (n == 2) return sprintf("`%s` -> `%s`", from, to)
  return sprintf("`%s` -> %d frame%s -> `%s`", from, n - 2, (n == 3 ? "" : "s"), to)
}

# Collects "`pkg`, `pkg`, +N more" for the tiers that get one line per capability.
function add_to_group(cap, pkg, order, seen, count, list,    n) {
  if (!(cap in seen)) {
    seen[cap] = 1
    order[++order[0]] = cap
  }
  n = ++count[cap]
  if (n <= pkg_limit)
    list[cap] = list[cap] (n > 1 ? ", " : "") "`" pkg "`"
}

function group_line(cap, count, list,    extra) {
  extra = count[cap] - pkg_limit
  return sprintf("- **%s** (%d): %s%s\n", cap, count[cap], list[cap],
                 (extra > 0 ? sprintf(", +%d more", extra) : ""))
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
      # Removals are summarised by capability, so their call paths are not collected.
      current = ""
      next
    }
  }
  # Blank lines separate findings; anything else belongs to the current call path.
  if ($0 ~ /^[[:space:]]*$/) { current = ""; next }
  if (current == "add") path[idx, ++path_n[idx]] = $0
}

END {
  for (i = 1; i <= n_add; i++) {
    cap = add_cap[i]
    # Judged before the capability class: our own package gaining EXEC is the pull request,
    # not a supply chain event, and must not reach `high` where it would fail the gate.
    if (is_local(add_pkg[i])) {
      tier[i] = "local"
      n_tier["local"]++
      continue
    }
    moved_from[i] = find_move(cap, add_pkg[i])
    # A high-signal capability is never demoted by the move heuristic: pairing on a shared
    # final path element is a guess, and `high` is the count CI blocks on, so letting an
    # unrelated removal produce a pairing would silently disarm the gate. The pairing is
    # reported alongside the finding instead, for a reviewer to dismiss.
    if (is_high(cap)) tier[i] = "high"
    else if (moved_from[i] != "") tier[i] = "moved"
    else tier[i] = "low"
    n_tier[tier[i]]++

    if (tier[i] == "high" && !(cap in high_seen)) {
      high_seen[cap] = 1
      high_order[++n_high_caps] = cap
    }
    if (tier[i] == "low") add_to_group(cap, add_pkg[i], low_order, low_seen, low_n, low_list)
    if (tier[i] == "moved") {
      pair = add_pkg[i] SUBSEP moved_from[i]
      if (!(pair in move_seen)) {
        move_seen[pair] = 1
        move_order[++n_move_pairs] = pair
        move_to[n_move_pairs] = add_pkg[i]
        move_from[n_move_pairs] = moved_from[i]
      }
      move_caps[pair] = move_caps[pair] (move_n[pair]++ ? ", " : "") cap
    }
  }
  for (i = 1; i <= n_rem; i++) {
    # Removals carry no security question either way, so the cheaper test on the attributed
    # package is enough here; no call path is collected for them.
    if (is_local(rem_pkg[i])) {
      n_local_rem++
      continue
    }
    n_rem_dep++
    add_to_group(rem_cap[i], rem_pkg[i], rem_order, rem_seen, rem_n, rem_list)
  }

  if (n_tier["high"] > 0) {
    printf "### :rotating_light: %d new high-signal capability use(s)\n\n", n_tier["high"]
    print "A dependency gaining one of these without a matching change in what it does is the"
    print "signature of a compromised release. Each line names the ends of the call path; read"
    print "the path in full in the Capslock report before merging.\n"
    for (c = 1; c <= n_high_caps; c++) {
      printf "**%s**\n", high_order[c]
      for (i = 1; i <= n_add; i++) {
        if (tier[i] != "high" || add_cap[i] != high_order[c]) continue
        printf "- `%s`: %s", add_pkg[i], path_summary(i)
        # Noted, not acted on: the same capability leaving a like-named package often means
        # a move, but only a reviewer can confirm the two packages are the same code.
        if (moved_from[i] != "")
          printf " (left `%s` in this diff -- may be a package move)", moved_from[i]
        printf "\n"
      }
      print ""
    }
  }

  if (n_tier["moved"] > 0) {
    printf "### Likely package moves: %d capability use(s) across %d package pair(s)\n\n",
           n_tier["moved"], n_move_pairs
    print "The same capability left another package in this diff, which is what a vendored or"
    print "renamed package looks like rather than new privilege. The two packages are matched"
    print "on their final path element alone, so each pairing is a guess: unrelated packages"
    print "that share a name produce one too. Both sides are named so it can be judged here.\n"
    # Same "->" idiom as the high-signal call paths, and it reads the same whether one
    # capability moved or several.
    for (c = 1; c <= n_move_pairs; c++)
      printf "- %s: `%s` -> `%s`\n", move_caps[move_order[c]], move_from[c], move_to[c]
    print ""
  }

  if (n_tier["low"] > 0) {
    printf "### Lower signal: %d capability use(s)\n\n", n_tier["low"]
    print "These describe the Capslock analysis more than they describe privilege.\n"
    for (c = 1; c <= low_order[0]; c++) printf "%s", group_line(low_order[c], low_n, low_list)
    print ""
  }

  if (n_rem_dep > 0) {
    printf "### No longer present: %d capability use(s)\n\n", n_rem_dep
    print "Reported for completeness; removing code legitimately drops capabilities.\n"
    for (c = 1; c <= rem_order[0]; c++) printf "%s", group_line(rem_order[c], rem_n, rem_list)
    print ""
  }

  if (n_tier["local"] + n_local_rem > 0) {
    n_local = n_tier["local"] + n_local_rem
    plural = (n_local == 1 ? "" : "s")
    printf "_Not listed: %d capability change%s in packages belonging to this repository\nrather than to a dependency, which the pull request diff already shows._\n\n", n_local, plural
  }

  n_add_dep = n_add - n_tier["local"]
  if (n_add_dep == 0 && n_rem_dep == 0)
    print ":white_check_mark: No capability changes in dependencies compared to the baseline."

  if (counts_file != "") {
    printf "high=%d\n",    n_tier["high"] + 0 > counts_file
    printf "moved=%d\n",   n_tier["moved"] + 0 > counts_file
    printf "low=%d\n",     n_tier["low"] + 0 > counts_file
    printf "local=%d\n",   n_tier["local"] + n_local_rem + 0 > counts_file
    printf "added=%d\n",   n_add_dep + 0 > counts_file
    printf "removed=%d\n", n_rem_dep + 0 > counts_file
  }
}
' "${REPORT}" "${REPORT}"
