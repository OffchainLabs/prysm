package validator

import (
	"reflect"
	"testing"

	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	mockstategen "github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen/mock"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/d4l3k/messagediff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestValidatorStatus_PartiallyDeposited(t *testing.T) {
	pubKey1 := pubKey(1)
	stateObj, err := state_native.InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{
		Validators: []*ethpb.Validator{
			{
				PublicKey:                  pubKey1,
				ActivationEligibilityEpoch: 1,
				EffectiveBalance:           params.BeaconConfig().MinDepositAmount,
			},
		},
	})
	require.NoError(t, err)
	vs := &Server{
		HeadFetcher: &mockChain.ChainService{
			State: stateObj,
		},
	}
	req := &ethpb.ValidatorStatusRequest{
		PublicKey: pubKey1,
	}
	resp, err := vs.ValidatorStatus(t.Context(), req)
	require.NoError(t, err, "Could not get validator status")
	assert.Equal(t, ethpb.ValidatorStatus_PARTIALLY_DEPOSITED, resp.Status)
}

func TestValidatorStatus_Pending_MultipleDeposits(t *testing.T) {
	pubKey1 := pubKey(1)
	stateObj, err := state_native.InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{
		Validators: []*ethpb.Validator{
			{
				PublicKey:                  pubKey1,
				ActivationEligibilityEpoch: 1,
				EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
				ActivationEpoch:            params.BeaconConfig().FarFutureEpoch,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				WithdrawableEpoch:          params.BeaconConfig().FarFutureEpoch,
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, stateObj.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	vs := &Server{
		HeadFetcher: &mockChain.ChainService{
			State: stateObj,
		},
	}
	req := &ethpb.ValidatorStatusRequest{
		PublicKey: pubKey1,
	}
	resp, err := vs.ValidatorStatus(t.Context(), req)
	require.NoError(t, err, "Could not get validator status")
	assert.Equal(t, ethpb.ValidatorStatus_PENDING, resp.Status)
}

func TestValidatorStatus_Pending(t *testing.T) {
	pubKey := pubKey(1)
	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	// Pending active because activation epoch is still defaulted at far future slot.
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(5000))
	err = st.SetValidators([]*ethpb.Validator{
		{
			ActivationEpoch:       params.BeaconConfig().FarFutureEpoch,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
			PublicKey:             pubKey,
			WithdrawalCredentials: make([]byte, 32),
		},
	})
	require.NoError(t, err)

	vs := &Server{
		HeadFetcher: &mockChain.ChainService{State: st, Root: genesisRoot[:]},
	}
	req := &ethpb.ValidatorStatusRequest{
		PublicKey: pubKey,
	}
	resp, err := vs.ValidatorStatus(t.Context(), req)
	require.NoError(t, err, "Could not get validator status")
	assert.Equal(t, ethpb.ValidatorStatus_PENDING, resp.Status)
}

func TestValidatorStatus_Exiting(t *testing.T) {
	pubKey := pubKey(1)

	// Initiated exit because validator exit epoch and withdrawable epoch are not FAR_FUTURE_EPOCH
	slot := primitives.Slot(10000)
	epoch := slots.ToEpoch(slot)
	exitEpoch := helpers.ActivationExitEpoch(epoch)
	withdrawableEpoch := exitEpoch + params.BeaconConfig().MinValidatorWithdrawabilityDelay
	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	st := &ethpb.BeaconState{
		Slot: slot,
		Validators: []*ethpb.Validator{{
			PublicKey:         pubKey,
			ActivationEpoch:   0,
			ExitEpoch:         exitEpoch,
			WithdrawableEpoch: withdrawableEpoch},
		}}
	stateObj, err := state_native.InitializeFromProtoUnsafePhase0(st)
	require.NoError(t, err)
	vs := &Server{
		HeadFetcher: &mockChain.ChainService{State: stateObj, Root: genesisRoot[:]},
	}
	req := &ethpb.ValidatorStatusRequest{
		PublicKey: pubKey,
	}
	resp, err := vs.ValidatorStatus(t.Context(), req)
	require.NoError(t, err, "Could not get validator status")
	assert.Equal(t, ethpb.ValidatorStatus_EXITING, resp.Status)
}

func TestValidatorStatus_Slashing(t *testing.T) {
	pubKey := pubKey(1)

	// Exit slashed because slashed is true, exit epoch is =< current epoch and withdrawable epoch > epoch .
	slot := primitives.Slot(10000)
	epoch := slots.ToEpoch(slot)
	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	st := &ethpb.BeaconState{
		Slot: slot,
		Validators: []*ethpb.Validator{{
			Slashed:           true,
			PublicKey:         pubKey,
			WithdrawableEpoch: epoch + 1},
		}}
	stateObj, err := state_native.InitializeFromProtoUnsafePhase0(st)
	require.NoError(t, err)
	vs := &Server{
		HeadFetcher: &mockChain.ChainService{State: stateObj, Root: genesisRoot[:]},
	}
	req := &ethpb.ValidatorStatusRequest{
		PublicKey: pubKey,
	}
	resp, err := vs.ValidatorStatus(t.Context(), req)
	require.NoError(t, err, "Could not get validator status")
	assert.Equal(t, ethpb.ValidatorStatus_EXITED, resp.Status)
}

func TestValidatorStatus_Exited(t *testing.T) {
	pubKey := pubKey(1)

	// Exit because only exit epoch is =< current epoch.
	slot := primitives.Slot(10000)
	epoch := slots.ToEpoch(slot)
	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(slot))
	err = st.SetValidators([]*ethpb.Validator{{
		PublicKey:             pubKey,
		WithdrawableEpoch:     epoch + 1,
		WithdrawalCredentials: make([]byte, 32)},
	})
	require.NoError(t, err)
	vs := &Server{
		HeadFetcher: &mockChain.ChainService{State: st, Root: genesisRoot[:]},
	}
	req := &ethpb.ValidatorStatusRequest{
		PublicKey: pubKey,
	}
	resp, err := vs.ValidatorStatus(t.Context(), req)
	require.NoError(t, err, "Could not get validator status")
	assert.Equal(t, ethpb.ValidatorStatus_EXITED, resp.Status)
}

func TestValidatorStatus_UnknownStatus(t *testing.T) {
	pubKey := pubKey(1)
	stateObj, err := state_native.InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{
		Slot: 0,
	})
	require.NoError(t, err)
	vs := &Server{
		HeadFetcher: &mockChain.ChainService{
			State: stateObj,
		},
	}
	req := &ethpb.ValidatorStatusRequest{
		PublicKey: pubKey,
	}
	resp, err := vs.ValidatorStatus(t.Context(), req)
	require.NoError(t, err, "Could not get validator status")
	assert.Equal(t, ethpb.ValidatorStatus_UNKNOWN_STATUS, resp.Status)
}

