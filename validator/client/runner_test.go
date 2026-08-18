package client

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"

	"github.com/OffchainLabs/prysm/v7/async/event"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/interop"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/wallet"
	testing2 "github.com/OffchainLabs/prysm/v7/validator/db/testing"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/local"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/emptypb"
)

// walletBackedKeymanager builds a local-keymanager wallet preloaded with numKeys
// deterministic keys, so the runner's WaitForKeymanagerInitialization resolves a
// real keymanager from the wallet path.
func walletBackedKeymanager(t *testing.T, ctx context.Context, numKeys uint64) *wallet.Wallet {
	privs, pubs, err := interop.DeterministicallyGenerateKeys(0, numKeys)
	require.NoError(t, err)
	w := wallet.New(&wallet.Config{
		WalletDir:      t.TempDir(),
		KeymanagerKind: keymanager.Local,
		WalletPassword: "TestWalletPassword123!",
	})
	require.NoError(t, w.SaveWallet())
	km, err := local.NewKeymanager(ctx, &local.SetupConfig{Wallet: w})
	require.NoError(t, err)
	privBytes := make([][]byte, numKeys)
	pubBytes := make([][]byte, numKeys)
	for i := range privs {
		privBytes[i] = privs[i].Marshal()
		pubBytes[i] = pubs[i].Marshal()
	}
	require.NoError(t, km.ImportKeypairs(ctx, privBytes, pubBytes))
	return w
}

// runnerTestValidator builds a real validator wired to mocked beacon-node clients, so
// what the runner does is observable as the RPCs it drives.
func runnerTestValidator(t *testing.T, ctx context.Context) (*validator, *validatormock.MockValidatorClient, *validatormock.MockNodeClient) {
	ctrl := gomock.NewController(t)
	vc := validatormock.NewMockValidatorClient(ctrl)
	nc := validatormock.NewMockNodeClient(ctrl)
	v := &validator{
		validatorClient:              vc,
		nodeClient:                   nc,
		db:                           testing2.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{}, false),
		wallet:                       walletBackedKeymanager(t, ctx, 1),
		duties:                       &dutyStore{},
		slotFeed:                     &event.Feed{},
		pubkeyToStatus:               make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
		signedValidatorRegistrations: make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
		submittedAtts:                make(map[submittedAttKey]*submittedAtt),
		submittedAggregates:          make(map[submittedAttKey]*submittedAtt),
		attestedSlotsByKeyByEpoch:    make(map[primitives.Epoch]map[[fieldparams.BLSPubkeyLength]byte]primitives.Slot),
		accountsChangedChannel:       make(chan [][fieldparams.BLSPubkeyLength]byte, 1),
	}
	v.aggSelector = testLocalSelector(t, v)
	return v, vc, nc
}

// expectChainStart answers the genesis handshake initialize() starts with.
func expectChainStart(vc *validatormock.MockValidatorClient) *gomock.Call {
	return vc.EXPECT().WaitForChainStart(gomock.Any(), gomock.Any()).Return(&ethpb.ChainStartResponse{
		Started:               true,
		GenesisTime:           uint64(time.Now().Unix()) - params.BeaconConfig().SecondsPerSlot,
		GenesisValidatorsRoot: make([]byte, fieldparams.RootLength),
	}, nil)
}

// expectStatuses answers status lookups with the given status for every key asked about.
func expectStatuses(vc *validatormock.MockValidatorClient, status ethpb.ValidatorStatus) *gomock.Call {
	return vc.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *ethpb.MultipleValidatorStatusRequest) (*ethpb.MultipleValidatorStatusResponse, error) {
			res := &ethpb.MultipleValidatorStatusResponse{}
			for i, pk := range req.PublicKeys {
				res.PublicKeys = append(res.PublicKeys, pk)
				res.Statuses = append(res.Statuses, &ethpb.ValidatorStatusResponse{Status: status})
				res.Indices = append(res.Indices, primitives.ValidatorIndex(i))
			}
			return res, nil
		})
}

