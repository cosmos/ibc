#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Turns the `capslock -output=compare` reports for one Go module into a short markdown
# report of the capabilities its changed dependencies newly bring in.
#
#   ./scripts/capslock-classify.sh --modules m.tsv --intermediate i.txt --function f.txt
#       [--dependency d.txt --imports imports.txt --linked symbols.txt] [--name cli]
#       [--base-packages pkgs.txt]
#       [--counts counts.env]
#
# --modules lists the module's build list as "path<TAB>status<TAB>base<TAB>head" lines, with
# status `changed` for added, bumped or newly replaced modules; the "Find changed
# dependencies" step of capslock-diff.yml writes it. The two reports compare the same code at
# -granularity=intermediate, where a finding is a package newly on a path to a capability,
# and -granularity=function, where it is a first-party function newly reaching one. The
# second catches a dependency that already held a capability adding a new use of it, such
# as an init() that reads a file and posts it. --dependency is a third, -granularity=function
# report run on the bumped dependencies' own packages that the pull request's code imports,
# old version against new, so that a new use inside a function of the dependency shows even
# when everything above it already had the capability, and also when only the pull
# request's new calls reach it. A function that only changed version path counts as a move.
# --linked lists the functions the binaries and test binaries link (`go tool nm`, built
# with -gcflags=all=-l so nothing is inlined away); a dependency function missing from it
# cannot run here, so it is counted, not gated. The linker also keeps what the standard
# library calls back and generic instantiations, which a call graph from here misses.
# --imports, "module package" lines from `go list -deps`, names changed dependencies that
# only tests import: Capslock does not load tests, so the report says they are unchecked.
#
# The gate is about dependencies, so a finding counts only when its call path goes through
# a changed module, and it is listed under the last such module on the path. Everything
# else -- this repository's own code newly calling into unchanged code -- is counted, not
# listed. The function report should come from the base code built against the new
# dependencies, so that the pull request's own new functions are not findings at all, and
# it alone covers this repository's packages: intermediate findings about them are not
# counted, since a pull request that bumps a dependency and newly calls into what it
# already had would otherwise gate.
#
# Not gated either: a capability that left a package for one whose path differs only in
# version elements (/v2, /v1.43.0, .v3) and that the base build did not load, which is a
# move; and a non-standard package reached through a package of the same dependency that
# already had the capability, such as a helper split out of it, since the same code in
# that caller would not show at this granularity either. --base-packages lists the
# packages the base build loaded; without it every version-sibling counts as a move.
#
# The report is one line per module, then one line per changed dependency that brings in a
# capability, with an example path to the most severe one; full paths stay in the reports.
#
# Not seen: a new use of a capability inside a dependency function that already had it, or
# inside a method of a generic type the dependency never instantiates itself; a finding
# whose one example path, the shortest, avoids the changed dependency while a longer one
# goes through it; dependencies only tests import; code built only for platforms other
# than the runner's; and calls made through reflection or unsafe (REFLECT, UNSAFE_POINTER),
# which are listed but do not gate.
#
# --counts writes high, low, moved, held, unlinked, unrelated and changed as key=value
# lines; `high` is what CI blocks on.

set -euo pipefail

# Capability classes that block until someone reads the call path. MODIFY_SYSTEM_STATE
# matches its subcategories too (/ENV, /SIGNALS, ...).
HIGH_SIGNAL="EXEC NETWORK FILES ARBITRARY_EXECUTION SYSTEM_CALLS MODIFY_SYSTEM_STATE"
NAME="module" MODULES="" INTERMEDIATE="" FUNCTION="" DEPENDENCY="" IMPORTS="" LINKED="" COUNTS="" BASE_PACKAGES=""

die() { echo "capslock-classify: $*" >&2; exit 2; }
usage() { sed -n '3,${/^#/!q; s|^# \{0,1\}||; p;}' "${BASH_SOURCE[0]}"; }

while (($# > 0)); do
  case "$1" in
    -h | --help) usage; exit 0 ;;
    --name | --modules | --intermediate | --function | --dependency | --imports | --linked | --counts | --high-signal | --base-packages)
      [[ $# -ge 2 && -n "$2" ]] || die "$1 requires a value"
      case "$1" in
        --modules) MODULES="$2" ;;
        --intermediate) INTERMEDIATE="$2" ;;
        --function) FUNCTION="$2" ;;
        --dependency) DEPENDENCY="$2" ;;
        --imports) IMPORTS="$2" ;;
        --linked) LINKED="$2" ;;
        --counts) COUNTS="$2" ;;
        --name) NAME="$2" ;;
        --high-signal) HIGH_SIGNAL="$2" ;;
        --base-packages) BASE_PACKAGES="$2" ;;
      esac
      shift 2 ;;
    *) die "unknown argument '$1' (see --help)" ;;
  esac
