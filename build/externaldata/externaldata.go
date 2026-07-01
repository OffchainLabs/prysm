package externaldata

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"

	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/sirupsen/logrus"
)

// Archive names. These are the logical identifiers passed to Fetch and used as
// marker file names; callers should reference these constants rather than string
// literals.
const (
	ConsensusSpecTestsGeneral = "consensus_spec_tests_general"
	ConsensusSpecTestsMinimal = "consensus_spec_tests_minimal"
	ConsensusSpecTestsMainnet = "consensus_spec_tests_mainnet"
	ConsensusSpec             = "consensus_spec"
	Mainnet                   = "mainnet"
	HoleskyTestnet            = "holesky_testnet"
	SepoliaTestnet            = "sepolia_testnet"
	HoodiTestnet              = "hoodi_testnet"
	BLSSpecTests              = "bls_spec_tests"
	EIP3076SpecTests          = "eip3076_spec_tests"
	EIP4881SpecTests          = "eip4881_spec_tests"
	Lighthouse                = "lighthouse"
	Web3signer                = "web3signer"
)

type archive struct {
	name    string // logical name + marker file name
	url     string
	sha256  string // hex, or "sha256-"+base64 (both accepted, matching WORKSPACE)
	dest    string // sub-dir under the test-data root; "." extracts into the root
	strip   int    // leading path components to strip from each tar entry
	include string // optional shell glob; only matching entries are extracted
}

var (
	manifest    = sync.OnceValue(buildManifest)
	e2eArchives = sync.OnceValue(buildE2EArchives)
)

func buildManifest() []archive {
	specRel := "https://github.com/ethereum/consensus-specs/releases/download/" + consensusSpecVersion()

	return []archive{
		{ConsensusSpecTestsGeneral, specRel + "/general.tar.gz", specTestHash("general"), ".", 0, ""},
		{ConsensusSpecTestsMinimal, specRel + "/minimal.tar.gz", specTestHash("minimal"), ".", 0, ""},
		{ConsensusSpecTestsMainnet, specRel + "/mainnet.tar.gz", specTestHash("mainnet"), ".", 0, ""},
		{ConsensusSpec, "https://github.com/ethereum/consensus-specs/archive/refs/tags/" + consensusSpecVersion() + ".tar.gz", archiveHash(workspaceContent(), workspaceFile, ConsensusSpec), "external/consensus_spec", 1, ""},
		{Mainnet, archiveURL(workspaceContent(), workspaceFile, Mainnet), archiveHash(workspaceContent(), workspaceFile, Mainnet), "external/mainnet", 1, ""},
		{HoleskyTestnet, archiveURL(workspaceContent(), workspaceFile, HoleskyTestnet), archiveHash(workspaceContent(), workspaceFile, HoleskyTestnet), "external/holesky_testnet", 1, ""},
		{SepoliaTestnet, archiveURL(workspaceContent(), workspaceFile, SepoliaTestnet), archiveHash(workspaceContent(), workspaceFile, SepoliaTestnet), "external/sepolia_testnet", 1, ""},
		{HoodiTestnet, archiveURL(workspaceContent(), workspaceFile, HoodiTestnet), archiveHash(workspaceContent(), workspaceFile, HoodiTestnet), "external/hoodi_testnet", 1, ""},
		{BLSSpecTests, "https://github.com/ethereum/bls12-381-tests/releases/download/" + blsVersion() + "/bls_tests_yaml.tar.gz", archiveHash(workspaceContent(), workspaceFile, BLSSpecTests), ".", 0, ""},
		{EIP3076SpecTests, archiveURL(workspaceContent(), workspaceFile, EIP3076SpecTests), archiveHash(workspaceContent(), workspaceFile, EIP3076SpecTests), "external/eip3076_spec_tests", 1, ""},
		{EIP4881SpecTests, archiveURL(workspaceContent(), workspaceFile, EIP4881SpecTests), archiveHash(workspaceContent(), workspaceFile, EIP4881SpecTests), "external/eip4881_spec_tests", 1, "*/assets/eip-4881/*"},
	}
}

func buildE2EArchives() []archive {
	return []archive{
		{Lighthouse, "https://github.com/sigp/lighthouse/releases/download/" + lighthouseVersion() + "/lighthouse-" + lighthouseVersion() + "-x86_64-unknown-linux-gnu.tar.gz", archiveHash(e2eDepsContent(), e2eDepsFile, Lighthouse), "external/lighthouse", 0, ""},
		{Web3signer, archiveURL(e2eDepsContent(), e2eDepsFile, Web3signer), archiveHash(e2eDepsContent(), e2eDepsFile, Web3signer), "external/web3signer", 1, ""},
	}
}

