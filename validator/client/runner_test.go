package client

import (
	"context"
	"fmt"
	"math/bits"
	"math/rand"
	"runtime/debug"
	"strconv"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/config/proposer"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	validatormock "github.com/OffchainLabs/prysm/v6/testing/validator-mock"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	"github.com/OffchainLabs/prysm/v6/validator/client/testutil"
	testing2 "github.com/OffchainLabs/prysm/v6/validator/db/testing"
	"github.com/OffchainLabs/prysm/v6/validator/keymanager/local"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
)

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// Helper function to run the validator runner for tests
func runTest(t *testing.T, ctx context.Context, v iface.Validator) {
	r, err := newRunner(ctx, v, &healthMonitor{isHealthy: true})
	require.NoError(t, err)
	r.run(ctx)
}

func TestCancelledContext_CleansUpValidator(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	v := &testutil.FakeValidator{
		Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}},
	}
	runTest(t, cancelledContext(), v)
	assert.Equal(t, true, v.DoneCalled, "Expected Done() to be called")
}

func TestCancelledContext_WaitsForChainStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	v := &testutil.FakeValidator{
		Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}},
	}
	runTest(t, cancelledContext(), v)
	assert.Equal(t, 1, v.WaitForChainStartCalled, "Expected WaitForChainStart() to be called")
}

func TestRetry_On_ConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	retry := 10
	v := &testutil.FakeValidator{
		Km:               &mockKeymanager{accountsChangedFeed: &event.Feed{}},
		RetryTillSuccess: retry,
	}
	backOffPeriod = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(t.Context())
	go runTest(t, ctx, v)

	// each step will fail (retry times)=10 this sleep times will wait more then
	// the time it takes for all steps to succeed before main loop.
	time.Sleep(time.Duration(retry*6) * backOffPeriod)
	cancel()
	// every call will fail retry=10 times so first one will be called 4 * retry=10.
	assert.Equal(t, retry*2+1, v.WaitForChainStartCalled, "Expected WaitForChainStart() to be called")
	assert.Equal(t, retry+1, v.WaitForSyncCalled, "Expected WaitForSync() to be called")
	assert.Equal(t, 1, v.WaitForActivationCalled, "Expected WaitForActivation() to be called")
}

func TestCancelledContext_WaitsForActivation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	v := &testutil.FakeValidator{
		Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}},
	}
	runTest(t, cancelledContext(), v)
	assert.Equal(t, 1, v.WaitForActivationCalled, "Expected WaitForActivation() to be called")
}

func TestUpdateDuties_NextSlot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}}
	ctx, cancel := context.WithCancel(t.Context())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	go func() {
		ticker <- slot

		cancel()
	}()

	runTest(t, ctx, v)

	require.Equal(t, true, v.UpdateDutiesCalled, "Expected UpdateAssignments(%d) to be called", slot)
}

func TestUpdateDuties_HandlesError(t *testing.T) {
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}}
	ctx, cancel := context.WithCancel(t.Context())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	go func() {
		ticker <- slot

		cancel()
	}()
	v.UpdateDutiesRet = errors.New("bad")

	runTest(t, ctx, v)

	require.LogsContain(t, hook, "Failed to update assignments")
}

func TestRoleAt_NextSlot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}}
	ctx, cancel := context.WithCancel(t.Context())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	go func() {
		ticker <- slot

		cancel()
	}()

	runTest(t, ctx, v)

	require.Equal(t, true, v.RoleAtCalled, "Expected RoleAt(%d) to be called", slot)
	assert.Equal(t, uint64(slot), v.RoleAtArg1, "RoleAt called with the wrong arg")
}

func TestAttests_NextSlot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	attSubmitted := make(chan interface{})
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}, AttSubmitted: attSubmitted}
	ctx, cancel := context.WithCancel(t.Context())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	v.RolesAtRet = []iface.ValidatorRole{iface.RoleAttester}
	go func() {
		ticker <- slot

		cancel()
	}()
	runTest(t, ctx, v)
	<-attSubmitted
	require.Equal(t, true, v.AttestToBlockHeadCalled, "SubmitAttestation(%d) was not called", slot)
	assert.Equal(t, uint64(slot), v.AttestToBlockHeadArg1, "SubmitAttestation was called with wrong arg")
}