// TestInitialize covers what the cancelled-context tests used to: the whole startup
// sequence runs and leaves the validator ready to attest.
func TestInitialize(t *testing.T) {
	t.Run("leaves the validator ready to attest", func(t *testing.T) {
		ctx := t.Context()
		v, vc, nc := runnerTestValidator(t, ctx)
		expectChainStart(vc)
		nc.EXPECT().SyncStatus(gomock.Any(), gomock.Any()).Return(&ethpb.SyncStatus{Syncing: false}, nil)
		expectStatuses(vc, ethpb.ValidatorStatus_ACTIVE).AnyTimes()

		require.NoError(t, initialize(ctx, v))

		assert.Equal(t, false, v.genesisTime.IsZero(), "Expected genesis time from the chain start response")
		require.NotNil(t, v.km, "Expected the keymanager to be initialized from the wallet")
		keys, err := v.km.FetchValidatingPublicKeys(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, len(keys))
		_, err = v.indexFromPubkey(keys[0])
		require.NoError(t, err, "Expected the status cache to be populated for the validating key")
	})

	t.Run("logs a duties failure instead of failing", func(t *testing.T) {
		hook := logTest.NewGlobal()
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		v, vc, nc := runnerTestValidator(t, ctx)
		expectChainStart(vc)
		nc.EXPECT().SyncStatus(gomock.Any(), gomock.Any()).Return(&ethpb.SyncStatus{Syncing: false}, nil)
		expectStatuses(vc, ethpb.ValidatorStatus_ACTIVE).AnyTimes()
		vc.EXPECT().ConnectionGeneration().Return(uint64(0)).AnyTimes()
		vc.EXPECT().Duties(gomock.Any(), gomock.Any()).Return(nil, errors.New("bad")).AnyTimes()
		vc.EXPECT().SubmitValidatorRegistrations(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		vc.EXPECT().PrepareBeaconProposer(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		vc.EXPECT().DomainData(gomock.Any(), gomock.Any()).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil).AnyTimes()

		_, err := newRunner(ctx, v, &healthMonitor{isHealthy: true})
		require.NoError(t, err) // duties failures are logged, not fatal
		require.LogsContain(t, hook, "Failed to update assignments")
	})
}

func TestRetry_On_ConnectionError(t *testing.T) {
	retry := 10
	ctx := t.Context()
	v, vc, nc := runnerTestValidator(t, ctx)

	// Each failure sends initialize() back to the top of its loop, so the steps before
	// the failing one run again.
	var chainStart, sync, activation atomic.Int32
	vc.EXPECT().WaitForChainStart(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty) (*ethpb.ChainStartResponse, error) {
			if int(chainStart.Add(1)) <= retry {
				return nil, io.EOF
			}
			return &ethpb.ChainStartResponse{
				Started:               true,
				GenesisTime:           uint64(time.Now().Unix()) - params.BeaconConfig().SecondsPerSlot,
				GenesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			}, nil
		}).AnyTimes()
	nc.EXPECT().SyncStatus(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty) (*ethpb.SyncStatus, error) {
			if int(sync.Add(1)) <= retry {
				return nil, errors.New("no connection")
			}
			return &ethpb.SyncStatus{Syncing: false}, nil
		}).AnyTimes()
	expectStatuses(vc, ethpb.ValidatorStatus_ACTIVE).Do(func(any, any) { activation.Add(1) }).AnyTimes()

	backOffPeriod = 10 * time.Millisecond
	require.NoError(t, initialize(ctx, v))

	// every call will fail retry=10 times so first one will be called 2 * retry=10 + 1.
	assert.Equal(t, int32(retry*2+1), chainStart.Load(), "Expected WaitForChainStart() to be retried")
	assert.Equal(t, int32(retry+1), sync.Load(), "Expected WaitForSync() to be retried")
	assert.Equal(t, int32(1), activation.Load(), "Expected WaitForActivation() to be reached once")
}

