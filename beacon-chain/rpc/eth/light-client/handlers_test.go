package lightclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	lightclient "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/light-client"
	dbtesting "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/testutil"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	light_client "github.com/prysmaticlabs/prysm/v5/consensus-types/light-client"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestLightClientHandler_GetLightClientBootstrap(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.AltairForkEpoch = 0
	cfg.BellatrixForkEpoch = 1
	cfg.CapellaForkEpoch = 2
	cfg.DenebForkEpoch = 3
	cfg.ElectraForkEpoch = 4
	params.OverrideBeaconConfig(cfg)

	t.Run("altair", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestAltair()

		slot := primitives.Slot(params.BeaconConfig().AltairForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)
		stateRoot, err := l.State.HashTreeRoot(l.Ctx)
		require.NoError(t, err)

		mockBlocker := &testutil.MockBlocker{BlockToReturn: l.Block}
		mockChainService := &mock.ChainService{Optimistic: true, Slot: &slot}
		mockChainInfoFetcher := &mock.ChainService{Slot: &slot}
		s := &Server{
			Stater: &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{
				slot: l.State,
			}},
			Blocker:          mockBlocker,
			HeadFetcher:      mockChainService,
			ChainInfoFetcher: mockChainInfoFetcher,
		}
		request := httptest.NewRequest("GET", "http://foo.com/", nil)
		request.SetPathValue("block_root", hexutil.Encode(stateRoot[:]))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLightClientBootstrap(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		var resp structs.LightClientBootstrapResponse
		err = json.Unmarshal(writer.Body.Bytes(), &resp)
		require.NoError(t, err)
		var respHeader structs.LightClientHeader
		err = json.Unmarshal(resp.Data.Header, &respHeader)
		require.NoError(t, err)
		require.Equal(t, "altair", resp.Version)

		blockHeader, err := l.Block.Header()
		require.NoError(t, err)
		require.Equal(t, hexutil.Encode(blockHeader.Header.BodyRoot), respHeader.Beacon.BodyRoot)
		require.Equal(t, strconv.FormatUint(uint64(blockHeader.Header.Slot), 10), respHeader.Beacon.Slot)

		require.NotNil(t, resp.Data.CurrentSyncCommittee)
		require.NotNil(t, resp.Data.CurrentSyncCommitteeBranch)
	})
	t.Run("bellatrix", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestBellatrix()

		slot := primitives.Slot(params.BeaconConfig().BellatrixForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)
		blockRoot, err := l.Block.Block().HashTreeRoot()
		require.NoError(t, err)

		mockBlocker := &testutil.MockBlocker{BlockToReturn: l.Block}
		mockChainService := &mock.ChainService{Optimistic: true, Slot: &slot}
		mockChainInfoFetcher := &mock.ChainService{Slot: &slot}
		s := &Server{
			Stater: &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{
				slot: l.State,
			}},
			Blocker:          mockBlocker,
			HeadFetcher:      mockChainService,
			ChainInfoFetcher: mockChainInfoFetcher,
		}
		request := httptest.NewRequest("GET", "http://foo.com/", nil)
		request.SetPathValue("block_root", hexutil.Encode(blockRoot[:]))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLightClientBootstrap(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		var resp structs.LightClientBootstrapResponse
		err = json.Unmarshal(writer.Body.Bytes(), &resp)
		require.NoError(t, err)
		var respHeader structs.LightClientHeader
		err = json.Unmarshal(resp.Data.Header, &respHeader)
		require.NoError(t, err)
		require.Equal(t, "altair", resp.Version)

		blockHeader, err := l.Block.Header()
		require.NoError(t, err)
		require.Equal(t, hexutil.Encode(blockHeader.Header.BodyRoot), respHeader.Beacon.BodyRoot)
		require.Equal(t, strconv.FormatUint(uint64(blockHeader.Header.Slot), 10), respHeader.Beacon.Slot)

		require.NotNil(t, resp.Data.CurrentSyncCommittee)
		require.NotNil(t, resp.Data.CurrentSyncCommitteeBranch)
	})
	t.Run("capella", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestCapella(false) // result is same for true and false

		slot := primitives.Slot(params.BeaconConfig().CapellaForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)
		blockRoot, err := l.Block.Block().HashTreeRoot()
		require.NoError(t, err)

		mockBlocker := &testutil.MockBlocker{BlockToReturn: l.Block}
		mockChainService := &mock.ChainService{Optimistic: true, Slot: &slot}
		mockChainInfoFetcher := &mock.ChainService{Slot: &slot}
		s := &Server{
			Stater: &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{
				slot: l.State,
			}},
			Blocker:          mockBlocker,
			HeadFetcher:      mockChainService,
			ChainInfoFetcher: mockChainInfoFetcher,
		}
		request := httptest.NewRequest("GET", "http://foo.com/", nil)
		request.SetPathValue("block_root", hexutil.Encode(blockRoot[:]))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLightClientBootstrap(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		var resp structs.LightClientBootstrapResponse
		err = json.Unmarshal(writer.Body.Bytes(), &resp)
		require.NoError(t, err)
		var respHeader structs.LightClientHeader
		err = json.Unmarshal(resp.Data.Header, &respHeader)
		require.NoError(t, err)
		require.Equal(t, "capella", resp.Version)

		blockHeader, err := l.Block.Header()
		require.NoError(t, err)
		require.Equal(t, hexutil.Encode(blockHeader.Header.BodyRoot), respHeader.Beacon.BodyRoot)
		require.Equal(t, strconv.FormatUint(uint64(blockHeader.Header.Slot), 10), respHeader.Beacon.Slot)

		require.NotNil(t, resp.Data.CurrentSyncCommittee)
		require.NotNil(t, resp.Data.CurrentSyncCommitteeBranch)
	})
	t.Run("deneb", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestDeneb(false) // result is same for true and false

		slot := primitives.Slot(params.BeaconConfig().DenebForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)
		blockRoot, err := l.Block.Block().HashTreeRoot()
		require.NoError(t, err)

		mockBlocker := &testutil.MockBlocker{BlockToReturn: l.Block}
		mockChainService := &mock.ChainService{Optimistic: true, Slot: &slot}
		mockChainInfoFetcher := &mock.ChainService{Slot: &slot}
		s := &Server{
			Stater: &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{
				slot: l.State,
			}},
			Blocker:          mockBlocker,
			HeadFetcher:      mockChainService,
			ChainInfoFetcher: mockChainInfoFetcher,
		}
		request := httptest.NewRequest("GET", "http://foo.com/", nil)
		request.SetPathValue("block_root", hexutil.Encode(blockRoot[:]))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLightClientBootstrap(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		var resp structs.LightClientBootstrapResponse
		err = json.Unmarshal(writer.Body.Bytes(), &resp)
		require.NoError(t, err)
		var respHeader structs.LightClientHeader
		err = json.Unmarshal(resp.Data.Header, &respHeader)
		require.NoError(t, err)
		require.Equal(t, "deneb", resp.Version)

		blockHeader, err := l.Block.Header()
		require.NoError(t, err)
		require.Equal(t, hexutil.Encode(blockHeader.Header.BodyRoot), respHeader.Beacon.BodyRoot)
		require.Equal(t, strconv.FormatUint(uint64(blockHeader.Header.Slot), 10), respHeader.Beacon.Slot)

		require.NotNil(t, resp.Data.CurrentSyncCommittee)
		require.NotNil(t, resp.Data.CurrentSyncCommitteeBranch)
	})
	t.Run("electra", func(t *testing.T) {
		l := util.NewTestLightClient(t).SetupTestElectra(false) // result is same for true and false

		slot := primitives.Slot(params.BeaconConfig().ElectraForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)
		blockRoot, err := l.Block.Block().HashTreeRoot()
		require.NoError(t, err)

		mockBlocker := &testutil.MockBlocker{BlockToReturn: l.Block}
		mockChainService := &mock.ChainService{Optimistic: true, Slot: &slot}
		mockChainInfoFetcher := &mock.ChainService{Slot: &slot}
		s := &Server{
			Stater: &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{
				slot: l.State,
			}},
			Blocker:          mockBlocker,
			HeadFetcher:      mockChainService,
			ChainInfoFetcher: mockChainInfoFetcher,
		}
		request := httptest.NewRequest("GET", "http://foo.com/", nil)
		request.SetPathValue("block_root", hexutil.Encode(blockRoot[:]))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLightClientBootstrap(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		var resp structs.LightClientBootstrapResponse
		err = json.Unmarshal(writer.Body.Bytes(), &resp)
		require.NoError(t, err)
		var respHeader structs.LightClientHeader
		err = json.Unmarshal(resp.Data.Header, &respHeader)
		require.NoError(t, err)
		require.Equal(t, "electra", resp.Version)

		blockHeader, err := l.Block.Header()
		require.NoError(t, err)
		require.Equal(t, hexutil.Encode(blockHeader.Header.BodyRoot), respHeader.Beacon.BodyRoot)
		require.Equal(t, strconv.FormatUint(uint64(blockHeader.Header.Slot), 10), respHeader.Beacon.Slot)

		require.NotNil(t, resp.Data.CurrentSyncCommittee)
		require.NotNil(t, resp.Data.CurrentSyncCommitteeBranch)
	})
}