done

for f in "${MODULES}" "${INTERMEDIATE}" "${FUNCTION}"; do
  [[ -n "$f" ]] || die "--modules, --intermediate and --function are required (see --help)"
  [[ -f "$f" ]] || die "'$f' does not exist"
done
for f in "${DEPENDENCY}" "${IMPORTS}" "${LINKED}"; do [[ -z "$f" || -f "$f" ]] || die "'$f' does not exist"; done

awk -v high_signal="${HIGH_SIGNAL}" -v name="${NAME}" -v counts_file="${COUNTS}" \
    -v modules_file="${MODULES}" -v intermediate_file="${INTERMEDIATE}" -v base_packages_file="${BASE_PACKAGES}" \
    -v dependency_file="${DEPENDENCY}" -v imports_file="${IMPORTS}" -v linked_file="${LINKED}" '
# Packages the base build loaded, from the packageInfo of the intermediate baseline.
BEGIN {
  if (base_packages_file != "") while ((getline line < base_packages_file) > 0) in_base[line] = 1
  if (imports_file != "") while ((getline line < imports_file) > 0) { split(line, w, " "); imported[w[1]] = 1 }
  if (linked_file != "") while ((getline line < linked_file) > 0) { linked[from_linker(line)] = 1; n_linked++ }
}

# Files are told apart by name: an empty report has no first record to count.
FNR == 1 { file = (FILENAME == modules_file) ? 1 : (FILENAME == intermediate_file) ? 2 : (FILENAME == dependency_file) ? 4 : 3 }

