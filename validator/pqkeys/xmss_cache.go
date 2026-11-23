package pqkeys

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	leanxmss "github.com/nalepae/go-leanxmss"
	"github.com/sirupsen/logrus"
)

const (
	// xmssCacheRecordSize is the size of the record of a validator in the leanXMSS tree caches: the 256 nodes of the
	// top tree, then two bottom trees, each with its index, the signature of its root and its 256 nodes.
	xmssCacheRecordSize = 4096 + 2*5300

	// xmssPublicParamLength is the length of the public parameter of an XMSS key, the end of its public key.
	xmssPublicParamLength = 16

	// xmssCacheHTTPTimeout bounds the download of a record of the leanXMSS tree caches.
	xmssCacheHTTPTimeout = 30 * time.Second
)

// errNoXMSSCacheRecord is returned when the tree caches end before the record of a validator.
var errNoXMSSCacheRecord = errors.New("no record for this validator in the XMSS cache")

type (
	// XMSSPubkeysFetcher returns the XMSS public keys of the validators of the state, by validator index.
	XMSSPubkeysFetcher func(ctx context.Context) (map[primitives.ValidatorIndex]xmssPubkey, error)

	// XMSSCache loads XMSS keys from precomputed leanXMSS tree caches, at an HTTP(S) URL or in a file.
	//
	// The tree caches are the concatenation of the records of the validators, by validator index, each of
	// `xmssCacheRecordSize` bytes: the bytes leanVM's `cache_to_bytes` returns for the XMSS key of the validator with
	// two bottom trees built. A key loaded from its record needs none of the trees of the record built.
	//
	// A record does not say which key it is of. The validator of a key is the one of the state whose XMSS public key
	// ends with the public parameter of the key, which is cheap to derive from its seed.
	XMSSCache struct {
		url     string // HTTP(S) URL. Empty if the tree caches are in a file.
		path    string // Path of the file. Empty if the tree caches are at a URL.
		client  *http.Client
		pubkeys XMSSPubkeysFetcher
	}

	// xmssValidator is a validator of the state, by the public parameter of its XMSS public key.
	xmssValidator struct {
		index  primitives.ValidatorIndex
		pubkey xmssPubkey
		shared bool // Several validators have this public parameter: it identifies none.
	}
)

// NewXMSSCache returns the leanXMSS tree caches at `location`: an HTTP(S) URL, a file URI, or a path.
// `pubkeys` gives the validators of the state.
func NewXMSSCache(location string, pubkeys XMSSPubkeysFetcher) (*XMSSCache, error) {
	if pubkeys == nil {
		return nil, errors.New("nil XMSS public keys fetcher")
	}

	httpURL, path, err := parseLocation("XMSS cache", location)
	if err != nil {
		return nil, err
	}

	return &XMSSCache{url: httpURL, path: path, client: &http.Client{Timeout: xmssCacheHTTPTimeout}, pubkeys: pubkeys}, nil
}

// keys returns the keys of the validators whose BLS secret keys are `secretKeys` that it can load from the tree caches,
// indexed by BLS public key. It never fails: a key it cannot load is missing from the result, and logged.
func (c *XMSSCache) keys(ctx context.Context, secretKeys []bls.SecretKey) map[blsPubkey]*Keys {
	keysByPubkey := make(map[blsPubkey]*Keys, len(secretKeys))
	start := time.Now()

	validators, err := c.validators(ctx)
	if err != nil {
		log.WithError(err).Warn("Could not list the validators of the state: building the XMSS keys from their seeds")
		return keysByPubkey
	}

	for _, secretKey := range secretKeys {
		if ctx.Err() != nil {
			break
		}

		if secretKey == nil {
			continue
		}

		blsKey := blsPubkey(secretKey.PublicKey().Marshal())
		seed := xmssSeed(secretKey.Marshal())
		logger := log.WithField("pubkey", fmt.Sprintf("%#x", blsKey))

		validator, ok := validators[leanxmss.PublicParamFromSeed(seed)]
		if !ok || validator.shared {
			logger.Debug("No validator of the state has this XMSS key: building it from its seed")
			continue
		}

		logger = logger.WithField("validatorIndex", validator.index)

		record, err := c.record(ctx, validator.index)
		if errors.Is(err, errNoXMSSCacheRecord) {
			logger.Warn("The leanXMSS tree caches have no record for this validator: building its XMSS key from its seed")
			continue
		}

		if err != nil {
			logger.WithError(err).Warn("Could not read the leanXMSS tree caches: building the remaining XMSS keys from their seeds")
			break
		}

		key, err := loadXMSSKey(seed, validator.pubkey, record)
		if err != nil {
			logger.WithError(err).Warn("The leanXMSS tree cache is not of this XMSS key: building it from its seed")
			continue
		}

		keysByPubkey[blsKey] = &Keys{XMSS: key}
	}

	log.WithFields(logrus.Fields{
		"loaded":  len(keysByPubkey),
		"total":   len(secretKeys),
		"elapsed": time.Since(start),
	}).Info("Loaded XMSS keys from the leanXMSS tree caches")

	return keysByPubkey
}