func TestActivationStatus_OK(t *testing.T) {
	deposits, _, err := util.DeterministicDepositsAndKeys(4)
	require.NoError(t, err)
	pubKeys := [][]byte{deposits[0].Data.PublicKey, deposits[1].Data.PublicKey, deposits[2].Data.PublicKey, deposits[3].Data.PublicKey}
	stateObj, err := state_native.InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{
		Slot: 4000,
		Validators: []*ethpb.Validator{
			{
				ActivationEpoch: 0,
				ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
				PublicKey:       pubKeys[0],
			},
			{
				ActivationEpoch: 0,
				ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
				PublicKey:       pubKeys[1],
			},
			{
				ActivationEligibilityEpoch: 700,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				PublicKey:                  pubKeys[3],
				EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
			},
		},
	})
	require.NoError(t, err)
	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	vs := &Server{
		Ctx:         t.Context(),
		HeadFetcher: &mockChain.ChainService{State: stateObj, Root: genesisRoot[:]},
	}
	activeExists, response, err := vs.activationStatus(t.Context(), pubKeys)
	require.NoError(t, err)
	require.Equal(t, true, activeExists, "No activated validator exists when there was supposed to be 2")
	if response[0].Status.Status != ethpb.ValidatorStatus_ACTIVE {
		t.Errorf("Validator with pubkey %#x is not activated and instead has this status: %s",
			response[0].PublicKey, response[0].Status.Status.String())
	}
	if response[0].Index != 0 {
		t.Errorf("Validator with pubkey %#x is expected to have index %d, received %d", response[0].PublicKey, 0, response[0].Index)
	}

	if response[1].Status.Status != ethpb.ValidatorStatus_ACTIVE {
		t.Errorf("Validator with pubkey %#x was activated when not supposed to",
			response[1].PublicKey)
	}
	if response[1].Index != 1 {
		t.Errorf("Validator with pubkey %#x is expected to have index %d, received %d", response[1].PublicKey, 1, response[1].Index)
	}

	// Validator is not in the beacon state, so with the legacy eth1 deposit cache
	// removed it can no longer be resolved to a deposited status.
	if response[2].Status.Status != ethpb.ValidatorStatus_UNKNOWN_STATUS {
		t.Errorf("Validator with pubkey %#x is not unknown and instead has this status: %s",
			response[2].PublicKey, response[2].Status.Status.String())
	}
	if uint64(response[2].Index) != 0 {
		t.Errorf("Validator with pubkey %#x is expected to have index %d, received %d", response[2].PublicKey, 0, response[2].Index)
	}

	if response[3].Status.Status != ethpb.ValidatorStatus_DEPOSITED {
		t.Errorf("Validator with pubkey %#x was not deposited and has this status: %s",
			response[3].PublicKey, response[3].Status.Status.String())
	}
	if response[3].Index != 2 {
		t.Errorf("Validator with pubkey %#x is expected to have index %d, received %d", response[3].PublicKey, 2, response[3].Index)
	}
}

