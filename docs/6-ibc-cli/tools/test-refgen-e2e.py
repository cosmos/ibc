#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0

"""End-to-end tests for the self-healing loop: break the source, watch it fail.

tools/test-refgen.py tests the generator against the real pinned clone, which
cannot answer the question these pages actually depend on: when the code
changes, does the page go red, and does regenerating fix it?

So each case here copies the source into a temp directory, mutates it the way
a real commit would, and asserts one of three outcomes:

  * check mode goes red, regenerating heals it, and a second check is green
  * the generator raises, naming the decision a human has to make
  * the page is missing a marker for a region that now exists

Every "the generator fails loudly" claim in the design is one case below. A
claim with no case here is an assertion, not a property.

    python3 tools/test-refgen-e2e.py

Slower than the unit tests: four cases mutate the CLI wiring, and each of
those rebuilds the binary, because Cobra computes a flag's default when the
flag is registered.
"""
import difflib
import os
import re
import shutil
import subprocess
import sys
import tempfile
import traceback

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import refgen  # noqa: E402

ROOT = refgen.ROOT
TICK = chr(96)

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


class Sandbox:
    """A writable copy of the pinned source, with refgen pointed at it.

    Only cli/, proto/, and gen/ are copied, and cli/bin is skipped, so a
    copy is a couple of megabytes rather than the hundred the built binary
    costs.
    """

    def __enter__(self):
        self.dir = tempfile.mkdtemp(prefix="refgen-e2e-")
        # gen/ comes along because cli/go.mod replaces the generated ABI
        # module with a relative path into it
        for sub in ("cli", "proto", "gen"):
            shutil.copytree(os.path.join(refgen.IBC, sub), os.path.join(self.dir, sub),
                            ignore=shutil.ignore_patterns("bin", ".git"))
        self.saved_ibc = refgen.IBC
        self.saved_pages = dict(refgen.PAGES)
        refgen.IBC = self.dir
        return self

    def __exit__(self, *exc):
        refgen.IBC = self.saved_ibc
        refgen.PAGES.clear()
        refgen.PAGES.update(self.saved_pages)
        shutil.rmtree(self.dir, ignore_errors=True)
        return False

    def edit(self, path, old, new, count=1, optional=False):
        full = os.path.join(self.dir, path)
        text = open(full).read()
        if old not in text:
            if optional:
                return
            raise AssertionError(f"{path} no longer contains {old!r}; the mutation is stale")
        open(full, "w").write(text.replace(old, new, count))

    def rename(self, subdir, old, new):
        """Rename an identifier across a directory, and prove it happened.

        `edit(optional=True)` per file is silent when an anchor goes stale, so
        a rename test would keep passing while renaming nothing. Counting the
        files touched is what stops that.
        """
        full = os.path.join(self.dir, subdir)
        touched = 0
        for f in sorted(os.listdir(full)):
            if not f.endswith(".go"):
                continue
            path = os.path.join(full, f)
            text = open(path).read()
            if old in text:
                open(path, "w").write(text.replace(old, new))
                touched += 1
        if not touched:
            raise AssertionError(
                f"{old!r} appears in no file under {subdir}; the mutation is stale "
                "and this test would have passed without renaming anything")
        return touched

    def respace(self, path, decl):
        """Collapse the column padding inside one declaration.

        Whitespace only, and no line moves, so a citation's line number is
        still true and the only change is one a reader could never see.
        """
        full = os.path.join(self.dir, path)
        text = open(full).read()
        if decl not in text:
            raise AssertionError(f"{path} no longer contains {decl!r}; the mutation is stale")
        start = text.index(decl)
        end = text.index("\n}\n", start)
        block = re.sub(r"[ \t]{2,}", " ", text[start:end])
        open(full, "w").write(text[:start] + block + text[end:])

    def add_field(self, path, after_yaml_key, line):
        """Insert a struct field after the one carrying `after_yaml_key`.

        Anchoring on the literal source line means gofmt realigning the struct
        -- which it does whenever a longer field name lands beside it -- takes
        the mutation stale. The yaml key is the part a reader would recognise,
        so it is the part to anchor on.
        """
        full = os.path.join(self.dir, path)
        text = open(full).read()
        rx = re.compile(r"^.*`yaml:\"" + re.escape(after_yaml_key) + r"\"`.*$", re.M)
        m = rx.search(text)
        if not m:
            raise AssertionError(
                f"{path} declares no field with yaml key {after_yaml_key!r}; "
                "the mutation is stale")
        open(full, "w").write(text[:m.end()] + "\n" + line + text[m.end():])

    def append(self, path, text):
        with open(os.path.join(self.dir, path), "a") as fh:
            fh.write(text)

    def page(self, kind, own=False):
        """A copy of the real page, in the sandbox. `own` also makes it the
        page that kind owns, which turns on the missing-marker check."""
        src = os.path.join(ROOT, self.saved_pages[kind])
        dst = os.path.join(self.dir, os.path.basename(src))
        shutil.copyfile(src, dst)
        if own:
            refgen.PAGES[kind] = dst
        return dst


