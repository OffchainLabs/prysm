package beacon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	chainMock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	dbTest "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	doublylinkedtree "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/doubly-linked-tree"
	mockp2p "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/core"
	rpctesting "github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/prysm/testing"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stategen"
	mockstategen "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stategen/mock"
	mockSync "github.com/prysmaticlabs/prysm/v5/beacon-chain/sync/initial-sync/testing"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func individualVotesHelper(t *testing.T, request *structs.GetIndividualVotesRequest, s *Server) (string, *structs.GetIndividualVotesResponse) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(request)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(s.GetIndividualVotes))
	defer srv.Close()
	req := httptest.NewRequest(
		http.MethodGet,
		"http://example.com/eth/v1/beacon/individual_votes",
		&buf,
	)
	client := &http.Client{}
	rawResp, err := client.Post(srv.URL, "application/json", req.Body)
	require.NoError(t, err)
	defer func() {
		if err := rawResp.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	body, err := io.ReadAll(rawResp.Body)
	require.NoError(t, err)
	type ErrorResponse struct {
		Message string `json:"message"`
	}
	if rawResp.StatusCode != 200 {
		var errorResponse ErrorResponse
		err = json.Unmarshal(body, &errorResponse)
		require.NoError(t, err)
		return errorResponse.Message, &structs.GetIndividualVotesResponse{}
	}
	var votes *structs.GetIndividualVotesResponse
	err = json.Unmarshal(body, &votes)
	require.NoError(t, err)
	return "", votes
}

func TestServer_GetIndividualVotes_RequestFutureSlot(t *testing.T) {
	s := &Server{
		CoreService: &core.Service{
			GenesisTimeFetcher: &chainMock.ChainService{},
		},
	}
	request := &structs.GetIndividualVotesRequest{
		Epoch: fmt.Sprintf("%d", slots.ToEpoch(s.CoreService.GenesisTimeFetcher.CurrentSlot())+1),
	}
	errorResp, _ := individualVotesHelper(t, request, s)
	require.StringContains(t, "cannot retrieve information about an epoch in the future", errorResp)
}

func addDefaultReplayerBuilder(s *Server, h stategen.HistoryAccessor) {
	cc := &mockstategen.CanonicalChecker{Is: true, Err: nil}
	cs := &mockstategen.CurrentSlotter{Slot: math.MaxUint64 - 1}
	s.CoreService.ReplayerBuilder = stategen.NewCanonicalHistory(h, cc, cs)
}

