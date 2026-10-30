#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0

"""Tests for docs/scripts/specgen.py.

    python3 docs/scripts/test-specgen.py
"""
import os
import sys
import tempfile
import traceback

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import specgen  # noqa: E402

PASS, FAIL = [], []


def case(name):
    def deco(fn):
        try:
            fn()
            PASS.append(name)
        except Exception as e:
            FAIL.append((name, e, traceback.format_exc()))
        return fn
    return deco


REL = "core/ics-004-channel-and-packet-semantics/README.md"


@case("strips the .md suffix from internal links")
def _():
    out = specgen.transform("See [upgrades](./UPGRADES.md) for details.", REL)
    want = "](/ibc/spec/core/ics-004-channel-and-packet-semantics/UPGRADES)"
    assert want in out, out


@case("resolves a link written at a directory to its published README")
def _():
    # An existing cross-link: ics-024-host-requirements points at the ICS-2
    # directory, which is published as .../ics-002-client-semantics/README.
    src = "fulfilling the requirements of [ICS 2](../ics-002-client-semantics)."
    out = specgen.transform(src, REL)
    want = "](/ibc/spec/core/ics-002-client-semantics/README)"
    assert want in out, out


@case("keeps an anchor when resolving a directory link")
def _():
    src = "[ICS 2](../ics-002-client-semantics#validity-predicate)"
    out = specgen.transform(src, REL)
    assert "](/ibc/spec/core/ics-002-client-semantics/README#validity-predicate)" in out, out


@case("leaves a directory link alone when the directory has no README")
def _():
    src = "see [nothing](../ics-999-does-not-exist)."
    assert specgen.transform(src, REL) == src


@case("does not treat an image path as a directory link")
def _():
    src = "![Dataflow](dataflow.png)"
    out = specgen.transform(src, REL)
    assert "/README" not in out, out


@case("leaves external .md links alone")
def _():
    src = "See [the readme](https://github.com/cosmos/ibc/blob/main/README.md)."
    assert specgen.transform(src, REL) == src


@case("absolutizes a relative image path")
def _():
    out = specgen.transform("![Dataflow](dataflow.png)", REL)
    want = "/ibc/spec/core/ics-004-channel-and-packet-semantics/dataflow.png"
    assert want in out, out


@case("leaves an already-absolute image path alone")
def _():
    src = "![X](/ibc/spec/core/x/y.png)"
    assert specgen.transform(src, REL) == src


@case("does not rewrite inside fenced code blocks")
def _():
    src = "```go\n// see ./UPGRADES.md\n```\n"
    assert "./UPGRADES.md" in specgen.transform(src, REL)


@case("preserves ICS frontmatter untouched")
def _():
    src = "---\nics: 4\ntitle: Channel & Packet Semantics\n---\n\nBody.\n"
    out = specgen.transform(src, REL)
    assert out.startswith("---\nics: 4\ntitle: Channel & Packet Semantics\n---"), out


@case("passes a file that has frontmatter through unchanged")
def _():
    src = "---\nics: 4\ntitle: Channel\nstage: draft\n---\n\n# Other H1\n"
    assert specgen.ensure_title(src, REL) == src


@case("synthesises only title from the H1 when frontmatter is missing")
def _():
    out = specgen.ensure_title("# Upgrading Channels\n\nBody.\n", REL)
    want = '---\ntitle: "Upgrading Channels"\n---\n\n# Upgrading Channels\n\nBody.\n'
    assert out == want, out


@case("a title containing a colon round-trips through a YAML parser")
def _():
    title = "CCV: Technical Specification - Methods"
    out = specgen.ensure_title("# " + title + "\n", REL)
    fm = out.split("---\n")[1]
    try:
        import yaml
    except ImportError:
        yaml = None
    if yaml:
        assert yaml.safe_load(fm)["title"] == title, fm
    else:
        # No PyYAML in the stdlib. A JSON string is double-quoted YAML.
        import json
        assert json.loads(fm.split("title: ", 1)[1]) == title, fm
        assert fm.startswith('title: "'), fm