// GetLightClientByRange tests

func TestLightClientHandler_GetLightClientUpdatesByRangeAltair(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()

	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 0
	params.OverrideBeaconConfig(config)

	slot := primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	err = st.SetSlot(slot)
	require.NoError(t, err)

	db := dbtesting.SetupDB(t)

	updatePeriod := uint64(slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch)))

	update, err := createUpdate(t, version.Altair)
	require.NoError(t, err)
	err = db.SaveLightClientUpdate(ctx, updatePeriod, update)
	require.NoError(t, err)

	mockChainService := &mock.ChainService{State: st}
	s := &Server{
		HeadFetcher: mockChainService,
		BeaconDB:    db,
	}
	startPeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))
	url := fmt.Sprintf("http://foo.com/?count=1&start_period=%d", startPeriod)
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientUpdatesByRange(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp structs.LightClientUpdatesByRangeResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Updates))
	require.Equal(t, "altair", resp.Updates[0].Version)
	updateJson, err := structs.LightClientUpdateFromConsensus(update)
	require.NoError(t, err)
	require.DeepEqual(t, updateJson, resp.Updates[0].Data)
}

func TestLightClientHandler_GetLightClientUpdatesByRangeCapella(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 0
	config.CapellaForkEpoch = 1
	params.OverrideBeaconConfig(config)
	slot := primitives.Slot(config.CapellaForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	err = st.SetSlot(slot)
	require.NoError(t, err)

	db := dbtesting.SetupDB(t)

	updatePeriod := uint64(slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch)))

	update, err := createUpdate(t, version.Capella)
	require.NoError(t, err)

	err = db.SaveLightClientUpdate(ctx, updatePeriod, update)
	require.NoError(t, err)

	mockChainService := &mock.ChainService{State: st}
	s := &Server{
		HeadFetcher: mockChainService,
		BeaconDB:    db,
	}
	startPeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))
	url := fmt.Sprintf("http://foo.com/?count=1&start_period=%d", startPeriod)
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientUpdatesByRange(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp structs.LightClientUpdatesByRangeResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Updates))
	require.Equal(t, "capella", resp.Updates[0].Version)
	updateJson, err := structs.LightClientUpdateFromConsensus(update)
	require.NoError(t, err)
	require.DeepEqual(t, updateJson, resp.Updates[0].Data)
}

