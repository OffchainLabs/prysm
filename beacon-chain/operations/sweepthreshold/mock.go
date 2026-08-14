// Package sweepthreshold holds a devnet-only source of EIP-8148 set sweep threshold
// requests.
//
// EIP-8148 requests originate on the execution layer as EIP-7685 request type 0x05.
// No execution client implements that request type yet, so on a devnet the consensus
// side of the feature is otherwise unreachable. The pool below is filled by the debug
// Beacon API endpoint POST /prysm/v1/debug/beacon/sweep_threshold_requests. The Gloas
// block production path drains it into the locally built ExecutionRequestsGloas as if
// the execution client had returned the requests.
//
// This is strictly a stand-in for a real execution client and must not be used on a
// network whose other nodes source requests from their execution clients: they would
// not see the injected requests, and the injecting node would produce blocks the rest
// of the network rejects.
package sweepthreshold

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/config/params"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/pkg/errors"
)

// ErrPoolFull is returned when the queue already holds a full payload's worth of requests.
var ErrPoolFull = errors.New("set sweep threshold request pool is full")

// MockPool queues mocked set sweep threshold requests for injection into locally built
// payloads. It is safe for concurrent use, and a nil pool behaves as a permanently empty
// one so callers do not need nil checks.
type MockPool struct {
	lock     sync.Mutex
	requests []*enginev1.SetSweepThresholdRequest
}

// NewMockPool returns an empty pool.
func NewMockPool() *MockPool {
	return &MockPool{}
}

// Insert queues requests for the next block this node proposes. It rejects the whole batch
// if it would take the queue past MAX_SET_SWEEP_THRESHOLD_REQUESTS_PER_PAYLOAD, since
// anything over that limit could never be included anyway.
func (p *MockPool) Insert(requests ...*enginev1.SetSweepThresholdRequest) error {
	if p == nil {
		return errors.New("set sweep threshold request pool is not available")
	}
	if len(requests) == 0 {
		return nil
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	limit := params.BeaconConfig().MaxSetSweepThresholdRequestsPerPayload
	if uint64(len(p.requests)+len(requests)) > limit {
		return errors.Wrapf(ErrPoolFull, "queue holds %d of at most %d requests, cannot add %d more",
			len(p.requests), limit, len(requests))
	}

	p.requests = append(p.requests, requests...)
	return nil
}

// Pending returns the number of queued requests.
func (p *MockPool) Pending() int {
	if p == nil {
		return 0
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	return len(p.requests)
}

// Copy returns the queued requests without draining them.
func (p *MockPool) Copy() []*enginev1.SetSweepThresholdRequest {
	if p == nil {
		return nil
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	requests := make([]*enginev1.SetSweepThresholdRequest, 0, len(p.requests))
	for _, r := range p.requests {
		requests = append(requests, r.Copy())
	}

	return requests
}

// Drain returns the queued requests and empties the queue. Requests are handed out exactly
// once, so a re-proposal at a later slot does not duplicate them.
func (p *MockPool) Drain() []*enginev1.SetSweepThresholdRequest {
	if p == nil {
		return nil
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	requests := p.requests
	p.requests = nil

	return requests
}