@case("ignores a # line inside a fenced block when finding the H1")
def _():
    src = "```sh\n# not a heading\n```\n\n# Real Title\n"
    assert specgen.ensure_title(src, REL).startswith('---\ntitle: "Real Title"')


@case("fails loudly, naming the file, with neither frontmatter nor H1")
def _():
    try:
        specgen.ensure_title("Just prose.\n\n## Not an H1\n", REL)
    except ValueError as e:
        assert REL in str(e), e
    else:
        raise AssertionError("expected ValueError")


@case("every published page has a title in its frontmatter")
def _():
    with tempfile.TemporaryDirectory() as out:
        for rel in specgen.generate(specgen.ROOT, out):
            if not rel.endswith(".mdx"):
                continue
            text = open(os.path.join(out, rel), encoding="utf-8").read()
            head = text.split("\n---", 1)[0]
            assert head.startswith("---") and "\ntitle:" in head, rel


@case("turns an HTML comment into an MDX comment")
def _():
    out = specgen.transform("<!-- omit in toc -->\n# T\n", REL)
    assert out == "{/* omit in toc */}\n# T\n", out


@case("turns an autolink into a Markdown link")
def _():
    out = specgen.transform("see <https://example.com/a>.", REL)
    assert out == "see [https://example.com/a](https://example.com/a).", out


@case("escapes a prose angle bracket but not a real tag")
def _():
    out = specgen.transform("pairs <client,client> and <br> and <b>x</b>", REL)
    assert out == "pairs &lt;client,client> and <br> and <b>x</b>", out


@case("does not rewrite MDX hazards inside inline code or fences")
def _():
    src = "`<!-- x -->` and `<https://a.b>`\n```\n<!-- y -->\n```\n"
    assert specgen.transform(src, REL) == src


@case("resolves a ../ page link against the containing directory")
def _():
    out = specgen.transform("[x](../ics-002-client-semantics/README.md)", REL)
    assert out == "[x](/ibc/spec/core/ics-002-client-semantics/README)", out


@case("keeps the #fragment when rewriting a link")
def _():
    out = specgen.transform("[x](data_structures.md#ccv-state)", "app/a/b.md")
    assert out == "[x](/ibc/spec/app/a/data_structures#ccv-state)", out


@case("same filename in different directories resolves per file")
def _():
    a = specgen.transform("[u](UPGRADES.md)", "core/one/README.md")
    b = specgen.transform("[u](UPGRADES.md)", "core/two/README.md")
    assert a != b, (a, b)


@case("a link to an excluded governance file becomes a GitHub URL")
def _():
    out = specgen.transform("[p](../PROCESS.md)", "ics-001-ics-standard/README.md")
    assert out == "[p](https://github.com/cosmos/ibc/blob/main/spec/PROCESS.md)", out


@case("a link escaping spec/ becomes a GitHub URL")
def _():
    out = specgen.transform("[r](../../README.md)", "ics-001-ics-standard/README.md")
    assert out == "[r](https://github.com/cosmos/ibc/blob/main/README.md)", out


@case("leaves an already-absolute .md link alone")
def _():
    src = "[x](/ibc/spec/core/x/README)  [y](/docs/a.md)"
    assert specgen.transform(src, REL) == src


@case("check reports an orphan page left behind in the output tree")
def _():
    with tempfile.TemporaryDirectory() as out:
        specgen.generate(specgen.ROOT, out)
        assert specgen.check(specgen.ROOT, out) == []
        os.makedirs(os.path.join(out, "core"), exist_ok=True)
        open(os.path.join(out, "core", "ghost.mdx"), "w").write("x")
        open(os.path.join(out, "core", "ghost.png"), "w").write("x")
        got = specgen.check(specgen.ROOT, out)
        assert got == ["orphan: docs/ibc/spec/core/ghost.mdx",
                       "orphan: docs/ibc/spec/core/ghost.png"], got


@case("check reports a missing and a stale page")
def _():
    with tempfile.TemporaryDirectory() as out:
        first = sorted(specgen.generate(specgen.ROOT, out))
        os.remove(os.path.join(out, first[0]))
        open(os.path.join(out, first[1]), "ab").write(b"edit")
        got = specgen.check(specgen.ROOT, out)
        assert f"missing: docs/ibc/spec/{first[0]}" in got, got
        assert f"stale: docs/ibc/spec/{first[1]}" in got, got


