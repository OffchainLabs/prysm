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
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/detect"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// ValidatorsRoot returns the genesis validators root.
func ValidatorsRoot() [32]byte {
	return data.ValidatorsRoot
}

// Time returns the genesis time.
func Time() time.Time {
	return data.Time
}

// State returns the full genesis BeaconState. It can return an error because this value is lazy loaded.
// The returned value will always be a copy of the underlying value.
func State() (state.BeaconState, error) {
	st, err := stateInternal()
	if state.IsNil(st) || err != nil {
		return nil, err
	}
	return st.Copy(), nil
}

func stateInternal() (state.BeaconState, error) {
	gd := getPkgVar()
	if !gd.initialized {
		// If the state is not explicitly initialized, try to load embedded states if available.
		name := params.BeaconConfig().ConfigName
		if ed, ok := embeddedGenesisData[name]; ok {
			return ed.embeddedState()
		}
		return nil, ErrGenesisStateNotInitialized
	}
	if !state.IsNil(gd.State) {
		return gd.State, nil
	}
	if gd.embeddedState != nil {
		st, err := gd.embeddedState()
		if err != nil {
			return nil, errors.Wrap(err, "load embedded genesis state")
		}
		if !state.IsNil(st) {
			gd.State = st
			return gd.State, nil
		}
	}
	return loadState()
}

// Store is an exported method that allows another package to set the genesis data value and persist it to disk.
// It is exported to be used by implementations of the Provider interface.
func Store(d GenesisData) error {
	if err := ensureWritable(d.FileDir); err != nil {
		return err
	}
	if err := persist(d); err != nil {
		return errors.Wrap(err, "persist genesis data")
	}
	setPkgVar(d, true)
	return nil
}

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

// GenesisData bundles all the package level data. It is exported to allow implementations of the Provider interface to set genesis data.
type GenesisData struct {
	ValidatorsRoot [32]byte
	Time           time.Time
	FileDir        string
	State          state.BeaconState
	embeddedBytes  func() ([]byte, error)
	embeddedState  func() (state.BeaconState, error)
	initialized    bool
}

// FilePath returns the full path to the genesis state file.
func (d GenesisData) FilePath() string {
	parts := [3]string{}
	parts[genesisPart] = "genesis"
	parts[timePart] = strconv.FormatInt(d.Time.Unix(), 10)
	parts[gvrPart] = hexutil.Encode(d.ValidatorsRoot[:])
	return path.Join(d.FileDir, strings.Join(parts[:], "-")+".ssz")
}

func persist(d GenesisData) error {
	if state.IsNil(d.State) {
		return ErrGenesisStateNotInitialized
	}
	if d.FileDir == "" {
		return ErrFilePathUnset
	}
	fpath := d.FilePath()
	sb, err := d.State.MarshalSSZ()
	if err != nil {
		return errors.Wrap(err, "marshal ssz")
	}
	if err := file.WriteFile(fpath, sb); err != nil {
		return fmt.Errorf("error writing genesis state to %s: %w", fpath, err)
	}
	log.WithField("filePath", fpath).Info("Genesis state written to disk.")
	return nil
}

func getPkgVar() GenesisData {
	stateMu.Lock()
	defer stateMu.Unlock()
	return data
}

func setPkgVar(d GenesisData, initialized bool) {
	stateMu.Lock()
	defer stateMu.Unlock()
	d.initialized = initialized
	data = d
}

func loadState() (state.BeaconState, error) {
	stateMu.Lock()
	defer stateMu.Unlock()

	s, err := stateFromFile(data.FilePath())
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
	return detect.UnmarshalState(sb)
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
	return os.WriteFile(lockPath, []byte{1}, 0600)
}
