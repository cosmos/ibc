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


DISCOVER_KINDS = os.environ.get("REFGEN_TEST_DISCOVER_KINDS")


def raises(box, kind, expect_in_message, expect_kind=None):
    """The generator refuses, and refuses about the right thing.

    `expect_kind` is the point. Matching only a substring accepted any refusal
    that happened to name the same identifier: two cases named for
    `stale_fallback` both passed on `dead_description`, and a refusal an agent
    accidentally removed could be replaced by an unrelated one without the
    suite noticing. A `MarkerError` carries no kind, so those cases pass None.
    """
    try:
        box.page(kind, own=True)
        refgen.GENERATORS[kind]()
    except (refgen.SourceError, refgen.MarkerError) as e:
        got = getattr(e, "kind", None)
        if DISCOVER_KINDS:
            print(f"    KIND {kind} {expect_in_message!r} -> {got!r}")
        assert expect_in_message in str(e), f"raised, but not about {expect_in_message!r}: {e}"
        if expect_kind is not None:
            assert got == expect_kind, (
                f"raised {got!r}, expected {expect_kind!r}: {e}")
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
        raises(box, "cli", "fakegroup", "ungrouped_command")


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
        raises(box, "config", "Fake", "missing_description")


@case("a key that gains a doc comment upstream raises, so the fallback cannot shadow it")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 '\tListenAddress string `yaml:"listenAddr"`',
                 '\t// ListenAddress is the address the server binds.\n'
                 '\tListenAddress string `yaml:"listenAddr"`')
        raises(box, "config", "FALLBACK_DOCS", "stale_fallback")


@case("a renamed default constant raises rather than leaving a stale number")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/relay/pipeline/opts.go",
                 "DefaultBatchSize", "DefaultBatchSizeRenamed", count=99)
        raises(box, "config", "DefaultBatchSize", "unreadable_default")


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
                 'ListenAddress ListenAddr `yaml:"listenAddr"`')
        box.edit("cli/internal/config/config.go", "type ServerConfig struct {",
                 "type ListenAddr = string\n\ntype ServerConfig struct {")
        raises(box, "config", "fingerprint", "fingerprint_mismatch")


@case("a validation rule added to a described key raises on its fingerprint")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 '''func (c ServerConfig) Validate() error {
	if err := network.ValidateListenAddr(c.ListenAddress); err != nil {''',
                 '''func (c ServerConfig) Validate() error {
	if c.ListenAddress == "" {
		return errPathf("listenAddr", "required")
	}
	if err := network.ValidateListenAddr(c.ListenAddress); err != nil {''')
        raises(box, "config", "fingerprint", "fingerprint_mismatch")


@case("a described key that is removed leaves no dead description")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/relayer.go",
                 'GasTipCapMultiplier *float64 `yaml:"gasTipCapMultiplier,omitempty"`',
                 'GasTipCapMultiplier *float64 `yaml:"-"`')
        raises(box, "config", "FALLBACK_DOCS describes fields that are gone",
               "dead_description")


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
        raises(box, "api", "extra", "unlisted_service")


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
        # `yaml:"-"` is how a key stops being part of the file while the Go
        # that reads it keeps compiling -- deleting the field outright is a
        # change to four other packages, not a documentation change.
        box.edit("cli/internal/config/config.go",
                 'FinalityOffset uint `yaml:"finalityOffset"`',
                 'FinalityOffset uint `yaml:"-"`')
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
        raises(box, "api", "TransactionInfo.chain_id", "dead_field_doc")


@case("a removed proto message stops the page, because the schema stops compiling")
def _():
    with Sandbox() as box:
        # An rpc still names it, so this is not a documentation problem before
        # it is a schema problem. The compiler says so first, and says it
        # better than a text scan could.
        box.edit("proto/cli/attestor.proto",
                 "message InfoRequest { string attestor = 1; }", "")
        raises(box, "api", "buf build failed", "proto_build_failed")