@case("write_tree refuses a target outside docs/ibc and deletes nothing")
def _():
    with tempfile.TemporaryDirectory() as d:
        victim = os.path.join(d, "keep")
        os.makedirs(victim)
        open(os.path.join(victim, "f"), "w").write("x")
        try:
            specgen.write_tree(specgen.ROOT, victim)
        except SystemExit:
            pass
        else:
            raise AssertionError("expected SystemExit")
        assert os.path.exists(os.path.join(victim, "f"))


@case("a failed regeneration leaves the existing tree intact")
def _():
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, "spec"))
        open(os.path.join(root, "spec", "bad.md"), "w").write("no heading\n")
        out = os.path.join(root, "docs", "ibc", "spec")
        os.makedirs(out)
        open(os.path.join(out, "old.mdx"), "w").write("old")
        try:
            specgen.write_tree(root, out)
        except ValueError:
            pass
        else:
            raise AssertionError("expected ValueError")
        assert open(os.path.join(out, "old.mdx")).read() == "old"
        assert os.listdir(os.path.dirname(out)) == ["spec"]


@case("write_tree refuses when there is no spec/ directory")
def _():
    with tempfile.TemporaryDirectory() as root:
        out = os.path.join(root, "docs", "ibc", "spec")
        os.makedirs(out)
        open(os.path.join(out, "keep.mdx"), "w").write("k")
        try:
            specgen.write_tree(root, out)
        except SystemExit:
            pass
        else:
            raise AssertionError("expected SystemExit")
        assert open(os.path.join(out, "keep.mdx")).read() == "k"


@case("a failed swap restores the existing tree")
def _():
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, "spec"))
        open(os.path.join(root, "spec", "a.md"), "w").write("# A\n")
        out = os.path.join(root, "docs", "ibc", "spec")
        os.makedirs(out)
        open(os.path.join(out, "old.mdx"), "w").write("old")
        real, calls = os.rename, []

        def flaky(src, dst):
            calls.append(src)
            if len(calls) == 2:  # the new -> target rename
                raise OSError("simulated failure")
            return real(src, dst)

        os.rename = flaky
        try:
            specgen.write_tree(root, out)
        except OSError:
            pass
        else:
            raise AssertionError("expected OSError")
        finally:
            os.rename = real
        assert open(os.path.join(out, "old.mdx")).read() == "old"
        assert os.listdir(os.path.dirname(out)) == ["spec"]


@case("write_tree recovers a tree stranded in .old by an interrupted run")
def _():
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, "spec"))
        open(os.path.join(root, "spec", "a.md"), "w").write("# A\n")
        base = os.path.join(root, "docs", "ibc")
        os.makedirs(os.path.join(base, "spec.old"))
        specgen.write_tree(root, os.path.join(base, "spec"))
        assert sorted(os.listdir(base)) == ["spec"], os.listdir(base)
        assert os.path.exists(os.path.join(base, "spec", "a.mdx"))


@case("fails loudly when frontmatter is opened but never closed")
def _():
    try:
        specgen.ensure_title("---\ntitle: x\n\n# H\n", REL)
    except ValueError as e:
        assert REL in str(e) and "never closed" in str(e), e
    else:
        raise AssertionError("expected ValueError")


@case("fails loudly with no frontmatter and no H1 when run through generate")
def _():
    with tempfile.TemporaryDirectory() as root, tempfile.TemporaryDirectory() as out:
        os.makedirs(os.path.join(root, "spec", "core"))
        open(os.path.join(root, "spec", "core", "x.md"), "w").write("prose\n")
        try:
            specgen.generate(root, out)
        except ValueError as e:
            assert "core/x.md" in str(e), e
        else:
            raise AssertionError("expected ValueError")


@case("excludes the governance files from publication")
def _():
    for name in ("CONTRIBUTING.md", "PROCESS.md",
                 "STANDARDS_COMMITTEE.md", "ics-template.md"):
        assert specgen.excluded(name), name
    assert not specgen.excluded("core/ics-004-channel-and-packet-semantics/README.md")


