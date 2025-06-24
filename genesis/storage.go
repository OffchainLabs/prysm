package genesis

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/detect"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

type fnamePart int

const (
	genesisPart fnamePart = 0
	timePart    fnamePart = 1
	gvrPart     fnamePart = 2
)

// data is a private package level variable that holds the genesis data.
// Other packages interact with it via wrapper functions like Set() and State().
var data GenesisData
var stateMu sync.Mutex

// GenesisData bundles all the package level data.
type GenesisData struct {
	ValidatorsRoot [32]byte
	Time           time.Time
	FileDir        string
	State          state.BeaconState
	embeddedBytes  func() ([]byte, error)
	embeddedState  func() (state.BeaconState, error)
}

func ValidatorsRoot() [32]byte {
	return data.ValidatorsRoot
}

func Time() time.Time {
	return data.Time
}

func State() (state.BeaconState, error) {
	stateMu.Lock()
	defer stateMu.Unlock()
	if !state.IsNil(data.State) {
		return data.State, nil
	}
	if data.embeddedState != nil {
		st, err := data.embeddedState()
		if err != nil {
			return nil, errors.Wrap(err, "load embedded genesis state")
		}
		if !state.IsNil(st) {
			data.State = st
			return data.State, nil
		}
	}
	return loadState()
}

func (d GenesisData) filePath() string {
	parts := [3]string{}
	parts[genesisPart] = "genesis"
	parts[timePart] = strconv.FormatInt(d.Time.Unix(), 10)
	parts[gvrPart] = hexutil.Encode(d.ValidatorsRoot[:])
	return path.Join(d.FileDir, strings.Join(parts[:], "-")+".ssz")
}

func loadState() (state.BeaconState, error) {
	s, err := stateFromFile(data.filePath())
	if err != nil {
		return nil, errors.Wrapf(err, "InitializeFromProtoUnsafePhase0")
	}
	data.State = s
	return data.State, nil
}

func stateFromFile(fpath string) (state.BeaconState, error) {
	sb, err := file.ReadFileAsBytes(fpath)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading genesis state from %s", fpath)
	}
	return stateFromBytes(sb)
}

func stateFromBytes(sb []byte) (state.BeaconState, error) {
	return detect.UnmarshalState(sb)
}

func Store(d GenesisData) error {
	if err := ensureWritable(d.FileDir); err != nil {
		return err
	}
	if err := persist(d); err != nil {
		return errors.Wrap(err, "persist genesis data")
	}
	data = d
	return nil
}

func persist(d GenesisData) error {
	if state.IsNil(d.State) {
		return ErrGenesisStateNotInitialized
	}
	if d.FileDir == "" {
		return ErrFilePathUnset
	}
	fpath := d.filePath()
	sb, err := d.State.MarshalSSZ()
	if err != nil {
		return errors.Wrap(err, "marshal ssz")
	}
	if err := os.WriteFile(fpath, sb, 0644); err != nil {
		return fmt.Errorf("error writing genesis state to %s: %w", fpath, err)
	}
	log.WithField("filePath", fpath).Info("Genesis state written to disk.")
	return nil
}

func ensureWritable(dir string) (err error) {
	if dir == "" {
		return ErrFilePathUnset
	}
	if err := file.MkdirAll(dir); err != nil {
		return errors.Wrapf(err, "error creating genesis data directory %s", dir)
	}
	lockPath := path.Join(dir, "genesis.lock")
	defer func() {
		if err == nil {
			err = os.Remove(lockPath)
		}
	}()
	return os.WriteFile(lockPath, []byte{1}, 0644)
}

func uint64ToTime(ts uint64) time.Time {
	return time.Unix(int64(ts), 0) // lint:uintcast -- genesis timestamp won't exceed int64 range
}

// User specifies either genesis data file or beacon api
// User specifies data directory (use main db directory)
// Add new initializer for db type, which node.go sets if the other two are unset
// All initializers write to the state file, except embedded
// node.go makes sure the needful is in the db
// db code needs to work before db startup has happened
// All initializers should short circuit if a genesis file is found