def red_then_healed(box, kind, expect_in_diff):
    """check red, regenerate, check green, and the change is the one expected."""
    page = box.page(kind)
    before = open(page).read()
    assert refgen.run(kind, page, check=True) == 1, "check mode should have gone red"
    assert open(page).read() == before, "check mode must not write"
    assert refgen.run(kind, page, check=False) == 0
    healed = open(page).read()
    assert healed != before, "regenerating changed nothing"
    assert expect_in_diff in healed, f"{expect_in_diff!r} did not reach the page"
    assert refgen.run(kind, page, check=True) == 0, "second check should be green"


_BASELINE = {}


def baseline(kind):
    """What the generator produces from the source as it stands."""
    if kind not in _BASELINE:
        saved = refgen.IBC
        try:
            refgen.IBC = ROOT
            refgen._ANCHORS.clear()
            _BASELINE[kind] = refgen.GENERATORS[kind]()
        finally:
            refgen.IBC = saved
            refgen._ANCHORS.clear()
    return _BASELINE[kind]


CITE = re.compile(r"^<!-- \[.*\]\(.*\) -->$", re.M)


def unchanged(box, kind, rename=None, cites=True):
    """Every generated region is byte-identical to the baseline.

    The other direction of the same contract. `red_then_healed` and `raises`
    prove the generator notices a change; this proves it does not notice one a
    reader could never see. Without it a generator passes its whole suite while
    billing a developer for every rename, which is the complaint these cases
    exist to answer.

    `rename` is (old, new) for a case that moves a directory: a citation names
    a real path, so it is expected to follow the move, and only the path is
    allowed to differ.
    """
    got = refgen.GENERATORS[kind]()
    want = baseline(kind)
    if rename:
        old, new = rename
        got = {k: v.replace(new, old) for k, v in got.items()}
    if not cites:
        # a mutation that adds a line moves every line below it, so the
        # citations move with it. That is the citations staying right, not the
        # tables drifting.
        got = {k: CITE.sub("", v) for k, v in got.items()}
        want = {k: CITE.sub("", v) for k, v in want.items()}
    assert set(got) == set(want), (
        f"regions changed: only in new {sorted(set(got) - set(want))}, "
        f"only in old {sorted(set(want) - set(got))}")
    for region in sorted(want):
        assert got[region] == want[region], (
            f"{region} changed, but nothing a reader sees did:\n"
            + "\n".join(difflib.unified_diff(
                want[region].splitlines(), got[region].splitlines(),
                "before", "after", lineterm="")))


def raises(box, kind, expect_in_message):
    try:
        box.page(kind, own=True)
        refgen.GENERATORS[kind]()
    except (refgen.SourceError, refgen.MarkerError) as e:
        assert expect_in_message in str(e), f"raised, but not about {expect_in_message!r}: {e}"
        return
    raise AssertionError(f"expected a raise mentioning {expect_in_message!r}")