func TestLightClientHandler_GetLightClientUpdatesByRangeDeneb(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 0
	config.CapellaForkEpoch = 1
	config.DenebForkEpoch = 2
	params.OverrideBeaconConfig(config)
	slot := primitives.Slot(config.DenebForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	st, err := util.NewBeaconStateDeneb()
	require.NoError(t, err)
	err = st.SetSlot(slot)
	require.NoError(t, err)

	db := dbtesting.SetupDB(t)

	updatePeriod := uint64(slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch)))

	update, err := createUpdate(t, version.Deneb)
	require.NoError(t, err)
	err = db.SaveLightClientUpdate(ctx, updatePeriod, update)
	require.NoError(t, err)

	mockChainService := &mock.ChainService{State: st}
	s := &Server{
		HeadFetcher: mockChainService,
		BeaconDB:    db,
	}
	startPeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))
	url := fmt.Sprintf("http://foo.com/?count=1&start_period=%d", startPeriod)
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientUpdatesByRange(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp structs.LightClientUpdatesByRangeResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Updates))
	require.Equal(t, "deneb", resp.Updates[0].Version)
	updateJson, err := structs.LightClientUpdateFromConsensus(update)
	require.NoError(t, err)
	require.DeepEqual(t, updateJson, resp.Updates[0].Data)
}

func TestLightClientHandler_GetLightClientUpdatesByRangeMultipleAltair(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 0
	config.EpochsPerSyncCommitteePeriod = 1
	params.OverrideBeaconConfig(config)
	slot := primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	headSlot := slot.Add(2 * uint64(config.SlotsPerEpoch) * uint64(config.EpochsPerSyncCommitteePeriod)) // 2 periods
	err = st.SetSlot(headSlot)
	require.NoError(t, err)

	db := dbtesting.SetupDB(t)

	updatePeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

	updates := make([]interfaces.LightClientUpdate, 0)
	for i := 1; i <= 2; i++ {
		update, err := createUpdate(t, version.Altair)
		require.NoError(t, err)
		updates = append(updates, update)
	}

	for _, update := range updates {
		err := db.SaveLightClientUpdate(ctx, uint64(updatePeriod), update)
		require.NoError(t, err)
		updatePeriod++
	}

	mockChainService := &mock.ChainService{State: st}
	s := &Server{
		HeadFetcher: mockChainService,
		BeaconDB:    db,
	}
	startPeriod := slot.Sub(1).Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))
	url := fmt.Sprintf("http://foo.com/?count=100&start_period=%d", startPeriod)
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientUpdatesByRange(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp structs.LightClientUpdatesByRangeResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.Updates))
	for i, update := range updates {
		require.Equal(t, "altair", resp.Updates[i].Version)
		updateJson, err := structs.LightClientUpdateFromConsensus(update)
		require.NoError(t, err)
		require.DeepEqual(t, updateJson, resp.Updates[i].Data)
	}
}

func TestLightClientHandler_GetLightClientUpdatesByRangeMultipleCapella(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 0
	config.CapellaForkEpoch = 1
	config.EpochsPerSyncCommitteePeriod = 1
	params.OverrideBeaconConfig(config)
	slot := primitives.Slot(config.CapellaForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	headSlot := slot.Add(2 * uint64(config.SlotsPerEpoch) * uint64(config.EpochsPerSyncCommitteePeriod)) // 2 periods
	err = st.SetSlot(headSlot)
	require.NoError(t, err)

	db := dbtesting.SetupDB(t)

	updatePeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

	updates := make([]interfaces.LightClientUpdate, 0)
	for i := 0; i < 2; i++ {
		update, err := createUpdate(t, version.Capella)
		require.NoError(t, err)
		updates = append(updates, update)
	}

	for _, update := range updates {
		err := db.SaveLightClientUpdate(ctx, uint64(updatePeriod), update)
		require.NoError(t, err)
		updatePeriod++
	}

	mockChainService := &mock.ChainService{State: st}
	s := &Server{
		HeadFetcher: mockChainService,
		BeaconDB:    db,
	}
	startPeriod := slot.Sub(1).Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))
	url := fmt.Sprintf("http://foo.com/?count=100&start_period=%d", startPeriod)
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientUpdatesByRange(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp structs.LightClientUpdatesByRangeResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.Updates))
	for i, update := range updates {
		require.Equal(t, "capella", resp.Updates[i].Version)
		updateJson, err := structs.LightClientUpdateFromConsensus(update)
		require.NoError(t, err)
		require.DeepEqual(t, updateJson, resp.Updates[i].Data)
	}
}

func TestLightClientHandler_GetLightClientUpdatesByRangeMultipleDeneb(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 0
	config.CapellaForkEpoch = 1
	config.DenebForkEpoch = 2
	config.EpochsPerSyncCommitteePeriod = 1
	params.OverrideBeaconConfig(config)
	slot := primitives.Slot(config.DenebForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	headSlot := slot.Add(2 * uint64(config.SlotsPerEpoch) * uint64(config.EpochsPerSyncCommitteePeriod))
	err = st.SetSlot(headSlot)
	require.NoError(t, err)

	db := dbtesting.SetupDB(t)

	updatePeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

	updates := make([]interfaces.LightClientUpdate, 0)
	for i := 0; i < 2; i++ {
		update, err := createUpdate(t, version.Deneb)
		require.NoError(t, err)
		updates = append(updates, update)
	}

	for _, update := range updates {
		err := db.SaveLightClientUpdate(ctx, uint64(updatePeriod), update)
		require.NoError(t, err)
		updatePeriod++
	}
	mockChainService := &mock.ChainService{State: st}
	s := &Server{
		HeadFetcher: mockChainService,
		BeaconDB:    db,
	}
	startPeriod := slot.Sub(1).Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))
	url := fmt.Sprintf("http://foo.com/?count=100&start_period=%d", startPeriod)
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientUpdatesByRange(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp structs.LightClientUpdatesByRangeResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.Updates))
	for i, update := range updates {
		require.Equal(t, "deneb", resp.Updates[i].Version)
		updateJson, err := structs.LightClientUpdateFromConsensus(update)
		require.NoError(t, err)
		require.DeepEqual(t, updateJson, resp.Updates[i].Data)
	}
}

