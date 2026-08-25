package peerscoring

import (
	"testing"
	"time"

	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
)

func TestRpcStatusScorer(t *testing.T) {
	scorer := rpcStatusScorer{}
	tests := []struct {
		name           string
		status         *RpcStatus
		ourHead        primitives.Slot
		highestHead    primitives.Slot
		wantScore      float64
		wantGreylisted bool
	}{
		{"no status", nil, 0, 100, 0, false},
		{"no chain state", &RpcStatus{}, 0, 100, 0, false},
		{"behind our head", &RpcStatus{chainState: &pb.StatusV2{HeadSlot: 10}}, 20, 100, 0, false},
		{"no known highest head", &RpcStatus{chainState: &pb.StatusV2{HeadSlot: 10}}, 0, 0, 0, false},
		{"halfway to highest head", &RpcStatus{chainState: &pb.StatusV2{HeadSlot: 50}}, 10, 100, 0.125, false}, // 0.5 * 0.25
		{"at highest head", &RpcStatus{chainState: &pb.StatusV2{HeadSlot: 100}}, 10, 100, 0.25, false},
		{
			"non-terminal validation error still scores",
			&RpcStatus{chainState: &pb.StatusV2{HeadSlot: 50}, validationError: errors.New("temporary")},
			0, 100, 0.125, false,
		},
		// Terminal validation errors greylist the peer; scoring is judged independently.
		{"wrong fork digest greylists", &RpcStatus{validationError: p2ptypes.ErrWrongForkDigestVersion}, 0, 100, 0, true},
		{"invalid finalized root greylists", &RpcStatus{validationError: p2ptypes.ErrInvalidFinalizedRoot}, 0, 100, 0, true},
		{"invalid request greylists", &RpcStatus{validationError: p2ptypes.ErrInvalidRequest}, 0, 100, 0, true},
		{
			"wrapped terminal error greylists",
			&RpcStatus{validationError: errors.Wrap(p2ptypes.ErrWrongForkDigestVersion, "status")},
			0, 100, 0, true,
		},
		{
			"terminal error scores independently of greylisting",
			&RpcStatus{chainState: &pb.StatusV2{HeadSlot: 50}, validationError: p2ptypes.ErrInvalidRequest},
			0, 100, 0.125, true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			si := testInfo(&PeerScoringInfo{rpcStatus: tc.status}, tc.ourHead, tc.highestHead)
			require.Equal(t, tc.wantScore, scorer.Score(testPid, si))
			require.Equal(t, tc.wantGreylisted, scorer.IsPeerGreylisted(testPid, si))
			require.Equal(t, time.Duration(0), scorer.TimeToWhitelisting(testPid, si)) // never time based
		})
	}
}