@case("a streaming rpc refuses rather than rendering as a unary call")
def _():
    with Sandbox() as box:
        # Valid schema this time -- the descriptor reads streaming fine. The
        # refusal is the page's, not the parser's: its rows hold one request
        # body and one response body, and a stream has neither.
        box.edit("proto/cli/relayer.proto", "message RelayRequest {",
                 "message WatchRequest { string chain_id = 1; }\n"
                 "message WatchResponse { string chain_id = 1; }\n"
                 "message RelayRequest {")
        box.edit("proto/cli/relayer.proto", "  rpc Relay(",
                 "  // Watch follows packets as they arrive.\n"
                 "  rpc Watch(stream WatchRequest) returns (stream WatchResponse);\n"
                 "  rpc Relay(")
        raises(box, "api", "streams", "streaming_rpc")


@case("a map field is read, and is described like any other field")
def _():
    with Sandbox() as box:
        # This used to refuse, because a text scan could not read `map<>`. The
        # compiler can, so the only thing left to ask for is a description --
        # and the synthetic entry type protobuf generates behind a map must not
        # surface as a message of its own.
        box.edit("proto/cli/relayer.proto", "message RelayRequest {",
                 "message RelayRequest {\n"
                 "  // Labels are forwarded to the receipt.\n"
                 "  map<string, string> labels = 99;")
        blocks = refgen.GENERATORS["api"]()
        body = blocks["api:msg:RelayRequest"]
        assert "`labels`" in body, f"the map field is missing:\n{body}"
        assert "map<string, string>" in body, \
            f"a map must not publish protobuf's internal entry type:\n{body}"
        assert "Labels are forwarded to the receipt." in body, \
            f"description lost:\n{body}"
        assert not [r for r in blocks if "LabelsEntry" in r], \
            f"protobuf's synthetic map entry surfaced as a message: {sorted(blocks)}"


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
        raises(box, "config", "does not read", "unreadable_member")


@case("an anonymous nested struct raises rather than losing its rows")
def _():
    with Sandbox() as box:
        box.add_field("cli/internal/config/config.go", "deployer,omitempty",
                      "\tTuning struct{ N int } `yaml:\"tuning\"`")
        raises(box, "config", "does not read", "unreadable_member")


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
        # the builder assembling into a local rather than returning the literal
        # directly. Changing its return type instead would be a change to every
        # caller in two packages -- a refactor of the CLI, not of the builder.
        box.edit("cli/internal/config/config.go",
                 "\treturn Config{", "\tbuilt := Config{")
        box.edit("cli/internal/config/config.go",
                 "\t\tSigners:   Signers{},\n\t}\n}",
                 "\t\tSigners:   Signers{},\n\t}\n\treturn built\n}")
        unchanged(box, "config")


# "splitting the root command into its own file changes nothing" was removed.
# Its mutation wrote a root.go importing only cobra while the moved code needed
# five more packages, so the tree never compiled -- and it asserted on the
# config generator, which never built the binary and so never noticed. It
# tested neither the split nor the generator it named. Path-independence is
# covered by "renaming the CLI's entry point file changes nothing", which does
# build.


@case("a trailing comment on a config field changes nothing")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/config.go",
                 "Deployer string %syaml:\"deployer,omitempty\"%s" % (TICK, TICK),
                 "Deployer string %syaml:\"deployer,omitempty\"%s // set per chain"
                 % (TICK, TICK))
        unchanged(box, "config")


@case("the required-flag canary fires when the probe stops recognising Cobra")
def _():
    # The CLI-side twin of the config canary, and the other guard with no test:
    # both could be deleted and all 110 cases stayed green. Simulated at the
    # regexes rather than by rewording Cobra, because the regexes are what goes
    # stale when Cobra rewords itself.
    saved, saved_also = refgen._REQUIRED, list(refgen._REQUIRED_ALSO)
    refgen._REQUIRED = re.compile(r"cobra-said-something-else-entirely")
    refgen._REQUIRED_ALSO[:] = []
    try:
        refgen.GENERATORS["cli"]()
    except refgen.SourceError as e:
        assert e.kind == "all_flags_optional", f"raised {e.kind!r}: {e}"
        return
    finally:
        refgen._REQUIRED = saved
        refgen._REQUIRED_ALSO[:] = saved_also
    raise AssertionError("expected the canary to refuse")