func TestOptimisticStatus(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	server := &Server{OptimisticModeFetcher: &mockChain.ChainService{}, TimeFetcher: &mockChain.ChainService{}}
	err := server.optimisticStatus(t.Context())
	require.NoError(t, err)

	cfg := params.BeaconConfig().Copy()
	cfg.BellatrixForkEpoch = 2
	params.OverrideBeaconConfig(cfg)

	server = &Server{OptimisticModeFetcher: &mockChain.ChainService{Optimistic: true}, TimeFetcher: &mockChain.ChainService{}}
	err = server.optimisticStatus(t.Context())
	s, ok := status.FromError(err)
	require.Equal(t, true, ok)
	require.DeepEqual(t, codes.Unavailable, s.Code())
	require.ErrorContains(t, errOptimisticMode.Error(), err)

	server = &Server{OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false}, TimeFetcher: &mockChain.ChainService{}}
	err = server.optimisticStatus(t.Context())
	require.NoError(t, err)
}

func TestValidatorStatus_CorrectActivationQueue(t *testing.T) {
	pbKey := pubKey(5)
	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	currentSlot := primitives.Slot(5000)
	// Pending active because activation epoch is still defaulted at far future slot.
	validators := []*ethpb.Validator{
		{
			ActivationEpoch:       0,
			PublicKey:             pubKey(0),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
		},
		{
			ActivationEpoch:       0,
			PublicKey:             pubKey(1),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
		},
		{
			ActivationEpoch:       0,
			PublicKey:             pubKey(2),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
		},
		{
			ActivationEpoch:       0,
			PublicKey:             pubKey(3),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
		},
		{
			ActivationEpoch:       primitives.Epoch(currentSlot/params.BeaconConfig().SlotsPerEpoch + 1),
			PublicKey:             pbKey,
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
		},
		{
			ActivationEpoch:       primitives.Epoch(currentSlot/params.BeaconConfig().SlotsPerEpoch + 4),
			PublicKey:             pubKey(5),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
		},
	}
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, st.SetValidators(validators))
	require.NoError(t, st.SetSlot(currentSlot))

	vs := &Server{
		HeadFetcher: &mockChain.ChainService{State: st, Root: genesisRoot[:]},
	}
	req := &ethpb.ValidatorStatusRequest{
		PublicKey: pbKey,
	}
	resp, err := vs.ValidatorStatus(t.Context(), req)
	require.NoError(t, err, "Could not get validator status")
	assert.Equal(t, ethpb.ValidatorStatus_PENDING, resp.Status)
}