func TestServer_GetIndividualVotes_ValidatorsDontExist(t *testing.T) {
	beaconDB := dbTest.SetupDB(t)
	ctx := context.Background()

	var slot primitives.Slot = 0
	validators := uint64(64)
	stateWithValidators, _ := util.DeterministicGenesisState(t, validators)
	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetValidators(stateWithValidators.Validators()))
	require.NoError(t, beaconState.SetSlot(slot))

	b := util.NewBeaconBlock()
	b.Block.Slot = slot
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	gen := stategen.New(beaconDB, doublylinkedtree.New())
	require.NoError(t, gen.SaveState(ctx, gRoot, beaconState))
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	s := &Server{
		CoreService: &core.Service{
			StateGen:           gen,
			GenesisTimeFetcher: &chainMock.ChainService{},
		},
	}
	addDefaultReplayerBuilder(s, beaconDB)

	// Test non exist public key.
	request := &structs.GetIndividualVotesRequest{
		PublicKeys: []string{"0xaa"},
		Epoch:      "0",
	}
	errStr, resp := individualVotesHelper(t, request, s)
	require.Equal(t, "", errStr)
	want := &structs.GetIndividualVotesResponse{
		IndividualVotes: []*structs.IndividualVote{
			{
				Epoch:                            "0",
				PublicKey:                        "0xaa",
				ValidatorIndex:                   fmt.Sprintf("%d", ^uint64(0)),
				CurrentEpochEffectiveBalanceGwei: "0",
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
		},
	}
	assert.DeepEqual(t, want, resp, "Unexpected response")

	// Test non-existent validator index.
	request = &structs.GetIndividualVotesRequest{
		Indices: []string{"100"},
		Epoch:   "0",
	}
	errStr, resp = individualVotesHelper(t, request, s)
	require.Equal(t, "", errStr)
	want = &structs.GetIndividualVotesResponse{
		IndividualVotes: []*structs.IndividualVote{
			{
				Epoch:                            "0",
				PublicKey:                        "0x",
				ValidatorIndex:                   "100",
				CurrentEpochEffectiveBalanceGwei: "0",
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
		},
	}
	assert.DeepEqual(t, want, resp, "Unexpected response")

	// Test both.
	request = &structs.GetIndividualVotesRequest{
		PublicKeys: []string{"0xaa", "0xbb"},
		Indices:    []string{"100", "101"},
		Epoch:      "0",
	}
	errStr, resp = individualVotesHelper(t, request, s)
	require.Equal(t, "", errStr)
	want = &structs.GetIndividualVotesResponse{
		IndividualVotes: []*structs.IndividualVote{
			{Epoch: "0", PublicKey: "0xaa", ValidatorIndex: fmt.Sprintf("%d", ^uint64(0)), CurrentEpochEffectiveBalanceGwei: "0", InclusionSlot: "0", InclusionDistance: "0", InactivityScore: "0"},
			{
				Epoch:                            "0",
				PublicKey:                        "0xbb",
				ValidatorIndex:                   fmt.Sprintf("%d", ^uint64(0)),
				CurrentEpochEffectiveBalanceGwei: "0",
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
			{
				Epoch:                            "0",
				PublicKey:                        "0x",
				ValidatorIndex:                   "100",
				CurrentEpochEffectiveBalanceGwei: "0",
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
			{
				Epoch:                            "0",
				PublicKey:                        "0x",
				ValidatorIndex:                   "101",
				CurrentEpochEffectiveBalanceGwei: "0",
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
		},
	}
	assert.DeepEqual(t, want, resp, "Unexpected response")
}

func TestServer_GetIndividualVotes_Working(t *testing.T) {
	helpers.ClearCache()
	beaconDB := dbTest.SetupDB(t)
	ctx := context.Background()

	validators := uint64(32)
	stateWithValidators, _ := util.DeterministicGenesisState(t, validators)
	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetValidators(stateWithValidators.Validators()))

	bf := bitfield.NewBitlist(validators / uint64(params.BeaconConfig().SlotsPerEpoch))
	att1 := util.NewAttestation()
	att1.AggregationBits = bf
	att2 := util.NewAttestation()
	att2.AggregationBits = bf
	rt := [32]byte{'A'}
	att1.Data.Target.Root = rt[:]
	att1.Data.BeaconBlockRoot = rt[:]
	br := beaconState.BlockRoots()
	newRt := [32]byte{'B'}
	br[0] = newRt[:]
	require.NoError(t, beaconState.SetBlockRoots(br))
	att2.Data.Target.Root = rt[:]
	att2.Data.BeaconBlockRoot = newRt[:]
	err = beaconState.AppendPreviousEpochAttestations(&ethpb.PendingAttestation{
		Data: att1.Data, AggregationBits: bf, InclusionDelay: 1,
	})
	require.NoError(t, err)
	err = beaconState.AppendCurrentEpochAttestations(&ethpb.PendingAttestation{
		Data: att2.Data, AggregationBits: bf, InclusionDelay: 1,
	})
	require.NoError(t, err)

	b := util.NewBeaconBlock()
	b.Block.Slot = 0
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	gen := stategen.New(beaconDB, doublylinkedtree.New())
	require.NoError(t, gen.SaveState(ctx, gRoot, beaconState))
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	s := &Server{
		CoreService: &core.Service{
			StateGen:           gen,
			GenesisTimeFetcher: &chainMock.ChainService{},
		},
	}
	addDefaultReplayerBuilder(s, beaconDB)

	request := &structs.GetIndividualVotesRequest{
		Indices: []string{"0", "1"},
		Epoch:   "0",
	}
	errStr, resp := individualVotesHelper(t, request, s)
	require.Equal(t, "", errStr)
	want := &structs.GetIndividualVotesResponse{
		IndividualVotes: []*structs.IndividualVote{
			{
				Epoch:                            "0",
				ValidatorIndex:                   "0",
				PublicKey:                        hexutil.Encode(beaconState.Validators()[0].PublicKey),
				IsActiveInCurrentEpoch:           true,
				IsActiveInPreviousEpoch:          true,
				CurrentEpochEffectiveBalanceGwei: fmt.Sprintf("%d", params.BeaconConfig().MaxEffectiveBalance),
				InclusionSlot:                    fmt.Sprintf("%d", params.BeaconConfig().FarFutureSlot),
				InclusionDistance:                fmt.Sprintf("%d", params.BeaconConfig().FarFutureSlot),
				InactivityScore:                  "0",
			},
			{
				Epoch:                            "0",
				ValidatorIndex:                   "1",
				PublicKey:                        hexutil.Encode(beaconState.Validators()[1].PublicKey),
				IsActiveInCurrentEpoch:           true,
				IsActiveInPreviousEpoch:          true,
				CurrentEpochEffectiveBalanceGwei: fmt.Sprintf("%d", params.BeaconConfig().MaxEffectiveBalance),
				InclusionSlot:                    fmt.Sprintf("%d", params.BeaconConfig().FarFutureSlot),
				InclusionDistance:                fmt.Sprintf("%d", params.BeaconConfig().FarFutureSlot),
				InactivityScore:                  "0",
			},
		},
	}
	assert.DeepEqual(t, want, resp, "Unexpected response")
}

func TestServer_GetIndividualVotes_WorkingAltair(t *testing.T) {
	helpers.ClearCache()
	beaconDB := dbTest.SetupDB(t)
	ctx := context.Background()

	var slot primitives.Slot = 0
	validators := uint64(32)
	beaconState, _ := util.DeterministicGenesisStateAltair(t, validators)
	require.NoError(t, beaconState.SetSlot(slot))

	pb, err := beaconState.CurrentEpochParticipation()
	require.NoError(t, err)
	for i := range pb {
		pb[i] = 0xff
	}
	require.NoError(t, beaconState.SetCurrentParticipationBits(pb))
	require.NoError(t, beaconState.SetPreviousParticipationBits(pb))

	b := util.NewBeaconBlock()
	b.Block.Slot = slot
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	gen := stategen.New(beaconDB, doublylinkedtree.New())
	require.NoError(t, gen.SaveState(ctx, gRoot, beaconState))
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	s := &Server{
		CoreService: &core.Service{
			StateGen:           gen,
			GenesisTimeFetcher: &chainMock.ChainService{},
		},
	}
	addDefaultReplayerBuilder(s, beaconDB)

	request := &structs.GetIndividualVotesRequest{
		Indices: []string{"0", "1"},
		Epoch:   "0",
	}
	errStr, resp := individualVotesHelper(t, request, s)
	require.Equal(t, "", errStr)
	want := &structs.GetIndividualVotesResponse{
		IndividualVotes: []*structs.IndividualVote{
			{
				Epoch:                            "0",
				ValidatorIndex:                   "0",
				PublicKey:                        hexutil.Encode(beaconState.Validators()[0].PublicKey),
				IsActiveInCurrentEpoch:           true,
				IsActiveInPreviousEpoch:          true,
				IsCurrentEpochTargetAttester:     true,
				IsCurrentEpochAttester:           true,
				IsPreviousEpochAttester:          true,
				IsPreviousEpochHeadAttester:      true,
				IsPreviousEpochTargetAttester:    true,
				CurrentEpochEffectiveBalanceGwei: fmt.Sprintf("%d", params.BeaconConfig().MaxEffectiveBalance),
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
			{
				Epoch:                            "0",
				ValidatorIndex:                   "1",
				PublicKey:                        hexutil.Encode(beaconState.Validators()[1].PublicKey),
				IsActiveInCurrentEpoch:           true,
				IsActiveInPreviousEpoch:          true,
				IsCurrentEpochTargetAttester:     true,
				IsCurrentEpochAttester:           true,
				IsPreviousEpochAttester:          true,
				IsPreviousEpochHeadAttester:      true,
				IsPreviousEpochTargetAttester:    true,
				CurrentEpochEffectiveBalanceGwei: fmt.Sprintf("%d", params.BeaconConfig().MaxEffectiveBalance),
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
		},
	}
	assert.DeepEqual(t, want, resp, "Unexpected response")
}

func TestServer_GetIndividualVotes_AltairEndOfEpoch(t *testing.T) {
	helpers.ClearCache()
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.BeaconConfig())
	beaconDB := dbTest.SetupDB(t)
	ctx := context.Background()

	validators := uint64(32)
	beaconState, _ := util.DeterministicGenesisStateAltair(t, validators)
	startSlot, err := slots.EpochStart(1)
	assert.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(startSlot))

	b := util.NewBeaconBlock()
	b.Block.Slot = startSlot
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	gen := stategen.New(beaconDB, doublylinkedtree.New())
	require.NoError(t, gen.SaveState(ctx, gRoot, beaconState))
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	// Save State at the end of the epoch:
	endSlot, err := slots.EpochEnd(1)
	assert.NoError(t, err)

	beaconState, _ = util.DeterministicGenesisStateAltair(t, validators)
	require.NoError(t, beaconState.SetSlot(endSlot))

	pb, err := beaconState.CurrentEpochParticipation()
	require.NoError(t, err)
	for i := range pb {
		pb[i] = 0xff
	}
	require.NoError(t, beaconState.SetCurrentParticipationBits(pb))
	require.NoError(t, beaconState.SetPreviousParticipationBits(pb))

	b.Block.Slot = endSlot
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err = b.Block.HashTreeRoot()
	require.NoError(t, err)

	require.NoError(t, gen.SaveState(ctx, gRoot, beaconState))
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	s := &Server{
		CoreService: &core.Service{
			StateGen:           gen,
			GenesisTimeFetcher: &chainMock.ChainService{},
		},
	}
	addDefaultReplayerBuilder(s, beaconDB)

	request := &structs.GetIndividualVotesRequest{
		Indices: []string{"0", "1"},
		Epoch:   "1",
	}
	errStr, resp := individualVotesHelper(t, request, s)
	require.Equal(t, "", errStr)
	want := &structs.GetIndividualVotesResponse{
		IndividualVotes: []*structs.IndividualVote{
			{
				Epoch:                            "1",
				ValidatorIndex:                   "0",
				PublicKey:                        hexutil.Encode(beaconState.Validators()[0].PublicKey),
				IsActiveInCurrentEpoch:           true,
				IsActiveInPreviousEpoch:          true,
				IsCurrentEpochTargetAttester:     true,
				IsCurrentEpochAttester:           true,
				IsPreviousEpochAttester:          true,
				IsPreviousEpochHeadAttester:      true,
				IsPreviousEpochTargetAttester:    true,
				CurrentEpochEffectiveBalanceGwei: fmt.Sprintf("%d", params.BeaconConfig().MaxEffectiveBalance),
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
			{
				Epoch:                            "1",
				ValidatorIndex:                   "1",
				PublicKey:                        hexutil.Encode(beaconState.Validators()[1].PublicKey),
				IsActiveInCurrentEpoch:           true,
				IsActiveInPreviousEpoch:          true,
				IsCurrentEpochTargetAttester:     true,
				IsCurrentEpochAttester:           true,
				IsPreviousEpochAttester:          true,
				IsPreviousEpochHeadAttester:      true,
				IsPreviousEpochTargetAttester:    true,
				CurrentEpochEffectiveBalanceGwei: fmt.Sprintf("%d", params.BeaconConfig().MaxEffectiveBalance),
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
		},
	}
	assert.DeepEqual(t, want, resp, "Unexpected response")
}

func TestServer_GetIndividualVotes_BellatrixEndOfEpoch(t *testing.T) {
	helpers.ClearCache()
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.BeaconConfig())
	beaconDB := dbTest.SetupDB(t)
	ctx := context.Background()

	validators := uint64(32)
	beaconState, _ := util.DeterministicGenesisStateBellatrix(t, validators)
	startSlot, err := slots.EpochStart(1)
	assert.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(startSlot))

	b := util.NewBeaconBlock()
	b.Block.Slot = startSlot
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	gen := stategen.New(beaconDB, doublylinkedtree.New())
	require.NoError(t, gen.SaveState(ctx, gRoot, beaconState))
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	// Save State at the end of the epoch:
	endSlot, err := slots.EpochEnd(1)
	assert.NoError(t, err)

	beaconState, _ = util.DeterministicGenesisStateBellatrix(t, validators)
	require.NoError(t, beaconState.SetSlot(endSlot))

	pb, err := beaconState.CurrentEpochParticipation()
	require.NoError(t, err)
	for i := range pb {
		pb[i] = 0xff
	}
	require.NoError(t, beaconState.SetCurrentParticipationBits(pb))
	require.NoError(t, beaconState.SetPreviousParticipationBits(pb))

	b.Block.Slot = endSlot
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err = b.Block.HashTreeRoot()
	require.NoError(t, err)

	require.NoError(t, gen.SaveState(ctx, gRoot, beaconState))
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	s := &Server{
		CoreService: &core.Service{
			StateGen:           gen,
			GenesisTimeFetcher: &chainMock.ChainService{},
		},
	}
	addDefaultReplayerBuilder(s, beaconDB)

	request := &structs.GetIndividualVotesRequest{
		Indices: []string{"0", "1"},
		Epoch:   "1",
	}
	errStr, resp := individualVotesHelper(t, request, s)
	require.Equal(t, "", errStr)
	want := &structs.GetIndividualVotesResponse{
		IndividualVotes: []*structs.IndividualVote{
			{
				Epoch:                            "1",
				ValidatorIndex:                   "0",
				PublicKey:                        hexutil.Encode(beaconState.Validators()[0].PublicKey),
				IsActiveInCurrentEpoch:           true,
				IsActiveInPreviousEpoch:          true,
				IsCurrentEpochTargetAttester:     true,
				IsCurrentEpochAttester:           true,
				IsPreviousEpochAttester:          true,
				IsPreviousEpochHeadAttester:      true,
				IsPreviousEpochTargetAttester:    true,
				CurrentEpochEffectiveBalanceGwei: fmt.Sprintf("%d", params.BeaconConfig().MaxEffectiveBalance),
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
			{
				Epoch:                            "1",
				ValidatorIndex:                   "1",
				PublicKey:                        hexutil.Encode(beaconState.Validators()[1].PublicKey),
				IsActiveInCurrentEpoch:           true,
				IsActiveInPreviousEpoch:          true,
				IsCurrentEpochTargetAttester:     true,
				IsCurrentEpochAttester:           true,
				IsPreviousEpochAttester:          true,
				IsPreviousEpochHeadAttester:      true,
				IsPreviousEpochTargetAttester:    true,
				CurrentEpochEffectiveBalanceGwei: fmt.Sprintf("%d", params.BeaconConfig().MaxEffectiveBalance),
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
		},
	}
	assert.DeepEqual(t, want, resp, "Unexpected response")
}

func TestServer_GetIndividualVotes_CapellaEndOfEpoch(t *testing.T) {
	helpers.ClearCache()
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.BeaconConfig())
	beaconDB := dbTest.SetupDB(t)
	ctx := context.Background()

	validators := uint64(32)
	beaconState, _ := util.DeterministicGenesisStateCapella(t, validators)
	startSlot, err := slots.EpochStart(1)
	assert.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(startSlot))

	b := util.NewBeaconBlock()
	b.Block.Slot = startSlot
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	gen := stategen.New(beaconDB, doublylinkedtree.New())
	require.NoError(t, gen.SaveState(ctx, gRoot, beaconState))
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	// Save State at the end of the epoch:
	endSlot, err := slots.EpochEnd(1)
	assert.NoError(t, err)

	beaconState, _ = util.DeterministicGenesisStateCapella(t, validators)
	require.NoError(t, beaconState.SetSlot(endSlot))

	pb, err := beaconState.CurrentEpochParticipation()
	require.NoError(t, err)
	for i := range pb {
		pb[i] = 0xff
	}
	require.NoError(t, beaconState.SetCurrentParticipationBits(pb))
	require.NoError(t, beaconState.SetPreviousParticipationBits(pb))

	b.Block.Slot = endSlot
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err = b.Block.HashTreeRoot()
	require.NoError(t, err)

	require.NoError(t, gen.SaveState(ctx, gRoot, beaconState))
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	s := &Server{
		CoreService: &core.Service{
			StateGen:           gen,
			GenesisTimeFetcher: &chainMock.ChainService{},
		},
	}
	addDefaultReplayerBuilder(s, beaconDB)

	request := &structs.GetIndividualVotesRequest{
		Indices: []string{"0", "1"},
		Epoch:   "1",
	}
	errStr, resp := individualVotesHelper(t, request, s)
	require.Equal(t, "", errStr)
	want := &structs.GetIndividualVotesResponse{
		IndividualVotes: []*structs.IndividualVote{
			{
				Epoch:                            "1",
				ValidatorIndex:                   "0",
				PublicKey:                        hexutil.Encode(beaconState.Validators()[0].PublicKey),
				IsActiveInCurrentEpoch:           true,
				IsActiveInPreviousEpoch:          true,
				IsCurrentEpochTargetAttester:     true,
				IsCurrentEpochAttester:           true,
				IsPreviousEpochAttester:          true,
				IsPreviousEpochHeadAttester:      true,
				IsPreviousEpochTargetAttester:    true,
				CurrentEpochEffectiveBalanceGwei: fmt.Sprintf("%d", params.BeaconConfig().MaxEffectiveBalance),
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
			{
				Epoch:                            "1",
				ValidatorIndex:                   "1",
				PublicKey:                        hexutil.Encode(beaconState.Validators()[1].PublicKey),
				IsActiveInCurrentEpoch:           true,
				IsActiveInPreviousEpoch:          true,
				IsCurrentEpochTargetAttester:     true,
				IsCurrentEpochAttester:           true,
				IsPreviousEpochAttester:          true,
				IsPreviousEpochHeadAttester:      true,
				IsPreviousEpochTargetAttester:    true,
				CurrentEpochEffectiveBalanceGwei: fmt.Sprintf("%d", params.BeaconConfig().MaxEffectiveBalance),
				InclusionSlot:                    "0",
				InclusionDistance:                "0",
				InactivityScore:                  "0",
			},
		},
	}
	assert.DeepEqual(t, want, resp, "Unexpected response")
}

// ensures that if any of the checkpoints are zero-valued, an error will be generated without genesis being present
func TestServer_GetChainHead_NoGenesis(t *testing.T) {
	db := dbTest.SetupDB(t)

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.SetSlot(1))

	genBlock := util.NewBeaconBlock()
	genBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'G'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, genBlock)
	gRoot, err := genBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	cases := []struct {
		name       string
		zeroSetter func(val *ethpb.Checkpoint) error
	}{
		{
			name:       "zero-value prev justified",
			zeroSetter: s.SetPreviousJustifiedCheckpoint,
		},
		{
			name:       "zero-value current justified",
			zeroSetter: s.SetCurrentJustifiedCheckpoint,
		},
		{
			name:       "zero-value finalized",
			zeroSetter: s.SetFinalizedCheckpoint,
		},
	}
	finalized := &ethpb.Checkpoint{Epoch: 1, Root: gRoot[:]}
	prevJustified := &ethpb.Checkpoint{Epoch: 2, Root: gRoot[:]}
	justified := &ethpb.Checkpoint{Epoch: 3, Root: gRoot[:]}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.NoError(t, s.SetPreviousJustifiedCheckpoint(prevJustified))
			require.NoError(t, s.SetCurrentJustifiedCheckpoint(justified))
			require.NoError(t, s.SetFinalizedCheckpoint(finalized))
			require.NoError(t, c.zeroSetter(&ethpb.Checkpoint{Epoch: 0, Root: params.BeaconConfig().ZeroHash[:]}))
		})
		wsb, err := blocks.NewSignedBeaconBlock(genBlock)
		require.NoError(t, err)
		s := &Server{
			CoreService: &core.Service{
				BeaconDB:    db,
				HeadFetcher: &chainMock.ChainService{Block: wsb, State: s},
				FinalizedFetcher: &chainMock.ChainService{
					FinalizedCheckPoint:         s.FinalizedCheckpoint(),
					CurrentJustifiedCheckPoint:  s.CurrentJustifiedCheckpoint(),
					PreviousJustifiedCheckPoint: s.PreviousJustifiedCheckpoint(),
				},
				OptimisticModeFetcher: &chainMock.ChainService{},
			},
		}
		url := "http://example.com"
		request := httptest.NewRequest(http.MethodGet, url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetChainHead(writer, request)
		require.Equal(t, http.StatusInternalServerError, writer.Code)
		require.StringContains(t, "could not get genesis block", writer.Body.String())
	}
}