def _claimed_requiredness():
    """{key: True/False} from the page, for key names that mean one thing.

    A name used by two tables with different answers -- `type` has a default
    under `db` and is required under `signers` -- cannot be checked by name
    alone, so it is reported uncovered rather than guessed at.
    """
    claims, seen = {}, {}
    for body in refgen.GENERATORS["config"]().values():
        for line in body.split("\n"):
            if not line.startswith("| `"):
                continue
            cells = [c.strip() for c in line.split("|")]
            if len(cells) < 5:
                continue
            # the third cell is a default, or `optional`, or `**required**`.
            # A key with a default is not "optional": removing it is allowed to
            # fail for reasons that have nothing to do with the column, so only
            # the two cells that actually make a claim are judged.
            cell = cells[3]
            kind = ("required" if "required" in cell
                    else "optional" if cell == "optional" else "default")
            for key in cells[1].split(","):
                key = key.strip().strip("`").split(".")[-1].split("[")[0]
                if not key:
                    continue
                # every occurrence counts, including the ones carrying a
                # default: `type` is required under `signers` and defaulted
                # under `db`, and judging it by name alone read the defaulted
                # one as a disagreement
                seen.setdefault(key, set()).add(kind)
                claims[key] = kind == "required"
    usable = {k: v for k, v in claims.items()
              if seen[k] in ({"required"}, {"optional"})}
    return usable, sorted(k for k in seen if k not in usable)


@case("every key the page calls required is one the binary refuses to run without")
def _():
    # The requiredness column is read from words in the Go validation messages
    # (REQUIREMENT_VOCABULARY). Reword one and a key silently flips to
    # optional. The generator is not what fixes that here -- this is: blank a
    # key in the fixture the tests already keep valid, and ask the binary. A
    # reworded message now fails a test instead of changing a page.
    with Sandbox() as box:
        binary = refgen.build_cli()
        claims, ambiguous = _claimed_requiredness()
        fixture = os.path.join(box.dir, "cli/internal/config/testdata/sample.yml")
        original = open(fixture).read()
        lines = original.split("\n")

        checked, wrong, uncovered = 0, [], set(ambiguous)
        for i, line in enumerate(lines):
            m = re.match(r"^(\s*)(?:- )?([A-Za-z]\w*): +(\S.*)$", line)
            if not m or m.group(3).strip() in ("|", ">"):
                continue
            key = m.group(2)
            if key not in claims:
                uncovered.add(key)
                continue
            # removed, not emptied: "required" means absent, and an empty
            # string in an int or bool field fails as a type error, which is a
            # different refusal that would read as a disagreement
            kept = lines[:i] + lines[i + 1:]
            if line.lstrip().startswith("- "):
                # this key opens a list item, so it carries the `- `. Dropping
                # the line alone would orphan the rest of the item; the next
                # line at the same depth inherits the dash instead. Without
                # this, every list-opening key went untested -- which on this
                # fixture is most of the required ones.
                indent = len(line) - len(line.lstrip())
                sibling = indent + 2
                if i >= len(kept) or (len(kept[i]) - len(kept[i].lstrip())) != sibling:
                    uncovered.add(key)
                    continue
                kept[i] = " " * indent + "- " + kept[i].lstrip()
            open(fixture, "w").write("\n".join(kept))
            home = tempfile.mkdtemp(prefix="refgen-req-")
            try:
                shutil.copyfile(fixture, os.path.join(home, "ibc.yml"))
                r = subprocess.run([binary, "config", "validate", "--home", home],
                                   capture_output=True, text=True, timeout=30)
                complained = key in (r.stdout + r.stderr) and r.returncode != 0
            finally:
                shutil.rmtree(home, ignore_errors=True)
            checked += 1
            if complained != claims[key]:
                wrong.append(
                    f"line {i + 1}: the page calls `{key}` "
                    f"{'required' if claims[key] else 'optional'}, "
                    f"but removing it {'is accepted' if not complained else 'is refused'}")
        open(fixture, "w").write(original)

        assert checked >= 15, f"only {checked} keys were exercised; the fixture shrank?"
        assert not wrong, ("the page and the binary disagree about what is required:\n  "
                           + "\n  ".join(wrong))
        print(f"      [{checked} keys verified against the binary; "
              f"{len(uncovered)} not covered by the fixture: {sorted(uncovered)}]")


