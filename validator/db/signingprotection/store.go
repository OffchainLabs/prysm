// Package signingprotection implements the signing protection database of a leanXMSS validator client.
//
// It refuses to sign two kinds of messages:
//   - A message at an already used leaf. leanXMSS is stateful: signing two different messages
//     at the same leaf exposes the leaf and lets anyone forge signatures at it.
//     This also prevents double proposals and double votes, which are signed at the same leaf.
//   - A surround vote. An attestation that surrounds, or is surrounded by, a previously signed
//     attestation is slashable. The two attestations have different target epochs, so they are
//     signed at different leaves, and leaf protection alone does not prevent it.
//
// Every leaf class is used in increasing order, so the database stores, for each XMSS public key,
// one watermark per leaf class, in the spirit of the EIP-3076 minimal slashing protection database.
//
// The database is made of one YAML file per XMSS public key, holding the genesis validators root of its chain.
// Each file is written atomically and durably, and a file that cannot be parsed or that belongs to another chain
// is an error, never an empty history.
package signingprotection

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"gopkg.in/yaml.v3"
)

const (
	DatabaseDirName          = "validator-client-data" // Name of the validator client data directory.
	PubkeyLength             = 32                      // Length of an XMSS public key, in bytes.
	ReservedLeafCount        = uint64(1 << 15)         // Leaves for messages without a leaf of their own, such as voluntary exits and preregistrations (`XMSS_RESERVED_LEAVES`).
	signingProtectionDirName = "signing-protection"
	fileExtension            = ".yaml"
)

// ErrSigningRefused is returned when signing a message would reuse a leaf or would be slashable.
var ErrSigningRefused = errors.New("signing refused by local signing protection")

type (
	// ValidatorSigningProtection holds the watermarks of an XMSS public key.
	ValidatorSigningProtection struct {
		// LastSignedReservedLeaf is the last allocated reserved leaf. Leaf 0 is always considered
		// as used, since it is the leaf of the key's preregistration.
		LastSignedReservedLeaf uint64 `yaml:"lastSignedReservedLeaf"`

		// LastSignedAttestationSourceEpoch and LastSignedAttestationTargetEpoch protect against
		// surround votes (EIP-3076 minimal), and the target epoch protects the attestation leaves.
		LastSignedAttestationSourceEpoch *uint64 `yaml:"lastSignedAttestationSourceEpoch,omitempty"`
		LastSignedAttestationTargetEpoch *uint64 `yaml:"lastSignedAttestationTargetEpoch,omitempty"`

		// One watermark per slot-based leaf offset.
		LastSignedBlockSlot                    *uint64 `yaml:"lastSignedBlockSlot,omitempty"`
		LastSignedAggregateAndProofSlot        *uint64 `yaml:"lastSignedAggregateAndProofSlot,omitempty"`
		LastSignedPayloadAttestationSlot       *uint64 `yaml:"lastSignedPayloadAttestationSlot,omitempty"`
		LastSignedProposerPreferencesSlot      *uint64 `yaml:"lastSignedProposerPreferencesSlot,omitempty"`
		LastSignedExecutionPayloadEnvelopeSlot *uint64 `yaml:"lastSignedExecutionPayloadEnvelopeSlot,omitempty"`
	}

	// signingProtectionFile is the content of a signing protection file.
	signingProtectionFile struct {
		GenesisValidatorsRoot      string `yaml:"genesisValidatorsRoot"`
		ValidatorSigningProtection `yaml:",inline"`
	}

	// Store is the signing protection database.
	Store struct {
		dirPath               string
		genesisValidatorsRoot [32]byte

		locks sync.Map // [PubkeyLength]byte -> *sync.Mutex
	}
)

// NewStore creates the signing protection database of the chain of `genesisValidatorsRoot` under `databaseParentPath`.
func NewStore(databaseParentPath string, genesisValidatorsRoot [32]byte) (*Store, error) {
	dirPath := filepath.Join(databaseParentPath, DatabaseDirName, signingProtectionDirName)
	if err := file.MkdirAll(dirPath); err != nil {
		return nil, fmt.Errorf("create directory %s: %w", dirPath, err)
	}

	store := &Store{
		dirPath:               dirPath,
		genesisValidatorsRoot: genesisValidatorsRoot,
	}

	return store, nil
}

// GenesisValidatorsRoot returns the genesis validators root of the chain of the store.
func (s *Store) GenesisValidatorsRoot() [32]byte {
	return s.genesisValidatorsRoot
}

// DirPath returns the directory holding the signing protection files.
func (s *Store) DirPath() string {
	return s.dirPath
}

// PublicKeys returns the XMSS public keys having a signing protection file.
func (s *Store) PublicKeys() ([][PubkeyLength]byte, error) {
	entries, err := os.ReadDir(s.dirPath)
	if err != nil {
		return nil, fmt.Errorf("read directory %s: %w", s.dirPath, err)
	}

	pubkeys := make([][PubkeyLength]byte, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		pubkey, ok := pubkeyFromFileName(entry.Name())
		if !ok {
			continue
		}

		pubkeys = append(pubkeys, pubkey)
	}

	return pubkeys, nil
}

