package sync

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func setupWatchedService(t *testing.T) *Service {
	beaconDB := testDB.SetupDB(t)
	return &Service{
		ctx: t.Context(),
		cfg: &config{
			stateGen: stategen.New(beaconDB, doublylinkedtree.New()),
		},
		watchedDuties:       make(map[primitives.Slot]*watchedSlotEntry),
		watchedSeededEpochs: make(map[primitives.Epoch]bool),
	}
}

// TestSeedWatchedDuties verifies expectations are derived from public committee assignments: the
// deterministic genesis state resolves committee 0 / slot 1 to validators {12, 2}.
func TestSeedWatchedDuties(t *testing.T) {
	s := setupWatchedService(t)
	state, _ := util.DeterministicGenesisState(t, 256)
	s.cfg.chain = &mock.ChainService{State: state}

	s.seedWatchedDuties(t.Context(), 0, []primitives.ValidatorIndex{12})

	e, ok := s.watchedDuties[1]
	require.Equal(t, true, ok)
	rec, ok := e.validators[12]
	require.Equal(t, true, ok)
	require.Equal(t, primitives.CommitteeIndex(0), rec.committee)
	require.Equal(t, false, rec.seen)
	require.Equal(t, true, s.watchedSeededEpochs[0])

	// A second call for an already-seeded epoch is a no-op (the entry is not duplicated/reset).
	rec.seen = true
	s.seedWatchedDuties(t.Context(), 0, []primitives.ValidatorIndex{12})
	require.Equal(t, true, s.watchedDuties[1].validators[12].seen)
}

func TestWatchedDuties_SeenInAggregate(t *testing.T) {
	reset := features.InitWithReset(&features.Flags{
		WatchedAttestationValidators: map[primitives.ValidatorIndex]bool{12: true},
	})
	defer reset()
	hook := logTest.NewGlobal()

	s := setupWatchedService(t)
	state, _ := util.DeterministicGenesisState(t, 256)
	root := bytesutil.PadTo([]byte("hello-world"), 32)
	require.NoError(t, s.cfg.stateGen.SaveState(t.Context(), bytesutil.ToBytes32(root), state))
	s.cfg.chain = &mock.ChainService{State: state}

	s.seedWatchedDuties(t.Context(), 0, []primitives.ValidatorIndex{12})

	// A gossiped aggregate at slot 1 that includes validators 12 and 2.
	aggregate := &ethpb.Attestation{Data: selfAttData(root), AggregationBits: bitfield.Bitlist{0b11, 0b1}}
	s.matchWatchedDuties(t.Context(), aggregate)
	require.Equal(t, true, s.watchedDuties[1].validators[12].seen)

	// Reporting past the retention window logs the watched validator as seen and drops the entry.
	retention := primitives.Slot(selfAttRetentionEpochs) * params.BeaconConfig().SlotsPerEpoch
	s.reportWatchedDuties(1 + retention + 1)
	require.LogsContain(t, hook, "All watched attestations seen in a gossiped aggregate")
	_, ok := s.watchedDuties[1]
	require.Equal(t, false, ok)
}

func TestWatchedDuties_NeverSeen(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupWatchedService(t)

	// Seed an expectation directly that no aggregate ever covers.
	s.watchedDuties[1] = &watchedSlotEntry{
		validators: map[primitives.ValidatorIndex]*watchedDutyRecord{
			99: {committee: 3},
		},
	}

	retention := primitives.Slot(selfAttRetentionEpochs) * params.BeaconConfig().SlotsPerEpoch
	s.reportWatchedDuties(1 + retention + 1)
	require.LogsContain(t, hook, "Watched attestations never seen in a gossiped aggregate")
	_, ok := s.watchedDuties[1]
	require.Equal(t, false, ok)
}