func TestServer_GetChainHead_NoFinalizedBlock(t *testing.T) {
	db := dbTest.SetupDB(t)

	bs, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, bs.SetSlot(1))
	require.NoError(t, bs.SetPreviousJustifiedCheckpoint(&ethpb.Checkpoint{Epoch: 3, Root: bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)}))
	require.NoError(t, bs.SetCurrentJustifiedCheckpoint(&ethpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte{'B'}, fieldparams.RootLength)}))
	require.NoError(t, bs.SetFinalizedCheckpoint(&ethpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'C'}, fieldparams.RootLength)}))

	genBlock := util.NewBeaconBlock()
	genBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'G'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, genBlock)
	gRoot, err := genBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), gRoot))

	wsb, err := blocks.NewSignedBeaconBlock(genBlock)
	require.NoError(t, err)

	s := &Server{
		CoreService: &core.Service{
			BeaconDB:    db,
			HeadFetcher: &chainMock.ChainService{Block: wsb, State: bs},
			FinalizedFetcher: &chainMock.ChainService{
				FinalizedCheckPoint:         bs.FinalizedCheckpoint(),
				CurrentJustifiedCheckPoint:  bs.CurrentJustifiedCheckpoint(),
				PreviousJustifiedCheckPoint: bs.PreviousJustifiedCheckpoint()},
			OptimisticModeFetcher: &chainMock.ChainService{},
		},
	}

	url := "http://example.com"
	request := httptest.NewRequest(http.MethodGet, url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetChainHead(writer, request)
	require.Equal(t, http.StatusInternalServerError, writer.Code)
	require.StringContains(t, "ould not get finalized block", writer.Body.String())
}

