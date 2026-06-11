package sync

import (
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/payloadattestation"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	payloadatt "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attestation"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

// genesisForSlot returns a genesis time such that the current slot equals slot.
func genesisForSlot(slot primitives.Slot) time.Time {
	return time.Now().Add(-time.Duration(uint64(slot)*params.BeaconConfig().SecondsPerSlot) * time.Second)
}

func pendingPayloadAtt(t *testing.T, root []byte, validatorIndex primitives.ValidatorIndex, slot primitives.Slot) (*ethpb.PayloadAttestationMessage, payloadatt.ROMessage) {
	att := &ethpb.PayloadAttestationMessage{
		ValidatorIndex: validatorIndex,
		Data: &ethpb.PayloadAttestationData{
			BeaconBlockRoot: bytesutil.PadTo(root, 32),
			Slot:            slot,
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
	pa, err := payloadatt.NewReadOnly(att)
	require.NoError(t, err)
	return att, pa
}

// queueTestService builds a Service whose head state is available so the
// signature check in queuePendingPayloadAttestation can run. NotFinalized keeps
// InForkchoice false so the block isn't treated as already arrived.
func queueTestService(t *testing.T) *Service {
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	return &Service{
		pendingPayloadAttestations: make(map[[32]byte][]*ethpb.PayloadAttestationMessage),
		seenPendingBlocks:          make(map[[32]byte]bool),
		cfg:                        &config{chain: &mock.ChainService{State: st, NotFinalized: true, FinalizedCheckPoint: &ethpb.Checkpoint{}}, p2p: p2ptest.NewTestP2P(t)},
	}
}

// validSigVerifier is a verifier whose signature check passes.
func validSigVerifier() verification.PayloadAttestationMsgVerifier {
	return &verification.MockPayloadAttestation{}
}

func TestQueuePendingPayloadAttestation_Queues(t *testing.T) {
	s := queueTestService(t)

	att, pa := pendingPayloadAtt(t, []byte{'a'}, 1, 1)
	res, err := s.queuePendingPayloadAttestation(t.Context(), validSigVerifier(), att)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, res)
	require.Equal(t, 1, len(s.pendingPayloadAttestations[pa.BeaconBlockRoot()]))
}

func TestQueuePendingPayloadAttestation_DeduplicatesByValidator(t *testing.T) {
	s := queueTestService(t)

	att, pa := pendingPayloadAtt(t, []byte{'a'}, 1, 1)
	_, err := s.queuePendingPayloadAttestation(t.Context(), validSigVerifier(), att)
	require.NoError(t, err)
	_, err = s.queuePendingPayloadAttestation(t.Context(), validSigVerifier(), att)
	require.NoError(t, err)
	require.Equal(t, 1, len(s.pendingPayloadAttestations[pa.BeaconBlockRoot()]))

	// A different validator for the same root is queued.
	att2, pa2 := pendingPayloadAtt(t, []byte{'a'}, 2, 1)
	_, err = s.queuePendingPayloadAttestation(t.Context(), validSigVerifier(), att2)
	require.NoError(t, err)
	require.Equal(t, 2, len(s.pendingPayloadAttestations[pa2.BeaconBlockRoot()]))
}

func TestQueuePendingPayloadAttestation_RootCap(t *testing.T) {
	s := queueTestService(t)

	for i := range maxPendingPayloadAttestationRoots {
		att, _ := pendingPayloadAtt(t, []byte{byte(i)}, 1, 1)
		_, err := s.queuePendingPayloadAttestation(t.Context(), validSigVerifier(), att)
		require.NoError(t, err)
	}
	require.Equal(t, maxPendingPayloadAttestationRoots, len(s.pendingPayloadAttestations))

	// A new root beyond the cap is ignored.
	att, pa := pendingPayloadAtt(t, []byte("overflow"), 1, 1)
	_, err := s.queuePendingPayloadAttestation(t.Context(), validSigVerifier(), att)
	require.NoError(t, err)
	require.Equal(t, maxPendingPayloadAttestationRoots, len(s.pendingPayloadAttestations))
	_, ok := s.pendingPayloadAttestations[pa.BeaconBlockRoot()]
	require.Equal(t, false, ok)
}

func TestQueuePendingPayloadAttestation_RejectsBadSignature(t *testing.T) {
	s := queueTestService(t)
	v := &verification.MockPayloadAttestation{ErrInvalidMessageSignature: errors.New("bad signature")}

	att, _ := pendingPayloadAtt(t, []byte{'a'}, 1, 1)
	res, err := s.queuePendingPayloadAttestation(t.Context(), v, att)
	require.ErrorContains(t, "bad signature", err)
	require.Equal(t, pubsub.ValidationReject, res)
	require.Equal(t, 0, len(s.pendingPayloadAttestations))
}

func TestQueuePendingPayloadAttestation_RejectsNonCommittee(t *testing.T) {
	s := queueTestService(t)
	v := &verification.MockPayloadAttestation{ErrIncorrectPayloadAttValidator: errors.New("not in PTC")}

	att, _ := pendingPayloadAtt(t, []byte{'a'}, 1, 1)
	res, err := s.queuePendingPayloadAttestation(t.Context(), v, att)
	require.ErrorContains(t, "not in PTC", err)
	require.Equal(t, pubsub.ValidationReject, res)
	require.Equal(t, 0, len(s.pendingPayloadAttestations))
}

func TestQueuePendingPayloadAttestation_DropsWhenNoState(t *testing.T) {
	s := &Service{
		pendingPayloadAttestations: make(map[[32]byte][]*ethpb.PayloadAttestationMessage),
		seenPendingBlocks:          make(map[[32]byte]bool),
		cfg:                        &config{chain: &mock.ChainService{HeadStateErr: errors.New("no state")}},
	}

	att, _ := pendingPayloadAtt(t, []byte{'a'}, 1, 1)
	res, err := s.queuePendingPayloadAttestation(t.Context(), validSigVerifier(), att)
	require.ErrorContains(t, "no state", err)
	require.Equal(t, pubsub.ValidationIgnore, res)
	require.Equal(t, 0, len(s.pendingPayloadAttestations))
}

func TestQueuePendingPayloadAttestation_DrainsWhenBlockInForkchoice(t *testing.T) {
	st, _ := util.DeterministicGenesisStateGloas(t, 64)
	ptc, err := st.PayloadCommitteeReadOnly(0)
	require.NoError(t, err)
	require.NotEmpty(t, ptc)

	pool := payloadattestation.NewPool()
	s := &Service{
		payloadAttestationCache:    &cache.PayloadAttestationCache{},
		pendingPayloadAttestations: make(map[[32]byte][]*ethpb.PayloadAttestationMessage),
		seenPendingBlocks:          make(map[[32]byte]bool),
		cfg: &config{
			chain:                  &mock.ChainService{State: st}, // InForkchoice true: block already arrived.
			p2p:                    p2ptest.NewTestP2P(t),
			payloadAttestationPool: pool,
			operationNotifier:      &mock.MockOperationNotifier{},
		},
	}
	s.newPayloadAttestationVerifier = func(pa payloadatt.ROMessage, reqs []verification.Requirement) verification.PayloadAttestationMsgVerifier {
		return &verification.MockPayloadAttestation{}
	}

	att, _ := pendingPayloadAtt(t, []byte{'a'}, ptc[0], 0)
	res, err := s.queuePendingPayloadAttestation(t.Context(), validSigVerifier(), att)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationIgnore, res)

	// The self-drain runs asynchronously; poll for the attestation to be processed.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(pool.PendingPayloadAttestations(0)) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, 1, len(pool.PendingPayloadAttestations(0)))
}

func TestPrunePendingPayloadAttestations(t *testing.T) {
	s := &Service{
		pendingPayloadAttestations: make(map[[32]byte][]*ethpb.PayloadAttestationMessage),
		cfg:                        &config{clock: startup.NewClock(genesisForSlot(10), [32]byte{})},
	}

	staleAtt, stalePa := pendingPayloadAtt(t, []byte{'a'}, 1, 1)
	s.pendingPayloadAttestations[stalePa.BeaconBlockRoot()] = []*ethpb.PayloadAttestationMessage{staleAtt}
	currentSlot := s.cfg.clock.CurrentSlot()
	freshAtt, freshPa := pendingPayloadAtt(t, []byte{'b'}, 1, currentSlot)
	s.pendingPayloadAttestations[freshPa.BeaconBlockRoot()] = []*ethpb.PayloadAttestationMessage{freshAtt}

	s.prunePendingPayloadAttestations()

	_, staleExists := s.pendingPayloadAttestations[stalePa.BeaconBlockRoot()]
	require.Equal(t, false, staleExists)
	_, freshExists := s.pendingPayloadAttestations[freshPa.BeaconBlockRoot()]
	require.Equal(t, true, freshExists)
}

func TestProcessPendingPayloadAttestation_DrainsAndProcesses(t *testing.T) {
	st, _ := util.DeterministicGenesisStateGloas(t, 64)
	ptc, err := st.PayloadCommitteeReadOnly(0)
	require.NoError(t, err)
	require.NotEmpty(t, ptc)

	pool := payloadattestation.NewPool()
	s := &Service{
		payloadAttestationCache:    &cache.PayloadAttestationCache{},
		pendingPayloadAttestations: make(map[[32]byte][]*ethpb.PayloadAttestationMessage),
		cfg: &config{
			chain:                  &mock.ChainService{State: st, Genesis: genesisForSlot(0)},
			p2p:                    p2ptest.NewTestP2P(t),
			clock:                  startup.NewClock(genesisForSlot(0), [32]byte{}),
			payloadAttestationPool: pool,
			operationNotifier:      &mock.MockOperationNotifier{},
		},
	}
	s.newPayloadAttestationVerifier = func(pa payloadatt.ROMessage, reqs []verification.Requirement) verification.PayloadAttestationMsgVerifier {
		return &verification.MockPayloadAttestation{}
	}

	root := []byte{'a'}
	att, pa := pendingPayloadAtt(t, root, ptc[0], 0)
	s.pendingPayloadAttestations[pa.BeaconBlockRoot()] = []*ethpb.PayloadAttestationMessage{att}

	s.processPendingPayloadAttestation(t.Context(), pa.BeaconBlockRoot())

	_, stillQueued := s.pendingPayloadAttestations[pa.BeaconBlockRoot()]
	require.Equal(t, false, stillQueued)
	require.Equal(t, 1, len(pool.PendingPayloadAttestations(0)))
}
