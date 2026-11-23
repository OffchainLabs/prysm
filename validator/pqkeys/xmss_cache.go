package pqkeys

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/io/file"
	leanxmss "github.com/nalepae/go-leanxmss"
)

const (
	XMSSCacheDirName = "xmss-cache"      // Directory of the tree caches on disk, in the data directory of the validator client.
	xmssCacheMagic   = "PRYXMSS\x00\x01" // Start of a tree cache on disk. Its last byte is the version of the format.

	xmssPubkeySize        = 32               // XMSS public key: the root of its top tree, then its public parameter.
	xmssPublicParamSize   = 16               // Public parameter of an XMSS key: the last bytes of its public key.
	treeCacheTopSize      = 4_096            // Top tree in a leanXMSS tree cache.
	treeCacheBottomSize   = 5_300            // One bottom tree in a leanXMSS tree cache.
	maxTreeCacheBottoms   = 8                // Bottom trees in the tree cache of a key on disk. go-leanxmss caches 8 per key.
	maxXMSSCacheRecords   = 10_000           // Records of the published tree cache: one per devnet-0 genesis key.
	xmssCacheHTTPTimeout  = 30 * time.Second // Download of one record of the published tree cache.
	xmssCacheFetchWorkers = 16               // Keys whose trees are read from the tree caches at once.
	xmssCacheTagSize      = sha256.Size      // Authentication tag of a tree cache on disk.

	// xmssRecordSize is the size of the record of a key in the published tree cache: the leanXMSS tree cache of its top
	// tree and of bottom trees 0 and 24,576.
	xmssRecordSize = treeCacheTopSize + 2*treeCacheBottomSize

	// maxXMSSCacheFileSize bounds the size of a tree cache on disk.
	maxXMSSCacheFileSize = len(xmssCacheMagic) + xmssPubkeySize + treeCacheTopSize + maxTreeCacheBottoms*treeCacheBottomSize + xmssCacheTagSize
)

// xmssCacheMACDomain separates the authentication key of the tree caches on disk from the other uses of the XMSS seed.
var xmssCacheMACDomain = []byte("Prysm leanXMSS disk cache v1")

type (
	// XMSSCacheSource gives the records of the published tree cache, the one Lighthouse reads with --xmss-cache-url.
	//
	// The published tree cache is the concatenation of the records of the devnet-0 genesis validators, in the order of
	// their validator indices, each of `xmssRecordSize` bytes: the leanXMSS tree cache of the top tree and of bottom
	// trees 0 and 24,576 of the key. It may hold fewer records than the genesis has validators.
	XMSSCacheSource interface {
		// Record returns the record of the key at `index`.
		Record(ctx context.Context, index uint32) ([]byte, error)
	}

	// xmssCacheSource reads the published tree cache from an HTTP(S) URL, with range requests, or from a file.
	xmssCacheSource struct {
		url    string // HTTP(S) URL. Empty if the tree cache is in a file.
		path   string // Path of the file. Empty if the tree cache is at a URL.
		client *http.Client
	}

	// XMSSPubkeysFunc returns the XMSS public keys of the first `count` validators of the validator registry, by
	// validator index. It returns fewer keys if the registry holds fewer validators.
	XMSSPubkeysFunc func(ctx context.Context, count int) ([][fieldparams.XMSSPubkeyLength]byte, error)

	// publishedKey is the record of a key in the published tree cache.
	publishedKey struct {
		index  uint32             // Validator index of the key, and index of its record.
		pubkey leanxmss.PublicKey // Public key the trees of the record are checked against.
	}

	// XMSSLoader gets the XMSS keys of validators from their tree caches: first the one on disk, then the published
	// one. It builds a key from its seed when neither has valid trees for it, which takes a few CPU seconds.
	//
	// The published tree cache holds no public key. The loader finds the record of a key, and the public key to check
	// it against, in the validator registry: it lists the XMSS public keys of the registry with `pubkeys` once, the
	// first time a key is not on disk.
	XMSSLoader struct {
		dir     string          // Directory of the tree caches on disk. Empty to use none.
		source  XMSSCacheSource // Published tree cache. Nil to use none.
		pubkeys XMSSPubkeysFunc // Lists the XMSS public keys of the validator registry. Nil to use no published tree cache.

		mu        sync.Mutex
		published map[[xmssPublicParamSize]byte]*publishedKey // Records of the published tree cache by public parameter. Nil until listed.
	}

	// xmssDiskCache is the tree cache of one key on disk.
	//
	// It holds the public key the trees are checked against, and is authenticated with a key derived from the XMSS
	// seed, so that the public key can be trusted: a file someone else wrote cannot pass for it.
	xmssDiskCache struct {
		path   string
		macKey []byte

		mu        sync.Mutex
		persisted [sha256.Size]byte // Hash of the trees last written or read. Zero if none.
	}
)