# --------------------------------------------------------------- the CLI wiring

@case("a flag's usage string changes: page goes red, regenerating heals it")
def _():
    with Sandbox() as box:
        box.edit("cli/cmd/ibc/main.go",
                 '"attestation signature threshold"',
                 '"attestation signature threshold, at least 1"')
        red_then_healed(box, "cli", "Attestation signature threshold, at least 1")


@case("a new flag on a flagless command reaches the page")
def _():
    with Sandbox() as box:
        box.edit("cli/cmd/ibc/main.go",
                 "\tdpf := cmdDeploy.PersistentFlags()",
                 '\t_ = cmdDeployCore.Flags().Bool("fake", false, "a flag that was not there before")\n'
                 "\tdpf := cmdDeploy.PersistentFlags()")
        red_then_healed(box, "cli", "A flag that was not there before")


@case("a new command in a group raises: its section does not exist yet")
def _():
    with Sandbox() as box:
        box.append("cli/cmd/ibc/migrate.go", '''
var cmdMigrateFake = &cobra.Command{
	Use:   "fake",
	Short: "A migrate command nobody documented",
	RunE:  func(_ *cobra.Command, _ []string) error { return nil },
}
''')
        box.edit("cli/cmd/ibc/main.go",
                 "cmdMigrate.AddCommand(cmdMigrateUp, cmdMigrateDown, cmdMigrateStatus)",
                 "cmdMigrate.AddCommand(cmdMigrateUp, cmdMigrateDown, cmdMigrateStatus, cmdMigrateFake)")
        page = box.page("cli", own=True)
        try:
            refgen.run("cli", page, check=True)
        except refgen.MarkerError as e:
            assert "cli:cmd:migrate-fake" in str(e), e
            return
        raise AssertionError("a new command must not pass unnoticed")


@case("a new command group has no section, and that raises")
def _():
    with Sandbox() as box:
        box.append("cli/cmd/ibc/migrate.go", '''
var cmdFakeGroup = &cobra.Command{
	Use:   "fakegroup",
	Short: "A group nobody ordered",
}

var cmdFakeGroupThing = &cobra.Command{
	Use:   "thing",
	Short: "A command in an ungrouped group",
	RunE:  func(_ *cobra.Command, _ []string) error { return nil },
}
''')
        box.edit("cli/cmd/ibc/main.go",
                 "cmdMigrate.AddCommand(cmdMigrateUp, cmdMigrateDown, cmdMigrateStatus)",
                 "cmdMigrate.AddCommand(cmdMigrateUp, cmdMigrateDown, cmdMigrateStatus)\n"
                 "\tcmdFakeGroup.AddCommand(cmdFakeGroupThing)\n"
                 "\trootCmd.AddCommand(cmdFakeGroup)")
        raises(box, "cli", "fakegroup")


@case("a new group flag reaches every command under it")
def _():
    with Sandbox() as box:
        box.edit("cli/cmd/ibc/main.go",
                 "\tdpf := cmdDeploy.PersistentFlags()",
                 "\tdpf := cmdDeploy.PersistentFlags()\n"
                 '\tdpf.Bool("fake-inherited", false, "a flag every deploy command gains")')
        blocks = refgen.gen_cli()
        under = [k for k in blocks if k.startswith("cli:cmd:deploy-")]
        assert len(under) == 8, under
        for k in under:
            assert "`--fake-inherited`" in blocks[k], k


# ------------------------------------------------------------------- the config

@case("a new config key with no description anywhere raises")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 'ListenAddress string `yaml:"listenAddr"`',
                 'ListenAddress string `yaml:"listenAddr"`\n\tFake string `yaml:"fake"`')
        raises(box, "config", "Fake")


@case("a key that gains a doc comment upstream raises, so the fallback cannot shadow it")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 '\tListenAddress string `yaml:"listenAddr"`',
                 '\t// ListenAddress is the address the server binds.\n'
                 '\tListenAddress string `yaml:"listenAddr"`')
        raises(box, "config", "FALLBACK_DOCS")