func TestRun_ExitsOnCancelledContext(t *testing.T) {
	ctx := t.Context()
	v, vc, nc := runnerTestValidator(t, ctx)
	expectChainStart(vc)
	nc.EXPECT().SyncStatus(gomock.Any(), gomock.Any()).Return(&ethpb.SyncStatus{Syncing: false}, nil)
	expectStatuses(vc, ethpb.ValidatorStatus_ACTIVE).AnyTimes()
	vc.EXPECT().ConnectionGeneration().Return(uint64(0)).AnyTimes()
	vc.EXPECT().Duties(gomock.Any(), gomock.Any()).Return(&ethpb.ValidatorDutiesContainer{}, nil).AnyTimes()
	vc.EXPECT().SubmitValidatorRegistrations(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	vc.EXPECT().PrepareBeaconProposer(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	vc.EXPECT().DomainData(gomock.Any(), gomock.Any()).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil).AnyTimes()
	vc.EXPECT().SubscribeCommitteeSubnets(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	r, err := newRunner(ctx, v, &healthMonitor{isHealthy: true})
	require.NoError(t, err)

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	done := make(chan struct{})
	go func() {
		r.run(cancelled)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return on a cancelled context")
	}
	assert.Equal(t, true, v.ticker != nil, "Expected the slot ticker to have been set")
}

func TestMaybeFetchNextDuties_Gating(t *testing.T) {
	spe := params.BeaconConfig().SlotsPerEpoch
	fetchSlot := nextDutiesFetchSlot()
	tests := []struct {
		name string
		slot primitives.Slot
		want bool
	}{
		{"early slot skips next fetch", 1, false},
		{"slot before threshold skips next fetch", fetchSlot - 1, false},
		{"threshold slot fetches next", fetchSlot, true},
		{"mid-epoch slot fetches next", spe/2 + 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldFetchNextDuties(tt.slot))
		})
	}
}

// TestKeyReload drives onAccountsChanged. The status lookup decides whether the
// reloaded key is active, so the key bytes themselves do not matter.
func TestKeyReload(t *testing.T) {
	reloaded := [][fieldparams.BLSPubkeyLength]byte{{1}}

	t.Run("active key needs no activation wait", func(t *testing.T) {
		ctx := t.Context()
		v, vc, _ := runnerTestValidator(t, ctx)
		// One status lookup: the key is active, so no activation wait follows.
		expectStatuses(vc, ethpb.ValidatorStatus_ACTIVE).Times(1)

		onAccountsChanged(ctx, v, reloaded)
	})

	t.Run("inactive key waits for activation", func(t *testing.T) {
		hook := logTest.NewGlobal()
		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		t.Cleanup(cancel)
		v, vc, _ := runnerTestValidator(t, ctx)
		require.NoError(t, v.WaitForKeymanagerInitialization(ctx))
		// The key is not active, so the reload is followed by a wait for activation that
		// looks the statuses up again and then blocks until the context expires.
		expectStatuses(vc, ethpb.ValidatorStatus_UNKNOWN_STATUS).MinTimes(2)

		onAccountsChanged(ctx, v, reloaded)

		require.LogsContain(t, hook, "Could not wait for validator activation")
	})
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
	cfg.SlotDurationMilliseconds = 1000
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
	vcm.EXPECT().ConnectionGeneration().Return(uint64(0)).AnyTimes()
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
	vcm.EXPECT().SubscribeCommitteeSubnets(liveCtx, gomock.Any()).AnyTimes().Do(func(_, _ any) { delay(t) })
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
		wallet:          walletBackedKeymanager(t, timedCtx, uint64(params.BeaconConfig().SlotsPerEpoch)*4),
		proposerSettings: &proposer.Settings{
			ProposeConfig: make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option),
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{
					FeeRecipient: common.BytesToAddress([]byte{1}),
				},
				BuilderConfig: &proposer.BuilderConfig{
					Enabled:  true,
					GasLimit: 60_000_000,
				},
				GraffitiConfig: &proposer.GraffitiConfig{
					Graffiti: "foobar",
				},
			},
		},
		signedValidatorRegistrations: make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
		duties:                       &dutyStore{},
		slotFeed:                     &event.Feed{},
		submittedAtts:                make(map[submittedAttKey]*submittedAtt),
		submittedAggregates:          make(map[submittedAttKey]*submittedAtt),
		attestedSlotsByKeyByEpoch:    make(map[primitives.Epoch]map[[fieldparams.BLSPubkeyLength]byte]primitives.Slot),
	}
	v.aggSelector = testLocalSelector(t, v)

	r, err := newRunner(timedCtx, v, &healthMonitor{isHealthy: true})
	require.NoError(t, err)
	r.run(timedCtx)
}

