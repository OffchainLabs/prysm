package beacon

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/core"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/shared"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// GetWeakSubjectivity computes the starting epoch of the current weak subjectivity period, and then also
// determines the best block root and state root to use for a Checkpoint Sync starting from that point.
func (s *Server) GetWeakSubjectivity(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetWeakSubjectivity")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	hs, err := s.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not get head state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	wsEpoch, err := helpers.LatestWeakSubjectivityEpoch(ctx, hs, params.BeaconConfig())
	if err != nil {
		httputil.HandleError(w, "Could not get weak subjectivity epoch: "+err.Error(), http.StatusInternalServerError)
		return
	}
	wsSlot, err := slots.EpochStart(wsEpoch)
	if err != nil {
		httputil.HandleError(w, "Could not get weak subjectivity slot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	cbr, err := s.CanonicalHistory.BlockRootForSlot(ctx, wsSlot)
	if err != nil {
		httputil.HandleError(w, fmt.Sprintf("Could not find highest block below slot %d: %s", wsSlot, err.Error()), http.StatusInternalServerError)
		return
	}
	cb, err := s.BeaconDB.Block(ctx, cbr)
	if err != nil {
		httputil.HandleError(
			w,
			fmt.Sprintf("Block with root %#x from slot index %d not found in db: %s", cbr, wsSlot, err.Error()),
			http.StatusInternalServerError,
		)
		return
	}
	stateRoot := cb.Block().StateRoot()
	log.Printf("Weak subjectivity checkpoint reported as epoch=%d, block root=%#x, state root=%#x", wsEpoch, cbr, stateRoot)

	resp := &structs.GetWeakSubjectivityResponse{
		Data: &structs.WeakSubjectivityData{
			WsCheckpoint: &structs.Checkpoint{
				Epoch: strconv.FormatUint(uint64(wsEpoch), 10),
				Root:  hexutil.Encode(cbr[:]),
			},
			StateRoot: hexutil.Encode(stateRoot[:]),
		},
	}
	httputil.WriteJson(w, resp)
}

// GetIndividualVotes returns a list of validators individual vote status of a given epoch.
func (s *Server) GetIndividualVotes(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetIndividualVotes")
	defer span.End()

	var req structs.GetIndividualVotesRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	publicKeyBytes := make([][]byte, len(req.PublicKeys))
	for i, s := range req.PublicKeys {
		bs, err := hexutil.Decode(s)
		if err != nil {
			httputil.HandleError(w, "could not decode public keys: "+err.Error(), http.StatusBadRequest)
			return
		}
		publicKeyBytes[i] = bs
	}
	epoch, err := strconv.ParseUint(req.Epoch, 10, 64)
	if err != nil {
		httputil.HandleError(w, "invalid epoch: "+err.Error(), http.StatusBadRequest)
		return
	}
	var indices []primitives.ValidatorIndex
	for _, i := range req.Indices {
		u, err := strconv.ParseUint(i, 10, 64)
		if err != nil {
			httputil.HandleError(w, "invalid indices: "+err.Error(), http.StatusBadRequest)
			return
		}
		indices = append(indices, primitives.ValidatorIndex(u))
	}
	votes, rpcError := s.CoreService.IndividualVotes(
		ctx,
		&ethpb.IndividualVotesRequest{
			Epoch:      primitives.Epoch(epoch),
			PublicKeys: publicKeyBytes,
			Indices:    indices,
		},
	)

	if rpcError != nil {
		httputil.HandleError(w, rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
		return
	}
	v := make([]*structs.IndividualVote, 0, len(votes.IndividualVotes))
	for _, vote := range votes.IndividualVotes {
		v = append(v, &structs.IndividualVote{
			Epoch:                            fmt.Sprintf("%d", vote.Epoch),
			PublicKey:                        hexutil.Encode(vote.PublicKey),
			ValidatorIndex:                   fmt.Sprintf("%d", vote.ValidatorIndex),
			IsSlashed:                        vote.IsSlashed,
			IsWithdrawableInCurrentEpoch:     vote.IsWithdrawableInCurrentEpoch,
			IsActiveInCurrentEpoch:           vote.IsActiveInCurrentEpoch,
			IsActiveInPreviousEpoch:          vote.IsActiveInPreviousEpoch,
			IsCurrentEpochAttester:           vote.IsCurrentEpochAttester,
			IsCurrentEpochTargetAttester:     vote.IsCurrentEpochTargetAttester,
			IsPreviousEpochAttester:          vote.IsPreviousEpochAttester,
			IsPreviousEpochTargetAttester:    vote.IsPreviousEpochTargetAttester,
			IsPreviousEpochHeadAttester:      vote.IsPreviousEpochHeadAttester,
			CurrentEpochEffectiveBalanceGwei: fmt.Sprintf("%d", vote.CurrentEpochEffectiveBalanceGwei),
			InclusionSlot:                    fmt.Sprintf("%d", vote.InclusionSlot),
			InclusionDistance:                fmt.Sprintf("%d", vote.InclusionDistance),
			InactivityScore:                  fmt.Sprintf("%d", vote.InactivityScore),
		})
	}
	response := &structs.GetIndividualVotesResponse{
		IndividualVotes: v,
	}
	httputil.WriteJson(w, response)
}

// GetChainHead retrieves information about the head of the beacon chain from
// the view of the beacon chain node.
func (s *Server) GetChainHead(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetChainHead")
	defer span.End()

	ch, rpcError := s.CoreService.ChainHead(ctx)
	if rpcError != nil {
		httputil.HandleError(w, rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
		return
	}
	response := &structs.ChainHead{
		HeadSlot:                   fmt.Sprintf("%d", ch.HeadSlot),
		HeadEpoch:                  fmt.Sprintf("%d", ch.HeadEpoch),
		HeadBlockRoot:              hexutil.Encode(ch.HeadBlockRoot),
		FinalizedSlot:              fmt.Sprintf("%d", ch.FinalizedSlot),
		FinalizedEpoch:             fmt.Sprintf("%d", ch.FinalizedEpoch),
		FinalizedBlockRoot:         hexutil.Encode(ch.FinalizedBlockRoot),
		JustifiedSlot:              fmt.Sprintf("%d", ch.JustifiedSlot),
		JustifiedEpoch:             fmt.Sprintf("%d", ch.JustifiedEpoch),
		JustifiedBlockRoot:         hexutil.Encode(ch.JustifiedBlockRoot),
		PreviousJustifiedSlot:      fmt.Sprintf("%d", ch.PreviousJustifiedSlot),
		PreviousJustifiedEpoch:     fmt.Sprintf("%d", ch.PreviousJustifiedEpoch),
		PreviousJustifiedBlockRoot: hexutil.Encode(ch.PreviousJustifiedBlockRoot),
		OptimisticStatus:           ch.OptimisticStatus,
	}
	httputil.WriteJson(w, response)
}

func (s *Server) PublishBlobs(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.PublishBlobs")
	defer span.End()
	if shared.IsSyncing(r.Context(), w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	var req structs.PublishBlobsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.HandleError(w, "Could not decode JSON request body", http.StatusBadRequest)
		return
	}
	if req.BlobSidecars == nil {
		httputil.HandleError(w, "Missing blob sidecars", http.StatusBadRequest)
		return
	}

	root, err := bytesutil.DecodeHexWithLength(req.BlockRoot, 32)
	if err != nil {
		httputil.HandleError(w, "Could not decode block root: "+err.Error(), http.StatusBadRequest)
		return
	}

	for _, blobSidecar := range req.BlobSidecars.Sidecars {
		sc, err := blobSidecar.ToConsensus()
		if err != nil {
			httputil.HandleError(w, "Could not decode blob sidecar: "+err.Error(), http.StatusBadRequest)
			return
		}

		readOnlySc, err := blocks.NewROBlobWithRoot(sc, bytesutil.ToBytes32(root))
		if err != nil {
			httputil.HandleError(w, "Could not create read-only blob: "+err.Error(), http.StatusInternalServerError)
			return
		}

		verifiedBlob := blocks.NewVerifiedROBlob(readOnlySc)
		if err := s.BlobReceiver.ReceiveBlob(ctx, verifiedBlob); err != nil {
			httputil.HandleError(w, "Could not receive blob: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := s.Broadcaster.BroadcastBlob(ctx, sc.Index, sc); err != nil {
			httputil.HandleError(w, "Failed to broadcast blob: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