func TestServer_GetChainHead_NoHeadBlock(t *testing.T) {
	s := &Server{
		CoreService: &core.Service{
			HeadFetcher:           &chainMock.ChainService{Block: nil},
			OptimisticModeFetcher: &chainMock.ChainService{},
		},
	}
	url := "http://example.com"
	request := httptest.NewRequest(http.MethodGet, url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetChainHead(writer, request)
	require.Equal(t, http.StatusNotFound, writer.Code)
	require.StringContains(t, "head block of chain was nil", writer.Body.String())
}

func TestServer_GetChainHead(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())

	db := dbTest.SetupDB(t)
	genBlock := util.NewBeaconBlock()
	genBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'G'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, genBlock)
	gRoot, err := genBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), gRoot))

	finalizedBlock := util.NewBeaconBlock()
	finalizedBlock.Block.Slot = 1
	finalizedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, finalizedBlock)
	fRoot, err := finalizedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	justifiedBlock := util.NewBeaconBlock()
	justifiedBlock.Block.Slot = 2
	justifiedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'B'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, justifiedBlock)
	jRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	prevJustifiedBlock := util.NewBeaconBlock()
	prevJustifiedBlock.Block.Slot = 3
	prevJustifiedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'C'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, prevJustifiedBlock)
	pjRoot, err := prevJustifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	st, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot:                        1,
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{Epoch: 3, Root: pjRoot[:]},
		CurrentJustifiedCheckpoint:  &ethpb.Checkpoint{Epoch: 2, Root: jRoot[:]},
		FinalizedCheckpoint:         &ethpb.Checkpoint{Epoch: 1, Root: fRoot[:]},
	})
	require.NoError(t, err)

	b := util.NewBeaconBlock()
	b.Block.Slot, err = slots.EpochStart(st.PreviousJustifiedCheckpoint().Epoch)
	require.NoError(t, err)
	b.Block.Slot++
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	s := &Server{
		CoreService: &core.Service{
			BeaconDB:              db,
			HeadFetcher:           &chainMock.ChainService{Block: wsb, State: st},
			OptimisticModeFetcher: &chainMock.ChainService{},
			FinalizedFetcher: &chainMock.ChainService{
				FinalizedCheckPoint:         st.FinalizedCheckpoint(),
				CurrentJustifiedCheckPoint:  st.CurrentJustifiedCheckpoint(),
				PreviousJustifiedCheckPoint: st.PreviousJustifiedCheckpoint()},
		},
	}

	url := "http://example.com"
	request := httptest.NewRequest(http.MethodGet, url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetChainHead(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)

	var ch *structs.ChainHead
	err = json.NewDecoder(writer.Body).Decode(&ch)
	require.NoError(t, err)

	assert.Equal(t, "3", ch.PreviousJustifiedEpoch, "Unexpected PreviousJustifiedEpoch")
	assert.Equal(t, "2", ch.JustifiedEpoch, "Unexpected JustifiedEpoch")
	assert.Equal(t, "1", ch.FinalizedEpoch, "Unexpected FinalizedEpoch")
	assert.Equal(t, "24", ch.PreviousJustifiedSlot, "Unexpected PreviousJustifiedSlot")
	assert.Equal(t, "16", ch.JustifiedSlot, "Unexpected JustifiedSlot")
	assert.Equal(t, "8", ch.FinalizedSlot, "Unexpected FinalizedSlot")
	assert.DeepEqual(t, hexutil.Encode(pjRoot[:]), ch.PreviousJustifiedBlockRoot, "Unexpected PreviousJustifiedBlockRoot")
	assert.DeepEqual(t, hexutil.Encode(jRoot[:]), ch.JustifiedBlockRoot, "Unexpected JustifiedBlockRoot")
	assert.DeepEqual(t, hexutil.Encode(fRoot[:]), ch.FinalizedBlockRoot, "Unexpected FinalizedBlockRoot")
	assert.Equal(t, false, ch.OptimisticStatus)
}