// SigningProtection returns the watermarks of `pubkey`.
// A public key without a file has the zero watermarks: only reserved leaf 0 is used.
func (s *Store) SigningProtection(pubkey [PubkeyLength]byte) (*ValidatorSigningProtection, error) {
	mu := s.lock(pubkey)
	defer mu.Unlock()

	protection, err := s.load(pubkey)
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}

	return protection, nil
}

// update loads the watermarks of `pubkey`, applies `fn` and saves the result, all under the lock of `pubkey`.
// If `fn` returns an error, nothing is saved.
func (s *Store) update(pubkey [PubkeyLength]byte, fn func(*ValidatorSigningProtection) error) error {
	mu := s.lock(pubkey)
	defer mu.Unlock()

	protection, err := s.load(pubkey)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}

	if err := fn(protection); err != nil {
		return fmt.Errorf("apply update: %w", err)
	}

	if err := s.save(pubkey, protection); err != nil {
		return fmt.Errorf("save: %w", err)
	}

	return nil
}

// lock locks and returns the mutex of `pubkey`.
func (s *Store) lock(pubkey [PubkeyLength]byte) *sync.Mutex {
	value, _ := s.locks.LoadOrStore(pubkey, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()

	return mu
}

// load reads the watermarks of `pubkey`. The caller must hold the lock of `pubkey`.
func (s *Store) load(pubkey [PubkeyLength]byte) (*ValidatorSigningProtection, error) {
	path := s.filePath(pubkey)

	data, err := os.ReadFile(path) // #nosec G304 -- The path is built from the store directory and a public key.
	if errors.Is(err, os.ErrNotExist) {
		return &ValidatorSigningProtection{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	content := new(signingProtectionFile)
	if err := decoder.Decode(content); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("decode %s: empty file", path)
		}

		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	root, err := hexutil.Decode(content.GenesisValidatorsRoot)
	if err != nil {
		return nil, fmt.Errorf("decode genesis validators root of %s: %w", path, err)
	}

	if !bytes.Equal(root, s.genesisValidatorsRoot[:]) {
		return nil, fmt.Errorf("genesis validators root %#x of %s does not match expected %#x", root, path, s.genesisValidatorsRoot)
	}

	return &content.ValidatorSigningProtection, nil
}

// save writes the watermarks of `pubkey`. The caller must hold the lock of `pubkey`.
func (s *Store) save(pubkey [PubkeyLength]byte, protection *ValidatorSigningProtection) error {
	content := &signingProtectionFile{
		GenesisValidatorsRoot:      hexutil.Encode(s.genesisValidatorsRoot[:]),
		ValidatorSigningProtection: *protection,
	}

	data, err := yaml.Marshal(content)
	if err != nil {
		return fmt.Errorf("marshal signing protection: %w", err)
	}

	if err := writeFileAtomic(s.filePath(pubkey), data); err != nil {
		return fmt.Errorf("write signing protection: %w", err)
	}

	return nil
}

// filePath returns the path of the signing protection file of `pubkey`.
func (s *Store) filePath(pubkey [PubkeyLength]byte) string {
	return filepath.Join(s.dirPath, hexutil.Encode(pubkey[:])+fileExtension)
}

// pubkeyFromFileName returns the public key of a signing protection file name.
func pubkeyFromFileName(name string) ([PubkeyLength]byte, bool) {
	var pubkey [PubkeyLength]byte

	encoded, ok := strings.CutSuffix(name, fileExtension)
	if !ok {
		return pubkey, false
	}

	decoded, err := hexutil.Decode(encoded)
	if err != nil || len(decoded) != PubkeyLength {
		return pubkey, false
	}

	copy(pubkey[:], decoded)
	return pubkey, true
}

// writeFileAtomic durably replaces the content of `path` with `data`:
// a crash leaves either the previous or the new content, never a partial one.
func writeFileAtomic(path string, data []byte) (err error) {
	dirPath := filepath.Dir(path)

	tmp, err := os.CreateTemp(dirPath, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}

	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(params.BeaconIoConfig().ReadWritePermissions); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpPath, err)
	}

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", tmpPath, err)
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", tmpPath, err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpPath, path, err)
	}

	if err := syncDir(dirPath); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}

	return nil
}

// syncDir makes a rename in `dirPath` durable.
func syncDir(dirPath string) error {
	dir, err := os.Open(dirPath) // #nosec G304 -- The path is the store directory.
	if err != nil {
		return fmt.Errorf("open %s: %w", dirPath, err)
	}

	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf("sync %s: %w", dirPath, err)
	}

	if err := dir.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dirPath, err)
	}

	return nil
}