@case("the example config tracks the fixture the Go tests validate")
def _():
    with Sandbox() as box:
        box.edit("cli/internal/config/testdata/sample.yml",
                 "listenAddr: 0.0.0.0:3000", "listenAddr: 0.0.0.0:3999")
        red_then_healed(box, "config", "0.0.0.0:3999")


@case("the example config is the fixture byte for byte")
def _():
    with Sandbox() as box:
        # A copy that quietly reformats is a copy a reader cannot trust: the
        # whole point is that what the page shows is what the tests validate.
        block = refgen.GENERATORS["config"]()["config:example"]
        body = block.split("```yaml\n", 1)[1].rsplit("\n```", 1)[0]
        fixture = open(os.path.join(
            box.dir, "cli/internal/config/testdata/sample.yml")).read()
        stripped = fixture[fixture.index("server:"):].strip("\n")
        assert body == stripped, "the published example is not the fixture"


@case("the example config refuses when the fixture the tests load is gone")
def _():
    with Sandbox() as box:
        os.remove(os.path.join(box.dir,
                               "cli/internal/config/testdata/sample.yml"))
        raises(box, "config", "not there to read", "unreadable_example_config")


@case("a second test fixture refuses rather than picking one")
def _():
    with Sandbox() as box:
        # Which config a reference page should show is an editorial choice, and
        # picking the first silently would publish whichever sorted first.
        src = os.path.join(box.dir, "cli/internal/config/testdata")
        shutil.copyfile(os.path.join(src, "sample.yml"),
                        os.path.join(src, "other.yml"))
        box.edit("cli/internal/config/relayer_test.go",
                 'filepath.Join("testdata", "sample.yml")',
                 'filepath.Join("testdata", "other.yml")', count=1)
        raises(box, "config", "more than one yaml fixture",
               "ambiguous_example_config")


@case("a group flag registered non-persistently refuses instead of reaching every command")
def _():
    with Sandbox() as box:
        # `.Flags()` where `.PersistentFlags()` was meant is one character in
        # review. Cobra prints a group's local and persistent flags together
        # under `Flags:`, so inferring inheritance from the group's own help
        # put this row on all eight commands under `deploy`, every one of which
        # answers `unknown flag: --audit-log`. The child's help distinguishes
        # them -- only genuinely inherited flags appear under `Global Flags:`
        # -- so the flag now reaches no table, and reaching no table is refused.
        box.edit("cli/cmd/ibc/main.go",
                 'dpf.BoolVar(&flagDeployYes, "yes", false, "skip confirmation prompts")',
                 'dpf.BoolVar(&flagDeployYes, "yes", false, "skip confirmation prompts")\n'
                 '\tcmdDeploy.Flags().Bool("audit-log", false, "write an audit log")')
        raises(box, "cli", "audit-log", "uninherited_flag")


@case("a group flag registered persistently still reaches every command under it")
def _():
    with Sandbox() as box:
        # The other direction of the same contract: the fix above must not make
        # a correctly-registered group flag disappear.
        box.edit("cli/cmd/ibc/main.go",
                 'dpf.BoolVar(&flagDeployYes, "yes", false, "skip confirmation prompts")',
                 'dpf.BoolVar(&flagDeployYes, "yes", false, "skip confirmation prompts")\n'
                 '\tdpf.Bool("audit-log", false, "write an audit log")')
        blocks = refgen.GENERATORS["cli"]()
        under = [r for r in blocks if r.startswith("cli:cmd:deploy-")]
        assert under, "no deploy subcommand regions found"
        missing = [r for r in under if "--audit-log" not in blocks[r]]
        assert not missing, f"group flag missing from {missing}"