func TestPerformRolesDispatch(t *testing.T) {
	cfg := params.BeaconConfig()
	cfg.ElectraForkEpoch = 1
	cfg.GloasForkEpoch = 2
	params.SetActiveTestCleanup(t, cfg)

	stop := errors.New("stop after dispatch")
	tests := []struct {
		name   string
		role   validatorRole
		slot   primitives.Slot
		expect func(*validator, *mocks, [fieldparams.BLSPubkeyLength]byte)
	}{
		{
			name: "attester",
			role: roleAttester,
			slot: 1,
			expect: func(_ *validator, m *mocks, _ [fieldparams.BLSPubkeyLength]byte) {
				m.validatorClient.EXPECT().AttestationData(gomock.Any(), gomock.Any()).Return(nil, stop).Times(1)
			},
		},
		{
			name: "proposer",
			role: roleProposer,
			slot: 1,
			expect: func(_ *validator, m *mocks, _ [fieldparams.BLSPubkeyLength]byte) {
				m.validatorClient.EXPECT().DomainData(gomock.Any(), gomock.Any()).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, fieldparams.RootLength)}, nil).Times(1)
				m.validatorClient.EXPECT().BeaconBlock(gomock.Any(), gomock.Any()).Return(nil, stop).Times(1)
			},
		},
		{
			name: "aggregator",
			role: roleAggregator,
			slot: 1,
			expect: func(v *validator, m *mocks, pubKey [fieldparams.BLSPubkeyLength]byte) {
				v.aggSelector = &stubAggregatorSelector{proofs: map[[fieldparams.BLSPubkeyLength]byte][]byte{pubKey: {1}}}
				m.validatorClient.EXPECT().SubmitAggregateSelectionProof(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, stop).Times(1)
			},
		},
		{
			name: "sync committee",
			role: roleSyncCommittee,
			slot: 1,
			expect: func(_ *validator, m *mocks, _ [fieldparams.BLSPubkeyLength]byte) {
				m.validatorClient.EXPECT().SyncMessageBlockRoot(gomock.Any(), gomock.Any()).Return(nil, stop).Times(1)
			},
		},
		{
			name: "sync committee aggregator",
			role: roleSyncCommitteeAggregator,
			slot: 1,
			expect: func(_ *validator, m *mocks, _ [fieldparams.BLSPubkeyLength]byte) {
				m.validatorClient.EXPECT().SyncSubcommitteeIndex(gomock.Any(), gomock.Any()).Return(nil, stop).Times(1)
			},
		},
		{
			name: "PTC member",
			role: rolePTCMember,
			slot: cfg.SlotsPerEpoch.Mul(uint64(cfg.GloasForkEpoch)),
			expect: func(_ *validator, m *mocks, _ [fieldparams.BLSPubkeyLength]byte) {
				m.validatorClient.EXPECT().PayloadAttestationData(gomock.Any(), gomock.Any()).Return(nil, stop).Times(1)
			},
		},
		{
			name: "unknown",
			role: roleUnknown,
			slot: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, m, validatorKey, finish := setup(t, false)
			defer finish()
			pubKey := bytesutil.ToBytes48(validatorKey.PublicKey().Marshal())
			v.duties = testDutyStore(&ethpb.ValidatorDuty{
				PublicKey:       pubKey[:],
				ValidatorIndex:  1,
				CommitteeLength: 1,
			})
			if tt.expect != nil {
				tt.expect(v, m, pubKey)
			}

			var wg sync.WaitGroup
			performRoles(t.Context(), map[[fieldparams.BLSPubkeyLength]byte][]validatorRole{pubKey: {tt.role}}, v, tt.slot, &wg, trace.SpanFromContext(t.Context()))
			wg.Wait()
		})
	}
}
