package blockchain

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/time"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var stateDefragmentationTime = promauto.NewSummary(prometheus.SummaryOpts{
	Name: "head_state_defragmentation_milliseconds",
	Help: "Milliseconds it takes to defragment the head state",
})

// This method defragments our state, so that any specific fields which have
// a higher number of fragmented indexes are reallocated to a new separate slice for
// that field.
func (s *Service) defragmentState(st state.BeaconState) {
	startTime := time.Now()
	st.Defragment()
	elapsedTime := time.Since(startTime)
	stateDefragmentationTime.Observe(float64(elapsedTime.Milliseconds()))
}

// defragmentRoutine consumes imported states queued by ReceiveBlock and
// defragments them off the block-import hot path. Using a single long-lived
// worker rather than a goroutine per block bounds resource usage under sync or
// backlog, and the routine exits with the service context.
func (s *Service) defragmentRoutine() {
	for {
		select {
		case st := <-s.defragmentRequests:
			s.defragmentState(st)
		case <-s.ctx.Done():
			return
		}
	}
}