@case("a renamed default constant raises rather than leaving a stale number")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/relay/pipeline/opts.go",
                 "DefaultBatchSize", "DefaultBatchSizeRenamed", count=99)
        raises(box, "config", "DefaultBatchSize")


@case("a changed default value reaches the page")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/relay/dispatch/dispatcher.go",
                 "const DefaultPollInterval = 1 * time.Second",
                 "const DefaultPollInterval = 9 * time.Second")
        red_then_healed(box, "config", "`9s`")


# ---------------------------------------------------------------------- the API

@case("a changed proto comment reaches the page")
def _():
    with Sandbox() as box:
        box.edit("proto/cli/relayer.proto",
                 "// The relayer is still processing the packet.",
                 "// The relayer has not finished with the packet yet.")
        red_then_healed(box, "api", "The relayer has not finished with the packet yet.")


@case("a new proto message has no marker on the page, and that raises")
def _():
    with Sandbox() as box:
        box.append("proto/cli/relayer.proto", '''
message FakeThing {
  // Fake is a field nobody reads, described so this case tests the marker
  // rule rather than the description rule.
  string fake = 1;
}
''')
        page = box.page("api", own=True)
        try:
            refgen.run("api", page, check=True)
        except refgen.MarkerError as e:
            assert "api:msg:FakeThing" in str(e), e
            return
        raise AssertionError("expected a MarkerError naming the new message")


@case("a key whose meaning changes under a stable name raises on its fingerprint")
def _():
    with Sandbox() as box:
        # the name, the yaml tag, and the absence of a doc comment all stay put:
        # only the type changes, which no table cell of its own would reveal as
        # a meaning change
        box.edit("cli/internal/config/config.go",
                 'ListenAddress string `yaml:"listenAddr"`',
                 'ListenAddress []string `yaml:"listenAddr"`')
        raises(box, "config", "fingerprint")


@case("a validation rule added to a described key raises on its fingerprint")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 '''func (c ServerConfig) Validate() error {
	if err := network.ValidateListenAddr(c.ListenAddress); err != nil {''',
                 '''func (c ServerConfig) Validate() error {
	if c.ListenAddress == "" {
		return errors.New(".listenAddr required")
	}
	if err := network.ValidateListenAddr(c.ListenAddress); err != nil {''')
        raises(box, "config", "fingerprint")


@case("a described key that is removed leaves no dead description")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/relayer.go",
                 '\tGasTipCapMultiplier *float64 `yaml:"gasTipCapMultiplier,omitempty"`\n', "")
        raises(box, "config", "FALLBACK_DOCS describes fields that are gone")


# ----------------------------------------------- discovery, not hardcoded lists

@case("a new config struct reachable from Config is discovered, not listed")
def _():
    with Sandbox() as box:
        box.add_field("cli/internal/config/config.go", "signers",
                      '\tMetrics MetricsConfig `yaml:"metrics"`')
        box.append("cli/internal/config/config.go",
                   '\n\ntype MetricsConfig struct {\n'
                   '\tListenAddress string `yaml:"listenAddr"`\n}\n')
        page = box.page("config", own=True)
        try:
            refgen.run("config", page, check=True)
        except (refgen.SourceError, refgen.MarkerError) as e:
            assert "etrics" in str(e), e
            return
        raise AssertionError("a new block must not pass unnoticed")


@case("a new config struct's table and heading arrive in the plan")
def _():
    with Sandbox() as box:
        box.add_field("cli/internal/config/config.go", "signers",
                      '\tMetrics MetricsConfig `yaml:"metrics"`')
        box.append("cli/internal/config/config.go",
                   '\n\n// MetricsConfig config for the metrics endpoint.\n'
                   'type MetricsConfig struct {\n'
                   '\t// ListenAddress is where metrics are served.\n'
                   '\tListenAddress string `yaml:"listenAddr"`\n}\n')
        page = box.page("config", own=True)
        p = refgen.plan("config", page)
        new = [m for m in p["missing_marker"] if m["region"] == "config:metrics"]
        assert new, p["missing_marker"]
        assert "`listenAddr`" in new[0]["table"]
        assert new[0]["suggested_heading"] == "### `metrics`"
        assert new[0]["insert_after"], "a new section needs somewhere to go"