@case("a flag with both an author default and a Cobra default publishes the Cobra one")
def _():
    with Sandbox() as box:
        # `re.search` is leftmost and `(.+)` is greedy, so a flag carrying both
        # spanned from the author's parenthetical to Cobra's closing paren and
        # published `cli-<a>-<b>, chain ids sorted) (default "auto` as the
        # default. Cobra's is the binary's answer and wins; the author's prose
        # about the computed value stays in the description, where it reads.
        box.edit("cli/cmd/ibc/main.go",
                 'StringVar(&flagDeployClientID, "client-id", "", '
                 '"client id (default: cli-<a>-<b>, chain ids sorted)")',
                 'StringVar(&flagDeployClientID, "client-id", "auto", '
                 '"client id (default: cli-<a>-<b>, chain ids sorted)")')
        blocks = refgen.GENERATORS["cli"]()
        row = [l for b in blocks.values() for l in b.split("\n")
               if "`--client-id" in l and "chain ids sorted" in l]
        assert row, "the client-id row disappeared"
        for l in row:
            cell = l.split("|")[2].strip()
            assert cell == "`auto`", f"default cell is {cell!r}, not Cobra's:\n{l}"
            assert '(default "' not in l, f"Cobra's parenthetical leaked:\n{l}"
            assert "chain ids sorted" in l, f"author's prose lost:\n{l}"


@case("an exported config field with no yaml tag publishes the key the binary accepts")
def _():
    with Sandbox() as box:
        # Forgetting the tag is the most ordinary omission in Go, and the
        # fallback published the Go field name. goccy lowercases an untagged
        # field, and the CLI runs with DisallowUnknownField, so the page named
        # a key the binary answers `unknown field "MaxRecvBytes"` to -- while
        # refgen's own sample config wrote `maxrecvbytes`. Verified against the
        # binary: plain ToLower of the whole name, not camelCase.
        box.edit("cli/internal/config/config.go",
                 'ListenAddress string %syaml:"listenAddr"%s' % (TICK, TICK),
                 'ListenAddress string %syaml:"listenAddr"%s\n\n'
                 '\t// MaxRecvBytes is the largest request the gRPC server accepts.\n'
                 '\tMaxRecvBytes int' % (TICK, TICK))
        server = refgen.GENERATORS["config"]()["config:server"]
        assert "`maxrecvbytes`" in server, f"lowercased key missing:\n{server}"
        assert "MaxRecvBytes" not in server, f"Go field name published:\n{server}"


@case("an untagged field whose name is an acronym is lowercased whole")
def _():
    with Sandbox() as box:
        # `TLSCertFile` becomes `tlscertfile`, not `tlsCertFile`: the rule is
        # ToLower over the whole name, which a camelCase guess would get wrong.
        box.edit("cli/internal/config/config.go",
                 'ListenAddress string %syaml:"listenAddr"%s' % (TICK, TICK),
                 'ListenAddress string %syaml:"listenAddr"%s\n\n'
                 '\t// TLSCertFile is the certificate the gRPC server presents.\n'
                 '\tTLSCertFile string' % (TICK, TICK))
        server = refgen.GENERATORS["config"]()["config:server"]
        assert "`tlscertfile`" in server, f"expected tlscertfile:\n{server}"


def _cell(box, region, key):
    for line in refgen.GENERATORS["config"]()[region].split("\n"):
        if line.startswith(f"| `{key}`"):
            return [c.strip() for c in line.split("|")][3]
    raise AssertionError(f"no row for {key} in {region}")