func archiveByName(name string) (archive, int, bool) {
	for i, a := range allArchives() {
		if a.name == name {
			return a, i, true
		}
	}

	return archive{}, 0, false
}

func allArchives() []archive {
	return slices.Concat(manifest(), e2eArchives())
}

// DestDir returns the directory the named archive extracts into.
func DestDir(name string) (string, bool) {
	a, _, ok := archiveByName(name)
	if !ok {
		return "", false
	}

	root := Root()
	if root == "" {
		return "", false
	}

	if a.dest == "." {
		return root, true
	}

	return filepath.Join(root, a.dest), true
}

// Names returns every archive name in the manifest.
func Names() []string {
	m := manifest()
	out := make([]string, len(m))
	for i, a := range m {
		out[i] = a.name
	}

	return out
}

var onces sync.Map // name -> *sync.Once, so each archive is fetched at most once per process.

// Fetch ensures the named archive is present in the test-data cache, downloading
// and extracting it if needed.
func Fetch(name string) error {
	if _, err := fetchSized(name); err != nil {
		return fmt.Errorf("fetch sized: %w", err)
	}

	return nil
}

// fetchSized is Fetch plus the number of bytes downloaded (0 if the archive was
// already cached), used by FetchAll to report totals.
func fetchSized(name string) (int64, error) {
	o, _ := onces.LoadOrStore(name, &sync.Once{})
	var (
		size int64
		err  error
	)

	o.(*sync.Once).Do(func() { size, err = fetch(name) })
	return size, err
}

// FetchAll downloads every archive in the manifest (used by `make testdata`) and
// logs the total bytes downloaded and elapsed time.
func FetchAll() error {
	start := time.Now()
	var total int64
	for _, a := range manifest() {
		n, err := fetchSized(a.name)
		if err != nil {
			return fmt.Errorf("fetch sized: %w", err)
		}

		total += n
	}

	logrus.WithFields(logrus.Fields{
		"size":     humanize.Bytes(uint64(total)),
		"duration": time.Since(start).Round(time.Millisecond),
	}).Info("Fetched all external test data")

	return nil
}

// fetch downloads, verifies and extracts a single archive, returning the number
// of bytes downloaded (0 if it was already cached).
func fetch(name string) (int64, error) {
	a, idx, ok := archiveByName(name)
	if !ok {
		return 0, fmt.Errorf("externaldata: unknown archive %q", name)
	}

	log := logrus.WithFields(logrus.Fields{
		"archive": a.name,
		"count":   fmt.Sprintf("%d/%d", idx+1, len(allArchives())),
	})

	root := Root()
	if root == "" {
		return 0, fmt.Errorf("externaldata: could not locate test-data root")
	}

	markers := filepath.Join(root, ".markers")
	if err := os.MkdirAll(markers, 0o750); err != nil {
		return 0, fmt.Errorf("mkdirall: %w", err)
	}

	wantHex, err := normalizeSha(a.sha256)
	if err != nil {
		return 0, fmt.Errorf("normalize sha: %w", err)
	}

	marker := filepath.Join(markers, a.name)

	// Cross-process guard: only one process downloads a given archive at a time.
	lock, err := acquireLock(filepath.Join(root, ".lock."+a.name))
	if err != nil {
		return 0, fmt.Errorf("acquire lock: %w", err)
	}
	defer lock.release()

	start := time.Now()

	// Re-check after acquiring the lock — another process may have just fetched it.
	if sha, size := readMarker(marker); sha == wantHex {
		log.WithFields(logrus.Fields{
			"cached":   true,
			"size":     humanize.Bytes(uint64(size)),
			"duration": time.Since(start).Round(time.Millisecond),
		}).Info("Fetched external test data")
		return size, nil
	}

	data, err := httpDownload(a.url)
	if err != nil {
		return 0, fmt.Errorf("externaldata: download %s: %w", a.name, err)
	}

	size := int64(len(data))
	log.WithFields(logrus.Fields{
		"cached":   false,
		"size":     humanize.Bytes(uint64(size)),
		"duration": time.Since(start).Round(time.Millisecond),
	}).Info("Fetched external test data")

	gotHex := fmt.Sprintf("%x", sha256.Sum256(data))
	if gotHex != wantHex {
		return 0, fmt.Errorf("externaldata: %s sha256 mismatch: got %s want %s", a.name, gotHex, wantHex)
	}

	target := root
	if a.dest != "." {
		target = filepath.Join(root, a.dest)
		// dest "." extracts into the shared root; never wipe it.
		if err := os.RemoveAll(target); err != nil {
			return 0, fmt.Errorf("remove all: %w", err)
		}
	}

	if err := os.MkdirAll(target, 0o750); err != nil {
		return 0, fmt.Errorf("mkdir all: %w", err)
	}
	if err := extract(data, target, a.strip, a.include); err != nil {
		return 0, fmt.Errorf("externaldata: extract %s: %w", a.name, err)
	}

	// Record the sha and the archive size so a later cache hit can report the
	// size without re-downloading.
	markerBody := wantHex + "\n" + strconv.FormatInt(size, 10) + "\n"
	if err := os.WriteFile(marker, []byte(markerBody), 0o600); err != nil {
		return 0, fmt.Errorf("write marker: %w", err)
	}

	return size, nil
}