@case("a new proto file is discovered, and its missing section raises")
def _():
    with Sandbox() as box:
        os.makedirs(os.path.join(box.dir, "proto/cli"), exist_ok=True)
        open(os.path.join(box.dir, "proto/cli/extra.proto"), "w").write(
            'syntax = "proto3";\n\npackage ibc.v2.extra;\n\n'
            "// Something new.\nmessage NewThing {\n  string name = 1;\n}\n")
        raises(box, "api", "extra")


@case("a new group flag reaches every command in that group")
def _():
    with Sandbox() as box:
        box.edit("cli/cmd/ibc/main.go",
                 "\tcmdConfig.AddCommand(",
                 '\t_ = cmdConfig.PersistentFlags().Bool("fake-group-flag", false, "inherited")\n'
                 "\tcmdConfig.AddCommand(")
        blocks = refgen.gen_cli()
        for path in ("config-new", "config-validate", "config-add-chain"):
            assert "`--fake-group-flag`" in blocks[f"cli:cmd:{path}"], path


@case("plan mode collects every gap rather than stopping at the first")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 'ListenAddress string `yaml:"listenAddr"`',
                 'ListenAddress string `yaml:"listenAddr"`\n'
                 '\tFakeOne string `yaml:"fakeOne"`\n'
                 '\tFakeTwo string `yaml:"fakeTwo"`')
        page = box.page("config", own=True)
        gaps = [c for c in refgen.plan("config", page)["curation"]
                if c["kind"] == "missing_description"]
        assert len(gaps) == 2, gaps
        assert all(g["fingerprint"] for g in gaps), "a gap carries what the fix needs"


# ------------------------------------------------------- removals, not just adds

@case("a removed flag disappears from the page")
def _():
    with Sandbox() as box:
        box.edit("cli/cmd/ibc/main.go",
                 '\tcmdDeployClient.Flags().Uint8Var(&flagDeployThreshold, "threshold", 1, '
                 '"attestation signature threshold")\n', "")
        page = box.page("cli")
        assert refgen.run("cli", page, check=True) == 1
        assert refgen.run("cli", page, check=False) == 0
        assert "Attestation signature threshold" not in open(page).read()


@case("a removed command orphans its section, and that raises")
def _():
    with Sandbox() as box:
        box.edit("cli/cmd/ibc/main.go",
                 "cmdKeys.AddCommand(cmdKeysNew, cmdKeysShow, cmdKeysImport, cmdKeysList)",
                 "cmdKeys.AddCommand(cmdKeysNew, cmdKeysShow, cmdKeysImport)")
        page = box.page("cli", own=True)
        try:
            refgen.run("cli", page, check=True)
        except refgen.MarkerError as e:
            assert "cli:cmd:keys-list" in str(e), e
            assert "delete the page section" in str(e), e
            return
        raise AssertionError("a removed command must not pass unnoticed")


@case("a removed config key disappears from its table")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 '\tFinalityOffset uint `yaml:"finalityOffset"`', "")
        page = box.page("config")
        assert refgen.run("config", page, check=True) == 1
        assert refgen.run("config", page, check=False) == 0
        body = open(page).read()
        region = body.split("GEN:config:attestors:local START")[1].split("END")[0]
        assert "`finalityOffset`" not in region, region


@case("a removed proto field raises on its hand-written description")
def _():
    with Sandbox() as box:
        # TransactionInfo's own chain_id, not either message's source_chain_id
        box.edit("proto/cli/relayer.proto",
                 "message TransactionInfo {\n  string tx_hash = 1;\n  string chain_id = 2;\n}",
                 "message TransactionInfo {\n  string tx_hash = 1;\n}")
        # the field carried a hand-written description, so removing it leaves a
        # dead entry rather than quietly shrinking the table
        raises(box, "api", "TransactionInfo.chain_id")


