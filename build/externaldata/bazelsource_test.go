//go:build !bazel

package externaldata

import (
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// requirePanic fails unless fn panics.
func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected a panic, got none")
		}
	}()
	fn()
}

func TestBazelVar(t *testing.T) {
	const content = "other = \"x\"\nconsensus_spec_version = \"v1.2.3\"\nversion = consensus_spec_version\n"

	t.Run("found", func(t *testing.T) {
		require.Equal(t, "v1.2.3", bazelVar(content, "f", "consensus_spec_version"))
	})

	t.Run("ignores non-string reference", func(t *testing.T) {
		// `version = consensus_spec_version` (no quotes) must not match.
		require.Equal(t, "x", bazelVar(content, "f", "other"))
	})

	t.Run("missing panics", func(t *testing.T) {
		requirePanic(t, func() { bazelVar(content, "f", "absent") })
	})
}

// archiveFixture exercises both attribute layouts: integrity + literal url, and
// a urls list + sha256, with a build_file_content block (containing its own
// name/parens) in between to prove the forward scan does not get confused.
const archiveFixture = `
http_archive(
    name = "alpha",
    build_file_content = """
filegroup(
    name = "data",
    visibility = ["//visibility:public"],
)
    """,
    integrity = "sha256-AAAA",
    strip_prefix = "alpha-123",
    url = "https://example.com/alpha.tar.gz",
)

http_archive(
    name = "beta",
    urls = ["https://example.com/beta.tar.gz"],
    sha256 = "deadbeef",
)
`

func TestArchiveHash(t *testing.T) {
	t.Run("integrity attribute", func(t *testing.T) {
		require.Equal(t, "sha256-AAAA", archiveHash(archiveFixture, "f", "alpha"))
	})

	t.Run("sha256 attribute", func(t *testing.T) {
		require.Equal(t, "deadbeef", archiveHash(archiveFixture, "f", "beta"))
	})

	t.Run("missing archive panics", func(t *testing.T) {
		requirePanic(t, func() { archiveHash(archiveFixture, "f", "gamma") })
	})

	t.Run("archive without the attribute panics", func(t *testing.T) {
		// The archive exists but carries no integrity/sha256 attribute.
		const noHash = "http_archive(\n    name = \"hashless\",\n    url = \"https://example.com/x.tar.gz\",\n)\n"
		requirePanic(t, func() { archiveHash(noHash, "f", "hashless") })
	})
}

func TestArchiveURL(t *testing.T) {
	t.Run("url attribute", func(t *testing.T) {
		require.Equal(t, "https://example.com/alpha.tar.gz", archiveURL(archiveFixture, "f", "alpha"))
	})

	t.Run("urls list attribute", func(t *testing.T) {
		require.Equal(t, "https://example.com/beta.tar.gz", archiveURL(archiveFixture, "f", "beta"))
	})
}

func TestSpecTestHash(t *testing.T) {
	// Parsed from the real WORKSPACE flavors map.
	for _, flavor := range []string{"general", "minimal", "mainnet"} {
		require.NotEqual(t, "", specTestHash(flavor), "flavor %q has no hash", flavor)
	}

	requirePanic(t, func() { specTestHash("nonexistent_flavor") })
}

func bazelBlock(t *testing.T, content, archive string) string {
	t.Helper()
	start := strings.Index(content, `name = "`+archive+`"`)
	require.NotEqual(t, -1, start, "archive %q not found", archive)

	rest := content[start:]
	end := len(rest)
	for _, marker := range []string{"http_archive(", "consensus_spec_tests("} {
		if i := strings.Index(rest, marker); i != -1 && i < end {
			end = i
		}
	}

	return rest[:end]
}

func TestManifestHashesMatchBazel(t *testing.T) {
	flavorOf := map[string]string{
		ConsensusSpecTestsGeneral: "general",
		ConsensusSpecTestsMinimal: "minimal",
		ConsensusSpecTestsMainnet: "mainnet",
	}

	for _, a := range allArchives() {
		if flavor, ok := flavorOf[a.name]; ok {
			// The flavor hash must be the value paired with its key in the
			// consensus_spec_tests flavors map.
			require.StringContains(t, `"`+flavor+`": "`+a.sha256+`"`, workspaceContent(),
				"flavor %q hash is not paired with its key in WORKSPACE", flavor)
			continue
		}

		content := workspaceContent()
		if a.name == Lighthouse || a.name == Web3signer {
			content = e2eDepsContent()
		}
		require.StringContains(t, a.sha256, bazelBlock(t, content, a.name),
			"hash for archive %q does not appear in its own Bazel block", a.name)
	}
}

func TestManifestSourcedFromBazel(t *testing.T) {
	for _, a := range allArchives() {
		require.Equal(t, true, strings.HasPrefix(a.url, "https://"), "archive %q has a non-https url %q", a.name, a.url)
		require.NotEqual(t, "", a.sha256, "archive %q has an empty hash", a.name)
	}

	specTestFlavorArchives := map[string]bool{
		ConsensusSpecTestsGeneral: true,
		ConsensusSpecTestsMinimal: true,
		ConsensusSpecTestsMainnet: true,
	}

	bazel := workspaceContent() + "\n" + e2eDepsContent()
	for _, a := range allArchives() {
		if specTestFlavorArchives[a.name] {
			continue
		}
		require.StringContains(t, `name = "`+a.name+`"`, bazel,
			"archive %q is not declared in the Bazel files", a.name)
	}

	byName := func(name string) archive {
		a, _, ok := archiveByName(name)
		require.Equal(t, true, ok)
		return a
	}
	require.StringContains(t, consensusSpecVersion(), byName(ConsensusSpec).url)
	require.StringContains(t, blsVersion(), byName(BLSSpecTests).url)
	require.StringContains(t, lighthouseVersion(), byName(Lighthouse).url)
}