func TestPublishBlobs_InvalidJson(t *testing.T) {
	server := &Server{
		BlobReceiver: &chainMock.ChainService{},
		Broadcaster:  &mockp2p.MockBroadcaster{},
		SyncChecker:  &mockSync.Sync{IsSyncing: false},
	}

	request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.InvalidJson)))
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.PublishBlobs(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
	assert.StringContains(t, "Could not decode JSON request body", writer.Body.String())

	assert.Equal(t, len(server.BlobReceiver.(*chainMock.ChainService).Blobs), 0)
	assert.Equal(t, server.Broadcaster.(*mockp2p.MockBroadcaster).BroadcastCalled.Load(), false)
}

func TestPublishBlobs_MissingBlob(t *testing.T) {
	server := &Server{
		BlobReceiver: &chainMock.ChainService{},
		Broadcaster:  &mockp2p.MockBroadcaster{},
		SyncChecker:  &mockSync.Sync{IsSyncing: false},
	}

	request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.PublishBlobsRequestMissingBlob)))
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.PublishBlobs(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
	assert.StringContains(t, "Could not decode blob sidecar", writer.Body.String())

	assert.Equal(t, len(server.BlobReceiver.(*chainMock.ChainService).Blobs), 0)
	assert.Equal(t, server.Broadcaster.(*mockp2p.MockBroadcaster).BroadcastCalled.Load(), false)
}