@case("a newly required config key does not blind the flag probe")
def _():
    with Sandbox() as box:
        # `config add-chain` is probed like any other command, and it runs for
        # real rather than being rejected. With one shared home it wrote a
        # chain missing the new key, and every command sorting after it failed
        # at config load before reaching its own flag checks -- five required
        # flags published as `optional`. A home per command keeps one
        # command's side effects out of the next one's answer.
        box.edit("cli/internal/config/config.go",
                 'Deployer string %syaml:"deployer,omitempty"%s' % (TICK, TICK),
                 'Deployer string %syaml:"deployer,omitempty"%s\n\n'
                 '\t// Region is the provider region, such as "us-east-1".\n'
                 '\tRegion string %syaml:"region"%s' % (TICK, TICK, TICK, TICK))
        box.edit("cli/internal/config/config.go", "\tchainType := c.Type()",
                 '\tif c.Region == "" {\n'
                 '\t\treturn errPathf("region", "required")\n'
                 '\t}\n\tchainType := c.Type()')
        blocks = refgen.GENERATORS["cli"]()
        for region, flag in (("cli:cmd:deploy-core", "--chain"),
                             ("cli:cmd:deploy-render-config", "--signer-a")):
            row = [l for l in blocks[region].split("\n")
                   if l.startswith(f"| `{flag} ")]
            assert row, f"no {flag} row in {region}"
            assert "required" in row[0].split("|")[2], \
                f"{flag} lost its required mark in {region}: {row[0]}"


@case("rewording a variant rule does not move a key into the wrong table")
def _():
    with Sandbox() as box:
        # Before the membership probe this put `finalityOffset` into the remote
        # attestor table, where the binary rejects it -- a key a reader would
        # set and get an error for. The program refuses it either way, and
        # refusing is what is asked now rather than the wording.
        box.edit("cli/internal/config/config.go",
                 '"must not be set for local attestors"',
                 '"is not valid on a local attestor"')
        box.edit("cli/internal/config/config.go",
                 '"must not be set for remote attestors"',
                 '"is not valid on a remote attestor"', count=99)
        blocks = refgen.GENERATORS["config"]()
        remote = blocks["config:attestors:remote"]
        local = blocks["config:attestors:local"]
        for key in ("finalityOffset", "chainId", "signer"):
            assert f"`{key}`" not in remote, f"{key} reached the remote table:\n{remote}"
        assert "`grpc`" not in local, f"grpc reached the local table:\n{local}"
        assert "`grpc`" in remote and "`finalityOffset`" in local


@case("a requirement moved into a helper is still read")
def _():
    with Sandbox() as box:
        # The defect this replaced. `_requirement` read only what Validate
        # itself returns, so a rule delegated to a helper vanished and the key
        # rendered `optional` with nothing raising. The binary does not care
        # which function the rule lives in.
        box.edit("cli/internal/config/config.go",
                 """	case c.RPC == "":
		return errPathf("rpc", "required")
""",
                 """	case c.validateRPC() != nil:
		return c.validateRPC()
""")
        box.edit("cli/internal/config/config.go",
                 "func (c EVMChainConfig) Validate(validateICS26Router bool) error {",
                 """func (c EVMChainConfig) validateRPC() error {
	if c.RPC == "" {
		return errPathf("rpc", "required")
	}
	return nil
}

func (c EVMChainConfig) Validate(validateICS26Router bool) error {""")
        assert _cell(box, "config:chains", "evm.rpc") == "**required**", \
            _cell(box, "config:chains", "evm.rpc")


@case("a new required key stops the page until a fixture carries it")
def _():
    with Sandbox() as box:
        # The other direction: the column has to follow the code, not just
        # resist rewording. `deployer` carries no hand-written description, so
        # nothing else intercepts this.
        box.edit("cli/internal/config/config.go",
                 "func (c ChainConfig) Validate(", """func (c ChainConfig) validateDeployer() error {
	if c.Deployer == "" {
		return errPathf("deployer", "required")
	}
	return nil
}

func (c ChainConfig) Validate(""")
        box.edit("cli/internal/config/config.go",
                 "	chainType := c.Type()", """	if err := c.validateDeployer(); err != nil {
		return err
	}
	chainType := c.Type()""")
        # The fixture predates the new key, so the config it holds is no longer
        # one this CLI accepts, and nothing can be learned by removing keys
        # from it. Inventing a value to fill the gap is the guess this whole
        # mechanism exists to avoid, so it stops and names the key.
        try:
            refgen.GENERATORS["config"]()
        except refgen.SourceError as e:
            assert e.kind == "stale_probe_fixture", f"raised {e.kind!r}: {e}"
            assert "deployer" in str(e), f"the refusal does not name the key: {e}"
            return
        raise AssertionError("expected a refusal naming the new required key")