@case("a removed proto message raises, and the message says what to do")
def _():
    with Sandbox() as box:
        box.edit("proto/cli/attestor.proto",
                 "message InfoRequest { string attestor = 1; }", "")
        raises(box, "api", "InfoRequest")


@case("a streaming rpc raises rather than vanishing from the page")
def _():
    with Sandbox() as box:
        box.edit("proto/cli/relayer.proto", "  rpc Relay(",
                 "  rpc Watch(stream WatchRequest) returns (stream WatchResponse);\n"
                 "  rpc Relay(")
        raises(box, "api", "does not read")


@case("a map field raises rather than vanishing from its table")
def _():
    with Sandbox() as box:
        box.edit("proto/cli/relayer.proto", "message RelayRequest {",
                 "message RelayRequest {\n  map<string, string> labels = 99;")
        raises(box, "api", "does not read")


# --------------------------------------- source a reader's table depends on
#
# Each of these used to shorten a table rather than refuse. A shorter table
# reads exactly like a complete one, and regenerating makes the check green
# again, so the page can lose a whole block and look current.


@case("an embedded struct folded into the parent's keys raises")
def _():
    with Sandbox() as box:
        box.add_field("cli/internal/config/config.go", "deployer,omitempty",
                      "\tExtra ExtraFields `yaml:\",inline\"`")
        raises(box, "config", "does not read")


@case("an anonymous nested struct raises rather than losing its rows")
def _():
    with Sandbox() as box:
        box.add_field("cli/internal/config/config.go", "deployer,omitempty",
                      "\tTuning struct{ N int } `yaml:\"tuning\"`")
        raises(box, "config", "does not read")


@case("a second exported builder for the root config raises")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 "func DefaultConfig() Config {",
                 "func EmptyConfig() Config {\n\treturn Config{}\n}\n\n"
                 "func DefaultConfig() Config {")
        raises(box, "config", "cannot locate")


# ------------------------------------------- changes a reader cannot see
#
# Each case below is a change a developer makes without thinking about it, and
# which changes no word on the page. The generator used to stop for most of
# them and ask to be taught a new name. Now it must not notice at all.


@case("the config builder returning a pointer changes nothing")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 "func DefaultConfig() Config {", "func DefaultConfig() *Config {")
        box.edit("cli/internal/config/config.go", "\treturn Config{", "\treturn &Config{")
        unchanged(box, "config")


@case("splitting the root command into its own file changes nothing")
def _():
    with Sandbox() as box:
        src = os.path.join(box.dir, "cli/cmd/ibc")
        text = open(os.path.join(src, "main.go")).read()
        at = text.index("var rootCmd")
        with open(os.path.join(src, "root.go"), "w") as fh:
            fh.write('package main\n\nimport (\n\t"github.com/spf13/cobra"\n)\n\n'
                     + text[at:])
        with open(os.path.join(src, "main.go"), "w") as fh:
            fh.write(text[:at])
        refgen._ANCHORS.clear()
        unchanged(box, "config")


@case("a trailing comment on a config field changes nothing")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 "Deployer string %syaml:\"deployer,omitempty\"%s" % (TICK, TICK),
                 "Deployer string %syaml:\"deployer,omitempty\"%s // set per chain"
                 % (TICK, TICK))
        unchanged(box, "config")


@case("a named string constant added to the config package changes nothing")
def _():
    with Sandbox() as box:
        # `const X string = "..."` says its type is the builtin, not one of the
        # package's own string types. Reading it as an enum member rendered
        # every string key in every table as a list of unrelated values.
        box.edit("cli/internal/config/errors.go", "package config",
                 'package config\n\nconst DefaultListenAddr string = "127.0.0.1:9090"')
        unchanged(box, "config", cites=False)


