package sync

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestCommitteeAttGossipEpochStats_epochRolloverOnObserve(t *testing.T) {
	genesis := time.Unix(1_700_000_000, 0)
	var slot primitives.Slot
	cl := startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time {
		tt, err := slots.StartTime(genesis, slot)
		require.NoError(t, err)
		return tt
	}))

	var st committeeAttGossipEpochStats
	st.observe(cl, pubsub.ValidationAccept, "")

	st.mu.Lock()
	require.Equal(t, slots.ToEpoch(0), st.epoch)
	require.Equal(t, uint64(1), st.success)
	require.Empty(t, st.nonSuccess)
	st.mu.Unlock()

	slot = params.BeaconConfig().SlotsPerEpoch
	st.observe(cl, pubsub.ValidationReject, "reject_test")

	st.mu.Lock()
	require.Equal(t, slots.ToEpoch(slot), st.epoch)
	require.Equal(t, uint64(0), st.success)
	require.Equal(t, uint64(1), st.nonSuccess["reject_test"])
	st.mu.Unlock()
}

func TestCommitteeAttGossipEpochStats_rotateOnlyFlushesGap(t *testing.T) {
	genesis := time.Unix(1_700_000_000, 0)
	var slot primitives.Slot
	cl := startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time {
		tt, err := slots.StartTime(genesis, slot)
		require.NoError(t, err)
		return tt
	}))

	var st committeeAttGossipEpochStats
	st.observe(cl, pubsub.ValidationAccept, "")
	st.mu.Lock()
	require.Equal(t, primitives.Epoch(0), st.epoch)
	st.mu.Unlock()

	slot = 2 * params.BeaconConfig().SlotsPerEpoch
	st.rotateOnly(cl)

	st.mu.Lock()
	require.Equal(t, slots.ToEpoch(slot), st.epoch)
	require.Zero(t, st.success)
	require.Empty(t, st.nonSuccess)
	st.mu.Unlock()
}

// committeeAttSummaryHook captures Info logs emitted when an epoch of committee
// attestation gossip validation stats is flushed.
type committeeAttSummaryHook struct {
	mu        sync.Mutex
	summaries []logrus.Entry
}

func (h *committeeAttSummaryHook) Levels() []logrus.Level {
	return []logrus.Level{logrus.InfoLevel}
}

func (h *committeeAttSummaryHook) Fire(e *logrus.Entry) error {
	if e.Message != "Committee subnet attestation gossip validation epoch summary" {
		return nil
	}
	h.mu.Lock()
	h.summaries = append(h.summaries, *e)
	h.mu.Unlock()
	return nil
}

func (h *committeeAttSummaryHook) last() (logrus.Entry, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.summaries) == 0 {
		return logrus.Entry{}, false
	}
	return h.summaries[len(h.summaries)-1], true
}