var _ XMSSCacheSource = (*xmssCacheSource)(nil)

// indexPublishedKeys indexes by public parameter the records of the published tree cache, from the XMSS public keys
// `pubkeys` of the validator registry. It leaves out the public parameters several keys share, and the invalid keys.
func indexPublishedKeys(pubkeys [][fieldparams.XMSSPubkeyLength]byte) map[[xmssPublicParamSize]byte]*publishedKey {
	keys := make(map[[xmssPublicParamSize]byte]*publishedKey, len(pubkeys))
	ambiguous := make(map[[xmssPublicParamSize]byte]bool)

	for i, raw := range pubkeys[:min(len(pubkeys), maxXMSSCacheRecords)] {
		param := [xmssPublicParamSize]byte(raw[xmssPubkeySize-xmssPublicParamSize:])
		if _, ok := keys[param]; ok {
			ambiguous[param] = true
			continue
		}

		pubkey, err := leanxmss.PublicKeyFromBytes(raw[:])
		if err != nil {
			continue
		}

		keys[param] = &publishedKey{index: uint32(i), pubkey: pubkey} // #nosec G115 -- Below maxXMSSCacheRecords.
	}

	for param := range ambiguous {
		delete(keys, param)
	}

	return keys
}

// NewXMSSCacheSource returns the source of the published tree cache at `location`: an HTTP(S) URL, a file URI or a path.
func NewXMSSCacheSource(location string) (XMSSCacheSource, error) {
	if location == "" {
		return nil, errors.New("empty XMSS cache location")
	}

	if !strings.Contains(location, "://") {
		return &xmssCacheSource{path: location}, nil
	}

	parsed, err := url.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("parse XMSS cache URL: %w", err)
	}

	switch parsed.Scheme {
	case "http", "https":
		return &xmssCacheSource{url: location, client: &http.Client{Timeout: xmssCacheHTTPTimeout}}, nil
	case "file":
		if parsed.Path == "" {
			return nil, fmt.Errorf("file URI %s without a path", location)
		}

		return &xmssCacheSource{path: parsed.Path}, nil
	default:
		return nil, fmt.Errorf("XMSS cache URL scheme %q is neither http, https nor file", parsed.Scheme)
	}
}

// Record reads the record of the key at `index`: with a seek in a file, or with a range request at a URL.
func (s *xmssCacheSource) Record(ctx context.Context, index uint32) ([]byte, error) {
	if index >= maxXMSSCacheRecords {
		return nil, fmt.Errorf("no record at index %d: the XMSS cache holds at most %d", index, maxXMSSCacheRecords)
	}

	start := int64(index) * xmssRecordSize // lint:ignore uintcast -- A uint32 always fits in an int64.
	if s.path != "" {
		return s.fileRecord(index, start)
	}

	return s.httpRecord(ctx, index, start)
}

// fileRecord reads the record at `index`, which starts at byte `start` of the file.
func (s *xmssCacheSource) fileRecord(index uint32, start int64) ([]byte, error) {
	file, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("open XMSS cache: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.WithError(err).Debug("Could not close the XMSS cache")
		}
	}()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat XMSS cache: %w", err)
	}

	if !info.Mode().IsRegular() {
		return nil, errors.New("the XMSS cache is not a regular file")
	}

	if err := checkXMSSCacheLength(info.Size(), start); err != nil {
		return nil, fmt.Errorf("record at index %d: %w", index, err)
	}

	record := make([]byte, xmssRecordSize)
	if _, err := file.ReadAt(record, start); err != nil {
		return nil, fmt.Errorf("read the record at index %d: %w", index, err)
	}

	return record, nil
}