func TestMultipleValidatorStatus_Pubkeys(t *testing.T) {
	deposits, _, err := util.DeterministicDepositsAndKeys(6)
	require.NoError(t, err)
	pubKeys := [][]byte{
		deposits[0].Data.PublicKey,
		deposits[1].Data.PublicKey,
		deposits[2].Data.PublicKey,
		deposits[3].Data.PublicKey,
		deposits[4].Data.PublicKey,
		deposits[5].Data.PublicKey,
	}
	stateObj, err := state_native.InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{
		Slot: 4000,
		Validators: []*ethpb.Validator{
			{
				ActivationEpoch: 0,
				ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
				PublicKey:       pubKeys[0],
			},
			{
				ActivationEpoch: 0,
				ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
				PublicKey:       pubKeys[1],
			},
			{
				ActivationEligibilityEpoch: 700,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				PublicKey:                  pubKeys[3],
				EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
			},
			{
				ActivationEligibilityEpoch: 700,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				PublicKey:                  pubKeys[4],
				EffectiveBalance:           params.BeaconConfig().MinDepositAmount,
			},
			{
				ActivationEligibilityEpoch: 700,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				PublicKey:                  pubKeys[5],
			},
		},
	})
	require.NoError(t, err)
	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	vs := &Server{
		Ctx:         t.Context(),
		HeadFetcher: &mockChain.ChainService{State: stateObj, Root: genesisRoot[:]},
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}

	want := []*ethpb.ValidatorStatusResponse{
		{
			Status: ethpb.ValidatorStatus_ACTIVE,
		},
		{
			Status: ethpb.ValidatorStatus_ACTIVE,
		},
		{
			// Not in the beacon state; with the eth1 deposit cache removed this is unknown.
			Status:          ethpb.ValidatorStatus_UNKNOWN_STATUS,
			ActivationEpoch: 18446744073709551615,
		},
		{
			Status: ethpb.ValidatorStatus_DEPOSITED,
		},
		{
			Status: ethpb.ValidatorStatus_PARTIALLY_DEPOSITED,
		},
		{
			Status: ethpb.ValidatorStatus_PENDING,
		},
	}

	req := &ethpb.MultipleValidatorStatusRequest{PublicKeys: pubKeys}
	response, err := vs.MultipleValidatorStatus(t.Context(), req)
	require.NoError(t, err)

	assert.Equal(t, len(response.PublicKeys), len(pubKeys))
	for i, resp := range response.PublicKeys {
		require.DeepEqual(t, pubKeys[i], resp)
	}
	assert.Equal(t, len(pubKeys), len(response.Statuses))
	for i, resp := range response.Statuses {
		if !proto.Equal(want[i], resp) {
			t.Fatalf("Wanted %v\n Received: %v\n", want[i], resp)
		}
	}
}