func TestPublishBlobs_MissingSignedBlockHeader(t *testing.T) {
	server := &Server{
		BlobReceiver: &chainMock.ChainService{},
		Broadcaster:  &mockp2p.MockBroadcaster{},
		SyncChecker:  &mockSync.Sync{IsSyncing: false},
	}

	request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.PublishBlobsRequestMissingSignedBlockHeader)))
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.PublishBlobs(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
	assert.StringContains(t, "Could not decode blob sidecar", writer.Body.String())

	assert.Equal(t, len(server.BlobReceiver.(*chainMock.ChainService).Blobs), 0)
	assert.Equal(t, server.Broadcaster.(*mockp2p.MockBroadcaster).BroadcastCalled.Load(), false)
}

func TestPublishBlobs_MissingSidecars(t *testing.T) {
	server := &Server{
		BlobReceiver: &chainMock.ChainService{},
		Broadcaster:  &mockp2p.MockBroadcaster{},
		SyncChecker:  &mockSync.Sync{IsSyncing: false},
	}

	request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.PublishBlobsRequestMissingSidecars)))
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.PublishBlobs(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
	assert.StringContains(t, "Missing blob sidecars", writer.Body.String())

	assert.Equal(t, len(server.BlobReceiver.(*chainMock.ChainService).Blobs), 0)
	assert.Equal(t, server.Broadcaster.(*mockp2p.MockBroadcaster).BroadcastCalled.Load(), false)
}