@case("generating twice changes nothing the second time")
def _():
    # Idempotence is the property that makes --check meaningful in CI.
    with tempfile.TemporaryDirectory() as out:
        root = specgen.ROOT
        first = specgen.generate(root, out)
        before = {p: open(os.path.join(out, p), "rb").read() for p in first}
        second = specgen.generate(root, out)
        assert sorted(first) == sorted(second), (len(first), len(second))
        after = {p: open(os.path.join(out, p), "rb").read() for p in second}
        assert before == after, "second run changed bytes"


@case("never writes outside the output directory")
def _():
    with tempfile.TemporaryDirectory() as out:
        written = specgen.generate(specgen.ROOT, out)
        for rel in written:
            full = os.path.realpath(os.path.join(out, rel))
            assert full.startswith(os.path.realpath(out) + os.sep), full


@case("rejects a source whose code fences do not pair up")
def _():
    src = "intro\n\n```typescript\nx\n```\n\n```typescript\ny\n"
    try:
        specgen.check_fences(src, REL)
    except ValueError as e:
        assert "odd number" in str(e), e
        assert REL in str(e), e
    else:
        assert False, "an unclosed fence was accepted"


@case("accepts a source whose code fences pair up")
def _():
    specgen.check_fences("a\n\n```go\nx\n```\n\nb\n", REL)


@case("every generated page carries the generated-file header")
def _():
    with tempfile.TemporaryDirectory() as out:
        written = specgen.generate(specgen.ROOT, out)
        pages = [p for p in written if p.endswith(".mdx")]
        assert pages
        for rel in pages:
            body = open(os.path.join(out, rel), encoding="utf-8").read()
            assert "by docs/scripts/specgen.py" in body, rel
            assert "python3 docs/scripts/specgen.py" in body, rel


@case("the header follows the frontmatter rather than preceding it")
def _():
    out = specgen.add_header("---\ntitle: X\n---\n\nbody\n", REL)
    assert out.startswith("---\ntitle: X\n---\n{/*"), out
    assert out.rstrip().endswith("body"), out


@case("publishes the ICS-027 GMP spec the stale docs copy lacked")
def _():
    with tempfile.TemporaryDirectory() as out:
        written = specgen.generate(specgen.ROOT, out)
        assert "IBC_V2/app/ics-027-gmp/README.mdx" in written, written[:5]


@case("refuses to publish a Git LFS pointer as an image")
def _():
    with tempfile.TemporaryDirectory() as d:
        src = os.path.join(d, "spec", "core", "ics-001-x")
        os.makedirs(src)
        open(os.path.join(src, "README.md"), "w").write("# X\n\nbody\n")
        # What a clone without the LFS client leaves on disk.
        open(os.path.join(src, "diagram.png"), "w").write(
            "version https://git-lfs.github.com/spec/v1\n"
            "oid sha256:" + "0" * 64 + "\nsize 109961\n")
        with tempfile.TemporaryDirectory() as out:
            try:
                specgen.generate(d, out)
            except ValueError as e:
                assert "Git LFS pointer" in str(e), e
                assert "git lfs pull" in str(e), e
            else:
                raise AssertionError("published the pointer instead of refusing")


@case("copies a real image")
def _():
    with tempfile.TemporaryDirectory() as d:
        src = os.path.join(d, "spec", "core", "ics-001-x")
        os.makedirs(src)
        open(os.path.join(src, "README.md"), "w").write("# X\n\nbody\n")
        png = b"\x89PNG\r\n\x1a\n" + b"\x00" * 64
        open(os.path.join(src, "diagram.png"), "wb").write(png)
        with tempfile.TemporaryDirectory() as out:
            specgen.generate(d, out)
            got = open(os.path.join(out, "core/ics-001-x/diagram.png"), "rb").read()
            assert got == png, "image did not copy through unchanged"


for name in PASS:
    print(f"  ok    {name}")
for name, e, tb in FAIL:
    print(f"  FAIL  {name}: {e}")
print(f"\n{len(PASS)} passed, {len(FAIL)} failed")
if FAIL:
    print()
    print(FAIL[0][2])
sys.exit(1 if FAIL else 0)