func TestMultipleValidatorStatus_Indices(t *testing.T) {
	slot := primitives.Slot(10000)
	epoch := slots.ToEpoch(slot)
	pubKeys := [][]byte{pubKey(1), pubKey(2), pubKey(3), pubKey(4), pubKey(5), pubKey(6), pubKey(7)}
	beaconState := &ethpb.BeaconState{
		Slot: 4000,
		Validators: []*ethpb.Validator{
			{
				ActivationEpoch: 0,
				ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
				PublicKey:       pubKeys[0],
			},
			{
				ActivationEpoch: 0,
				ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
				PublicKey:       pubKeys[1],
			},
			{
				ActivationEligibilityEpoch: 700,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				PublicKey:                  pubKeys[2],
				EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
			},
			{
				Slashed:   true,
				ExitEpoch: epoch + 1,
				PublicKey: pubKeys[3],
			},
			{
				ActivationEligibilityEpoch: 700,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				PublicKey:                  pubKeys[4],
				EffectiveBalance:           params.BeaconConfig().MinDepositAmount,
			},
			{
				ActivationEligibilityEpoch: 700,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				PublicKey:                  pubKeys[5],
			},
		},
	}
	stateObj, err := state_native.InitializeFromProtoUnsafePhase0(beaconState)
	require.NoError(t, err)
	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	vs := &Server{
		Ctx:         t.Context(),
		HeadFetcher: &mockChain.ChainService{State: stateObj, Root: genesisRoot[:]},
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}

	want := []*ethpb.ValidatorStatusResponse{
		{
			Status: ethpb.ValidatorStatus_ACTIVE,
		},
		{
			Status: ethpb.ValidatorStatus_ACTIVE,
		},
		{
			Status: ethpb.ValidatorStatus_DEPOSITED,
		},
		{
			Status: ethpb.ValidatorStatus_SLASHING,
		},
		{
			Status: ethpb.ValidatorStatus_PARTIALLY_DEPOSITED,
		},
		{
			Status: ethpb.ValidatorStatus_PENDING,
		},
	}

	// Note: Index 6 should be skipped.
	req := &ethpb.MultipleValidatorStatusRequest{Indices: []int64{0, 1, 2, 3, 4, 5, 6}}
	response, err := vs.MultipleValidatorStatus(t.Context(), req)
	require.NoError(t, err)

	assert.Equal(t, len(beaconState.Validators), len(response.PublicKeys))
	for i, resp := range response.PublicKeys {
		expected := beaconState.Validators[i].PublicKey
		require.DeepEqual(t, expected, resp)
	}
	assert.Equal(t, len(beaconState.Validators), len(response.Statuses))
	for i, resp := range response.Statuses {
		if !proto.Equal(want[i], resp) {
			t.Fatalf("Wanted %v\n Received: %v\n", want[i], resp)
		}
	}
}

func Test_DepositStatus(t *testing.T) {
	assert.Equal(t, depositStatus(0), ethpb.ValidatorStatus_PENDING)
	assert.Equal(t, depositStatus(params.BeaconConfig().MinDepositAmount), ethpb.ValidatorStatus_PARTIALLY_DEPOSITED)
	assert.Equal(t, depositStatus(params.BeaconConfig().MaxEffectiveBalance), ethpb.ValidatorStatus_DEPOSITED)
}

