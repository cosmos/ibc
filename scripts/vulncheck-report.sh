#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Turns `make vulncheck` output into a markdown report of the vulnerable dependencies,
# splitting advisories that need a fix from ones this repository has accepted.
#
#   make vulncheck > out.txt 2>&1 || true
#   ./scripts/vulncheck-report.sh out.txt                      # markdown to stdout
#   ./scripts/vulncheck-report.sh out.txt --counts counts.env   # also write counts
#
# Accepted advisories are read from .govulncheck-ignore (override with --ignore-file).
# Only advisories govulncheck reports as reachable from our code appear here, because that
# is all its default output details.
#
# With --counts, writes `key=value` lines (unexpected, accepted, total) suitable for
# sourcing or for appending to $GITHUB_OUTPUT.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

REPORT=""
COUNTS=""
IGNORE_FILE="${REPO_ROOT}/.govulncheck-ignore"

die() {
  echo "vulncheck-report: $*" >&2
  exit 2
}

while (($# > 0)); do
  case "$1" in
    --counts) COUNTS="${2:-}"; shift 2 ;;
    --ignore-file) IGNORE_FILE="${2:-}"; shift 2 ;;
    -h | --help) sed -n '3,17p' "${BASH_SOURCE[0]}" | sed 's|^# \{0,1\}||'; exit 0 ;;
    -*) die "unknown argument '$1'" ;;
    *) REPORT="$1"; shift ;;
  esac
done

[[ -n "${REPORT}" ]] || die "usage: vulncheck-report.sh <govulncheck-output> [--counts <file>]"
[[ -f "${REPORT}" ]] || die "report '${REPORT}' does not exist"

# Comments and blank lines are stripped so the ignore file can explain each entry.
ignored=""
if [[ -f "${IGNORE_FILE}" ]]; then
  ignored="$(sed 's/#.*//' "${IGNORE_FILE}" | tr -s '[:space:]' ' ')"
fi

awk -v ignored="${ignored}" -v counts_file="${COUNTS}" '
function is_ignored(id,    i, n, parts) {
  n = split(ignored, parts, " ")
  for (i = 1; i <= n; i++)
    if (id == parts[i]) return 1
  return 0
}

# "  Module: x" and "    Found in: x@v" carry one value after the colon.
function value_after(line, label,    n) {
  n = index(line, label)
  if (n == 0) return ""
  return substr(line, n + length(label))
}

# Which repository module is being scanned; emitted by the vulncheck make target.
/^==> govulncheck / { scanned = value_after($0, "==> govulncheck "); next }

/^Vulnerability #/ {
  id = $NF
  if (!(id in seen)) {
    seen[id] = 1
    order[++n_ids] = id
  }
  cur = id
  # One advisory can surface in several of our modules; collect them all.
  if (index(", " affected[id] ", ", ", " scanned ", ") == 0)
    affected[id] = (affected[id] == "" ? scanned : affected[id] ", " scanned)
  next
}

cur == "" { next }

/^  Module: / { module[cur] = value_after($0, "  Module: "); next }
/^  Standard library/ { module[cur] = "Go standard library"; next }

/^    Found in: / {
  v = value_after($0, "    Found in: ")
  # Trim the package path, keep the version: "net/url@go1.26.4" -> "go1.26.4".
  sub(/^.*@/, "", v)
  if (found[cur] == "") found[cur] = v
  next
}

/^    Fixed in: / {
  v = value_after($0, "    Fixed in: ")
  sub(/^.*@/, "", v)
  if (fixed[cur] == "") fixed[cur] = v
  next
}

END {
  for (i = 1; i <= n_ids; i++) {
    id = order[i]
    if (is_ignored(id)) { accepted[++n_acc] = id } else { unexpected[++n_unexp] = id }
  }

  print "<!-- dependency-review:govulncheck -->"
  print "## Vulnerable dependencies\n"

  if (n_ids == 0) {
    print ":white_check_mark: govulncheck reports no advisories reachable from our code."
  } else {
    printf "%d %s need attention, %d accepted.\n\n", n_unexp + 0,
      (n_unexp == 1 ? "advisory" : "advisories"), n_acc + 0
  }

  if (n_unexp > 0) {
    print "### :rotating_light: Needs attention\n"
    print "| Advisory | Dependency | In use | Fixed in | Affects |"
    print "|---|---|---|---|---|"
    for (i = 1; i <= n_unexp; i++) {
      id = unexpected[i]
      printf "| [%s](https://pkg.go.dev/vuln/%s) | `%s` | `%s` | %s | %s |\n",
        id, id, module[id],
        (found[id] == "" ? "?" : found[id]),
        (fixed[id] == "" || fixed[id] == "N/A" ? "**no fix yet**" : "`" fixed[id] "`"),
        affected[id]
    }
    print ""
  }

  if (n_acc > 0) {
    print "### Accepted\n"
    print "Listed in `.govulncheck-ignore` with a reason.\n"
    print "| Advisory | Dependency | In use | Fixed in | Affects |"
    print "|---|---|---|---|---|"
    for (i = 1; i <= n_acc; i++) {
      id = accepted[i]
      printf "| [%s](https://pkg.go.dev/vuln/%s) | `%s` | `%s` | %s | %s |\n",
        id, id, module[id],
        (found[id] == "" ? "?" : found[id]),
        (fixed[id] == "" || fixed[id] == "N/A" ? "**no fix yet**" : "`" fixed[id] "`"),
        affected[id]
    }
    print ""
  }

  if (n_unexp > 0) {
    print "Run `make vulncheck` for the call paths. Either upgrade the dependency, or add"
    print "the advisory to `.govulncheck-ignore` with a reason if no fix exists yet."
  }

  if (counts_file != "") {
    printf "unexpected=%d\n", n_unexp + 0 > counts_file
    printf "accepted=%d\n",   n_acc + 0 > counts_file
    printf "total=%d\n",      n_ids + 0 > counts_file
  }
}
' "${REPORT}"