// httpRecord downloads the record at `index`, which starts at byte `start`, with a range request.
func (s *xmssCacheSource) httpRecord(ctx context.Context, index uint32, start int64) ([]byte, error) {
	end := start + xmssRecordSize - 1
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, fmt.Errorf("new XMSS cache request: %w", err)
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	// Go transparently decompresses gzip responses, which would break the byte range. Ask for the raw bytes.
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get the record at index %d: %w", index, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Debug("Could not close the XMSS cache response")
		}
	}()

	if resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("get the record at index %d: status %s", index, resp.Status)
	}

	total, err := contentRangeTotal(resp.Header.Get("Content-Range"), start, end)
	if err != nil {
		return nil, fmt.Errorf("record at index %d: %w", index, err)
	}

	if err := checkXMSSCacheLength(total, start); err != nil {
		return nil, fmt.Errorf("record at index %d: %w", index, err)
	}

	record, err := io.ReadAll(io.LimitReader(resp.Body, xmssRecordSize+1))
	if err != nil {
		return nil, fmt.Errorf("read the record at index %d: %w", index, err)
	}

	if len(record) != xmssRecordSize {
		return nil, fmt.Errorf("record at index %d of %d bytes, want %d", index, len(record), xmssRecordSize)
	}

	return record, nil
}

// contentRangeTotal returns the total length a Content-Range header gives, if it is the range `start`-`end`.
func contentRangeTotal(header string, start, end int64) (int64, error) {
	want := fmt.Sprintf("bytes %d-%d/", start, end)
	if !strings.HasPrefix(header, want) {
		return 0, fmt.Errorf("Content-Range %q, want the range %d-%d", header, start, end)
	}

	total, err := strconv.ParseInt(header[len(want):], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("Content-Range %q: %w", header, err)
	}

	return total, nil
}

// checkXMSSCacheLength checks that a published tree cache of `length` bytes is whole records, and holds the one
// starting at byte `start`.
func checkXMSSCacheLength(length, start int64) error {
	if length%xmssRecordSize != 0 || length > maxXMSSCacheRecords*xmssRecordSize {
		return fmt.Errorf("XMSS cache of %d bytes, want a multiple of %d of at most %d records", length, xmssRecordSize, maxXMSSCacheRecords)
	}

	if start+xmssRecordSize > length {
		return fmt.Errorf("XMSS cache of %d records: missing record", length/xmssRecordSize)
	}

	return nil
}

// NewXMSSLoader returns a loader of XMSS keys that keeps their tree caches in `dir`, and reads the published tree cache
// from `source`, with the XMSS public keys of the validator registry `pubkeys` lists. Each may be empty or nil.
func NewXMSSLoader(dir string, source XMSSCacheSource, pubkeys XMSSPubkeysFunc) *XMSSLoader {
	return &XMSSLoader{dir: dir, source: source, pubkeys: pubkeys}
}

// usesPublished reports whether the loader reads the published tree cache.
func (l *XMSSLoader) usesPublished() bool {
	return l.source != nil && l.pubkeys != nil
}

// publishedKeys returns the records of the published tree cache by public parameter. It lists the XMSS public keys of
// the validator registry the first time, and again after a failure.
func (l *XMSSLoader) publishedKeys(ctx context.Context) (map[[xmssPublicParamSize]byte]*publishedKey, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.published != nil {
		return l.published, nil
	}

	pubkeys, err := l.pubkeys(ctx, maxXMSSCacheRecords)
	if err != nil {
		return nil, fmt.Errorf("list the XMSS public keys of the validator registry: %w", err)
	}

	l.published = indexPublishedKeys(pubkeys)

	return l.published, nil
}

// loadFromDisk returns the XMSS key of `seed`, the validator of BLS public key `pubkey`, with the trees of its tree
// cache on disk `disk`. It returns nil if there is none, or if it is invalid.
func (l *XMSSLoader) loadFromDisk(pubkey blsPubkey, seed [32]byte, disk *xmssDiskCache) *leanxmss.PrivateKey {
	if disk == nil {
		return nil
	}

	key, err := disk.read(seed)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.WithError(err).WithField("pubkey", fmt.Sprintf("%#x", pubkey)).Warn("Ignoring the unusable XMSS tree cache on disk")
		}

		return nil
	}

	return key
}

// loadPublished returns the XMSS key of `seed`, the validator of BLS public key `pubkey`, with the trees of its record
// in the published tree cache, found in `published`. It returns nil if there is none, or if it is invalid.
func (l *XMSSLoader) loadPublished(ctx context.Context, pubkey blsPubkey, seed [32]byte, published map[[xmssPublicParamSize]byte]*publishedKey) *leanxmss.PrivateKey {
	logger := log.WithField("pubkey", fmt.Sprintf("%#x", pubkey))

	key, ok := published[leanxmss.SeedPublicParam(seed)]
	if !ok {
		logger.Debug("The validator has no record in the published XMSS tree cache")
		return nil
	}

	record, err := l.source.Record(ctx, key.index)
	if err != nil {
		logger.WithError(err).Warn("Could not read the published XMSS tree cache, building the key")
		return nil
	}

	xmssKey, err := leanxmss.NewKeyFromCache(seed, key.pubkey, record)
	if err != nil {
		logger.WithError(err).Warn("The published XMSS tree cache does not match the key, building it")
		return nil
	}

	return xmssKey
}