func TestServer_CheckDoppelGanger(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
		svSetup func(t *testing.T) (*Server, *ethpb.DoppelGangerRequest, *ethpb.DoppelGangerResponse)
	}{
		{
			name:    "normal doppelganger request",
			wantErr: false,
			svSetup: func(t *testing.T) (*Server, *ethpb.DoppelGangerRequest, *ethpb.DoppelGangerResponse) {
				hs, ps, keys := createStateSetupAltair(t, 3)
				rb := mockstategen.NewReplayerBuilder()
				rb.SetMockStateForSlot(ps, 23)
				vs := &Server{
					HeadFetcher: &mockChain.ChainService{
						State: hs,
					},
					SyncChecker:     &mockSync.Sync{IsSyncing: false},
					ReplayerBuilder: rb,
				}
				request := &ethpb.DoppelGangerRequest{
					ValidatorRequests: make([]*ethpb.DoppelGangerRequest_ValidatorRequest, 0),
				}
				response := &ethpb.DoppelGangerResponse{Responses: make([]*ethpb.DoppelGangerResponse_ValidatorResponse, 0)}
				for i := range 3 {
					request.ValidatorRequests = append(request.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{
						PublicKey:  keys[i].PublicKey().Marshal(),
						Epoch:      1,
						SignedRoot: []byte{'A'},
					})
					response.Responses = append(response.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{
						PublicKey:       keys[i].PublicKey().Marshal(),
						DuplicateExists: false,
					})
				}
				return vs, request, response
			},
		},
		{
			name:    "doppelganger exists current epoch",
			wantErr: false,
			svSetup: func(t *testing.T) (*Server, *ethpb.DoppelGangerRequest, *ethpb.DoppelGangerResponse) {
				hs, ps, keys := createStateSetupAltair(t, 3)
				rb := mockstategen.NewReplayerBuilder()
				rb.SetMockStateForSlot(ps, 23)
				currentIndices := make([]byte, 64)
				currentIndices[2] = 1
				require.NoError(t, hs.SetCurrentParticipationBits(currentIndices))

				vs := &Server{
					HeadFetcher: &mockChain.ChainService{
						State: hs,
					},
					SyncChecker:     &mockSync.Sync{IsSyncing: false},
					ReplayerBuilder: rb,
				}
				request := &ethpb.DoppelGangerRequest{
					ValidatorRequests: make([]*ethpb.DoppelGangerRequest_ValidatorRequest, 0),
				}
				response := &ethpb.DoppelGangerResponse{Responses: make([]*ethpb.DoppelGangerResponse_ValidatorResponse, 0)}
				for i := range 2 {
					request.ValidatorRequests = append(request.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{
						PublicKey:  keys[i].PublicKey().Marshal(),
						Epoch:      1,
						SignedRoot: []byte{'A'},
					})
					response.Responses = append(response.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{
						PublicKey:       keys[i].PublicKey().Marshal(),
						DuplicateExists: false,
					})
				}

				// Add in for duplicate validator
				request.ValidatorRequests = append(request.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{
					PublicKey:  keys[2].PublicKey().Marshal(),
					Epoch:      0,
					SignedRoot: []byte{'A'},
				})
				response.Responses = append(response.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{
					PublicKey:       keys[2].PublicKey().Marshal(),
					DuplicateExists: true,
				})
				return vs, request, response
			},
		},
		{
			name:    "doppelganger exists previous epoch",
			wantErr: false,
			svSetup: func(t *testing.T) (*Server, *ethpb.DoppelGangerRequest, *ethpb.DoppelGangerResponse) {
				hs, ps, keys := createStateSetupAltair(t, 3)
				prevIndices := make([]byte, 64)
				prevIndices[2] = 1
				require.NoError(t, ps.SetPreviousParticipationBits(prevIndices))
				rb := mockstategen.NewReplayerBuilder()
				rb.SetMockStateForSlot(ps, 23)

				vs := &Server{
					HeadFetcher: &mockChain.ChainService{
						State: hs,
					},
					SyncChecker:     &mockSync.Sync{IsSyncing: false},
					ReplayerBuilder: rb,
				}
				request := &ethpb.DoppelGangerRequest{
					ValidatorRequests: make([]*ethpb.DoppelGangerRequest_ValidatorRequest, 0),
				}
				response := &ethpb.DoppelGangerResponse{Responses: make([]*ethpb.DoppelGangerResponse_ValidatorResponse, 0)}
				for i := range 2 {
					request.ValidatorRequests = append(request.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{
						PublicKey:  keys[i].PublicKey().Marshal(),
						Epoch:      1,
						SignedRoot: []byte{'A'},
					})
					response.Responses = append(response.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{
						PublicKey:       keys[i].PublicKey().Marshal(),
						DuplicateExists: false,
					})
				}

				// Add in for duplicate validator
				request.ValidatorRequests = append(request.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{
					PublicKey:  keys[2].PublicKey().Marshal(),
					Epoch:      0,
					SignedRoot: []byte{'A'},
				})
				response.Responses = append(response.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{
					PublicKey:       keys[2].PublicKey().Marshal(),
					DuplicateExists: true,
				})
				return vs, request, response
			},
		},
		{
			name:    "multiple doppelganger exists",
			wantErr: false,
			svSetup: func(t *testing.T) (*Server, *ethpb.DoppelGangerRequest, *ethpb.DoppelGangerResponse) {
				hs, ps, keys := createStateSetupAltair(t, 3)
				currentIndices := make([]byte, 64)
				currentIndices[10] = 1
				currentIndices[11] = 2
				require.NoError(t, hs.SetPreviousParticipationBits(currentIndices))
				rb := mockstategen.NewReplayerBuilder()
				rb.SetMockStateForSlot(ps, 23)

				prevIndices := make([]byte, 64)
				for i := 12; i < 20; i++ {
					prevIndices[i] = 1
				}
				require.NoError(t, ps.SetCurrentParticipationBits(prevIndices))

				vs := &Server{
					HeadFetcher: &mockChain.ChainService{
						State: hs,
					},
					SyncChecker:     &mockSync.Sync{IsSyncing: false},
					ReplayerBuilder: rb,
				}
				request := &ethpb.DoppelGangerRequest{
					ValidatorRequests: make([]*ethpb.DoppelGangerRequest_ValidatorRequest, 0),
				}
				response := &ethpb.DoppelGangerResponse{Responses: make([]*ethpb.DoppelGangerResponse_ValidatorResponse, 0)}
				for i := 10; i < 15; i++ {
					// Add in for duplicate validator
					request.ValidatorRequests = append(request.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{
						PublicKey:  keys[i].PublicKey().Marshal(),
						Epoch:      0,
						SignedRoot: []byte{'A'},
					})
					response.Responses = append(response.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{
						PublicKey:       keys[i].PublicKey().Marshal(),
						DuplicateExists: true,
					})
				}
				for i := 15; i < 20; i++ {
					request.ValidatorRequests = append(request.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{
						PublicKey:  keys[i].PublicKey().Marshal(),
						Epoch:      3,
						SignedRoot: []byte{'A'},
					})
					response.Responses = append(response.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{
						PublicKey:       keys[i].PublicKey().Marshal(),
						DuplicateExists: false,
					})
				}

				return vs, request, response
			},
		},
		{
			name:    "attesters are too recent",
			wantErr: false,
			svSetup: func(t *testing.T) (*Server, *ethpb.DoppelGangerRequest, *ethpb.DoppelGangerResponse) {
				hs, ps, keys := createStateSetupAltair(t, 3)
				rb := mockstategen.NewReplayerBuilder()
				rb.SetMockStateForSlot(ps, 23)
				currentIndices := make([]byte, 64)
				currentIndices[0] = 1
				currentIndices[1] = 2
				require.NoError(t, hs.SetPreviousParticipationBits(currentIndices))
				vs := &Server{
					HeadFetcher: &mockChain.ChainService{
						State: hs,
					},
					SyncChecker:     &mockSync.Sync{IsSyncing: false},
					ReplayerBuilder: rb,
				}
				request := &ethpb.DoppelGangerRequest{
					ValidatorRequests: make([]*ethpb.DoppelGangerRequest_ValidatorRequest, 0),
				}
				response := &ethpb.DoppelGangerResponse{Responses: make([]*ethpb.DoppelGangerResponse_ValidatorResponse, 0)}
				for i := range 15 {
					request.ValidatorRequests = append(request.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{
						PublicKey:  keys[i].PublicKey().Marshal(),
						Epoch:      2,
						SignedRoot: []byte{'A'},
					})
					response.Responses = append(response.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{
						PublicKey:       keys[i].PublicKey().Marshal(),
						DuplicateExists: false,
					})
				}

				return vs, request, response
			},
		},
		{
			name:    "attesters are too recent(previous state)",
			wantErr: false,
			svSetup: func(t *testing.T) (*Server, *ethpb.DoppelGangerRequest, *ethpb.DoppelGangerResponse) {
				hs, ps, keys := createStateSetupAltair(t, 3)
				rb := mockstategen.NewReplayerBuilder()
				rb.SetMockStateForSlot(ps, 23)
				currentIndices := make([]byte, 64)
				currentIndices[0] = 1
				currentIndices[1] = 2
				require.NoError(t, ps.SetPreviousParticipationBits(currentIndices))
				vs := &Server{
					HeadFetcher: &mockChain.ChainService{
						State: hs,
					},
					SyncChecker:     &mockSync.Sync{IsSyncing: false},
					ReplayerBuilder: rb,
				}
				request := &ethpb.DoppelGangerRequest{
					ValidatorRequests: make([]*ethpb.DoppelGangerRequest_ValidatorRequest, 0),
				}
				response := &ethpb.DoppelGangerResponse{Responses: make([]*ethpb.DoppelGangerResponse_ValidatorResponse, 0)}
				for i := range 15 {
					request.ValidatorRequests = append(request.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{
						PublicKey:  keys[i].PublicKey().Marshal(),
						Epoch:      1,
						SignedRoot: []byte{'A'},
					})
					response.Responses = append(response.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{
						PublicKey:       keys[i].PublicKey().Marshal(),
						DuplicateExists: false,
					})
				}

				return vs, request, response
			},
		},
		{
			name:    "exit early for Phase 0",
			wantErr: false,
			svSetup: func(t *testing.T) (*Server, *ethpb.DoppelGangerRequest, *ethpb.DoppelGangerResponse) {
				hs, _, keys := createStateSetupPhase0(t, 3)

				vs := &Server{
					HeadFetcher: &mockChain.ChainService{
						State: hs,
					},
					SyncChecker: &mockSync.Sync{IsSyncing: false},
				}
				request := &ethpb.DoppelGangerRequest{
					ValidatorRequests: []*ethpb.DoppelGangerRequest_ValidatorRequest{
						{
							PublicKey:  keys[0].PublicKey().Marshal(),
							Epoch:      1,
							SignedRoot: []byte{'A'},
						},
					},
				}
				response := &ethpb.DoppelGangerResponse{
					Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
						{
							PublicKey:       keys[0].PublicKey().Marshal(),
							DuplicateExists: false,
						},
					},
				}

				return vs, request, response
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs, req, resp := tt.svSetup(t)
			got, err := vs.CheckDoppelGanger(t.Context(), req)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckDoppelGanger() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, resp) {
				diff, _ := messagediff.PrettyDiff(resp, got)
				t.Errorf("CheckDoppelGanger() difference = %v", diff)
			}
		})
	}
}