func TestProposes_NextSlot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	blockProposed := make(chan interface{})
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}, BlockProposed: blockProposed}
	ctx, cancel := context.WithCancel(t.Context())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	v.RolesAtRet = []iface.ValidatorRole{iface.RoleProposer}
	go func() {
		ticker <- slot

		cancel()
	}()
	runTest(t, ctx, v)
	<-blockProposed

	require.Equal(t, true, v.ProposeBlockCalled, "ProposeBlock(%d) was not called", slot)
	assert.Equal(t, uint64(slot), v.ProposeBlockArg1, "ProposeBlock was called with wrong arg")
}

func TestBothProposesAndAttests_NextSlot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	blockProposed := make(chan interface{})
	attSubmitted := make(chan interface{})
	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}, BlockProposed: blockProposed, AttSubmitted: attSubmitted}
	ctx, cancel := context.WithCancel(t.Context())

	slot := primitives.Slot(55)
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	v.RolesAtRet = []iface.ValidatorRole{iface.RoleAttester, iface.RoleProposer}
	go func() {
		ticker <- slot

		cancel()
	}()
	runTest(t, ctx, v)
	<-blockProposed
	<-attSubmitted
	require.Equal(t, true, v.AttestToBlockHeadCalled, "SubmitAttestation(%d) was not called", slot)
	assert.Equal(t, uint64(slot), v.AttestToBlockHeadArg1, "SubmitAttestation was called with wrong arg")
	require.Equal(t, true, v.ProposeBlockCalled, "ProposeBlock(%d) was not called", slot)
	assert.Equal(t, uint64(slot), v.ProposeBlockArg1, "ProposeBlock was called with wrong arg")
}