func TestPublishBlobs_EmptySidecarsList(t *testing.T) {
	server := &Server{
		BlobReceiver: &chainMock.ChainService{},
		Broadcaster:  &mockp2p.MockBroadcaster{},
		SyncChecker:  &mockSync.Sync{IsSyncing: false},
	}

	request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.PublishBlobsRequestEmptySidecarsList)))
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.PublishBlobs(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)

	assert.Equal(t, len(server.BlobReceiver.(*chainMock.ChainService).Blobs), 0)
	assert.Equal(t, server.Broadcaster.(*mockp2p.MockBroadcaster).BroadcastCalled.Load(), false)
}

func TestPublishBlobs_NullSidecar(t *testing.T) {
	server := &Server{
		BlobReceiver: &chainMock.ChainService{},
		Broadcaster:  &mockp2p.MockBroadcaster{},
		SyncChecker:  &mockSync.Sync{IsSyncing: false},
	}

	request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.PublishBlobsRequestNullSidecar)))
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.PublishBlobs(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
	assert.StringContains(t, "Could not decode blob sidecar", writer.Body.String())

	assert.Equal(t, len(server.BlobReceiver.(*chainMock.ChainService).Blobs), 0)
	assert.Equal(t, server.Broadcaster.(*mockp2p.MockBroadcaster).BroadcastCalled.Load(), false)
}