// TestValidateCommitteeIndexBeaconAttestation_epochGossipSummary drives
// validateCommitteeIndexBeaconAttestation with mixed outcomes (ignore/reject and
// one fully valid accept) in the same clock epoch, advances the clock to the
// next epoch, flushes stats, and asserts (and logs) the summary fields written
// by committeeAttGossipEpochStats.
func TestValidateCommitteeIndexBeaconAttestation_epochGossipSummary(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	hook := &committeeAttSummaryHook{}
	logrus.StandardLogger().AddHook(hook)
	t.Cleanup(func() {
		logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	})

	// ValidateAttestationTime uses wall clock vs genesis (not cfg.clock.CurrentSlot). Keep genesis
	// ~1 slot in the past so "current slot" is ~1 and a slot-1 attestation is in range.
	slotDur := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	genesis := time.Now().Add(-slotDur)
	vr := [32]byte{'B'}
	var slot primitives.Slot
	clock := startup.NewClock(genesis, vr, startup.WithNower(func() time.Time {
		tt, err := slots.StartTime(genesis, slot)
		require.NoError(t, err)
		return tt
	}))

	p := p2ptest.NewTestP2P(t)
	db := dbtest.SetupDB(t)
	syncMock := &mockSync.Sync{IsSyncing: true}
	ctx := t.Context()

	chainSvc := &mockChain.ChainService{
		Genesis:          genesis,
		ValidatorsRoot:   vr,
		DB:               db,
		ValidAttestation: true,
		Optimistic:       true,
	}

	s := &Service{
		ctx: ctx,
		cfg: &config{
			initialSync:         syncMock,
			p2p:                 p,
			beaconDB:            db,
			clock:               clock,
			chain:               chainSvc,
			attestationNotifier: (&mockChain.ChainService{}).OperationNotifier(),
		},
		blkRootToPendingAtts:             make(map[[32]byte][]any),
		seenUnAggregatedAttestationCache: lruwrpr.New(seenUnaggregatedAttSize),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	s.initCaches()
	go s.verifierRoutine()

	dig, err := s.currentForkDigest()
	require.NoError(t, err)
	topic := fmt.Sprintf("/eth2/%x/beacon_attestation_0", dig)

	// 1) Initial sync: ignore_initial_sync
	_, err = s.validateCommitteeIndexBeaconAttestation(ctx, "", &pubsub.Message{
		Message: &pubsubpb.Message{Topic: &topic},
	})
	require.NoError(t, err)

	syncMock.IsSyncing = false

	// 2) Nil topic: reject_invalid_topic
	badTopicMsg := &pubsub.Message{Message: &pubsubpb.Message{Topic: nil}}
	_, err = s.validateCommitteeIndexBeaconAttestation(ctx, "", badTopicMsg)
	require.Error(t, err)

	// 3) Slot 0 attestation: ignore_slot_zero
	attSlot0 := &ethpb.Attestation{
		AggregationBits: bitfield.Bitlist{0b11},
		Data: &ethpb.AttestationData{
			Slot:            0,
			CommitteeIndex:  0,
			BeaconBlockRoot: make([]byte, fieldparams.RootLength),
			Target:          &ethpb.Checkpoint{Epoch: 0, Root: make([]byte, fieldparams.RootLength)},
			Source:          &ethpb.Checkpoint{Epoch: 0, Root: make([]byte, fieldparams.RootLength)},
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
	buf := new(bytes.Buffer)
	_, encErr := p.Encoding().EncodeGossip(buf, attSlot0)
	require.NoError(t, encErr)
	_, err = s.validateCommitteeIndexBeaconAttestation(ctx, "", &pubsub.Message{
		Message: &pubsubpb.Message{Data: buf.Bytes(), Topic: &topic},
	})
	require.NoError(t, err)

	// 4) Fully valid attestation (accept): same setup as TestService_validateCommitteeIndexBeaconAttestation.
	helpers.ClearCache()
	blk := util.NewBeaconBlock()
	blk.Block.Slot = 1
	util.SaveBlock(t, ctx, db, blk)
	validBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	chainSvc.FinalizedCheckPoint = &ethpb.Checkpoint{
		Root:  validBlockRoot[:],
		Epoch: 0,
	}
	validators := uint64(64)
	savedState, keys := util.DeterministicGenesisState(t, validators)
	require.NoError(t, savedState.SetSlot(1))
	require.NoError(t, db.SaveState(ctx, savedState, validBlockRoot))
	chainSvc.State = savedState

	slot = 1
	dig, err = s.currentForkDigest()
	require.NoError(t, err)
	validTopic := fmt.Sprintf("/eth2/%x/beacon_attestation_1", dig)

	validAtt := &ethpb.Attestation{
		AggregationBits: bitfield.Bitlist{0b101},
		Data: &ethpb.AttestationData{
			BeaconBlockRoot: validBlockRoot[:],
			CommitteeIndex:  0,
			Slot:            1,
			Target: &ethpb.Checkpoint{
				Epoch: 0,
				Root:  validBlockRoot[:],
			},
			Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		},
	}
	com, err := helpers.BeaconCommitteeFromState(ctx, savedState, validAtt.GetData().Slot, validAtt.GetData().CommitteeIndex)
	require.NoError(t, err)
	domain, err := signing.Domain(savedState.Fork(), validAtt.GetData().Target.Epoch, params.BeaconConfig().DomainBeaconAttester, savedState.GenesisValidatorsRoot())
	require.NoError(t, err)
	attRoot, err := signing.ComputeSigningRoot(validAtt.GetData(), domain)
	require.NoError(t, err)
	for i := 0; ; i++ {
		if validAtt.GetAggregationBits().BitAt(uint64(i)) {
			validAtt.SetSignature(keys[com[i]].Sign(attRoot[:]).Marshal())
			break
		}
	}
	validBuf := new(bytes.Buffer)
	_, encErr = p.Encoding().EncodeGossip(validBuf, validAtt)
	require.NoError(t, encErr)
	res, err := s.validateCommitteeIndexBeaconAttestation(ctx, "", &pubsub.Message{
		Message: &pubsubpb.Message{Data: validBuf.Bytes(), Topic: &validTopic},
	})
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)

	// Advance clock to next epoch and flush completed epoch 0.
	slot = params.BeaconConfig().SlotsPerEpoch
	s.committeeAttGossipEpochStats.rotateOnly(clock)

	entry, ok := hook.last()
	require.True(t, ok, "expected epoch summary log line")

	epochVal, ok := entry.Data["epoch"].(primitives.Epoch)
	require.True(t, ok, "epoch field type")
	require.Equal(t, primitives.Epoch(0), epochVal)

	successVal, ok := entry.Data["successCount"].(uint64)
	require.True(t, ok, "successCount field type")
	require.Equal(t, uint64(1), successVal)

	totalVal, ok := entry.Data["nonSuccessTotal"].(uint64)
	require.True(t, ok, "nonSuccessTotal field type")
	require.Equal(t, uint64(3), totalVal)

	byReason, ok := entry.Data["nonSuccessByReason"].(map[string]uint64)
	require.True(t, ok, "nonSuccessByReason field type")
	require.Equal(t, uint64(1), byReason["ignore_initial_sync"])
	require.Equal(t, uint64(1), byReason["reject_invalid_topic"])
	require.Equal(t, uint64(1), byReason["ignore_slot_zero"])

	t.Logf("epoch gossip validation summary (epoch=%v success=%v nonSuccessTotal=%v byReason=%v)",
		entry.Data["epoch"], entry.Data["successCount"], entry.Data["nonSuccessTotal"], byReason)
}
