package lightclient

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/shared"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	types "github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/wealdtech/go-bytesutil"
)

// GetLightClientBootstrap - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/bootstrap.yaml
func (s *Server) GetLightClientBootstrap(w http.ResponseWriter, req *http.Request) {
	// Prepare
	ctx, span := trace.StartSpan(req.Context(), "beacon.GetLightClientBootstrap")
	defer span.End()

	// Get the block
	blockRootParam, err := hexutil.Decode(req.PathValue("block_root"))
	if err != nil {
		httputil.HandleError(w, "invalid block root: "+err.Error(), http.StatusBadRequest)
		return
	}

	blockRoot := bytesutil.ToBytes32(blockRootParam)
	blk, err := s.Blocker.Block(ctx, blockRoot[:])
	if !shared.WriteBlockFetchError(w, blk, err) {
		return
	}

	// Get the state
	state, err := s.Stater.StateBySlot(ctx, blk.Block().Slot())
	if err != nil {
		httputil.HandleError(w, "could not get state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	bootstrap, err := createLightClientBootstrap(ctx, s.ChainInfoFetcher.CurrentSlot(), state, blk)
	if err != nil {
		httputil.HandleError(w, "could not get light client bootstrap: "+err.Error(), http.StatusInternalServerError)
		return
	}
	response := &structs.LightClientBootstrapResponse{
		Version: version.String(blk.Version()),
		Data:    bootstrap,
	}
	w.Header().Set(api.VersionHeader, version.String(version.Deneb))

	httputil.WriteJson(w, response)
}

// GetLightClientUpdatesByRange - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/updates.yaml
func (s *Server) GetLightClientUpdatesByRange(w http.ResponseWriter, req *http.Request) {
	// Prepare
	ctx, span := trace.StartSpan(req.Context(), "beacon.GetLightClientUpdatesByRange")
	defer span.End()

	// Determine slots per period
	config := params.BeaconConfig()
	slotsPerPeriod := uint64(config.EpochsPerSyncCommitteePeriod) * uint64(config.SlotsPerEpoch)

	// Adjust count based on configuration
	_, count, gotCount := shared.UintFromQuery(w, req, "count", true)
	if !gotCount {
		return
	} else if count == 0 {
		httputil.HandleError(w, fmt.Sprintf("got invalid 'count' query variable '%d': count must be greater than 0", count), http.StatusInternalServerError)
		return
	}

	// Determine the start and end periods
	_, startPeriod, gotStartPeriod := shared.UintFromQuery(w, req, "start_period", true)
	if !gotStartPeriod {
		return
	}

	if count > config.MaxRequestLightClientUpdates {
		count = config.MaxRequestLightClientUpdates
	}

	// max possible slot is current head
	headState, err := s.HeadFetcher.HeadState(ctx)
	if err != nil {
		httputil.HandleError(w, "could not get head state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	maxSlot := uint64(headState.Slot())

	// min possible slot is Altair fork period
	minSlot := uint64(config.AltairForkEpoch) * uint64(config.SlotsPerEpoch)

	// Adjust startPeriod, the end of start period must be later than Altair fork epoch, otherwise, can not get the sync committee votes
	startPeriodEndSlot := (startPeriod+1)*slotsPerPeriod - 1
	if startPeriodEndSlot < minSlot {
		startPeriod = minSlot / slotsPerPeriod
	}

	// Get the initial endPeriod, then we will adjust
	endPeriod := startPeriod + count - 1

	// Adjust endPeriod, the end of end period must be earlier than current head slot
	endPeriodEndSlot := (endPeriod+1)*slotsPerPeriod - 1
	if endPeriodEndSlot > maxSlot {
		endPeriod = maxSlot / slotsPerPeriod
	}

	// Populate updates
	var updates []*structs.LightClientUpdateResponse
	for period := startPeriod; period <= endPeriod; period++ {
		// Get the last known state of the period,
		//    1. We wish the block has a parent in the same period if possible
		//	  2. We wish the block has a state in the same period
		lastSlotInPeriod := period*slotsPerPeriod + slotsPerPeriod - 1
		if lastSlotInPeriod > maxSlot {
			lastSlotInPeriod = maxSlot
		}
		firstSlotInPeriod := period * slotsPerPeriod

		// Let's not use the first slot in the period, otherwise the attested header will be in previous period
		firstSlotInPeriod++

		var state state.BeaconState
		var block interfaces.ReadOnlySignedBeaconBlock
		for slot := lastSlotInPeriod; slot >= firstSlotInPeriod; slot-- {
			state, err = s.Stater.StateBySlot(ctx, types.Slot(slot))
			if err != nil {
				continue
			}

			// Get the block
			latestBlockHeader := state.LatestBlockHeader()
			latestStateRoot, err := state.HashTreeRoot(ctx)
			if err != nil {
				continue
			}
			latestBlockHeader.StateRoot = latestStateRoot[:]
			blockRoot, err := latestBlockHeader.HashTreeRoot()
			if err != nil {
				continue
			}

			block, err = s.Blocker.Block(ctx, blockRoot[:])
			if err != nil || block == nil {
				continue
			}

			syncAggregate, err := block.Block().Body().SyncAggregate()
			if err != nil || syncAggregate == nil {
				continue
			}

			if syncAggregate.SyncCommitteeBits.Count()*3 < config.SyncCommitteeSize*2 {
				// Not enough votes
				continue
			}

			break
		}

		if block == nil {
			// No valid block found for the period
			continue
		}

		// Get attested state
		attestedRoot := block.Block().ParentRoot()
		attestedBlock, err := s.Blocker.Block(ctx, attestedRoot[:])
		if err != nil || attestedBlock == nil {
			continue
		}

		attestedSlot := attestedBlock.Block().Slot()
		attestedState, err := s.Stater.StateBySlot(ctx, attestedSlot)
		if err != nil {
			continue
		}

		// Get finalized block
		var finalizedBlock interfaces.ReadOnlySignedBeaconBlock
		finalizedCheckPoint := attestedState.FinalizedCheckpoint()
		if finalizedCheckPoint != nil {
			finalizedRoot := bytesutil.ToBytes32(finalizedCheckPoint.Root)
			finalizedBlock, err = s.Blocker.Block(ctx, finalizedRoot[:])
			if err != nil {
				finalizedBlock = nil
			}
		}

		update, err := newLightClientUpdateFromBeaconState(
			ctx,
			s.ChainInfoFetcher.CurrentSlot(),
			state,
			block,
			attestedState,
			attestedBlock,
			finalizedBlock,
		)

		if err == nil {
			updates = append(updates, &structs.LightClientUpdateResponse{
				Version: version.String(attestedState.Version()),
				Data:    update,
			})
		}
	}

	if len(updates) == 0 {
		httputil.HandleError(w, "no updates found", http.StatusNotFound)
		return
	}

	httputil.WriteJson(w, updates)
}

// GetLightClientFinalityUpdate - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/finality_update.yaml
func (s *Server) GetLightClientFinalityUpdate(w http.ResponseWriter, req *http.Request) {
	ctx, span := trace.StartSpan(req.Context(), "beacon.GetLightClientFinalityUpdate")
	defer span.End()

	// Finality update needs super majority of sync committee signatures
	minSyncCommitteeParticipants := float64(params.BeaconConfig().MinSyncCommitteeParticipants)
	minSignatures := uint64(math.Ceil(minSyncCommitteeParticipants * 2 / 3))

	block, err := s.suitableBlock(ctx, minSignatures)
	if !shared.WriteBlockFetchError(w, block, err) {
		return
	}

	st, err := s.Stater.StateBySlot(ctx, block.Block().Slot())
	if err != nil {
		httputil.HandleError(w, "Could not get state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	attestedRoot := block.Block().ParentRoot()
	attestedBlock, err := s.Blocker.Block(ctx, attestedRoot[:])
	if !shared.WriteBlockFetchError(w, block, errors.Wrap(err, "could not get attested block")) {
		return
	}
	attestedSlot := attestedBlock.Block().Slot()
	attestedState, err := s.Stater.StateBySlot(ctx, attestedSlot)
	if err != nil {
		httputil.HandleError(w, "Could not get attested state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var finalizedBlock interfaces.ReadOnlySignedBeaconBlock
	finalizedCheckpoint := attestedState.FinalizedCheckpoint()
	if finalizedCheckpoint == nil {
		httputil.HandleError(w, "Attested state does not have a finalized checkpoint", http.StatusInternalServerError)
		return
	}
	finalizedRoot := bytesutil.ToBytes32(finalizedCheckpoint.Root)
	finalizedBlock, err = s.Blocker.Block(ctx, finalizedRoot[:])
	if !shared.WriteBlockFetchError(w, block, errors.Wrap(err, "could not get finalized block")) {
		return
	}

	update, err := newLightClientFinalityUpdateFromBeaconState(ctx, s.ChainInfoFetcher.CurrentSlot(), st, block, attestedState, attestedBlock, finalizedBlock)
	if err != nil {
		httputil.HandleError(w, "Could not get light client finality update: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := &structs.LightClientFinalityUpdateResponse{
		Version: version.String(attestedState.Version()),
		Data:    update,
	}

	httputil.WriteJson(w, response)
}

// GetLightClientOptimisticUpdate - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/optimistic_update.yaml
func (s *Server) GetLightClientOptimisticUpdate(w http.ResponseWriter, req *http.Request) {
	ctx, span := trace.StartSpan(req.Context(), "beacon.GetLightClientOptimisticUpdate")
	defer span.End()

	block, err := s.suitableBlock(ctx, params.BeaconConfig().MinSyncCommitteeParticipants)
	if !shared.WriteBlockFetchError(w, block, err) {
		return
	}
	st, err := s.Stater.StateBySlot(ctx, block.Block().Slot())
	if err != nil {
		httputil.HandleError(w, "could not get state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	attestedRoot := block.Block().ParentRoot()
	attestedBlock, err := s.Blocker.Block(ctx, attestedRoot[:])
	if err != nil {
		httputil.HandleError(w, "Could not get attested block: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if attestedBlock == nil {
		httputil.HandleError(w, "Attested block is nil", http.StatusInternalServerError)
		return
	}
	attestedSlot := attestedBlock.Block().Slot()
	attestedState, err := s.Stater.StateBySlot(ctx, attestedSlot)
	if err != nil {
		httputil.HandleError(w, "Could not get attested state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	update, err := newLightClientOptimisticUpdateFromBeaconState(ctx, s.ChainInfoFetcher.CurrentSlot(), st, block, attestedState, attestedBlock)
	if err != nil {
		httputil.HandleError(w, "Could not get light client optimistic update: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := &structs.LightClientOptimisticUpdateResponse{
		Version: version.String(attestedState.Version()),
		Data:    update,
	}

	httputil.WriteJson(w, response)
}

// suitableBlock returns the latest block that satisfies all criteria required for creating a new update
func (s *Server) suitableBlock(ctx context.Context, minSignaturesRequired uint64) (interfaces.ReadOnlySignedBeaconBlock, error) {
	st, err := s.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get head state")
	}

	latestBlockHeader := st.LatestBlockHeader()
	stateRoot, err := st.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get state root")
	}
	latestBlockHeader.StateRoot = stateRoot[:]
	latestBlockHeaderRoot, err := latestBlockHeader.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get latest block header root")
	}

	block, err := s.Blocker.Block(ctx, latestBlockHeaderRoot[:])
	if err != nil {
		return nil, errors.Wrap(err, "could not get latest block")
	}
	if block == nil {
		return nil, errors.New("latest block is nil")
	}

	// Loop through the blocks until we find a block that satisfies minSignaturesRequired requirement
	var numOfSyncCommitteeSignatures uint64
	if syncAggregate, err := block.Block().Body().SyncAggregate(); err == nil {
		numOfSyncCommitteeSignatures = syncAggregate.SyncCommitteeBits.Count()
	}

	for numOfSyncCommitteeSignatures < minSignaturesRequired {
		// Get the parent block
		parentRoot := block.Block().ParentRoot()
		block, err = s.Blocker.Block(ctx, parentRoot[:])
		if err != nil {
			return nil, errors.Wrap(err, "could not get parent block")
		}
		if block == nil {
			return nil, errors.New("parent block is nil")
		}

		// Get the number of sync committee signatures
		numOfSyncCommitteeSignatures = 0
		if syncAggregate, err := block.Block().Body().SyncAggregate(); err == nil {
			numOfSyncCommitteeSignatures = syncAggregate.SyncCommitteeBits.Count()
		}
	}

	return block, nil
}