// readMarker reads a marker file written by fetch. The marker holds the verified
// sha256 (hex) on the first line and the downloaded archive size in bytes on the
// second; size is 0 for legacy markers written before sizes were recorded (and
// for any unreadable/absent marker, in which case hex is "" too).
func readMarker(marker string) (sha string, size int64) {
	b, err := os.ReadFile(marker) // #nosec G304 -- marker path is filepath.Join of our cache dir and a fixed manifest archive name
	if err != nil {
		return "", 0
	}

	lines := strings.SplitN(strings.TrimSpace(string(b)), "\n", 2)
	sha = strings.TrimSpace(lines[0])
	if len(lines) == 2 {
		size, _ = strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64)
	}

	return sha, size
}

func normalizeSha(s string) (string, error) {
	if rest, ok := strings.CutPrefix(s, "sha256-"); ok {
		b, err := base64.StdEncoding.DecodeString(rest)
		if err != nil {
			return "", fmt.Errorf("externaldata: bad sha256-base64 %q: %w", s, err)
		}

		return hex.EncodeToString(b), nil
	}

	return s, nil
}

// httpDownload fetches an archive's bytes. It is a package variable so tests can
// substitute archive contents for the network.
var httpDownload = download

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Minute}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("GET %s: %s", url, resp.Status)
			continue
		}

		b, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		return b, nil
	}

	return nil, lastErr
}

// extract unpacks a gzip'd tar into target, stripping `strip` leading path
// components and, if include != "", only entries whose (pre-strip) name matches
// that glob.
func extract(targz []byte, target string, strip int, include string) error {
	var includeRe *regexp.Regexp
	if include != "" {
		includeRe = globToRegexp(include)
	}

	gz, err := gzip.NewReader(bytes.NewReader(targz))
	if err != nil {
		return fmt.Errorf("gzip new reader: %w", err)
	}

	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}

		if includeRe != nil && !includeRe.MatchString(hdr.Name) {
			continue
		}

		rel := stripComponents(hdr.Name, strip)
		if rel == "" {
			continue
		}

		cleanTarget := filepath.Clean(target)
		dst := filepath.Join(cleanTarget, filepath.FromSlash(rel))
		if dst == cleanTarget {
			continue
		}

		if !strings.HasPrefix(dst, cleanTarget+string(os.PathSeparator)) {
			return fmt.Errorf("externaldata: unsafe path %q in archive", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o750); err != nil {
				return fmt.Errorf("mkdir all: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
				return fmt.Errorf("mkdir all: %w", err)
			}

			// #nosec G304 -- dst is verified above to stay within the target dir (HasPrefix check)
			f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777|0o600)
			if err != nil {
				return fmt.Errorf("open file: %w", err)
			}

			// #nosec G110 -- archive bytes are sha256-verified against a pinned hash before extraction
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return fmt.Errorf("copy file: %w", err)
			}

			if err := f.Close(); err != nil {
				return fmt.Errorf("close file: %w", err)
			}

		case tar.TypeSymlink:
			logrus.WithFields(logrus.Fields{
				"name":     hdr.Name,
				"linkname": hdr.Linkname,
			}).Debug("Skipping symlink entry in archive")
			continue
		}
	}

	return nil
}

func stripComponents(name string, n int) string {
	parts := strings.Split(strings.TrimPrefix(name, "./"), "/")
	if len(parts) <= n {
		return ""
	}
	return strings.Join(parts[n:], "/")
}

// globToRegexp converts a shell glob into an anchored regexp.
func globToRegexp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}

	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
