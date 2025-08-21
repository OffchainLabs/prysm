package validator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/event"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache/depositsnapshot"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	mockExecution "github.com/OffchainLabs/prysm/v6/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/genesis"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/mock"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestValidatorIndex_OK(t *testing.T) {
	st, err := util.NewBeaconState()
	require.NoError(t, err)

	pubKey := pubKey(1)

	err = st.SetValidators([]*ethpb.Validator{{PublicKey: pubKey}})
	require.NoError(t, err)

	Server := &Server{
		HeadFetcher: &mockChain.ChainService{State: st},
	}

	req := &ethpb.ValidatorIndexRequest{
		PublicKey: pubKey,
	}
	_, err = Server.ValidatorIndex(t.Context(), req)
	assert.NoError(t, err, "Could not get validator index")
}

func TestValidatorIndex_StateEmpty(t *testing.T) {
	Server := &Server{
		HeadFetcher: &mockChain.ChainService{},
	}
	pubKey := pubKey(1)
	req := &ethpb.ValidatorIndexRequest{
		PublicKey: pubKey,
	}
	_, err := Server.ValidatorIndex(t.Context(), req)
	assert.ErrorContains(t, "head state is empty", err)
}

func TestWaitForActivation_ContextClosed(t *testing.T) {
	beaconState, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot:       0,
		Validators: []*ethpb.Validator{},
	})
	require.NoError(t, err)
	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	ctx, cancel := context.WithCancel(t.Context())
	depositCache, err := depositsnapshot.New()
	require.NoError(t, err)

	vs := &Server{
		Ctx:               ctx,
		ChainStartFetcher: &mockExecution.Chain{},
		BlockFetcher:      &mockExecution.Chain{},
		Eth1InfoFetcher:   &mockExecution.Chain{},
		DepositFetcher:    depositCache,
		HeadFetcher:       &mockChain.ChainService{State: beaconState, Root: genesisRoot[:]},
	}
	req := &ethpb.ValidatorActivationRequest{
		PublicKeys: [][]byte{pubKey(1)},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockChainStream := mock.NewMockBeaconNodeValidator_WaitForActivationServer(ctrl)
	mockChainStream.EXPECT().Context().Return(t.Context())
	mockChainStream.EXPECT().Send(gomock.Any()).Return(nil)
	mockChainStream.EXPECT().Context().Return(t.Context())
	exitRoutine := make(chan bool)
	go func(tt *testing.T) {
		want := "context canceled"
		assert.ErrorContains(tt, want, vs.WaitForActivation(req, mockChainStream))
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestWaitForActivation_MultipleStatuses(t *testing.T) {
	priv1, err := bls.RandKey()
	require.NoError(t, err)
	priv2, err := bls.RandKey()
	require.NoError(t, err)
	priv3, err := bls.RandKey()
	require.NoError(t, err)

	pubKey1 := priv1.PublicKey().Marshal()
	pubKey2 := priv2.PublicKey().Marshal()
	pubKey3 := priv3.PublicKey().Marshal()

	beaconState := &ethpb.BeaconState{
		Slot: 4000,
		Validators: []*ethpb.Validator{
			{
				PublicKey:       pubKey1,
				ActivationEpoch: 1,
				ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
			},
			{
				PublicKey:                  pubKey2,
				ActivationEpoch:            params.BeaconConfig().FarFutureEpoch,
				ActivationEligibilityEpoch: 6,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
			},
			{
				PublicKey:                  pubKey3,
				ActivationEpoch:            0,
				ActivationEligibilityEpoch: 0,
				ExitEpoch:                  0,
			},
		},
	}
	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	s, err := state_native.InitializeFromProtoUnsafePhase0(beaconState)
	require.NoError(t, err)
	vs := &Server{
		Ctx:               t.Context(),
		ChainStartFetcher: &mockExecution.Chain{},
		HeadFetcher:       &mockChain.ChainService{State: s, Root: genesisRoot[:]},
	}
	req := &ethpb.ValidatorActivationRequest{
		PublicKeys: [][]byte{pubKey1, pubKey2, pubKey3},
	}
	ctrl := gomock.NewController(t)

	defer ctrl.Finish()
	mockChainStream := mock.NewMockBeaconNodeValidator_WaitForActivationServer(ctrl)
	mockChainStream.EXPECT().Context().Return(t.Context())
	mockChainStream.EXPECT().Send(
		&ethpb.ValidatorActivationResponse{
			Statuses: []*ethpb.ValidatorActivationResponse_Status{
				{
					PublicKey: pubKey1,
					Status: &ethpb.ValidatorStatusResponse{
						Status:          ethpb.ValidatorStatus_ACTIVE,
						ActivationEpoch: 1,
					},
					Index: 0,
				},
				{
					PublicKey: pubKey2,
					Status: &ethpb.ValidatorStatusResponse{
						Status:                    ethpb.ValidatorStatus_PENDING,
						ActivationEpoch:           params.BeaconConfig().FarFutureEpoch,
						PositionInActivationQueue: 1,
					},
					Index: 1,
				},
				{
					PublicKey: pubKey3,
					Status: &ethpb.ValidatorStatusResponse{
						Status: ethpb.ValidatorStatus_EXITED,
					},
					Index: 2,
				},
			},
		},
	).Return(nil)

	require.NoError(t, vs.WaitForActivation(req, mockChainStream), "Could not setup wait for activation stream")
}

func TestWaitForChainStart_ContextClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	chainService := &mockChain.ChainService{}
	server := &Server{
		Ctx: ctx,
		ChainStartFetcher: &mockExecution.FaultyExecutionChain{
			ChainFeed: new(event.Feed),
		},
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
		ClockWaiter:   startup.NewClockSynchronizer(),
	}

	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_WaitForChainStartServer(ctrl)
	mockStream.EXPECT().Context().Return(ctx)
	go func(tt *testing.T) {
		err := server.WaitForChainStart(&emptypb.Empty{}, mockStream)
		assert.ErrorContains(tt, "Context canceled", err)
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestWaitForChainStart_AlreadyStarted(t *testing.T) {
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(3))
	genesisValidatorsRoot := bytesutil.ToBytes32([]byte("validators"))
	require.NoError(t, st.SetGenesisValidatorsRoot(genesisValidatorsRoot[:]))

	chainService := &mockChain.ChainService{State: st, ValidatorsRoot: genesisValidatorsRoot}
	Server := &Server{
		Ctx: t.Context(),
		ChainStartFetcher: &mockExecution.Chain{
			ChainFeed: new(event.Feed),
		},
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_WaitForChainStartServer(ctrl)
	mockStream.EXPECT().Send(
		&ethpb.ChainStartResponse{
			Started:               true,
			GenesisTime:           uint64(time.Unix(0, 0).Unix()),
			GenesisValidatorsRoot: genesisValidatorsRoot[:],
		},
	).Return(nil)
	mockStream.EXPECT().Context().Return(t.Context())
	assert.NoError(t, Server.WaitForChainStart(&emptypb.Empty{}, mockStream), "Could not call RPC method")
}

func TestWaitForChainStart_HeadStateDoesNotExist(t *testing.T) {
	// Set head state to nil
	chainService := &mockChain.ChainService{State: nil}
	gs := startup.NewClockSynchronizer()
	Server := &Server{
		Ctx: t.Context(),
		ChainStartFetcher: &mockExecution.Chain{
			ChainFeed: new(event.Feed),
		},
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
		ClockWaiter:   gs,
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_WaitForChainStartServer(ctrl)
	mockStream.EXPECT().Context().Return(t.Context())

	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		err := Server.WaitForChainStart(&emptypb.Empty{}, mockStream)
		if s, _ := status.FromError(err); s.Code() != codes.Canceled {
			assert.NoError(t, err)
		}
		wg.Done()
	}()

	util.WaitTimeout(wg, time.Second)
}

func TestWaitForChainStart_NotStartedThenLogFired(t *testing.T) {
	hook := logTest.NewGlobal()

	genesisValidatorsRoot := bytesutil.ToBytes32([]byte("validators"))
	chainService := &mockChain.ChainService{}
	gs := startup.NewClockSynchronizer()

	Server := &Server{
		Ctx: t.Context(),
		ChainStartFetcher: &mockExecution.FaultyExecutionChain{
			ChainFeed: new(event.Feed),
		},
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
		ClockWaiter:   gs,
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_WaitForChainStartServer(ctrl)
	mockStream.EXPECT().Send(
		&ethpb.ChainStartResponse{
			Started:               true,
			GenesisTime:           uint64(time.Unix(0, 0).Unix()),
			GenesisValidatorsRoot: genesisValidatorsRoot[:],
		},
	).Return(nil)
	mockStream.EXPECT().Context().Return(t.Context())
	go func(tt *testing.T) {
		assert.NoError(tt, Server.WaitForChainStart(&emptypb.Empty{}, mockStream))
		<-exitRoutine
	}(t)

	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	require.NoError(t, gs.SetClock(startup.NewClock(time.Unix(0, 0), genesisValidatorsRoot)))

	exitRoutine <- true
	require.LogsContain(t, hook, "Sending genesis time")
}

func testSigDomainForSlot(t *testing.T, domain [4]byte, chsrv *mockChain.ChainService, epoch primitives.Epoch) *ethpb.DomainResponse {
	cfg := params.BeaconConfig()
	gvr := genesis.ValidatorsRoot()
	s, err := state_native.InitializeFromProtoUnsafeDeneb(&ethpb.BeaconStateDeneb{
		Slot:                  primitives.Slot(epoch) * cfg.SlotsPerEpoch,
		GenesisValidatorsRoot: gvr[:],
	})
	require.NoError(t, err)
	chsrv.State = s
	vs := &Server{
		Ctx:               t.Context(),
		ChainStartFetcher: &mockExecution.Chain{},
		HeadFetcher:       chsrv,
	}
	domainResp, err := vs.DomainData(t.Context(), &ethpb.DomainRequest{
		Epoch:  epoch,
		Domain: domain[:],
	})
	require.NoError(t, err)
	return domainResp
}

func requireSigningEqual(t *testing.T, name string, domain [4]byte, req, want primitives.Epoch, chsrv *mockChain.ChainService) {
	t.Run(fmt.Sprintf("%s_%#x", name, domain), func(t *testing.T) {
		gvr := genesis.ValidatorsRoot()
		resp := testSigDomainForSlot(t, domain, chsrv, req)
		entry := params.GetNetworkScheduleEntry(want)
		wanted, err := signing.ComputeDomain(domain, entry.ForkVersion[:], gvr[:])
		assert.NoError(t, err)
		assert.Equal(t, hexutil.Encode(wanted), hexutil.Encode(resp.SignatureDomain))
	})
}

func TestServer_DomainData_Exits(t *testing.T) {
	// This test makes 2 sets of assertions:
	// - the deposit domain is always computed wrt the fork version at the given epoch
	// - the exit domain is the same until deneb, at which point it is always computed wrt the capella fork version
	params.SetActiveTestCleanup(t, params.MainnetConfig())
	params.BeaconConfig().InitializeForkSchedule()
	cfg := params.BeaconConfig()

	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	chsrv := &mockChain.ChainService{Root: genesisRoot[:]}
	last := params.LastForkEpoch()
	requireSigningEqual(t, "genesis deposit", cfg.DomainDeposit, cfg.GenesisEpoch, cfg.GenesisEpoch, chsrv)
	requireSigningEqual(t, "altair deposit", cfg.DomainDeposit, cfg.AltairForkEpoch, cfg.AltairForkEpoch, chsrv)
	requireSigningEqual(t, "bellatrix deposit", cfg.DomainDeposit, cfg.BellatrixForkEpoch, cfg.BellatrixForkEpoch, chsrv)
	requireSigningEqual(t, "capella deposit", cfg.DomainDeposit, cfg.CapellaForkEpoch, cfg.CapellaForkEpoch, chsrv)
	requireSigningEqual(t, "deneb deposit", cfg.DomainDeposit, cfg.DenebForkEpoch, cfg.DenebForkEpoch, chsrv)
	requireSigningEqual(t, "last epoch deposit", cfg.DomainDeposit, last, last, chsrv)

	requireSigningEqual(t, "genesis exit", cfg.DomainVoluntaryExit, cfg.GenesisEpoch, cfg.GenesisEpoch, chsrv)
	requireSigningEqual(t, "altair exit", cfg.DomainVoluntaryExit, cfg.AltairForkEpoch, cfg.AltairForkEpoch, chsrv)
	requireSigningEqual(t, "bellatrix exit", cfg.DomainVoluntaryExit, cfg.BellatrixForkEpoch, cfg.BellatrixForkEpoch, chsrv)
	requireSigningEqual(t, "capella exit", cfg.DomainVoluntaryExit, cfg.CapellaForkEpoch, cfg.CapellaForkEpoch, chsrv)
	requireSigningEqual(t, "deneb exit", cfg.DomainVoluntaryExit, cfg.DenebForkEpoch, cfg.CapellaForkEpoch, chsrv)
	requireSigningEqual(t, "last epoch exit", cfg.DomainVoluntaryExit, last, cfg.CapellaForkEpoch, chsrv)
}
