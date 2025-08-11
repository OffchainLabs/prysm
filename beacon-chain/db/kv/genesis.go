package kv

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	dbIface "github.com/OffchainLabs/prysm/v6/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/detect"
	"github.com/OffchainLabs/prysm/v6/genesis"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// SaveGenesisData bootstraps the beaconDB with a given genesis state.
func (s *Store) SaveGenesisData(ctx context.Context, genesisState state.BeaconState) error {
	wsb, err := blocks.NewGenesisBlockForState(ctx, genesisState)
	if err != nil {
		return errors.Wrap(err, "could not get genesis block root")
	}
	genesisBlkRoot, err := wsb.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not get genesis block root")
	}
	if err := s.SaveBlock(ctx, wsb); err != nil {
		return errors.Wrap(err, "could not save genesis block")
	}
	if err := s.SaveState(ctx, genesisState, genesisBlkRoot); err != nil {
		return errors.Wrap(err, "could not save genesis state")
	}
	if err := s.SaveStateSummary(ctx, &ethpb.StateSummary{
		Slot: 0,
		Root: genesisBlkRoot[:],
	}); err != nil {
		return err
	}

	if err := s.SaveHeadBlockRoot(ctx, genesisBlkRoot); err != nil {
		return errors.Wrap(err, "could not save head block root")
	}
	if err := s.SaveGenesisBlockRoot(ctx, genesisBlkRoot); err != nil {
		return errors.Wrap(err, "could not save genesis block root")
	}
	return nil
}

// LoadGenesis loads a genesis state from a ssz-serialized byte slice, if no genesis exists already.
func (s *Store) LoadGenesis(ctx context.Context, sb []byte) error {
	if len(sb) < (1 << 10) {
		log.WithField("size", fmt.Sprintf("%d bytes", len(sb))).
			Warn("Genesis state is smaller than one 1Kb. This could be an empty file, git lfs metadata file, or corrupt genesis state.")
	}
	vu, err := detect.FromState(sb)
	if err != nil {
		return err
	}
	gs, err := vu.UnmarshalBeaconState(sb)
	if err != nil {
		return err
	}
	existing, err := s.GenesisState(ctx)
	if err != nil {
		return err
	}
	// If some different genesis state existed already, return an error. The same genesis state is
	// considered a no-op.
	if existing != nil && !existing.IsNil() {
		a, err := existing.HashTreeRoot(ctx)
		if err != nil {
			return err
		}
		b, err := gs.HashTreeRoot(ctx)
		if err != nil {
			return err
		}
		if a == b {
			return nil
		}
		return dbIface.ErrExistingGenesisState
	}

	return s.SaveGenesisData(ctx, gs)
}

// EnsureEmbeddedGenesis checks that a genesis block has been generated when an embedded genesis
// state is used. If a genesis block does not exist, but a genesis state does, then we should call
// SaveGenesisData on the existing genesis state.
func (s *Store) EnsureEmbeddedGenesis(ctx context.Context) error {
	gb, err := s.GenesisBlock(ctx)
	if err != nil {
		return err
	}
	if gb != nil && !gb.IsNil() {
		return nil
	}
	gs, err := s.GenesisState(ctx)
	if err != nil {
		return err
	}
	if !state.IsNil(gs) {
		return s.SaveGenesisData(ctx, gs)
	}
	return nil
}

type LegacyGenesisProvider struct {
	store *Store
}

func NewLegacyGenesisProvider(store *Store) *LegacyGenesisProvider {
	return &LegacyGenesisProvider{store: store}
}

var _ genesis.Provider = &LegacyGenesisProvider{}

func (p *LegacyGenesisProvider) Genesis(ctx context.Context) (state.BeaconState, error) {
	return p.store.LegacyGenesisState(ctx)
}