@case("a key no fixture contains reads as optional, because they validate without it")
def _():
    with Sandbox() as box:
        # Absence is a proof here, not a gap: the probe fixtures load, and they
        # do not carry this key, so the program runs without it. That is what
        # keeps adding a config key from also being a fixture edit.
        box.edit("cli/internal/config/config.go",
                 'Deployer string %syaml:"deployer,omitempty"%s' % (TICK, TICK),
                 'Deployer string %syaml:"deployer,omitempty"%s\n\n'
                 '\t// Nickname is a label for this chain.\n'
                 '\tNickname string %syaml:"nickname,omitempty"%s'
                 % (TICK, TICK, TICK, TICK))
        assert _cell(box, "config:chains", "nickname") == "optional", \
            _cell(box, "config:chains", "nickname")


@case("a probe fixture that no longer loads refuses instead of answering")
def _():
    saved = list(refgen.PROBE_FIXTURES)
    broken = os.path.join(tempfile.mkdtemp(prefix="refgen-badfix-"), "probe.yml")
    with open(broken, "w") as fh:
        # a type the loader rejects, not an unknown key: an unknown key means
        # the fixture is merely ahead of the schema and is healed by dropping
        # it, which is a repair rather than a refusal
        fh.write("server:\n  listenAddr:\n    - 0.0.0.0:3000\n")
    refgen.PROBE_FIXTURES = [broken]
    try:
        refgen.GENERATORS["config"]()
    except refgen.SourceError as e:
        assert e.kind == "stale_probe_fixture", f"raised {e.kind!r}: {e}"
        return
    finally:
        refgen.PROBE_FIXTURES = saved
    raise AssertionError("expected a refusal about the fixture")


@case("a trailing comment inside the defaults builder changes nothing")
def _():
    with Sandbox() as box:
        # The twin of the case above, and the one that bit. Every other Go read
        # in refgen goes through a comment-blanked copy; the defaults scan read
        # raw source with `$`-anchored row patterns, so annotating a default --
        # the most innocuous edit there is -- deleted it. `db.url` went from
        # `ibc.db` to `optional`, which is wrong twice: the default vanished and
        # the key is in fact required.
        box.edit("cli/internal/config/config.go",
                 'URL:  "ibc.db",',
                 'URL:  "ibc.db", // relative to the IBC home directory')
        unchanged(box, "config")


@case("a comment on the line opening a nested default struct changes nothing")
def _():
    with Sandbox() as box:
        # The same scan matches `Field: Type{$` to learn which struct the rows
        # below belong to. A comment there orphaned every default in the block,
        # not just one row.
        box.edit("cli/internal/config/config.go",
                 "DB: DBConfig{",
                 "DB: DBConfig{ // sqlite unless the operator says otherwise")
        unchanged(box, "config")


@case("a named string constant added to the config package changes nothing")
def _():
    with Sandbox() as box:
        # `const X string = "..."` says its type is the builtin, not one of the
        # package's own string types. Reading it as an enum member rendered
        # every string key in every table as a list of unrelated values.
        box.edit("cli/internal/config/errors.go", "package config",
                 "package config", count=1)
        box.append("cli/internal/config/errors.go",
                   '\n\nconst DefaultListenAddr string = "127.0.0.1:9090"\n')
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
        for sub in ("cli/internal/config", "cli/cmd/ibc", "cli/internal/bootstrap",
                    "cli/internal/otel"):
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
        # the whole function, receiver and every use of it: renaming only the
        # signature left a body referring to a name that no longer existed, so
        # the tree never compiled and the case proved nothing
        before = open(os.path.join(
            box.dir, "cli/internal/config/config.go")).read()
        start = before.index("func (c Observability) Validate() error {")
        end = before.index("\n}\n", start) + 3
        body = before[start:end].replace("c.", "obs.").replace(
            "func (c Observability)", "func (obs Observability)")
        with open(os.path.join(box.dir, "cli/internal/config/config.go"), "w") as fh:
            fh.write(before[:start] + body + before[end:])
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
