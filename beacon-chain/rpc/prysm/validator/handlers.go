package validator

import (
	"fmt"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/core"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/shared"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// GetParticipation retrieves the validator participation information for a given epoch,
// it returns the information about validator's participation rate in voting on the proof of stake
// rules based on their balance compared to the total active validator balance.
func (s *Server) GetParticipation(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetParticipation")
	defer span.End()

	stateId := r.PathValue("state_id")
	if stateId == "" {
		httputil.HandleError(w, "state_id is required in URL params", http.StatusBadRequest)
		return
	}

	st, err := s.Stater.State(ctx, []byte(stateId))
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}
	stEpoch := slots.ToEpoch(st.Slot())
	vp, rpcError := s.CoreService.ValidatorParticipation(ctx, stEpoch)
	if rpcError != nil {
		httputil.HandleError(w, rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
		return
	}

	response := &structs.GetValidatorParticipationResponse{
		Epoch:     fmt.Sprintf("%d", vp.Epoch),
		Finalized: vp.Finalized,
		Participation: &structs.ValidatorParticipation{
			GlobalParticipationRate:          fmt.Sprintf("%f", vp.Participation.GlobalParticipationRate),
			VotedEther:                       fmt.Sprintf("%d", vp.Participation.VotedEther),
			EligibleEther:                    fmt.Sprintf("%d", vp.Participation.EligibleEther),
			CurrentEpochActiveGwei:           fmt.Sprintf("%d", vp.Participation.CurrentEpochActiveGwei),
			CurrentEpochAttestingGwei:        fmt.Sprintf("%d", vp.Participation.CurrentEpochAttestingGwei),
			CurrentEpochTargetAttestingGwei:  fmt.Sprintf("%d", vp.Participation.CurrentEpochTargetAttestingGwei),
			PreviousEpochActiveGwei:          fmt.Sprintf("%d", vp.Participation.PreviousEpochActiveGwei),
			PreviousEpochAttestingGwei:       fmt.Sprintf("%d", vp.Participation.PreviousEpochAttestingGwei),
			PreviousEpochTargetAttestingGwei: fmt.Sprintf("%d", vp.Participation.PreviousEpochTargetAttestingGwei),
			PreviousEpochHeadAttestingGwei:   fmt.Sprintf("%d", vp.Participation.PreviousEpochHeadAttestingGwei),
		},
	}
	httputil.WriteJson(w, response)
}

// GetActiveSetChanges retrieves the active set changes for a given epoch.
//
// This data includes any activations, voluntary exits, and involuntary
// ejections.
func (s *Server) GetActiveSetChanges(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetActiveSetChanges")
	defer span.End()

	stateId := r.PathValue("state_id")
	if stateId == "" {
		httputil.HandleError(w, "state_id is required in URL params", http.StatusBadRequest)
		return
	}

	st, err := s.Stater.State(ctx, []byte(stateId))
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}
	stEpoch := slots.ToEpoch(st.Slot())

	as, rpcError := s.CoreService.ValidatorActiveSetChanges(ctx, stEpoch)
	if rpcError != nil {
		httputil.HandleError(w, rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
		return
	}

	response := &structs.ActiveSetChanges{
		Epoch:               fmt.Sprintf("%d", as.Epoch),
		ActivatedPublicKeys: byteSlice2dToStringSlice(as.ActivatedPublicKeys),
		ActivatedIndices:    uint64SliceToStringSlice(as.ActivatedIndices),
		ExitedPublicKeys:    byteSlice2dToStringSlice(as.ExitedPublicKeys),
		ExitedIndices:       uint64SliceToStringSlice(as.ExitedIndices),
		SlashedPublicKeys:   byteSlice2dToStringSlice(as.SlashedPublicKeys),
		SlashedIndices:      uint64SliceToStringSlice(as.SlashedIndices),
		EjectedPublicKeys:   byteSlice2dToStringSlice(as.EjectedPublicKeys),
		EjectedIndices:      uint64SliceToStringSlice(as.EjectedIndices),
	}
	httputil.WriteJson(w, response)
}

func byteSlice2dToStringSlice(byteArrays [][]byte) []string {
	s := make([]string, len(byteArrays))
	for i, b := range byteArrays {
		s[i] = hexutil.Encode(b)
	}
	return s
}

func uint64SliceToStringSlice(indices []primitives.ValidatorIndex) []string {
	s := make([]string, len(indices))
	for i, u := range indices {
		s[i] = fmt.Sprintf("%d", u)
	}
	return s
}