// newDiskCache returns the tree cache on disk of the validator of BLS public key `pubkey`, whose XMSS seed is `seed`.
// It returns nil if the loader keeps no tree cache on disk.
func (l *XMSSLoader) newDiskCache(pubkey blsPubkey, seed [32]byte) *xmssDiskCache {
	if l.dir == "" {
		return nil
	}

	mac := hmac.New(sha256.New, seed[:])
	mac.Write(xmssCacheMACDomain)

	return &xmssDiskCache{
		path:   filepath.Join(l.dir, fmt.Sprintf("%#x.cache", pubkey)),
		macKey: mac.Sum(nil),
	}
}

// read reads the tree cache from disk, and returns the XMSS key of `seed` with its trees.
// It returns an error wrapping os.ErrNotExist if there is no tree cache on disk.
func (c *xmssDiskCache) read(seed [32]byte) (*leanxmss.PrivateKey, error) {
	info, err := os.Lstat(c.path)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}

	if !info.Mode().IsRegular() || info.Size() > int64(maxXMSSCacheFileSize) {
		return nil, fmt.Errorf("not a regular file of at most %d bytes", maxXMSSCacheFileSize)
	}

	content, err := os.ReadFile(c.path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	if len(content) < len(xmssCacheMagic)+xmssPubkeySize+xmssCacheTagSize || len(content) > maxXMSSCacheFileSize {
		return nil, fmt.Errorf("file of %d bytes", len(content))
	}

	body, tag := content[:len(content)-xmssCacheTagSize], content[len(content)-xmssCacheTagSize:]
	if !strings.HasPrefix(string(body), xmssCacheMagic) {
		return nil, errors.New("unknown format")
	}

	if !hmac.Equal(tag, c.tag(body)) {
		return nil, errors.New("authentication failed")
	}

	payload := body[len(xmssCacheMagic):]
	pubkey, err := leanxmss.PublicKeyFromBytes(payload[:xmssPubkeySize])
	if err != nil {
		return nil, fmt.Errorf("public key: %w", err)
	}

	trees := payload[xmssPubkeySize:]
	key, err := leanxmss.NewKeyFromCache(seed, pubkey, trees)
	if err != nil {
		return nil, fmt.Errorf("trees: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.persisted = sha256.Sum256(trees)

	return key, nil
}

// write writes the trees of `key` to disk, unless they are those it last wrote or read.
// It writes to a temporary file and renames it, so that a crash leaves the old file or the new one.
func (c *xmssDiskCache) write(key *leanxmss.PrivateKey) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	trees := key.TreeCache()
	hash := sha256.Sum256(trees)
	if hash == c.persisted {
		return nil
	}

	content := make([]byte, 0, len(xmssCacheMagic)+xmssPubkeySize+len(trees)+xmssCacheTagSize)
	content = append(content, xmssCacheMagic...)
	content = append(content, key.PublicKey().Bytes()...)
	content = append(content, trees...)
	content = append(content, c.tag(content)...)

	dir := filepath.Dir(c.path)
	if err := file.MkdirAll(dir); err != nil {
		return fmt.Errorf("create the directory: %w", err)
	}

	if err := writeFileAtomic(c.path, content); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	c.persisted = hash

	return nil
}

// tag returns the authentication tag of `body`.
func (c *xmssDiskCache) tag(body []byte) []byte {
	mac := hmac.New(sha256.New, c.macKey)
	mac.Write(body)

	return mac.Sum(nil)
}

// writeFileAtomic writes `content` to a temporary file next to `path`, syncs it, and renames it to `path`.
func writeFileAtomic(path string, content []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create a temporary file: %w", err)
	}

	tempPath := temp.Name()
	if err := func() error {
		defer func() {
			if err := temp.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
				log.WithError(err).Debug("Could not close the temporary XMSS cache file")
			}
		}()

		if _, err := temp.Write(content); err != nil {
			return fmt.Errorf("write the temporary file: %w", err)
		}

		if err := temp.Sync(); err != nil {
			return fmt.Errorf("sync the temporary file: %w", err)
		}

		return nil
	}(); err != nil {
		if removeErr := os.Remove(tempPath); removeErr != nil {
			log.WithError(removeErr).Debug("Could not remove the temporary XMSS cache file")
		}

		return err
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename the temporary file: %w", err)
	}

	return nil
}
