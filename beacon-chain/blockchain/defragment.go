package blockchain

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/time"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var promoteToHeadTime = promauto.NewSummary(prometheus.SummaryOpts{
	Name: "head_state_promote_to_head_milliseconds",
	Help: "Milliseconds it takes to promote the head state's overrides to the shared base",
})

// promoteHeadState promotes the head state's MVSlice overrides into the shared
// base so that subsequent reads avoid override lookups.
func (s *Service) promoteHeadState(st state.BeaconState) {
	startTime := time.Now()
	st.PromoteToHead()
	elapsedTime := time.Since(startTime)
	promoteToHeadTime.Observe(float64(elapsedTime.Milliseconds()))
}
