package genesis

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// Initialize is mainly exported for the node initialization process to specify providers of the genesis data
// and the path to the local storage location via cli flags.
func Initialize(ctx context.Context, dir string, providers ...Provider) error {
	emb, ok := embeddedGenesisData[params.BeaconConfig().ConfigName]
	if ok {
		setPkgVar(emb, true)
		return nil
	}
	gd, err := FindStateFile(dir)
	if err == nil {
		setPkgVar(gd, true)
		return nil
	}
	if !errors.Is(err, ErrGenesisFileNotFound) {
		return err
	}
	return initializeFromProviders(ctx, dir, providers...)
}

func initializeFromProviders(ctx context.Context, dir string, providers ...Provider) error {
	for _, get := range providers {
		gs, err := get.Genesis(ctx)
		if err != nil {
			log.WithField("provider", fmt.Sprintf("%T", get)).Warn("genesis provider failed")
			continue
		}
		gd, err := newGenesisData(gs, dir)
		if err != nil {
			return errors.Wrapf(err, "new genesis data")
		}
		return Store(gd)
	}
	return ErrGenesisStateNotInitialized
}

func newGenesisData(st state.BeaconState, dir string) (GenesisData, error) {
	if state.IsNil(st) {
		return GenesisData{}, ErrGenesisStateNotInitialized
	}
	if dir == "" {
		return GenesisData{}, ErrFilePathUnset
	}
	return GenesisData{
		FileDir:        dir,
		State:          st,
		ValidatorsRoot: bytesutil.ToBytes32(st.GenesisValidatorsRoot()),
		Time:           st.GenesisTime(),
	}, nil
}

// FindStateFile searches for a valid genesis state file in the specified directory.
func FindStateFile(dir string) (GenesisData, error) {
	if dir == "" {
		return GenesisData{}, ErrFilePathUnset
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return GenesisData{}, fmt.Errorf("%w: %w", ErrGenesisFileNotFound, err)
	}
	for _, f := range files {
		gd, err := tryParseFname(dir, f)
		if err != nil {
			continue
		}
		return gd, nil
	}
	return GenesisData{}, ErrGenesisFileNotFound
}

func tryParseFname(dir string, f os.DirEntry) (GenesisData, error) {
	gd := GenesisData{FileDir: dir}
	if f.IsDir() {
		return gd, ErrNotGenesisStateFile
	}
	extParts := strings.Split(f.Name(), ".")
	if len(extParts) != 2 || extParts[1] != "ssz" {
		return gd, ErrNotGenesisStateFile
	}
	parts := strings.Split(extParts[0], "-")
	if len(parts) != 3 || parts[genesisPart] != "genesis" {
		return gd, ErrNotGenesisStateFile
	}
	ts, err := strconv.ParseInt(parts[timePart], 10, 64)
	if err != nil {
		return gd, errors.Wrap(err, "parse genesis time")
	}
	if ts < 0 {
		return gd, errors.New("genesis time cannot be negative")
	}
	gd.Time = time.Unix(ts, 0)
	if err := hexutil.UnmarshalFixedText("genesis_validators_root", []byte(parts[gvrPart]), gd.ValidatorsRoot[:]); err != nil {
		return gd, errors.Wrap(err, "unmarshal genesis validators root")
	}
	return gd, nil
}
