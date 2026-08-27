package peerscoring

import (
	"testing"
	"time"

	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
)

func TestRpcStatusScorer(t *testing.T) {
	scorer := rpcStatusScorer{}
	tests := []struct {
		name           string
		status         *RpcStatus
		wantGreyListed bool
	}{
		{"no status", nil, false},
		{"no chain state", &RpcStatus{}, false},
		{
			"non-terminal validation error does not greylist",
			&RpcStatus{chainState: &pb.StatusV2{HeadSlot: 50}, validationError: errors.New("temporary")},
			false,
		},
		// Terminal validation errors greylist the peer.
		{"wrong fork digest greylists", &RpcStatus{validationError: p2ptypes.ErrWrongForkDigestVersion}, true},
		{"invalid finalized root greylists", &RpcStatus{validationError: p2ptypes.ErrInvalidFinalizedRoot}, true},
		{"invalid request greylists", &RpcStatus{validationError: p2ptypes.ErrInvalidRequest}, true},
		{
			"wrapped terminal error greylists",
			&RpcStatus{validationError: errors.Wrap(p2ptypes.ErrWrongForkDigestVersion, "status")},
			true,
		},
		{
			"terminal error with chain state still greylists",
			&RpcStatus{chainState: &pb.StatusV2{HeadSlot: 50}, validationError: p2ptypes.ErrInvalidRequest},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			si := testInfo(&PeerScoringInfo{rpcStatus: tc.status})
			err := scorer.IsPeerGreyListed(testPid, si)
			if tc.wantGreyListed {
				require.ErrorIs(t, err, ErrPeerGreyListed)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, time.Duration(0), scorer.TimeToWhiteListing(testPid, si)) // never time based
		})
	}
}
