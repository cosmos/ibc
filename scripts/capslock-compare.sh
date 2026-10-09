#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# The "Compare" step of capslock-diff.yml. For each module in $MODULES, compares each changed
# dependency's own functions, old version against new, for the packages the pull request
# imports, and writes a markdown report of the capabilities they newly use. A package the old
# version lacks, and every package of an added module, is new code, compared against nothing;
# a major-version move is compared with the path it moved from.
#
# A finding gates when its capability is high-signal. Not gated:
#   - a move: a function whose path changed only in version elements (/v2, /v1.43.0, .v3),
#     with the capability gone from the old path;
#   - a function no binary or test binary links, which cannot run here;
#   - other capabilities, such as REFLECT and UNSAFE_POINTER, which are listed only.
# Changed dependencies that only tests import are named as unchecked: Capslock does not load
# tests.
#
# Not seen: a new use of a capability inside a function that already had it; calls made
# through reflection or unsafe; and code built only for platforms other than the runner's.
#
# Run from the repository root after the "Find changed dependencies" step, which writes
# capslock-out/<module>-modules.tsv ("path<TAB>status<TAB>base<TAB>head") and checks the base
# commit out at $RUNNER_TEMP/base. Environment: MODULES, BASE and HEAD_SHA (for the comment
# header), RUN_URL, RUNNER_TEMP, GITHUB_STEP_SUMMARY and GITHUB_OUTPUT, which gets high=<count>.

set -euo pipefail

# Capability classes that gate until someone reads the call path, most severe first.
# MODIFY_SYSTEM_STATE matches its subcategories too (/ENV, /SIGNALS, ...).
HIGH_SIGNAL="EXEC NETWORK FILES ARBITRARY_EXECUTION SYSTEM_CALLS MODIFY_SYSTEM_STATE"