func TestPublishBlobs_SeveralFieldsMissing(t *testing.T) {
	server := &Server{
		BlobReceiver: &chainMock.ChainService{},
		Broadcaster:  &mockp2p.MockBroadcaster{},
		SyncChecker:  &mockSync.Sync{IsSyncing: false},
	}

	request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.PublishBlobsRequestSeveralFieldsMissing)))
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.PublishBlobs(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
	assert.StringContains(t, "Could not decode blob sidecar", writer.Body.String())

	assert.Equal(t, len(server.BlobReceiver.(*chainMock.ChainService).Blobs), 0)
	assert.Equal(t, server.Broadcaster.(*mockp2p.MockBroadcaster).BroadcastCalled.Load(), false)
}

func TestPublishBlobs_BadBlockRoot(t *testing.T) {
	server := &Server{
		BlobReceiver: &chainMock.ChainService{},
		Broadcaster:  &mockp2p.MockBroadcaster{},
		SyncChecker:  &mockSync.Sync{IsSyncing: false},
	}

	request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.PublishBlobsRequestBadBlockRoot)))
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.PublishBlobs(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
	assert.StringContains(t, "Could not decode block root", writer.Body.String())

	assert.Equal(t, len(server.BlobReceiver.(*chainMock.ChainService).Blobs), 0)
	assert.Equal(t, server.Broadcaster.(*mockp2p.MockBroadcaster).BroadcastCalled.Load(), false)
}

func TestPublishBlobs(t *testing.T) {
	server := &Server{
		BlobReceiver: &chainMock.ChainService{},
		Broadcaster:  &mockp2p.MockBroadcaster{},
		SyncChecker:  &mockSync.Sync{IsSyncing: false},
	}

	request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.PublishBlobsRequest)))
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	server.PublishBlobs(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)

	assert.Equal(t, len(server.BlobReceiver.(*chainMock.ChainService).Blobs), 1)
	assert.Equal(t, server.Broadcaster.(*mockp2p.MockBroadcaster).BroadcastCalled.Load(), true)
}
