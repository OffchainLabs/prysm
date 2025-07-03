package embedded

import (
	"context"
	_ "embed"
	"errors"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/golang/snappy"
)

var ErrNotFound = errors.New("embedded genesis state not found")

var embeddedStates = map[string]*[]byte{}

// ByName returns a copy of the genesis state from a hardcoded value.
func ByName(name string) (state.BeaconState, error) {
	sb, exists := embeddedStates[name]
	if exists {
		return load(*sb)
	}
	return nil, nil
}

func BytesByName(name string) ([]byte, error) {
	sb, exists := embeddedStates[name]
	if exists {
		return *sb, nil
	}
	return nil, ErrNotFound
}

func Has(name string) bool {
	_, exists := embeddedStates[name]
	return exists
}

// load a compressed ssz state file into a beacon state struct.
func load(b []byte) (state.BeaconState, error) {
	st := &ethpb.BeaconState{}
	b, err := snappy.Decode(nil /*dst*/, b)
	if err != nil {
		return nil, err
	}
	if err := st.UnmarshalSSZ(b); err != nil {
		return nil, err
	}
	return state_native.InitializeFromProtoUnsafePhase0(st)
}

type embeddedProvider struct{}

func (p embeddedProvider) Genesis(ctx context.Context) (state.BeaconState, error) {
	// Use the mainnet genesis state as default
	st, err := ByName(params.BeaconConfig().ConfigName)
	if err == nil && st == nil {
		return nil, ErrNotFound
	}
	return st, nil
}

var EmbeddedProvider = &embeddedProvider{}
