package node

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"testing/synctest"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// blockingCustodyManager mimics a node whose custody info is never initialized:
// like p2p.Service, it blocks until the context is done.
type blockingCustodyManager struct {
	p2p.CustodyManager
}

func (blockingCustodyManager) CustodyGroupCount(ctx context.Context) (uint64, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

func (blockingCustodyManager) EarliestAvailableSlot(ctx context.Context) (primitives.Slot, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

func getCustody(t *testing.T, s *Server) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "http://example.com/prysm/v1/node/custody", nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}
	s.GetCustody(writer, request)
	return writer
}

func uint64SliceToStringsForTest(values []uint64) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strconv.FormatUint(value, 10))
	}
	return result
}

func TestGetCustody_FuluNotScheduled(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = cfg.FarFutureEpoch
	params.OverrideBeaconConfig(cfg)

	writer := getCustody(t, &Server{})
	assert.Equal(t, http.StatusBadRequest, writer.Code)
}

func TestGetCustody_CustodyInfoUnavailable(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	// synctest fakes time within the bubble: the custodyInfoTimeout deadline
	// fires as soon as the handler blocks on the custody manager, without the
	// test waiting for it in real time.
	synctest.Test(t, func(t *testing.T) {
		writer := getCustody(t, &Server{CustodyManager: blockingCustodyManager{}})
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.StringContains(t, context.DeadlineExceeded.Error(), writer.Body.String())
	})
}

func TestGetCustody(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	t.Run("fullnode", func(t *testing.T) {
		custodyRequirement := params.BeaconConfig().CustodyRequirement
		p2pService := mockp2p.NewTestP2P(t)
		_, _, err := p2pService.UpdateCustodyInfo(123, custodyRequirement)
		require.NoError(t, err)
		s := &Server{
			BeaconDB:       dbtest.SetupDB(t),
			PeerManager:    p2pService,
			CustodyManager: p2pService,
		}

		writer := getCustody(t, s)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetCustodyResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))

		assert.Equal(t, strconv.FormatUint(custodyRequirement, 10), resp.Data.CustodyGroupCount)
		assert.Equal(t, "123", resp.Data.EarliestAvailableSlot)
		assert.Equal(t, false, resp.Data.IsSupernode)
		assert.Equal(t, false, resp.Data.IsSemiSupernode)
		// The node is synced from genesis, so no backfill status is expected.
		assert.Equal(t, true, resp.Data.Backfill == nil)

		expectedGroups, err := peerdas.CustodyGroups(p2pService.NodeID(), custodyRequirement)
		require.NoError(t, err)
		assert.DeepEqual(t, uint64SliceToStringsForTest(expectedGroups), resp.Data.CustodyGroups)

		expectedColumnsMap, err := peerdas.CustodyColumns(expectedGroups)
		require.NoError(t, err)
		require.Equal(t, len(expectedColumnsMap), len(resp.Data.CustodyColumns))
		previous := int64(-1)
		for _, column := range resp.Data.CustodyColumns {
			c, err := strconv.ParseUint(column, 10, 64)
			require.NoError(t, err)
			assert.Equal(t, true, expectedColumnsMap[c])
			assert.Equal(t, true, int64(c) > previous, "custody columns are not sorted")
			previous = int64(c)
		}
	})

	t.Run("supernode with backfill status", func(t *testing.T) {
		numberOfGroups := params.BeaconConfig().NumberOfCustodyGroups
		p2pService := mockp2p.NewTestP2P(t)
		_, _, err := p2pService.UpdateCustodyInfo(2000, numberOfGroups)
		require.NoError(t, err)
		beaconDB := dbtest.SetupDB(t)
		require.NoError(t, beaconDB.SaveBackfillStatus(context.Background(), &dbval.BackfillStatus{LowSlot: 1000, OriginSlot: 5000}))
		s := &Server{
			BeaconDB:       beaconDB,
			PeerManager:    p2pService,
			CustodyManager: p2pService,
		}

		writer := getCustody(t, s)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetCustodyResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))

		assert.Equal(t, strconv.FormatUint(numberOfGroups, 10), resp.Data.CustodyGroupCount)
		assert.Equal(t, true, resp.Data.IsSupernode)
		assert.Equal(t, false, resp.Data.IsSemiSupernode)
		require.Equal(t, int(numberOfGroups), len(resp.Data.CustodyGroups))
		assert.Equal(t, "0", resp.Data.CustodyGroups[0])
		assert.Equal(t, strconv.FormatUint(numberOfGroups-1, 10), resp.Data.CustodyGroups[numberOfGroups-1])

		require.Equal(t, true, resp.Data.Backfill != nil)
		assert.Equal(t, "5000", resp.Data.Backfill.OriginSlot)
		assert.Equal(t, "1000", resp.Data.Backfill.LowSlot)
	})

	t.Run("semi-supernode", func(t *testing.T) {
		semiSupernodeTarget, err := peerdas.MinimumCustodyGroupCountToReconstruct()
		require.NoError(t, err)
		p2pService := mockp2p.NewTestP2P(t)
		_, _, err = p2pService.UpdateCustodyInfo(456, semiSupernodeTarget)
		require.NoError(t, err)
		s := &Server{
			BeaconDB:       dbtest.SetupDB(t),
			PeerManager:    p2pService,
			CustodyManager: p2pService,
		}

		writer := getCustody(t, s)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.GetCustodyResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))

		assert.Equal(t, strconv.FormatUint(semiSupernodeTarget, 10), resp.Data.CustodyGroupCount)
		assert.Equal(t, "456", resp.Data.EarliestAvailableSlot)
		assert.Equal(t, false, resp.Data.IsSupernode)
		assert.Equal(t, true, resp.Data.IsSemiSupernode)
	})
}