@case("a comment that spells out a constant changes nothing")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/errors.go", "package config",
                 'package config\n\n// Note: DBTypePostgres = "mysql" before v2.')
        unchanged(box, "config", cites=False)


@case("a trailing comment on a proto field changes nothing")
def _():
    with Sandbox() as box:
        box.edit("proto/cli/relayer.proto",
                 "  string source_chain_id = 2;",
                 "  string source_chain_id = 2; // as the chain reports it")
        unchanged(box, "api")


@case("a comment detached from the field below it documents nothing")
def _():
    with Sandbox() as box:
        box.edit("proto/cli/relayer.proto", "message RelayRequest {",
                 "message RelayRequest {\n  // TODO: add a total before v3.\n")
        unchanged(box, "api", cites=False)


@case("renaming the CLI's entry point file changes nothing")
def _():
    with Sandbox() as box:
        src = os.path.join(box.dir, "cli/cmd/ibc")
        os.rename(os.path.join(src, "main.go"), os.path.join(src, "root.go"))
        refgen._ANCHORS.clear()
        unchanged(box, "cli", rename=("main.go", "root.go"))


@case("renaming a Go field while keeping its yaml key changes nothing")
def _():
    with Sandbox() as box:
        # the key a reader writes is the yaml one; the Go spelling beside it is
        # the package's business. Keying the hand-written descriptions on the
        # Go name made this rename a refusal for about thirty fields.
        for sub in ("cli/internal/config",):
            box.rename(sub, "ListenAddress", "Listen")
        unchanged(box, "config")


@case("renaming a validation helper changes nothing")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 "validateChainReferences", "checkChainReferences", count=-1)
        unchanged(box, "config")


@case("renaming a Validate receiver changes nothing")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 "func (c Observability) Validate() error {",
                 "func (obs Observability) Validate() error {")
        box.edit("cli/internal/config/config.go",
                 "\tswitch {\n\tcase !c.Metrics:", "\tswitch {\n\tcase !obs.Metrics:")
        unchanged(box, "config")


@case("renaming an error constructor changes nothing")
def _():
    with Sandbox() as box:
        for f in ("errors.go", "config.go", "relayer.go"):
            box.edit(f"cli/internal/config/{f}", "errPathf", "cfgErrorf", count=-1)
        unchanged(box, "config")


@case("renaming a command variable changes nothing")
def _():
    with Sandbox() as box:
        box.rename("cli/cmd/ibc", "cmdDeployClient", "deployClientCmd")
        unchanged(box, "cli")


@case("renaming the root command variable changes nothing")
def _():
    with Sandbox() as box:
        box.rename("cli/cmd/ibc", "rootCmd", "cliRoot")
        unchanged(box, "cli")


@case("moving the CLI to another directory changes only the citations")
def _():
    with Sandbox() as box:
        os.rename(os.path.join(box.dir, "cli"), os.path.join(box.dir, "link"))
        refgen._ANCHORS.clear()
        unchanged(box, "config", rename=("cli/", "link/"))
        unchanged(box, "cli", rename=("cli/", "link/"))


@case("realigning a struct's columns changes nothing")
def _():
    with Sandbox() as box:
        # gofmt re-pads a struct's columns whenever a longer field name lands
        # beside it. Squeeze the padding out without moving a line, so the only
        # difference is the whitespace a reader never sees.
        box.respace("cli/internal/config/config.go", "type Config struct {")
        r = subprocess.run(["gofmt", "-l", "internal/config/config.go"],
                           cwd=os.path.join(box.dir, "cli"),
                           capture_output=True, text=True)
        assert r.returncode == 0, r.stderr
        assert r.stdout.strip(), "the respacing should have left the file unformatted"
        unchanged(box, "config")


for name in PASS:
    print(f"  ok    {name}")
for name, e, tb in FAIL:
    print(f"  FAIL  {name}: {e}")
print(f"\n{len(PASS)} passed, {len(FAIL)} failed")
if FAIL:
    print()
    print(FAIL[0][2])
sys.exit(1 if FAIL else 0)