func TestKeyReload_ActiveKey(t *testing.T) {
	ctx := t.Context()
	km := &mockKeymanager{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	v := &testutil.FakeValidator{Km: km, AccountsChannel: make(chan [][fieldparams.BLSPubkeyLength]byte)}
	current := [][fieldparams.BLSPubkeyLength]byte{testutil.ActiveKey}
	onAccountsChanged(ctx, v, current)
	assert.Equal(t, true, v.HandleKeyReloadCalled)
	// HandleKeyReloadCalled in the FakeValidator returns true if one of the keys is equal to the
	// ActiveKey. WaitForActivation is only called if none of the keys are active, so it shouldn't be called at all.
	assert.Equal(t, 0, v.WaitForActivationCalled)
}

func TestKeyReload_NoActiveKey(t *testing.T) {
	na := notActive(t)
	ctx := t.Context()
	km := &mockKeymanager{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	v := &testutil.FakeValidator{Km: km, AccountsChannel: make(chan [][fieldparams.BLSPubkeyLength]byte)}
	current := [][fieldparams.BLSPubkeyLength]byte{na}
	onAccountsChanged(ctx, v, current)
	assert.Equal(t, true, v.HandleKeyReloadCalled)
	// HandleKeyReloadCalled in the FakeValidator returns true if one of the keys is equal to the
	// ActiveKey. Since we are using a key we know is not active, it should return false, which
	// should cause the account change handler to call WaitForActivationCalled.
	assert.Equal(t, 1, v.WaitForActivationCalled)
}

func notActive(t *testing.T) [fieldparams.BLSPubkeyLength]byte {
	var r [fieldparams.BLSPubkeyLength]byte
	copy(r[:], testutil.ActiveKey[:])
	for i := 0; i < len(r); i++ {
		r[i] = bits.Reverse8(r[i])
	}
	require.DeepNotEqual(t, r, testutil.ActiveKey)
	return r
}

func TestUpdateProposerSettingsAt_EpochStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	v := &testutil.FakeValidator{Km: &mockKeymanager{accountsChangedFeed: &event.Feed{}}}
	err := v.SetProposerSettings(t.Context(), &proposer.Settings{
		DefaultConfig: &proposer.Option{
			FeeRecipientConfig: &proposer.FeeRecipientConfig{
				FeeRecipient: common.HexToAddress("0x046Fb65722E7b2455012BFEBf6177F1D2e9738D9"),
			},
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	hook := logTest.NewGlobal()
	slot := params.BeaconConfig().SlotsPerEpoch
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	go func() {
		ticker <- slot

		cancel()
	}()

	runTest(t, ctx, v)
	assert.LogsContain(t, hook, "updated proposer settings")
}

func TestUpdateProposerSettingsAt_EpochEndOk(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	v := &testutil.FakeValidator{
		Km:                  &mockKeymanager{accountsChangedFeed: &event.Feed{}},
		ProposerSettingWait: time.Duration(params.BeaconConfig().SecondsPerSlot-1) * time.Second,
	}
	err := v.SetProposerSettings(t.Context(), &proposer.Settings{
		DefaultConfig: &proposer.Option{
			FeeRecipientConfig: &proposer.FeeRecipientConfig{
				FeeRecipient: common.HexToAddress("0x046Fb65722E7b2455012BFEBf6177F1D2e9738D9"),
			},
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	hook := logTest.NewGlobal()
	slot := params.BeaconConfig().SlotsPerEpoch - 1 //have it set close to the end of epoch
	ticker := make(chan primitives.Slot)
	v.NextSlotRet = ticker
	go func() {
		ticker <- slot
		cancel()
	}()

	runTest(t, ctx, v)
	// can't test "Failed to update proposer settings" because of log.fatal
	assert.LogsContain(t, hook, "Mock updated proposer settings")
}

type tlogger struct {
	t testing.TB
}

func (t tlogger) Write(p []byte) (n int, err error) {
	t.t.Log(fmt.Sprintf("%s", p))
	return len(p), nil
}

func delay(t testing.TB) {
	const timeout = 100 * time.Millisecond

	select {
	case <-t.Context().Done():
		return
	case <-time.After(timeout):
		return
	}
}

// assertValidContext, but only when the parent context is still valid. This is testing that mocked methods are called
// and maintain a valid context while processing, except when the test is shutting down.
func assertValidContext(t testing.TB, parent, ctx context.Context) {
	if ctx.Err() != nil && parent.Err() == nil && t.Context().Err() == nil {
		t.Logf("stack: %s", debug.Stack())
		t.Fatalf("Context is no longer valid during a mocked RPC call: %v", ctx.Err())
	}
}

func TestRunnerPushesProposerSettings_ValidContext(t *testing.T) {
	logrus.SetOutput(tlogger{t})

	cfg := params.BeaconConfig()
	cfg.SecondsPerSlot = 1
	params.SetActiveTestCleanup(t, cfg)

	timedCtx, cancel := context.WithTimeout(t.Context(), 1*time.Minute)
	defer cancel()

	// This test is meant to ensure that PushProposerSettings is called successfully on a next slot event.
	// This is a regresion test for PR 15369, however the same methodology of context checking is applied
	// to many other methods as well.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	// We want to test that mocked methods are called with a live context, but only while the timed context is valid.
	liveCtx := gomock.Cond(func(ctx context.Context) bool { return ctx.Err() == nil || timedCtx.Err() != nil })
	// Mocked client(s) setup.
	vcm := validatormock.NewMockValidatorClient(ctrl)
	vcm.EXPECT().WaitForChainStart(liveCtx, gomock.Any()).Return(&ethpb.ChainStartResponse{
		GenesisTime: uint64(time.Now().Unix()) - params.BeaconConfig().SecondsPerSlot,
	}, nil)
	vcm.EXPECT().MultipleValidatorStatus(liveCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, req *ethpb.MultipleValidatorStatusRequest) (*ethpb.MultipleValidatorStatusResponse, error) {
		defer assertValidContext(t, timedCtx, ctx)
		res := &ethpb.MultipleValidatorStatusResponse{}
		for i, pk := range req.PublicKeys {
			res.PublicKeys = append(res.PublicKeys, pk)
			res.Statuses = append(res.Statuses, &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE})
			res.Indices = append(res.Indices, primitives.ValidatorIndex(i))
		}
		return res, nil
	}).AnyTimes()
	vcm.EXPECT().Duties(liveCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, req *ethpb.DutiesRequest) (*ethpb.ValidatorDutiesContainer, error) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)

		s := slots.UnsafeEpochStart(req.Epoch)
		res := &ethpb.ValidatorDutiesContainer{}
		for i, pk := range req.PublicKeys {
			var ps []primitives.Slot
			if i < int(params.BeaconConfig().SlotsPerEpoch) {
				ps = []primitives.Slot{s + primitives.Slot(i)}
			}
			res.CurrentEpochDuties = append(res.CurrentEpochDuties, &ethpb.ValidatorDuty{
				CommitteeLength:  uint64(len(req.PublicKeys)),
				CommitteeIndex:   0,
				AttesterSlot:     s + primitives.Slot(i)%params.BeaconConfig().SlotsPerEpoch,
				ProposerSlots:    ps,
				PublicKey:        pk,
				Status:           ethpb.ValidatorStatus_ACTIVE,
				ValidatorIndex:   primitives.ValidatorIndex(i),
				IsSyncCommittee:  i%5 == 0,
				CommitteesAtSlot: 1,
			})
			res.NextEpochDuties = append(res.NextEpochDuties, &ethpb.ValidatorDuty{
				CommitteeLength:  uint64(len(req.PublicKeys)),
				CommitteeIndex:   0,
				AttesterSlot:     s + primitives.Slot(i)%params.BeaconConfig().SlotsPerEpoch + params.BeaconConfig().SlotsPerEpoch,
				ProposerSlots:    ps,
				PublicKey:        pk,
				Status:           ethpb.ValidatorStatus_ACTIVE,
				ValidatorIndex:   primitives.ValidatorIndex(i),
				IsSyncCommittee:  i%7 == 0,
				CommitteesAtSlot: 1,
			})
		}
		return res, nil
	}).AnyTimes()
	vcm.EXPECT().PrepareBeaconProposer(liveCtx, gomock.Any()).Return(nil, nil).AnyTimes().Do(func(ctx context.Context, _ any) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)
	})
	vcm.EXPECT().EventStreamIsRunning().Return(true).AnyTimes().Do(func() { delay(t) })
	vcm.EXPECT().SubmitValidatorRegistrations(liveCtx, gomock.Any()).Do(func(ctx context.Context, _ any) {
		defer assertValidContext(t, timedCtx, ctx) // This is the specific regression test assertion for PR 15369.
		delay(t)
	}).MinTimes(1)
	// DomainData calls are really fast, no delay needed.
	vcm.EXPECT().DomainData(liveCtx, gomock.Any()).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil).AnyTimes()
	vcm.EXPECT().SubscribeCommitteeSubnets(liveCtx, gomock.Any(), gomock.Any()).AnyTimes().Do(func(_, _, _ any) { delay(t) })
	vcm.EXPECT().AttestationData(liveCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, req *ethpb.AttestationDataRequest) (*ethpb.AttestationData, error) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)
		r := rand.New(rand.NewSource(123))
		root := bytesutil.PadTo([]byte("root_"+strconv.Itoa(r.Intn(100_000))), 32)
		root2 := bytesutil.PadTo([]byte("root_"+strconv.Itoa(r.Intn(100_000))), 32)
		ckpt := &ethpb.Checkpoint{Root: root2, Epoch: slots.ToEpoch(req.Slot)}
		return &ethpb.AttestationData{
			Slot:            req.Slot,
			CommitteeIndex:  req.CommitteeIndex,
			BeaconBlockRoot: root,
			Target:          ckpt,
			Source:          ckpt,
		}, nil
	}).AnyTimes()
	vcm.EXPECT().ProposeAttestation(liveCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, req *ethpb.Attestation) (*ethpb.AttestResponse, error) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)
		return &ethpb.AttestResponse{AttestationDataRoot: make([]byte, fieldparams.RootLength)}, nil
	}).AnyTimes()
	vcm.EXPECT().SubmitAggregateSelectionProof(liveCtx, gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, req *ethpb.AggregateSelectionRequest, index primitives.ValidatorIndex, committeeLength uint64) (*ethpb.AggregateSelectionResponse, error) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)
		ckpt := &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)}
		return &ethpb.AggregateSelectionResponse{
			AggregateAndProof: &ethpb.AggregateAttestationAndProof{
				AggregatorIndex: index,
				Aggregate: &ethpb.Attestation{
					Data:            &ethpb.AttestationData{Slot: req.Slot, BeaconBlockRoot: make([]byte, fieldparams.RootLength), Source: ckpt, Target: ckpt},
					AggregationBits: bitfield.Bitlist{0b00011111},
					Signature:       make([]byte, fieldparams.BLSSignatureLength),
				},
				SelectionProof: make([]byte, fieldparams.BLSSignatureLength),
			},
		}, nil
	}).AnyTimes()
	vcm.EXPECT().SubmitSignedAggregateSelectionProof(liveCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, req *ethpb.SignedAggregateSubmitRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)
		return &ethpb.SignedAggregateSubmitResponse{AttestationDataRoot: make([]byte, fieldparams.RootLength)}, nil
	}).AnyTimes()
	vcm.EXPECT().BeaconBlock(liveCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, req *ethpb.BlockRequest) (*ethpb.GenericBeaconBlock, error) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Electra{Electra: &ethpb.BeaconBlockContentsElectra{Block: util.HydrateBeaconBlockElectra(&ethpb.BeaconBlockElectra{})}}}, nil
	}).AnyTimes()
	vcm.EXPECT().ProposeBeaconBlock(liveCtx, gomock.Any()).AnyTimes().DoAndReturn(func(ctx context.Context, req *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)
		return &ethpb.ProposeResponse{BlockRoot: make([]byte, fieldparams.RootLength)}, nil
	})
	vcm.EXPECT().SyncSubcommitteeIndex(liveCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, req *ethpb.SyncSubcommitteeIndexRequest) (*ethpb.SyncSubcommitteeIndexResponse, error) {
		defer assertValidContext(t, timedCtx, ctx)
		//delay(t)
		return &ethpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{0}}, nil
	}).AnyTimes()
	vcm.EXPECT().SyncMessageBlockRoot(liveCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, _ any) (*ethpb.SyncMessageBlockRootResponse, error) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)
		return &ethpb.SyncMessageBlockRootResponse{Root: make([]byte, fieldparams.RootLength)}, nil
	}).AnyTimes()
	vcm.EXPECT().SubmitSyncMessage(liveCtx, gomock.Any()).Do(func(ctx context.Context, _ any) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)
	}).AnyTimes()
	vcm.EXPECT().SyncCommitteeContribution(liveCtx, gomock.Any()).DoAndReturn(func(ctx context.Context, req *ethpb.SyncCommitteeContributionRequest) (*ethpb.SyncCommitteeContribution, error) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)
		bits := bitfield.NewBitvector128()
		bits.SetBitAt(0, true)
		return &ethpb.SyncCommitteeContribution{Slot: req.Slot, BlockRoot: make([]byte, fieldparams.RootLength), SubcommitteeIndex: req.SubnetId, AggregationBits: bits, Signature: make([]byte, fieldparams.BLSSignatureLength)}, nil
	}).AnyTimes()
	vcm.EXPECT().SubmitSignedContributionAndProof(liveCtx, gomock.Any()).Do(func(ctx context.Context, _ any) {
		defer assertValidContext(t, timedCtx, ctx)
		delay(t)
	}).AnyTimes()
	ncm := validatormock.NewMockNodeClient(ctrl)
	ncm.EXPECT().SyncStatus(liveCtx, gomock.Any()).Return(&ethpb.SyncStatus{Syncing: false}, nil)

	// Setup the actual validator service.
	v := &validator{
		validatorClient: vcm,
		nodeClient:      ncm,
		db:              testing2.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{}, false),
		interopKeysConfig: &local.InteropKeymanagerConfig{
			NumValidatorKeys: uint64(params.BeaconConfig().SlotsPerEpoch) * 4, // 4 Attesters per slot.
		},
		proposerSettings: &proposer.Settings{
			ProposeConfig: make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option),
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{
					FeeRecipient: common.BytesToAddress([]byte{1}),
				},
				BuilderConfig: &proposer.BuilderConfig{
					Enabled:  true,
					GasLimit: 60_000_000,
					Relays:   []string{"https://example.com"},
				},
				GraffitiConfig: &proposer.GraffitiConfig{
					Graffiti: "foobar",
				},
			},
		},
		signedValidatorRegistrations:   make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
		slotFeed:                       &event.Feed{},
		aggregatedSlotCommitteeIDCache: lru.New(100),
		submittedAtts:                  make(map[submittedAttKey]*submittedAtt),
		submittedAggregates:            make(map[submittedAttKey]*submittedAtt),
	}

	runTest(t, timedCtx, v)
}