# Module list: path, status, base version, head version.
file == 1 {
  split($0, f, "\t")
  status[f[1]] = f[2]
  # This repository: the main module and modules replaced by a relative path, whose code is
  # analysed in their own module run.
  if (f[2] == "unchanged" && (f[4] == "main" || f[4] ~ / => \.\.?\//)) is_main[f[1]] = 1
  if (f[2] == "changed") {
    changed_order[++n_changed] = f[1]
    from[f[1]] = f[3]; to[f[1]] = f[4]
  }
  next
}

function trim(s) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", s); return s }

# A call path line is "<file>:<line>:<col><padding><symbol>" or just "<symbol>".
function symbol(line) {
  line = trim(line)
  sub(/^[^[:space:]]+:[0-9]+:[0-9]+[[:space:]]+/, "", line)
  return line
}

# The module a symbol or package path belongs to: the longest module path it starts with,
# followed by "/" or ".". Symbols carry their package path, as in "(*example.com/m/p.T).M".
function module_of(s,    i, c, best) {
  sub(/^\(\*?/, "", s)
  best = ""
  for (i = 2; i <= length(s) + 1; i++) {
    c = substr(s, i, 1)
    if (c == "[") break
    if ((c == "/" || c == "." || c == "") && (substr(s, 1, i - 1) in status)) best = substr(s, 1, i - 1)
  }
  return best
}

# The package of a symbol: its path up to the last "." before the name, once receiver
# parentheses, type arguments and closure suffixes are removed.
function package_of(s) {
  sub(/^\(\*?/, "", s); sub(/[[$)].*/, "", s); sub(/\.[^.\/]*$/, "", s)
  return s
}

function changed_module(s,    m) {
  m = module_of(s)
  return (m != "" && status[m] == "changed") ? m : ""
}

# The position of a capability in the high-signal list, which is ordered by severity, or 0.
function is_high(cap,    i, n, parts) {
  n = split(high_signal, parts, " ")
  for (i = 1; i <= n; i++)
    if (cap == parts[i] || index(cap, parts[i] "/") == 1) return i
  return 0
}

# A package path with version elements dropped, so that a move to a new major version or
# versioned directory compares equal to where it came from.
function unversioned(p,    i, n, parts, out) {
  n = split(p, parts, "/")
  out = ""
  for (i = 1; i <= n; i++) {
    if (parts[i] ~ /^v[0-9]+(\.[0-9]+)*$/) continue
    sub(/\.v[0-9]+$/, "", parts[i])
    out = out "/" parts[i]
  }
  return out
}

# A function name with version elements dropped, as unversioned() does for packages.
function unversioned_fn(s) {
  while (match(s, /\/v[0-9]+(\.[0-9]+)*[\/.]/)) s = substr(s, 1, RSTART - 1) substr(s, RSTART + RLENGTH - 1)
  while (match(s, /\.v[0-9]+[\/.]/)) s = substr(s, 1, RSTART - 1) substr(s, RSTART + RLENGTH - 1)
  return s
}

# Function names as Capslock and the linker write them, reduced to one form: no type
# arguments, closures folded into their function, and "pkg.T.M" for methods. The linker
# escapes a "." in the last import path element, as in "gopkg.in/yaml%2ev3"; left as it is,
# every function of such a package would count as unlinked and never gate.
function untyped(s) { while (gsub(/\[[^][]*\]/, "", s)) ; return s }
function from_capslock(s) {
  s = untyped(s); sub(/\$.*/, "", s); sub(/#[0-9]+$/, "", s)
  if (sub(/^\(\*?/, "", s)) sub(/\)\./, ".", s)
  return s
}
function from_linker(s) {
  gsub(/%2e/, ".", s); gsub(/%22/, "\"", s); gsub(/%25/, "%", s)
  s = untyped(s); gsub(/\.(func|gowrap|deferwrap)[0-9]+(\.[0-9]+)*/, "", s)
  sub(/-fm$/, "", s); sub(/\.init\.[0-9]+$/, ".init", s); sub(/\(\*?/, "", s); sub(/\)/, "", s)
  return s
}

function short(s) { gsub(/github\.com\//, "", s); sub(/\[.*\]/, "", s); return s }

# A version as shown in the report: a replace is noted, not spelled out, and a pseudo-version
# is shortened to its commit.
function version(v,    r) {
  r = index(v, " => ") ? " (replaced)" : ""
  if (r != "") v = substr(v, 1, index(v, " => ") - 1)
  if (v ~ /-[0-9]{14}-[0-9a-f]{12}$/) v = substr(v, length(v) - 11, 7)
  return v r
}

function path_of(i,    k) {
  k = n_frames[i]
  if (k <= 1) return sprintf("`%s`", short(frames[i, 1]))
  return sprintf("`%s` → %s`%s`", short(frames[i, 1]),
    (k == 2 ? "" : sprintf("%d frame%s → ", k - 2, (k == 3 ? "" : "s"))), short(frames[i, k]))
}

function add_cap(list, cap) { return index(", " list ", ", ", " cap ", ") ? list : list (list == "" ? "" : ", ") cap }

# "Package <key> <phrase> <CAP> <suffix>". The key can hold spaces: function keys name
# generic instantiations such as "F[a.A, b.B]".
function parse(line, phrase,    i) {
  i = index(line, phrase)
  parsed_key = substr(line, 9, i - 9)
  parsed_cap = substr(line, i + length(phrase))
  sub(/ .*/, "", parsed_cap)
}

# Reports: file 2 is intermediate, file 3 function. Each finding is a header line plus the
# indented call path that follows it, up to a blank line.
/^Package .* has new capability / {
  parse($0, " has new capability ")
  if (file == 2) is_new[parsed_cap, parsed_key] = 1
  n++
  gran[n] = file; key[n] = parsed_key; cap[n] = parsed_cap; current = n
  next
}
/^Package .* no longer has capability / {
  parse($0, " no longer has capability ")
  if (file == 2) removed[parsed_cap] = removed[parsed_cap] " " parsed_key
  if (file == 4) removed_fn[parsed_cap, unversioned_fn(parsed_key)] = 1
  current = 0
  next
}
/^[[:space:]]*$/ { current = 0; next }
current { frames[current, ++n_frames[current]] = symbol($0) }

END {
  for (i = 1; i <= n; i++) {
    # Attributed to the last changed module on the path: the dependency closest to where
    # the capability is used. An intermediate finding is first of all about its package.
    # Packages of this repository are left to the function report, which compares the code
    # of main built against the new dependencies. At intermediate granularity they would gate
    # a pull request that bumps a dependency and newly calls into what it already had.
    if (gran[i] == 2 && (module_of(key[i]) in is_main)) { tier["unrelated"]++; continue }

    via = ""
    for (j = 1; j <= n_frames[i]; j++) if ((m = changed_module(frames[i, j])) != "") via = m
    if (gran[i] != 3 && (m = changed_module(key[i])) != "") via = m
    if (via == "") { tier["unrelated"]++; continue }

    if (gran[i] == 4 && (cap[i], unversioned_fn(key[i])) in removed_fn) { tier["moved"]++; continue }
    if (gran[i] == 4 && n_linked && !(from_capslock(key[i]) in linked)) { tier["unlinked"]++; continue }

    # Only a package the base build did not load can be where a capability moved to.
    if (gran[i] == 2 && cap[i] in removed && !(key[i] in in_base)) {
      k = split(removed[cap[i]], gone, " ")
      for (j = 1; j <= k; j++)
        if (gone[j] != key[i] && unversioned(gone[j]) == unversioned(key[i])) break
      if (j <= k) { tier["moved"]++; continue }
    }

    # Upstream of its own package only: below it, a package of the same dependency that
    # already had the capability is what this finding newly reaches, not where it came from.
    # Not for the standard library, whose first path element has no dot: those packages
    # would show however the dependency arranged its own code.
    if (gran[i] == 2 && key[i] ~ /^[^\/]*\./) {
      for (j = 1; j <= n_frames[i] && (p = package_of(frames[i, j])) != key[i]; j++)
        if (module_of(p) == via && !((cap[i], p) in is_new)) break
      if (j <= n_frames[i] && p != key[i]) { tier["held"]++; continue }
    }

    if (!(via in high_caps)) { dep_order[++n_deps] = via; high_caps[via] = low_caps[via] = "" }
    if (is_high(cap[i])) {
      tier["high"]++
      c = cap[i]; sub(/\/.*/, "", c)
      high_caps[via] = add_cap(high_caps[via], c)
      # The example is the shortest path to the most severe capability.
      if (!(via in example)) example[via] = i
      e = example[via]
      if (is_high(cap[i]) < is_high(cap[e]) || (is_high(cap[i]) == is_high(cap[e]) && n_frames[i] < n_frames[e]))
        example[via] = i
    } else {
      tier["low"]++
      if (!index(", " low_caps[via] ", ", ", " cap[i] ", ")) n_low[via]++
      low_caps[via] = add_cap(low_caps[via], cap[i])
    }
  }

  # Changed dependencies that only tests import, which none of the reports cover.
  if (imports_file != "")
    for (c = 1; c <= n_changed; c++)
      if (!(changed_order[c] in imported)) untested = untested (untested == "" ? "" : ", ") "`" short(changed_order[c]) "`"

  for (c = 1; c <= n_deps; c++) if (high_caps[dep_order[c]] != "") n_high_deps++
  if (n_changed == 0) printf ":white_check_mark: **`%s`**: no dependency changed.\n", name
  else if (n_high_deps > 0)
    printf ":rotating_light: **`%s`**: %d changed dependenc%s, %d with new high-signal capabilities.\n",
      name, n_changed, (n_changed == 1 ? "y" : "ies"), n_high_deps
  else
    printf "%s **`%s`**: %d changed dependenc%s, no new high-signal capabilities.\n",
      (untested == "" ? ":white_check_mark:" : ":warning:"), name, n_changed, (n_changed == 1 ? "y" : "ies")

  # Dependencies that gate first, then those with only lower-signal capabilities.
  for (pass = 1; pass <= 2; pass++)
    for (c = 1; c <= n_deps; c++) {
      m = dep_order[c]
      if ((pass == 1) != (high_caps[m] != "")) continue
      if (from[m] == "") printf "- `%s` added at %s: ", short(m), version(to[m])
      else printf "- `%s` %s → %s: ", short(m), version(from[m]), version(to[m])
      if (pass == 1) {
        printf "**%s**", high_caps[m]
        if (n_low[m]) printf " (+%d lower-signal)", n_low[m]
        printf ". E.g. %s\n", path_of(example[m])
      } else printf "%s\n", low_caps[m]
    }

  if (untested != "") printf "\n_Not checked, only tests import them: %s._\n", untested

  if (tier["moved"] + tier["held"] + tier["unlinked"] + tier["unrelated"] > 0) {
    sep = "\n_Not gated: "
    if (tier["moved"]) { printf "%s%d version move%s", sep, tier["moved"], (tier["moved"] == 1 ? "" : "s"); sep = ", " }
    if (tier["held"]) { printf "%s%d new package%s reached from one that already had the capability", sep, tier["held"], (tier["held"] == 1 ? "" : "s"); sep = ", " }
    if (tier["unlinked"]) { printf "%s%d in dependency functions no binary here links", sep, tier["unlinked"]; sep = ", " }
    if (tier["unrelated"]) printf "%s%d change%s outside the changed dependencies", sep, tier["unrelated"], (tier["unrelated"] == 1 ? "" : "s")
    print "._"
  }

  if (counts_file != "")
    printf "high=%d\nlow=%d\nmoved=%d\nheld=%d\nunlinked=%d\nunrelated=%d\nchanged=%d\n", tier["high"],
      tier["low"], tier["moved"], tier["held"], tier["unlinked"], tier["unrelated"], n_changed > counts_file
}
' "${MODULES}" "${INTERMEDIATE}" "${FUNCTION}" ${DEPENDENCY:+"${DEPENDENCY}"}