func TestLightClientHandler_GetLightClientUpdatesByRangeMultipleForksAltairCapella(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 0
	config.CapellaForkEpoch = 1
	config.EpochsPerSyncCommitteePeriod = 1
	params.OverrideBeaconConfig(config)
	slotCapella := primitives.Slot(config.CapellaForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
	slotAltair := primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	headSlot := slotCapella.Add(1)
	err = st.SetSlot(headSlot)
	require.NoError(t, err)

	db := dbtesting.SetupDB(t)

	updates := make([]interfaces.LightClientUpdate, 2)

	updatePeriod := slotAltair.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

	updates[0], err = createUpdate(t, version.Altair)
	require.NoError(t, err)

	err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[0])
	require.NoError(t, err)

	updatePeriod = slotCapella.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

	updates[1], err = createUpdate(t, version.Capella)
	require.NoError(t, err)

	err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[1])
	require.NoError(t, err)

	mockChainService := &mock.ChainService{State: st}
	s := &Server{
		HeadFetcher: mockChainService,
		BeaconDB:    db,
	}
	startPeriod := 0
	url := fmt.Sprintf("http://foo.com/?count=100&start_period=%d", startPeriod)
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientUpdatesByRange(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp structs.LightClientUpdatesByRangeResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.Updates))
	for i, update := range updates {
		if i < 1 {
			require.Equal(t, "altair", resp.Updates[i].Version)
		} else {
			require.Equal(t, "capella", resp.Updates[i].Version)
		}
		updateJson, err := structs.LightClientUpdateFromConsensus(update)
		require.NoError(t, err)
		require.DeepEqual(t, updateJson, resp.Updates[i].Data)
	}
}

func TestLightClientHandler_GetLightClientUpdatesByRangeMultipleForksCapellaDeneb(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 0
	config.CapellaForkEpoch = 1
	config.DenebForkEpoch = 2
	config.EpochsPerSyncCommitteePeriod = 1
	params.OverrideBeaconConfig(config)
	slotDeneb := primitives.Slot(config.DenebForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
	slotCapella := primitives.Slot(config.CapellaForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	headSlot := slotDeneb.Add(1)
	err = st.SetSlot(headSlot)
	require.NoError(t, err)

	db := dbtesting.SetupDB(t)

	updates := make([]interfaces.LightClientUpdate, 2)

	updatePeriod := slotCapella.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

	updates[0], err = createUpdate(t, version.Capella)
	require.NoError(t, err)

	err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[0])
	require.NoError(t, err)

	updatePeriod = slotDeneb.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

	updates[1], err = createUpdate(t, version.Deneb)
	require.NoError(t, err)

	err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[1])
	require.NoError(t, err)

	mockChainService := &mock.ChainService{State: st}
	s := &Server{
		HeadFetcher: mockChainService,
		BeaconDB:    db,
	}
	startPeriod := 1
	url := fmt.Sprintf("http://foo.com/?count=100&start_period=%d", startPeriod)
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientUpdatesByRange(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp structs.LightClientUpdatesByRangeResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.Updates))
	for i, update := range updates {
		if i < 1 {
			require.Equal(t, "capella", resp.Updates[i].Version)
		} else {
			require.Equal(t, "deneb", resp.Updates[i].Version)
		}
		updateJson, err := structs.LightClientUpdateFromConsensus(update)
		require.NoError(t, err)
		require.DeepEqual(t, updateJson, resp.Updates[i].Data)
	}
}

func TestLightClientHandler_GetLightClientUpdatesByRangeCountBiggerThanLimit(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 0
	config.EpochsPerSyncCommitteePeriod = 1
	config.MaxRequestLightClientUpdates = 2
	params.OverrideBeaconConfig(config)
	slot := primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	headSlot := slot.Add(4 * uint64(config.SlotsPerEpoch) * uint64(config.EpochsPerSyncCommitteePeriod)) // 4 periods
	err = st.SetSlot(headSlot)
	require.NoError(t, err)

	db := dbtesting.SetupDB(t)

	updates := make([]interfaces.LightClientUpdate, 3)

	updatePeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

	for i := 0; i < 3; i++ {

		updates[i], err = createUpdate(t, version.Altair)
		require.NoError(t, err)

		err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[i])
		require.NoError(t, err)

		updatePeriod++
	}

	mockChainService := &mock.ChainService{State: st}
	s := &Server{
		HeadFetcher: mockChainService,
		BeaconDB:    db,
	}
	startPeriod := 0
	url := fmt.Sprintf("http://foo.com/?count=4&start_period=%d", startPeriod)
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientUpdatesByRange(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp structs.LightClientUpdatesByRangeResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.Updates))
	for i, update := range updates {
		if i < 2 {
			require.Equal(t, "altair", resp.Updates[i].Version)
			updateJson, err := structs.LightClientUpdateFromConsensus(update)
			require.NoError(t, err)
			require.DeepEqual(t, updateJson, resp.Updates[i].Data)
		}
	}
}

func TestLightClientHandler_GetLightClientUpdatesByRangeCountBiggerThanMax(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 0
	config.EpochsPerSyncCommitteePeriod = 1
	config.MaxRequestLightClientUpdates = 2
	params.OverrideBeaconConfig(config)
	slot := primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	headSlot := slot.Add(4 * uint64(config.SlotsPerEpoch) * uint64(config.EpochsPerSyncCommitteePeriod)) // 4 periods
	err = st.SetSlot(headSlot)
	require.NoError(t, err)

	db := dbtesting.SetupDB(t)

	updates := make([]interfaces.LightClientUpdate, 3)

	updatePeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

	for i := 0; i < 3; i++ {
		updates[i], err = createUpdate(t, version.Altair)
		require.NoError(t, err)

		err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[i])
		require.NoError(t, err)

		updatePeriod++
	}

	mockChainService := &mock.ChainService{State: st}
	s := &Server{
		HeadFetcher: mockChainService,
		BeaconDB:    db,
	}
	startPeriod := 0
	url := fmt.Sprintf("http://foo.com/?count=10&start_period=%d", startPeriod)
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientUpdatesByRange(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp structs.LightClientUpdatesByRangeResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.Updates))
	for i, update := range updates {
		if i < 2 {
			require.Equal(t, "altair", resp.Updates[i].Version)
			updateJson, err := structs.LightClientUpdateFromConsensus(update)
			require.NoError(t, err)
			require.DeepEqual(t, updateJson, resp.Updates[i].Data)
		}
	}
}

