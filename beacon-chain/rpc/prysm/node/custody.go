package node

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/pkg/errors"
)

// custodyInfoTimeout bounds how long GetCustody waits for the custody info to be
// initialized at startup before failing the request.
const custodyInfoTimeout = 3 * time.Second

// GetCustody returns the current data column custody state of the node: the custody
// group count, the custody groups and columns, the earliest available slot, the
// supernode/semi-supernode status, and the backfill progress.
func (s *Server) GetCustody(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "node.GetCustody")
	defer span.End()

	if !params.FuluEnabled() {
		httputil.HandleError(w, "Fulu is not scheduled", http.StatusBadRequest)
		return
	}

	// Custody info is initialized asynchronously at node startup. Bound the wait so
	// the request does not hang if it is not available yet.
	ctx, cancel := context.WithTimeout(ctx, custodyInfoTimeout)
	defer cancel()

	custodyGroupCount, err := s.CustodyManager.CustodyGroupCount(ctx)
	if err != nil {
		httputil.HandleError(w, "Custody info is not available yet: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	earliestAvailableSlot, err := s.CustodyManager.EarliestAvailableSlot(ctx)
	if err != nil {
		httputil.HandleError(w, "Custody info is not available yet: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	custodyGroups, err := peerdas.CustodyGroups(s.PeerManager.NodeID(), custodyGroupCount)
	if err != nil {
		httputil.HandleError(w, "Could not compute custody groups: "+err.Error(), http.StatusInternalServerError)
		return
	}

	custodyColumnsMap, err := peerdas.CustodyColumns(custodyGroups)
	if err != nil {
		httputil.HandleError(w, "Could not compute custody columns: "+err.Error(), http.StatusInternalServerError)
		return
	}

	custodyColumns := make([]uint64, 0, len(custodyColumnsMap))
	for column := range custodyColumnsMap {
		custodyColumns = append(custodyColumns, column)
	}
	slices.Sort(custodyColumns)

	minGroupCountToReconstruct, err := peerdas.MinimumCustodyGroupCountToReconstruct()
	if err != nil {
		httputil.HandleError(w, "Could not compute minimum custody group count to reconstruct: "+err.Error(), http.StatusInternalServerError)
		return
	}

	isSupernode := custodyGroupCount == params.BeaconConfig().NumberOfCustodyGroups

	data := &structs.CustodyData{
		CustodyGroupCount:     strconv.FormatUint(custodyGroupCount, 10),
		CustodyGroups:         uint64SliceToStrings(custodyGroups),
		CustodyColumns:        uint64SliceToStrings(custodyColumns),
		EarliestAvailableSlot: strconv.FormatUint(uint64(earliestAvailableSlot), 10),
		IsSupernode:           isSupernode,
		IsSemiSupernode:       !isSupernode && custodyGroupCount >= minGroupCountToReconstruct,
	}

	backfillStatus, err := s.BeaconDB.BackfillStatus(ctx)
	switch {
	case err == nil:
		data.Backfill = &structs.BackfillStatus{
			OriginSlot: strconv.FormatUint(backfillStatus.OriginSlot, 10),
			LowSlot:    strconv.FormatUint(backfillStatus.LowSlot, 10),
		}
	case !errors.Is(err, db.ErrNotFound):
		httputil.HandleError(w, "Could not get backfill status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// db.ErrNotFound means the node was synced from genesis and never needed backfill.

	httputil.WriteJson(w, &structs.GetCustodyResponse{Data: data})
}

func uint64SliceToStrings(values []uint64) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strconv.FormatUint(value, 10))
	}
	return result
}
