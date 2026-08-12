package sync

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
)

func TestAttestationSubnetFromTopic(t *testing.T) {
	subnet, err := attestationSubnetFromTopic("/eth2/abcd1234/beacon_attestation_37/ssz_snappy")
	require.NoError(t, err)
	require.Equal(t, uint64(37), subnet)

	_, err = attestationSubnetFromTopic("/eth2/abcd1234/beacon_block/ssz_snappy")
	require.ErrorContains(t, "not an attestation subnet topic", err)

	_, err = attestationSubnetFromTopic("/eth2/abcd1234/beacon_attestation_x/ssz_snappy")
	require.NotNil(t, err)
}

// partialAttTestSetup spins up a sync service at an Electra slot with a saved
// block+state, returning everything needed to drive the partial attestation
// paths.
type partialAttTestSetup struct {
	s         *Service
	p         *p2ptest.TestP2P
	data      *ethpb.AttestationData
	topic     string
	subnet    uint64
	committee []primitives.ValidatorIndex
	keys      []bls.SecretKey
}

func newPartialAttTestSetup(t *testing.T) *partialAttTestSetup {
	t.Helper()
	return newPartialAttTestSetupAt(t, time.Now(), dbtest.SetupDB(t))
}

// newPartialAttTestSetupAt anchors the clock to now and uses the given
// database; two setups sharing both agree on the current slot and hold
// identical deterministic states, so attestations signed on one validate on
// the other. (The database is shared because a test process can register its
// metrics collectors only once.)
func newPartialAttTestSetupAt(t *testing.T, now time.Time, db db.Database) *partialAttTestSetup {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()

	p := p2ptest.NewTestP2P(t)
	currentSlot := 1 + (primitives.Slot(params.BeaconConfig().ElectraForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
	genesisOffset := time.Duration(currentSlot) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	chain := &mockChain.ChainService{
		Genesis:          now.Add(-1 * genesisOffset),
		ValidatorsRoot:   params.BeaconConfig().GenesisValidatorsRoot,
		ValidAttestation: true,
		DB:               db,
		Optimistic:       true,
	}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	s := &Service{
		ctx: ctx,
		cfg: &config{
			initialSync:         &mockSync.Sync{IsSyncing: false},
			p2p:                 p,
			beaconDB:            db,
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attestationNotifier: (&mockChain.ChainService{}).OperationNotifier(),
			attPool:             attestations.NewPool(),
		},
		blkRootToPendingAtts:             make(map[[32]byte][]any),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	s.initCaches()
	go s.verifierRoutine()

	blk := util.NewBeaconBlock()
	blk.Block.Slot = s.cfg.clock.CurrentSlot()
	util.SaveBlock(t, ctx, db, blk)
	validBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	chain.FinalizedCheckPoint = &ethpb.Checkpoint{Root: validBlockRoot[:], Epoch: 0}

	validators := uint64(64)
	savedState, keys := util.DeterministicGenesisState(t, validators)
	require.NoError(t, savedState.SetSlot(s.cfg.clock.CurrentSlot()))
	require.NoError(t, db.SaveState(ctx, savedState, validBlockRoot))
	chain.State = savedState

	slot := s.cfg.clock.CurrentSlot()
	committee, err := helpers.BeaconCommitteeFromState(ctx, savedState, slot, 0)
	require.NoError(t, err)

	data := &ethpb.AttestationData{
		BeaconBlockRoot: validBlockRoot[:],
		CommitteeIndex:  0,
		Slot:            slot,
		Target:          &ethpb.Checkpoint{Epoch: s.cfg.clock.CurrentEpoch(), Root: validBlockRoot[:]},
		Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
	}

	valCount, err2 := helpers.ActiveValidatorCount(ctx, savedState, slots.ToEpoch(slot))
	require.NoError(t, err2)
	subnet := helpers.ComputeSubnetForAttestation(valCount, &ethpb.SingleAttestation{Data: data})
	digest := params.ForkDigest(slots.ToEpoch(slot))
	topic := fmt.Sprintf(p2p.AttestationSubnetTopicFormat, digest, subnet) + p.Encoding().ProtocolSuffix()

	return &partialAttTestSetup{
		s:         s,
		p:         p,
		data:      data,
		topic:     topic,
		subnet:    subnet,
		committee: committee,
		keys:      keys,
	}
}

func (ts *partialAttTestSetup) sign(t *testing.T, state interface {
	Fork() *ethpb.Fork
	GenesisValidatorsRoot() []byte
}, position uint64) []byte {
	t.Helper()
	domain, err := signing.Domain(state.Fork(), ts.data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester, state.GenesisValidatorsRoot())
	require.NoError(t, err)
	attRoot, err := signing.ComputeSigningRoot(ts.data, domain)
	require.NoError(t, err)
	return ts.keys[ts.committee[position]].Sign(attRoot[:]).Marshal()
}

func TestProcessPartialAttestation(t *testing.T) {
	ts := newPartialAttTestSetup(t)
	state := ts.s.cfg.chain.(*mockChain.ChainService).State

	att := func(attester primitives.ValidatorIndex, sig []byte) *ethpb.SingleAttestation {
		return &ethpb.SingleAttestation{
			CommitteeId:   0,
			AttesterIndex: attester,
			Data:          ts.data,
			Signature:     sig,
		}
	}
	goodSig0 := ts.sign(t, state, 0)
	goodSig1 := ts.sign(t, state, 1)
	badSig := make([]byte, 96)
	copy(badSig, goodSig0)
	badSig[10] ^= 0xff

	t.Run("valid attestations are accepted", func(t *testing.T) {
		ts.s.initCaches() // reset the seen cache
		for i, a := range []*ethpb.SingleAttestation{att(ts.committee[0], goodSig0), att(ts.committee[1], goodSig1)} {
			accepted, err := ts.s.processPartialAttestation(ts.topic, a)
			require.NoError(t, err)
			require.Equal(t, true, accepted, "attestation %d not accepted", i)
		}
		// Accepted attestations are pooled and rebroadcast on classic gossip.
		require.Equal(t, true, ts.p.BroadcastCalled.Load(), "attestation must be rebroadcast on classic gossip")
		require.Equal(t, 2, ts.s.cfg.attPool.UnaggregatedAttestationCount())
	})

	t.Run("already seen attestation is not accepted again", func(t *testing.T) {
		accepted, err := ts.s.processPartialAttestation(ts.topic, att(ts.committee[0], goodSig0))
		// Ignored, not rejected: no error, but not newly accepted.
		require.NoError(t, err)
		require.Equal(t, false, accepted)
		require.Equal(t, 2, ts.s.cfg.attPool.UnaggregatedAttestationCount())
	})

	t.Run("invalid signature is rejected", func(t *testing.T) {
		ts.s.initCaches()
		accepted, err := ts.s.processPartialAttestation(ts.topic, att(ts.committee[1], badSig))
		require.ErrorContains(t, "rejected", err)
		require.Equal(t, false, accepted)
	})

	t.Run("attester outside the committee is rejected", func(t *testing.T) {
		ts.s.initCaches()
		outsider := ts.committee[0]
		for slices.Contains(ts.committee, outsider) {
			outsider++
		}
		accepted, err := ts.s.processPartialAttestation(ts.topic, att(outsider, goodSig0))
		require.ErrorContains(t, "rejected", err)
		require.Equal(t, false, accepted)
	})
}

// A classic-gossip-accepted attestation is fed into the partial broadcaster via
// submitPartialAtt: gossipsub withholds the classic copy from partial peers.
func TestClassicAcceptFeedsPartialBroadcaster(t *testing.T) {
	ts := newPartialAttTestSetup(t)
	state := ts.s.cfg.chain.(*mockChain.ChainService).State

	type submitted struct {
		topic string
		att   *ethpb.SingleAttestation
	}
	got := make(chan submitted, 1)
	ts.s.submitPartialAtt = func(topic string, att *ethpb.SingleAttestation) {
		got <- submitted{topic: topic, att: att}
	}

	single := &ethpb.SingleAttestation{
		CommitteeId:   0,
		AttesterIndex: ts.committee[0],
		Data:          ts.data,
		Signature:     ts.sign(t, state, 0),
	}
	buf := new(bytes.Buffer)
	_, err := ts.p.Encoding().EncodeGossip(buf, single)
	require.NoError(t, err)
	msg := &pubsub.Message{Message: &pubsubpb.Message{Data: buf.Bytes(), Topic: &ts.topic}}

	res, err := ts.s.validateCommitteeIndexBeaconAttestation(t.Context(), "some-peer", msg)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)

	select {
	case s := <-got:
		require.Equal(t, ts.topic, s.topic)
		require.Equal(t, ts.committee[0], s.att.AttesterIndex)
	default:
		t.Fatal("accept path did not feed the partial broadcaster")
	}
}
