package blockchain

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestHeadSlot_DataRace(t *testing.T) {
	s := testServiceWithDB(t)
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)
	st, _ := util.DeterministicGenesisState(t, 1)
	wait := make(chan struct{})
	go func() {
		defer close(wait)
		require.NoError(t, s.saveHead(t.Context(), [32]byte{}, b, st))
	}()
	s.HeadSlot()
	<-wait
}

func TestHeadRoot_DataRace(t *testing.T) {
	s := testServiceWithDB(t)
	s.head = &head{root: [32]byte{'A'}}
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)
	wait := make(chan struct{})
	st, _ := util.DeterministicGenesisState(t, 1)
	go func() {
		defer close(wait)
		require.NoError(t, s.saveHead(t.Context(), [32]byte{}, b, st))

	}()
	_, err = s.HeadRoot(t.Context())
	require.NoError(t, err)
	<-wait
}

func TestHeadBlock_DataRace(t *testing.T) {
	wsb, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlock{Block: &ethpb.BeaconBlock{Body: &ethpb.BeaconBlockBody{}}})
	require.NoError(t, err)
	s := testServiceWithDB(t)
	s.head = &head{block: wsb}
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)
	wait := make(chan struct{})
	st, _ := util.DeterministicGenesisState(t, 1)
	go func() {
		defer close(wait)
		require.NoError(t, s.saveHead(t.Context(), [32]byte{}, b, st))

	}()
	_, err = s.HeadBlock(t.Context())
	require.NoError(t, err)
	<-wait
}

func TestHeadState_DataRace(t *testing.T) {
	s := testServiceWithDB(t)
	beaconDB := s.cfg.BeaconDB
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)
	wait := make(chan struct{})
	st, _ := util.DeterministicGenesisState(t, 1)
	root := bytesutil.ToBytes32(bytesutil.PadTo([]byte{'s'}, 32))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(t.Context(), root))
	require.NoError(t, beaconDB.SaveState(t.Context(), st, root))
	go func() {
		defer close(wait)
		require.NoError(t, s.saveHead(t.Context(), [32]byte{}, b, st))

	}()
	_, err = s.HeadState(t.Context())
	require.NoError(t, err)
	<-wait
}
