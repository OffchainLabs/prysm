package sync

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func setupSelfAttService(t *testing.T) *Service {
	beaconDB := testDB.SetupDB(t)
	return &Service{
		ctx: t.Context(),
		cfg: &config{
			stateGen: stategen.New(beaconDB, doublylinkedtree.New()),
		},
		selfSubmittedAtts: make(map[[32]byte]*selfAttEntry),
	}
}

// selfAttData returns AttestationData for slot 1 / committee 0, matching the committee that the
// deterministic genesis state resolves to validators {12, 2} for aggregation bits 0b11,0b1.
func selfAttData(root []byte) *ethpb.AttestationData {
	return &ethpb.AttestationData{
		Slot:            1,
		CommitteeIndex:  0,
		BeaconBlockRoot: root,
		Source:          &ethpb.Checkpoint{Epoch: 0, Root: root},
		Target:          &ethpb.Checkpoint{Epoch: 1, Root: root},
	}
}

func TestRecordSelfSubmittedAttestation_SingleAttestation(t *testing.T) {
	s := setupSelfAttService(t)
	root := bytesutil.PadTo([]byte("hello-world"), 32)
	att := &ethpb.SingleAttestation{CommitteeId: 0, AttesterIndex: 42, Data: selfAttData(root)}

	s.recordSelfSubmittedAttestation(att)

	dataRoot, err := att.Data.HashTreeRoot()
	require.NoError(t, err)
	e, ok := s.selfSubmittedAtts[dataRoot]
	require.Equal(t, true, ok)
	sub, ok := e.validators[42]
	require.Equal(t, true, ok)
	require.Equal(t, false, sub.seen)
}

func TestMatchSelfSubmittedAttestation_MarksSeenAndPruneLogsMiss(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupSelfAttService(t)
	state, _ := util.DeterministicGenesisState(t, 256)
	root := bytesutil.PadTo([]byte("hello-world"), 32)
	require.NoError(t, s.cfg.stateGen.SaveState(t.Context(), bytesutil.ToBytes32(root), state))

	// Entry A: validators 2 and 12 (which the committee resolves to), fully covered by the aggregate.
	for _, idx := range []primitives.ValidatorIndex{2, 12} {
		s.recordSelfSubmittedAttestation(&ethpb.SingleAttestation{CommitteeId: 0, AttesterIndex: idx, Data: selfAttData(root)})
	}

	// Entry B: validator 99 for a different data root (same slot) that is never aggregated.
	otherData := selfAttData(bytesutil.PadTo([]byte("other-root"), 32))
	s.recordSelfSubmittedAttestation(&ethpb.SingleAttestation{CommitteeId: 0, AttesterIndex: 99, Data: otherData})

	// A gossiped aggregate covering entry A's data that includes validators 12 and 2.
	aggregate := &ethpb.Attestation{Data: selfAttData(root), AggregationBits: bitfield.Bitlist{0b11, 0b1}}
	s.matchSelfSubmittedAttestation(t.Context(), aggregate)

	dataRoot, err := aggregate.Data.HashTreeRoot()
	require.NoError(t, err)
	e := s.selfSubmittedAtts[dataRoot]
	require.Equal(t, true, e.validators[2].seen)
	require.Equal(t, true, e.validators[12].seen)
	require.LogsContain(t, hook, "All submitted attestations seen in gossiped aggregate")

	// Pruning past the retention window logs a miss for the never-seen validator 99 and drops both entries.
	retention := primitives.Slot(selfAttRetentionEpochs) * params.BeaconConfig().SlotsPerEpoch
	s.pruneSelfSubmittedAttestations(e.slot + retention + 1)
	require.LogsContain(t, hook, "Submitted attestations never seen in a gossiped aggregate")
	_, ok := s.selfSubmittedAtts[dataRoot]
	require.Equal(t, false, ok)
}

func TestMatchSelfSubmittedAttestation_LogsAtMostOncePerRoot(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupSelfAttService(t)
	state, _ := util.DeterministicGenesisState(t, 256)
	root := bytesutil.PadTo([]byte("hello-world"), 32)
	require.NoError(t, s.cfg.stateGen.SaveState(t.Context(), bytesutil.ToBytes32(root), state))

	aggregate := &ethpb.Attestation{Data: selfAttData(root), AggregationBits: bitfield.Bitlist{0b11, 0b1}}

	// Validator 2 submits and is fully matched: this logs once.
	s.recordSelfSubmittedAttestation(&ethpb.SingleAttestation{CommitteeId: 0, AttesterIndex: 2, Data: selfAttData(root)})
	s.matchSelfSubmittedAttestation(t.Context(), aggregate)

	// Validator 12 (also in the aggregate) submits for the same root after it already completed. The
	// entry completes again, but it must not be logged a second time.
	s.recordSelfSubmittedAttestation(&ethpb.SingleAttestation{CommitteeId: 0, AttesterIndex: 12, Data: selfAttData(root)})
	s.matchSelfSubmittedAttestation(t.Context(), aggregate)

	dataRoot, err := aggregate.Data.HashTreeRoot()
	require.NoError(t, err)
	// Validator 12 is still marked seen (so it is not later reported as a miss)...
	require.Equal(t, true, s.selfSubmittedAtts[dataRoot].validators[12].seen)

	// ...but the log was emitted exactly once.
	count := 0
	for _, entry := range hook.AllEntries() {
		if entry.Message == "All submitted attestations seen in gossiped aggregate" {
			count++
		}
	}
	require.Equal(t, 1, count)
}

func TestPruneSelfSubmittedAttestations_KeepsRecentEntries(t *testing.T) {
	s := setupSelfAttService(t)
	root := bytesutil.PadTo([]byte("hello-world"), 32)
	s.recordSelfSubmittedAttestation(&ethpb.SingleAttestation{CommitteeId: 0, AttesterIndex: 1, Data: selfAttData(root)})

	// currentSlot below the retention window is a no-op.
	s.pruneSelfSubmittedAttestations(1)
	require.Equal(t, 1, len(s.selfSubmittedAtts))
}