// loadXMSSKey returns the XMSS key of `seed`, whose public key is `pubkey`, from its record in the tree caches.
func loadXMSSKey(seed [32]byte, pubkey xmssPubkey, record []byte) (*leanxmss.PrivateKey, error) {
	if len(record) != xmssCacheRecordSize {
		return nil, fmt.Errorf("record of %d bytes instead of %d", len(record), xmssCacheRecordSize)
	}

	publicKey, err := leanxmss.PublicKeyFromBytes(pubkey[:])
	if err != nil {
		return nil, fmt.Errorf("decode XMSS public key: %w", err)
	}

	return leanxmss.NewKeyFromCache(seed, publicKey, record)
}

// validators returns the validators of the state by the public parameter of their XMSS public key.
func (c *XMSSCache) validators(ctx context.Context) (map[[xmssPublicParamLength]byte]xmssValidator, error) {
	pubkeys, err := c.pubkeys(ctx)
	if err != nil {
		return nil, err
	}

	validators := make(map[[xmssPublicParamLength]byte]xmssValidator, len(pubkeys))
	for index, pubkey := range pubkeys {
		publicParam := [xmssPublicParamLength]byte(pubkey[len(pubkey)-xmssPublicParamLength:])
		_, shared := validators[publicParam]
		validators[publicParam] = xmssValidator{index: index, pubkey: pubkey, shared: shared}
	}

	return validators, nil
}

// record returns the record of the validator of index `index`.
func (c *XMSSCache) record(ctx context.Context, index primitives.ValidatorIndex) ([]byte, error) {
	if uint64(index) > (1<<62)/xmssCacheRecordSize {
		return nil, fmt.Errorf("validator index %d too large", index)
	}

	start := int64(index) * xmssCacheRecordSize // lint:ignore uintcast -- Checked above.
	if c.path != "" {
		return c.fileRecord(start)
	}

	return c.httpRecord(ctx, start)
}

// fileRecord returns the record at offset `start` of the file.
func (c *XMSSCache) fileRecord(start int64) ([]byte, error) {
	file, err := os.Open(c.path)
	if err != nil {
		return nil, fmt.Errorf("open XMSS cache: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.WithError(err).Debug("Could not close the XMSS cache")
		}
	}()

	record := make([]byte, xmssCacheRecordSize)
	_, err = file.ReadAt(record, start)
	if errors.Is(err, io.EOF) {
		return nil, errNoXMSSCacheRecord
	}

	if err != nil {
		return nil, fmt.Errorf("read XMSS cache at offset %d: %w", start, err)
	}

	return record, nil
}

// httpRecord returns the record at offset `start` of the URL, with a range request.
func (c *XMSSCache) httpRecord(ctx context.Context, start int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("new XMSS cache request: %w", err)
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, start+xmssCacheRecordSize-1))
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get XMSS cache: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Debug("Could not close the XMSS cache response")
		}
	}()

	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return nil, errNoXMSSCacheRecord
	}

	if resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("get XMSS cache: status %s instead of a partial content", resp.Status)
	}

	record, err := io.ReadAll(io.LimitReader(resp.Body, xmssCacheRecordSize+1))
	if err != nil {
		return nil, fmt.Errorf("read XMSS cache: %w", err)
	}

	if len(record) != xmssCacheRecordSize {
		return nil, fmt.Errorf("XMSS cache record at offset %d of %d bytes instead of %d", start, len(record), xmssCacheRecordSize)
	}

	return record, nil
}