# Prints the markdown report for module $1 from its files in capslock-out, and writes
# high=<count> to capslock-out/$1.counts.
classify() {
  local out="capslock-out/$1"
  awk -v high_signal="$HIGH_SIGNAL" -v name="$1" -v counts_file="$out.counts" -v modules_file="$out-modules.tsv" \
    -v imports_file="$out-imports.txt" -v test_imports_file="$out-test-imports.txt" -v linked_file="$out-linked.txt" '
  BEGIN {
    while ((getline line < imports_file) > 0) { split(line, w, " "); imported[w[1]] = 1 }
    while ((getline line < test_imports_file) > 0) test_imported[line] = 1
    while ((getline line < linked_file) > 0) { linked[from_linker(line)] = 1; n_linked++ }
  }

  # Module list: path, status, base version, head version.
  FILENAME == modules_file {
    split($0, f, "\t")
    status[f[1]] = f[2]
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

  # The module a symbol belongs to: the longest module path it starts with, followed by "/"
  # or ".". Symbols carry their package path, as in "(*example.com/m/p.T).M".
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

  # The position of a capability in the high-signal list, which is ordered by severity, or 0.
  function is_high(cap,    i, n, parts) {
    n = split(high_signal, parts, " ")
    for (i = 1; i <= n; i++)
      if (cap == parts[i] || index(cap, parts[i] "/") == 1) return i
    return 0
  }

  # A function name with version elements dropped, so that a move to a new major version or
  # versioned directory compares equal to where it came from.
  function unversioned(s) {
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
  # is shortened to its commit. A dot precedes the timestamp when the pseudo-version follows a
  # tag, as in v1.2.4-0.20260101120000-abcdef123456.
  function version(v,    r) {
    r = index(v, " => ") ? " (replaced)" : ""
    if (r != "") v = substr(v, 1, index(v, " => ") - 1)
    if (v ~ /[-.][0-9]{14}-[0-9a-f]{12}$/) v = substr(v, length(v) - 11, 7)
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

  # Each finding is a header line plus the indented call path that follows it, up to a blank line.
  /^Package .* has new capability / {
    parse($0, " has new capability ")
    n++
    key[n] = parsed_key; cap[n] = parsed_cap; current = n
    next
  }
  /^Package .* no longer has capability / {
    parse($0, " no longer has capability ")
    removed[parsed_cap, unversioned(parsed_key)] = 1
    current = 0
    next
  }
  /^[[:space:]]*$/ { current = 0; next }
  current { frames[current, ++n_frames[current]] = symbol($0) }

  END {
    for (i = 1; i <= n; i++) {
      if ((cap[i], unversioned(key[i])) in removed) { tier["moved"]++; continue }
      if (n_linked && !(from_capslock(key[i]) in linked)) { tier["unlinked"]++; continue }

      m = module_of(key[i])
      if (!(m in high_caps)) { dep_order[++n_deps] = m; high_caps[m] = low_caps[m] = "" }
      if (is_high(cap[i])) {
        tier["high"]++
        c = cap[i]; sub(/\/.*/, "", c)
        high_caps[m] = add_cap(high_caps[m], c)
        # The example is the shortest path to the most severe capability.
        if (!(m in example)) example[m] = i
        e = example[m]
        if (is_high(cap[i]) < is_high(cap[e]) || (is_high(cap[i]) == is_high(cap[e]) && n_frames[i] < n_frames[e]))
          example[m] = i
      } else {
        tier["low"]++
        if (!index(", " low_caps[m] ", ", ", " cap[i] ", ")) n_low[m]++
        low_caps[m] = add_cap(low_caps[m], cap[i])
      }
    }

    # Changed dependencies that only tests import, which the report does not cover.
    for (c = 1; c <= n_changed; c++)
      if ((changed_order[c] in test_imported) && !(changed_order[c] in imported)) untested = untested (untested == "" ? "" : ", ") "`" short(changed_order[c]) "`"

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

    if (tier["moved"] + tier["unlinked"] > 0) {
      sep = "\n_Not gated: "
      if (tier["moved"]) { printf "%s%d version move%s", sep, tier["moved"], (tier["moved"] == 1 ? "" : "s"); sep = ", " }
      if (tier["unlinked"]) printf "%s%d in functions no binary here links", sep, tier["unlinked"]
      print "._"
    }

    printf "high=%d\n", tier["high"] > counts_file
  }
  ' "$out-modules.tsv" "$out-dependency.txt"
}

body=capslock-out/comment.md
printf '<!-- capslock-diff -->\n### Capslock dependency check\n\nmain `%.12s` → PR `%.12s` · [call paths](%s)\n' \
  "$BASE" "$HEAD_SHA" "$RUN_URL" > "$body"
high=0
for m in $MODULES; do
  out="capslock-out/$m" tmp="$RUNNER_TEMP/$m"
  mkdir -p "$tmp/bin"
  # "module package" for each package the build loads; module paths only, with tests.
  (cd "$m" && go list -deps -f '{{with .Module}}{{.Path}} {{$.ImportPath}}{{end}}' ./...) > "$out-imports.txt"
  (cd "$m" && go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...) > "$out-test-imports.txt"

  # Functions the binaries and test binaries link, built without inlining: what can
  # run, callbacks from the standard library and generic instantiations included. Test
  # binaries one package at a time: go test -c refuses two packages of one name.
  (cd "$m" && go build -gcflags=all=-l -o "$tmp/bin/" ./... \
    && go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./... \
    | xargs -P "$(getconf _NPROCESSORS_ONLN)" -I{} sh -c 'go test -gcflags=all=-l -c -o "$1/$(echo "$2" | tr / _).test" "$2"' _ \
      "$tmp/bin" {})
  # The name is everything after the address and type; generic ones contain spaces.
  find "$tmp/bin" -type f -exec go tool nm {} \; \
    | awk '$2 ~ /^[Tt]$/ { $1 = $2 = ""; sub(/^ +/, ""); print }' | sort -u > "$out-linked.txt"

  # "old new" module pairs, old being "-" for an added module, then the packages the
  # pull request imports from each, and the same packages at the old path.
  : > "$tmp/new.txt"; : > "$tmp/old.txt"
  while read -r old new; do
    awk -v m="$new" '$1 == m { print $2 }' "$out-imports.txt" | tee -a "$tmp/new.txt" \
      | awk -v o="$old" -v n="$new" 'o != "-" { print o substr($0, length(n) + 1) }' >> "$tmp/old.txt"
  done < <(awk -F '\t' '
    function u(p) { sub(/\/v[0-9]+$/, "", p); sub(/\.v[0-9]+$/, "", p); return p }
    $2 == "changed" && $3 != "" { print $1, $1 }
    $2 == "changed" && $3 == "" { added[u($1)] = $1 }
    $2 == "removed" { gone[u($1)] = $1 }
    END { for (k in added) print (k in gone ? gone[k] : "-"), added[k] }' "$out-modules.tsv")

  : > "$out-dependency.txt"
  if [ -s "$tmp/new.txt" ]; then
    # Of the old packages, those the base commit has.
    echo '{}' > "$tmp/old.json"
    old=""
    if [ -s "$tmp/old.txt" ]; then
      old="$(cd "$RUNNER_TEMP/base/$m" && xargs go list -e -f '{{if not .Error}}{{.ImportPath}}{{end}}' < "$tmp/old.txt" | paste -sd, -)"
    fi
    if [ -n "$old" ]; then
      (cd "$RUNNER_TEMP/base/$m" && capslock -force_local_module -packages="$old" -granularity=function -output=json) > "$tmp/old.json"
    fi
    # Exit status 1 means differences were found, 2 or above that capslock failed.
    status=0
    (cd "$m" && capslock -force_local_module -packages="$(paste -sd, - < "$tmp/new.txt")" -granularity=function \
      -output=compare "$tmp/old.json") > "$out-dependency.txt" 2>&1 || status=$?
    [ "$status" -le 1 ] || { cat "$out-dependency.txt"; echo "::error::capslock failed on $m"; exit 1; }
  fi
  echo "::group::$m call paths"; cat "$out-dependency.txt"; echo "::endgroup::"

  echo >> "$body"
  classify "$m" >> "$body"
  high=$((high + $(sed -n 's/^high=//p' "$out.counts")))
done
cat "$body" >> "$GITHUB_STEP_SUMMARY"
echo "high=$high" >> "$GITHUB_OUTPUT"