func createStateSetupPhase0(t *testing.T, head primitives.Epoch) (state.BeaconState,
	state.BeaconState, []bls.SecretKey) {
	gs, keys := util.DeterministicGenesisState(t, 64)
	hs := gs.Copy()

	// Head State
	headSlot := primitives.Slot(head)*params.BeaconConfig().SlotsPerEpoch + params.BeaconConfig().SlotsPerEpoch/2
	assert.NoError(t, hs.SetSlot(headSlot))

	// Previous Epoch State
	prevSlot := headSlot - params.BeaconConfig().SlotsPerEpoch
	ps := gs.Copy()
	assert.NoError(t, ps.SetSlot(prevSlot))

	return hs, ps, keys
}

func createStateSetupAltair(t *testing.T, head primitives.Epoch) (state.BeaconState,
	state.BeaconState, []bls.SecretKey) {
	gs, keys := util.DeterministicGenesisStateAltair(t, 64)
	hs := gs.Copy()

	// Head State
	headSlot := primitives.Slot(head)*params.BeaconConfig().SlotsPerEpoch + params.BeaconConfig().SlotsPerEpoch/2
	assert.NoError(t, hs.SetSlot(headSlot))

	// Previous Epoch State
	prevSlot := headSlot - params.BeaconConfig().SlotsPerEpoch
	ps := gs.Copy()
	assert.NoError(t, ps.SetSlot(prevSlot))

	return hs, ps, keys
}
