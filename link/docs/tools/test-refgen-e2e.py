#!/usr/bin/env python3
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
import os
import shutil
import subprocess
import sys
import tempfile
import traceback

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import refgen  # noqa: E402

ROOT = refgen.ROOT
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

    Only link/, proto/, and gen/ are copied, and link/bin is skipped, so a
    copy is a couple of megabytes rather than the hundred the built binary
    costs.
    """

    def __enter__(self):
        self.dir = tempfile.mkdtemp(prefix="refgen-e2e-")
        # gen/ comes along because link/go.mod replaces the generated ABI
        # module with a relative path into it
        for sub in ("link", "proto", "gen"):
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

    def edit(self, path, old, new, count=1):
        full = os.path.join(self.dir, path)
        text = open(full).read()
        if old not in text:
            raise AssertionError(f"{path} no longer contains {old!r}; the mutation is stale")
        open(full, "w").write(text.replace(old, new, count))

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
        box.edit("link/cmd/ibc/main.go",
                 '"attestation signature threshold"',
                 '"attestation signature threshold, at least 1"')
        red_then_healed(box, "cli", "attestation signature threshold, at least 1")


@case("a new flag on a command below the threshold reaches the page")
def _():
    with Sandbox() as box:
        box.edit("link/cmd/ibc/main.go",
                 "\tdpf := cmdDeploy.PersistentFlags()",
                 '\t_ = cmdDeployCore.Flags().Bool("fake", false, "a flag that was not there before")\n'
                 "\tdpf := cmdDeploy.PersistentFlags()")
        red_then_healed(box, "cli", "a flag that was not there before")


@case("a third flag earns a section the page does not have, and that raises")
def _():
    with Sandbox() as box:
        box.edit("link/cmd/ibc/main.go",
                 "\tdpf := cmdDeploy.PersistentFlags()",
                 '\t_ = cmdDeployCore.Flags().Bool("fake-one", false, "one")\n'
                 '\t_ = cmdDeployCore.Flags().Bool("fake-two", false, "two")\n'
                 '\t_ = cmdDeployCore.Flags().Bool("fake-three", false, "three")\n'
                 "\tdpf := cmdDeploy.PersistentFlags()")
        raises(box, "cli", "deploy core")


@case("a new command belongs to no task group, and that raises")
def _():
    with Sandbox() as box:
        box.append("link/cmd/ibc/migrate.go", '''
var cmdMigrateFake = &cobra.Command{
	Use:   "fake",
	Short: "A command nobody grouped",
	RunE:  func(_ *cobra.Command, _ []string) error { return nil },
}
''')
        box.edit("link/cmd/ibc/main.go",
                 "cmdMigrate.AddCommand(cmdMigrateUp, cmdMigrateDown, cmdMigrateStatus)",
                 "cmdMigrate.AddCommand(cmdMigrateUp, cmdMigrateDown, cmdMigrateStatus, cmdMigrateFake)")
        raises(box, "cli", "migrate fake")


# ------------------------------------------------------------------- the config

@case("a new config key with no description anywhere raises")
def _():
    with Sandbox() as box:
        box.edit("link/internal/config/config.go",
                 'ListenAddress string `yaml:"listenAddr"`',
                 'ListenAddress string `yaml:"listenAddr"`\n\tFake string `yaml:"fake"`')
        raises(box, "config", "Fake")


@case("a key that gains a doc comment upstream raises, so the fallback cannot shadow it")
def _():
    with Sandbox() as box:
        box.edit("link/internal/config/config.go",
                 '\tListenAddress string `yaml:"listenAddr"`',
                 '\t// ListenAddress is the address the server binds.\n'
                 '\tListenAddress string `yaml:"listenAddr"`')
        raises(box, "config", "FALLBACK_DOCS")


@case("a renamed default constant raises rather than leaving a stale number")
def _():
    with Sandbox() as box:
        box.edit("link/internal/relay/pipeline/opts.go",
                 "DefaultBatchSize", "DefaultBatchSizeRenamed", count=99)
        raises(box, "config", "DefaultBatchSize")


@case("a changed default value reaches the page")
def _():
    with Sandbox() as box:
        box.edit("link/internal/relay/dispatch/dispatcher.go",
                 "const DefaultPollInterval = 5 * time.Second",
                 "const DefaultPollInterval = 9 * time.Second")
        red_then_healed(box, "config", "`9s`")


# ---------------------------------------------------------------------- the API

@case("a changed proto comment reaches the page")
def _():
    with Sandbox() as box:
        box.edit("proto/link/relayer.proto",
                 "// The relayer is still processing the packet.",
                 "// The relayer has not finished with the packet yet.")
        red_then_healed(box, "api", "The relayer has not finished with the packet yet.")


@case("a new proto message has no marker on the page, and that raises")
def _():
    with Sandbox() as box:
        box.append("proto/link/relayer.proto", '''
message FakeThing {
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


# ------------------------------------------------------- removals, not just adds

@case("a removed flag disappears from the page")
def _():
    with Sandbox() as box:
        box.edit("link/cmd/ibc/main.go",
                 '\tcmdDeployClient.Flags().Uint8Var(&flagDeployThreshold, "threshold", 1, '
                 '"attestation signature threshold")\n', "")
        page = box.page("cli")
        assert refgen.run("cli", page, check=True) == 1
        assert refgen.run("cli", page, check=False) == 0
        assert "attestation signature threshold" not in open(page).read()


@case("a removed command raises: the page still lists it")
def _():
    with Sandbox() as box:
        box.edit("link/cmd/ibc/main.go",
                 "cmdKeys.AddCommand(cmdKeysNew, cmdKeysShow, cmdKeysImport, cmdKeysList)",
                 "cmdKeys.AddCommand(cmdKeysNew, cmdKeysShow, cmdKeysImport)")
        raises(box, "cli", "keys list")


@case("a removed config key disappears from its table")
def _():
    with Sandbox() as box:
        box.edit("link/internal/config/config.go",
                 '\tFinalityOffset uint `yaml:"finalityOffset"`', "")
        page = box.page("config")
        assert refgen.run("config", page, check=True) == 1
        assert refgen.run("config", page, check=False) == 0
        assert "`finalityOffset`" not in open(page).read()


@case("a removed proto field disappears from its table")
def _():
    with Sandbox() as box:
        # TransactionInfo's own chain_id, not either message's source_chain_id
        box.edit("proto/link/relayer.proto",
                 "message TransactionInfo {\n  string tx_hash = 1;\n  string chain_id = 2;\n}",
                 "message TransactionInfo {\n  string tx_hash = 1;\n}")
        page = box.page("api")
        assert refgen.run("api", page, check=True) == 1
        assert refgen.run("api", page, check=False) == 0
        # `chain_id` also names a field of the attestor service's InfoResponse,
        # so the assertion has to look inside TransactionInfo's own region
        body = open(page).read()
        region = body.split("GEN:api:msg:TransactionInfo START")[1].split("END")[0]
        assert "`chain_id`" not in region, region


@case("a removed proto message raises, and the message says what to do")
def _():
    with Sandbox() as box:
        box.edit("proto/link/attestor.proto",
                 "message InfoRequest { string attestor = 1; }", "")
        page = box.page("api", own=True)
        try:
            refgen.run("api", page, check=True)
        except refgen.MarkerError as e:
            assert "api:msg:InfoRequest" in str(e), e
            assert "delete the page section" in str(e), f"the error must name the fix: {e}"
            return
        raise AssertionError("expected a MarkerError naming the removed message")


for name in PASS:
    print(f"  ok    {name}")
for name, e, tb in FAIL:
    print(f"  FAIL  {name}: {e}")
print(f"\n{len(PASS)} passed, {len(FAIL)} failed")
if FAIL:
    print()
    print(FAIL[0][2])
sys.exit(1 if FAIL else 0)