func TestLightClientHandler_GetLightClientUpdatesByRangeStartPeriodBeforeAltair(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 1
	config.EpochsPerSyncCommitteePeriod = 1
	params.OverrideBeaconConfig(config)

	db := dbtesting.SetupDB(t)

	s := &Server{
		BeaconDB: db,
	}
	startPeriod := 0
	url := fmt.Sprintf("http://foo.com/?count=2&start_period=%d", startPeriod)
	request := httptest.NewRequest("GET", url, nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientUpdatesByRange(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp structs.LightClientUpdatesByRangeResponse
	err := json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
	require.NoError(t, err)
	require.Equal(t, 0, len(resp.Updates))
}

func TestLightClientHandler_GetLightClientUpdatesByRangeMissingUpdates(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 0
	config.EpochsPerSyncCommitteePeriod = 1
	params.OverrideBeaconConfig(config)
	slot := primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	headSlot := slot.Add(4 * uint64(config.SlotsPerEpoch) * uint64(config.EpochsPerSyncCommitteePeriod)) // 4 periods
	err = st.SetSlot(headSlot)
	require.NoError(t, err)

	t.Run("missing update in the middle", func(t *testing.T) {
		db := dbtesting.SetupDB(t)

		updates := make([]interfaces.LightClientUpdate, 3)

		updatePeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

		for i := 0; i < 3; i++ {
			if i == 1 { // skip this update
				updatePeriod++
				continue
			}

			updates[i], err = createUpdate(t, version.Altair)
			require.NoError(t, err)

			err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[i])
			require.NoError(t, err)

			updatePeriod++
		}

		mockChainService := &mock.ChainService{State: st}
		s := &Server{
			HeadFetcher: mockChainService,
			BeaconDB:    db,
		}
		startPeriod := 0
		url := fmt.Sprintf("http://foo.com/?count=10&start_period=%d", startPeriod)
		request := httptest.NewRequest("GET", url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLightClientUpdatesByRange(writer, request)

		require.Equal(t, http.StatusOK, writer.Code)
		var resp structs.LightClientUpdatesByRangeResponse
		err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
		require.NoError(t, err)
		require.Equal(t, 1, len(resp.Updates))
		require.Equal(t, "altair", resp.Updates[0].Version)
		updateJson, err := structs.LightClientUpdateFromConsensus(updates[0])
		require.NoError(t, err)
		require.DeepEqual(t, updateJson, resp.Updates[0].Data)
	})

	t.Run("missing update at the beginning", func(t *testing.T) {
		db := dbtesting.SetupDB(t)

		updates := make([]interfaces.LightClientUpdate, 3)

		updatePeriod := slot.Div(uint64(config.EpochsPerSyncCommitteePeriod)).Div(uint64(config.SlotsPerEpoch))

		for i := 0; i < 3; i++ {
			if i == 0 { // skip this update
				updatePeriod++
				continue
			}

			updates[i], err = createUpdate(t, version.Altair)
			require.NoError(t, err)

			err = db.SaveLightClientUpdate(ctx, uint64(updatePeriod), updates[i])
			require.NoError(t, err)

			updatePeriod++
		}

		mockChainService := &mock.ChainService{State: st}
		s := &Server{
			HeadFetcher: mockChainService,
			BeaconDB:    db,
		}
		startPeriod := 0
		url := fmt.Sprintf("http://foo.com/?count=10&start_period=%d", startPeriod)
		request := httptest.NewRequest("GET", url, nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetLightClientUpdatesByRange(writer, request)

		require.Equal(t, http.StatusOK, writer.Code)
		var resp structs.LightClientUpdatesByRangeResponse
		err = json.Unmarshal(writer.Body.Bytes(), &resp.Updates)
		require.NoError(t, err)
		require.Equal(t, 0, len(resp.Updates))
	})

}

func TestLightClientHandler_GetLightClientFinalityUpdate(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	config := params.BeaconConfig()
	slot := primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	attestedState, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	err = attestedState.SetSlot(slot.Sub(1))
	require.NoError(t, err)

	require.NoError(t, attestedState.SetFinalizedCheckpoint(&pb.Checkpoint{
		Epoch: config.AltairForkEpoch - 10,
		Root:  make([]byte, 32),
	}))

	parent := util.NewBeaconBlockAltair()
	parent.Block.Slot = slot.Sub(1)

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	parentHeader, err := signedParent.Header()
	require.NoError(t, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(t, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	err = st.SetSlot(slot)
	require.NoError(t, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(t, err)

	block := util.NewBeaconBlockAltair()
	block.Block.Slot = slot
	block.Block.ParentRoot = parentRoot[:]

	for i := uint64(0); i < config.SyncCommitteeSize; i++ {
		block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
	}

	signedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	h, err := signedBlock.Header()
	require.NoError(t, err)

	err = st.SetLatestBlockHeader(h.Header)
	require.NoError(t, err)
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	block.Block.StateRoot = stateRoot[:]
	signedBlock, err = blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	root, err := block.Block.HashTreeRoot()
	require.NoError(t, err)

	mockBlocker := &testutil.MockBlocker{
		RootBlockMap: map[[32]byte]interfaces.ReadOnlySignedBeaconBlock{
			parentRoot: signedParent,
			root:       signedBlock,
		},
		SlotBlockMap: map[primitives.Slot]interfaces.ReadOnlySignedBeaconBlock{
			slot.Sub(1): signedParent,
			slot:        signedBlock,
		},
	}
	mockChainService := &mock.ChainService{Optimistic: true, Slot: &slot, State: st, FinalizedRoots: map[[32]byte]bool{
		root: true,
	}}
	mockChainInfoFetcher := &mock.ChainService{Slot: &slot}
	s := &Server{
		Stater: &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{
			slot.Sub(1): attestedState,
			slot:        st,
		}},
		Blocker:          mockBlocker,
		HeadFetcher:      mockChainService,
		ChainInfoFetcher: mockChainInfoFetcher,
	}
	request := httptest.NewRequest("GET", "http://foo.com", nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientFinalityUpdate(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp *structs.LightClientUpdateResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp)
	require.NoError(t, err)
	var respHeader structs.LightClientHeader
	err = json.Unmarshal(resp.Data.AttestedHeader, &respHeader)
	require.NoError(t, err)
	require.Equal(t, "altair", resp.Version)
	require.Equal(t, hexutil.Encode(attestedHeader.BodyRoot), respHeader.Beacon.BodyRoot)
	require.NotNil(t, resp.Data)
}

func TestLightClientHandler_GetLightClientOptimisticUpdateAltair(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	config := params.BeaconConfig()
	slot := primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	attestedState, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	err = attestedState.SetSlot(slot.Sub(1))
	require.NoError(t, err)

	require.NoError(t, attestedState.SetFinalizedCheckpoint(&pb.Checkpoint{
		Epoch: config.AltairForkEpoch - 10,
		Root:  make([]byte, 32),
	}))

	parent := util.NewBeaconBlockAltair()
	parent.Block.Slot = slot.Sub(1)

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	parentHeader, err := signedParent.Header()
	require.NoError(t, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(t, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	err = st.SetSlot(slot)
	require.NoError(t, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(t, err)

	block := util.NewBeaconBlockAltair()
	block.Block.Slot = slot
	block.Block.ParentRoot = parentRoot[:]

	for i := uint64(0); i < config.SyncCommitteeSize; i++ {
		block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
	}

	signedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	h, err := signedBlock.Header()
	require.NoError(t, err)

	err = st.SetLatestBlockHeader(h.Header)
	require.NoError(t, err)
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	block.Block.StateRoot = stateRoot[:]
	signedBlock, err = blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	root, err := block.Block.HashTreeRoot()
	require.NoError(t, err)

	mockBlocker := &testutil.MockBlocker{
		RootBlockMap: map[[32]byte]interfaces.ReadOnlySignedBeaconBlock{
			parentRoot: signedParent,
			root:       signedBlock,
		},
		SlotBlockMap: map[primitives.Slot]interfaces.ReadOnlySignedBeaconBlock{
			slot.Sub(1): signedParent,
			slot:        signedBlock,
		},
	}
	mockChainService := &mock.ChainService{Optimistic: true, Slot: &slot, State: st, FinalizedRoots: map[[32]byte]bool{
		root: true,
	}}
	mockChainInfoFetcher := &mock.ChainService{Slot: &slot}
	s := &Server{
		Stater: &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{
			slot.Sub(1): attestedState,
			slot:        st,
		}},
		Blocker:          mockBlocker,
		HeadFetcher:      mockChainService,
		ChainInfoFetcher: mockChainInfoFetcher,
	}
	request := httptest.NewRequest("GET", "http://foo.com", nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientOptimisticUpdate(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp *structs.LightClientUpdateResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp)
	require.NoError(t, err)
	var respHeader structs.LightClientHeader
	err = json.Unmarshal(resp.Data.AttestedHeader, &respHeader)
	require.NoError(t, err)
	require.Equal(t, "altair", resp.Version)
	require.Equal(t, hexutil.Encode(attestedHeader.BodyRoot), respHeader.Beacon.BodyRoot)
	require.NotNil(t, resp.Data)
}

func TestLightClientHandler_GetLightClientOptimisticUpdateCapella(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	config := params.BeaconConfig()
	slot := primitives.Slot(config.CapellaForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	attestedState, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	err = attestedState.SetSlot(slot.Sub(1))
	require.NoError(t, err)

	require.NoError(t, attestedState.SetFinalizedCheckpoint(&pb.Checkpoint{
		Epoch: config.AltairForkEpoch - 10,
		Root:  make([]byte, 32),
	}))

	parent := util.NewBeaconBlockCapella()
	parent.Block.Slot = slot.Sub(1)

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	parentHeader, err := signedParent.Header()
	require.NoError(t, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(t, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	err = st.SetSlot(slot)
	require.NoError(t, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(t, err)

	block := util.NewBeaconBlockCapella()
	block.Block.Slot = slot
	block.Block.ParentRoot = parentRoot[:]

	for i := uint64(0); i < config.SyncCommitteeSize; i++ {
		block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
	}

	signedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	h, err := signedBlock.Header()
	require.NoError(t, err)

	err = st.SetLatestBlockHeader(h.Header)
	require.NoError(t, err)
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	block.Block.StateRoot = stateRoot[:]
	signedBlock, err = blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	root, err := block.Block.HashTreeRoot()
	require.NoError(t, err)

	mockBlocker := &testutil.MockBlocker{
		RootBlockMap: map[[32]byte]interfaces.ReadOnlySignedBeaconBlock{
			parentRoot: signedParent,
			root:       signedBlock,
		},
		SlotBlockMap: map[primitives.Slot]interfaces.ReadOnlySignedBeaconBlock{
			slot.Sub(1): signedParent,
			slot:        signedBlock,
		},
	}
	mockChainService := &mock.ChainService{Optimistic: true, Slot: &slot, State: st, FinalizedRoots: map[[32]byte]bool{
		root: true,
	}}
	mockChainInfoFetcher := &mock.ChainService{Slot: &slot}
	s := &Server{
		Stater: &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{
			slot.Sub(1): attestedState,
			slot:        st,
		}},
		Blocker:          mockBlocker,
		HeadFetcher:      mockChainService,
		ChainInfoFetcher: mockChainInfoFetcher,
	}
	request := httptest.NewRequest("GET", "http://foo.com", nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientOptimisticUpdate(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp *structs.LightClientUpdateResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp)
	require.NoError(t, err)
	var respHeader structs.LightClientHeaderCapella
	err = json.Unmarshal(resp.Data.AttestedHeader, &respHeader)
	require.NoError(t, err)
	require.Equal(t, "capella", resp.Version)
	require.Equal(t, hexutil.Encode(attestedHeader.BodyRoot), respHeader.Beacon.BodyRoot)
	require.NotNil(t, resp.Data)
}

func TestLightClientHandler_GetLightClientOptimisticUpdateDeneb(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableLightClient: true,
	})
	defer resetFn()

	helpers.ClearCache()
	ctx := context.Background()
	config := params.BeaconConfig()
	slot := primitives.Slot(config.DenebForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	attestedState, err := util.NewBeaconStateDeneb()
	require.NoError(t, err)
	err = attestedState.SetSlot(slot.Sub(1))
	require.NoError(t, err)

	require.NoError(t, attestedState.SetFinalizedCheckpoint(&pb.Checkpoint{
		Epoch: config.AltairForkEpoch - 10,
		Root:  make([]byte, 32),
	}))

	parent := util.NewBeaconBlockDeneb()
	parent.Block.Slot = slot.Sub(1)

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	parentHeader, err := signedParent.Header()
	require.NoError(t, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(t, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	st, err := util.NewBeaconStateDeneb()
	require.NoError(t, err)
	err = st.SetSlot(slot)
	require.NoError(t, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(t, err)

	block := util.NewBeaconBlockDeneb()
	block.Block.Slot = slot
	block.Block.ParentRoot = parentRoot[:]

	for i := uint64(0); i < config.SyncCommitteeSize; i++ {
		block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
	}

	signedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	h, err := signedBlock.Header()
	require.NoError(t, err)

	err = st.SetLatestBlockHeader(h.Header)
	require.NoError(t, err)
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	block.Block.StateRoot = stateRoot[:]
	signedBlock, err = blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	root, err := block.Block.HashTreeRoot()
	require.NoError(t, err)

	mockBlocker := &testutil.MockBlocker{
		RootBlockMap: map[[32]byte]interfaces.ReadOnlySignedBeaconBlock{
			parentRoot: signedParent,
			root:       signedBlock,
		},
		SlotBlockMap: map[primitives.Slot]interfaces.ReadOnlySignedBeaconBlock{
			slot.Sub(1): signedParent,
			slot:        signedBlock,
		},
	}
	mockChainService := &mock.ChainService{Optimistic: true, Slot: &slot, State: st, FinalizedRoots: map[[32]byte]bool{
		root: true,
	}}
	mockChainInfoFetcher := &mock.ChainService{Slot: &slot}
	s := &Server{
		Stater: &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{
			slot.Sub(1): attestedState,
			slot:        st,
		}},
		Blocker:          mockBlocker,
		HeadFetcher:      mockChainService,
		ChainInfoFetcher: mockChainInfoFetcher,
	}
	request := httptest.NewRequest("GET", "http://foo.com", nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s.GetLightClientOptimisticUpdate(writer, request)

	require.Equal(t, http.StatusOK, writer.Code)
	var resp *structs.LightClientUpdateResponse
	err = json.Unmarshal(writer.Body.Bytes(), &resp)
	require.NoError(t, err)
	var respHeader structs.LightClientHeaderDeneb
	err = json.Unmarshal(resp.Data.AttestedHeader, &respHeader)
	require.NoError(t, err)
	require.Equal(t, "deneb", resp.Version)
	require.Equal(t, hexutil.Encode(attestedHeader.BodyRoot), respHeader.Beacon.BodyRoot)
	require.NotNil(t, resp.Data)
}

func TestLightClientHandler_GetLightClientEventBlock(t *testing.T) {
	helpers.ClearCache()
	ctx := context.Background()
	config := params.BeaconConfig()
	slot := primitives.Slot(config.CapellaForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	attestedState, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	err = attestedState.SetSlot(slot.Sub(1))
	require.NoError(t, err)

	require.NoError(t, attestedState.SetFinalizedCheckpoint(&pb.Checkpoint{
		Epoch: config.AltairForkEpoch - 10,
		Root:  make([]byte, 32),
	}))

	parent := util.NewBeaconBlockCapella()
	parent.Block.Slot = slot.Sub(1)

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	parentHeader, err := signedParent.Header()
	require.NoError(t, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(t, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	err = st.SetSlot(slot)
	require.NoError(t, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(t, err)

	block := util.NewBeaconBlockCapella()
	block.Block.Slot = slot
	block.Block.ParentRoot = parentRoot[:]

	for i := uint64(0); i < config.SyncCommitteeSize; i++ {
		block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
	}

	signedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	h, err := signedBlock.Header()
	require.NoError(t, err)

	err = st.SetLatestBlockHeader(h.Header)
	require.NoError(t, err)
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	block.Block.StateRoot = stateRoot[:]
	signedBlock, err = blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	root, err := block.Block.HashTreeRoot()
	require.NoError(t, err)

	mockBlocker := &testutil.MockBlocker{
		RootBlockMap: map[[32]byte]interfaces.ReadOnlySignedBeaconBlock{
			parentRoot: signedParent,
			root:       signedBlock,
		},
		SlotBlockMap: map[primitives.Slot]interfaces.ReadOnlySignedBeaconBlock{
			slot.Sub(1): signedParent,
			slot:        signedBlock,
		},
	}
	mockChainService := &mock.ChainService{Optimistic: true, Slot: &slot, State: st, FinalizedRoots: map[[32]byte]bool{
		root: true,
	}}
	mockChainInfoFetcher := &mock.ChainService{Slot: &slot}
	s := &Server{
		Stater: &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{
			slot.Sub(1): attestedState,
			slot:        st,
		}},
		Blocker:          mockBlocker,
		HeadFetcher:      mockChainService,
		ChainInfoFetcher: mockChainInfoFetcher,
	}

	minSignaturesRequired := uint64(100)
	eventBlock, err := s.suitableBlock(ctx, minSignaturesRequired)

	require.NoError(t, err)
	require.NotNil(t, eventBlock)
	require.Equal(t, slot, eventBlock.Block().Slot())
	syncAggregate, err := eventBlock.Block().Body().SyncAggregate()
	require.NoError(t, err)
	require.Equal(t, true, syncAggregate.SyncCommitteeBits.Count() >= minSignaturesRequired)
}

func TestLightClientHandler_GetLightClientEventBlock_NeedFetchParent(t *testing.T) {
	helpers.ClearCache()
	ctx := context.Background()
	config := params.BeaconConfig()
	slot := primitives.Slot(config.CapellaForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)

	attestedState, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	err = attestedState.SetSlot(slot.Sub(1))
	require.NoError(t, err)

	require.NoError(t, attestedState.SetFinalizedCheckpoint(&pb.Checkpoint{
		Epoch: config.AltairForkEpoch - 10,
		Root:  make([]byte, 32),
	}))

	parent := util.NewBeaconBlockCapella()
	parent.Block.Slot = slot.Sub(1)
	for i := uint64(0); i < config.SyncCommitteeSize; i++ {
		parent.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
	}

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	parentHeader, err := signedParent.Header()
	require.NoError(t, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(t, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(t, err)

	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	err = st.SetSlot(slot)
	require.NoError(t, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(t, err)

	block := util.NewBeaconBlockCapella()
	block.Block.Slot = slot
	block.Block.ParentRoot = parentRoot[:]

	for i := uint64(0); i < 10; i++ {
		block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
	}

	signedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	h, err := signedBlock.Header()
	require.NoError(t, err)

	err = st.SetLatestBlockHeader(h.Header)
	require.NoError(t, err)
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)

	// get a new signed block so the root is updated with the new state root
	block.Block.StateRoot = stateRoot[:]
	signedBlock, err = blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	root, err := block.Block.HashTreeRoot()
	require.NoError(t, err)

	mockBlocker := &testutil.MockBlocker{
		RootBlockMap: map[[32]byte]interfaces.ReadOnlySignedBeaconBlock{
			parentRoot: signedParent,
			root:       signedBlock,
		},
		SlotBlockMap: map[primitives.Slot]interfaces.ReadOnlySignedBeaconBlock{
			slot.Sub(1): signedParent,
			slot:        signedBlock,
		},
	}
	mockChainService := &mock.ChainService{Optimistic: true, Slot: &slot, State: st, FinalizedRoots: map[[32]byte]bool{
		root: true,
	}}
	mockChainInfoFetcher := &mock.ChainService{Slot: &slot}
	s := &Server{
		Stater: &testutil.MockStater{StatesBySlot: map[primitives.Slot]state.BeaconState{
			slot.Sub(1): attestedState,
			slot:        st,
		}},
		Blocker:          mockBlocker,
		HeadFetcher:      mockChainService,
		ChainInfoFetcher: mockChainInfoFetcher,
	}

	minSignaturesRequired := uint64(100)
	eventBlock, err := s.suitableBlock(ctx, minSignaturesRequired)

	require.NoError(t, err)
	require.NotNil(t, eventBlock)
	syncAggregate, err := eventBlock.Block().Body().SyncAggregate()
	require.NoError(t, err)
	require.Equal(t, true, syncAggregate.SyncCommitteeBits.Count() >= minSignaturesRequired)
	require.Equal(t, slot-1, eventBlock.Block().Slot())
}

func createUpdate(t *testing.T, v int) (interfaces.LightClientUpdate, error) {
	config := params.BeaconConfig()
	var slot primitives.Slot
	var header interfaces.LightClientHeader
	var st state.BeaconState
	var err error

	sampleRoot := make([]byte, 32)
	for i := 0; i < 32; i++ {
		sampleRoot[i] = byte(i)
	}

	sampleExecutionBranch := make([][]byte, fieldparams.ExecutionBranchDepth)
	for i := 0; i < 4; i++ {
		sampleExecutionBranch[i] = make([]byte, 32)
		for j := 0; j < 32; j++ {
			sampleExecutionBranch[i][j] = byte(i + j)
		}
	}

	switch v {
	case version.Altair:
		slot = primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
		})
		require.NoError(t, err)
		st, err = util.NewBeaconStateAltair()
		require.NoError(t, err)
	case version.Capella:
		slot = primitives.Slot(config.CapellaForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderCapella{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
			Execution: &enginev1.ExecutionPayloadHeaderCapella{
				ParentHash:       make([]byte, fieldparams.RootLength),
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: make([]byte, fieldparams.RootLength),
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
			},
			ExecutionBranch: sampleExecutionBranch,
		})
		require.NoError(t, err)
		st, err = util.NewBeaconStateCapella()
		require.NoError(t, err)
	case version.Deneb:
		slot = primitives.Slot(config.DenebForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderDeneb{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
			Execution: &enginev1.ExecutionPayloadHeaderDeneb{
				ParentHash:       make([]byte, fieldparams.RootLength),
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: make([]byte, fieldparams.RootLength),
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
			},
			ExecutionBranch: sampleExecutionBranch,
		})
		require.NoError(t, err)
		st, err = util.NewBeaconStateDeneb()
		require.NoError(t, err)
	case version.Electra:
		slot = primitives.Slot(config.ElectraForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderDeneb{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
			Execution: &enginev1.ExecutionPayloadHeaderElectra{
				ParentHash:       make([]byte, fieldparams.RootLength),
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: make([]byte, fieldparams.RootLength),
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
			},
			ExecutionBranch: sampleExecutionBranch,
		})
		require.NoError(t, err)
		st, err = util.NewBeaconStateElectra()
		require.NoError(t, err)
	default:
		return nil, fmt.Errorf("unsupported version %s", version.String(v))
	}

	update, err := lightclient.CreateDefaultLightClientUpdate(slot, st)
	require.NoError(t, err)
	update.SetSignatureSlot(slot - 1)
	syncCommitteeBits := make([]byte, 64)
	syncCommitteeSignature := make([]byte, 96)
	update.SetSyncAggregate(&pb.SyncAggregate{
		SyncCommitteeBits:      syncCommitteeBits,
		SyncCommitteeSignature: syncCommitteeSignature,
	})

	require.NoError(t, update.SetAttestedHeader(header))
	require.NoError(t, update.SetFinalizedHeader(header))

	return update, nil
}
