package evaluators

import (
	"context"
	"testing"
	"time"

	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	mock "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestWaitForMatchingHeadsRetriesUntilConverged(t *testing.T) {
	oldDelay := connTimeDelay
	connTimeDelay = time.Millisecond
	t.Cleanup(func() {
		connTimeDelay = oldDelay
	})

	ctrl := gomock.NewController(t)
	client0 := mock.NewMockBeaconChainClient(ctrl)
	client1 := mock.NewMockBeaconChainClient(ctrl)

	matching := &eth.ChainHead{
		HeadEpoch:                  2,
		HeadBlockRoot:              []byte{1},
		JustifiedBlockRoot:         []byte{2},
		PreviousJustifiedBlockRoot: []byte{3},
		FinalizedBlockRoot:         []byte{4},
	}
	divergent := &eth.ChainHead{
		HeadEpoch:                  2,
		HeadBlockRoot:              []byte{9},
		JustifiedBlockRoot:         []byte{2},
		PreviousJustifiedBlockRoot: []byte{3},
		FinalizedBlockRoot:         []byte{4},
	}

	client0.EXPECT().GetChainHead(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.ChainHead, error) {
			return matching, nil
		},
	).AnyTimes()

	var calls int
	client1.EXPECT().GetChainHead(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.ChainHead, error) {
			calls++
			if calls < 3 {
				return divergent, nil
			}
			return matching, nil
		},
	).AnyTimes()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.NoError(t, waitForMatchingHeads(ctx, client0, client1))
}

func TestWaitForMatchingHeadsReturnsLastMismatchOnTimeout(t *testing.T) {
	oldDelay := connTimeDelay
	connTimeDelay = time.Millisecond
	t.Cleanup(func() {
		connTimeDelay = oldDelay
	})

	ctrl := gomock.NewController(t)
	client0 := mock.NewMockBeaconChainClient(ctrl)
	client1 := mock.NewMockBeaconChainClient(ctrl)

	matching := &eth.ChainHead{
		HeadEpoch:                  2,
		HeadBlockRoot:              []byte{1},
		JustifiedBlockRoot:         []byte{2},
		PreviousJustifiedBlockRoot: []byte{3},
		FinalizedBlockRoot:         []byte{4},
	}
	divergent := &eth.ChainHead{
		HeadEpoch:                  2,
		HeadBlockRoot:              []byte{9},
		JustifiedBlockRoot:         []byte{2},
		PreviousJustifiedBlockRoot: []byte{3},
		FinalizedBlockRoot:         []byte{4},
	}

	client0.EXPECT().GetChainHead(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.ChainHead, error) {
			return matching, nil
		},
	).AnyTimes()
	client1.EXPECT().GetChainHead(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.ChainHead, error) {
			return divergent, nil
		},
	).AnyTimes()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	err := waitForMatchingHeads(ctx, client0, client1)
	require.ErrorContains(t, "received conflicting head block roots", err)
}

func TestWaitForPeersConnectedRetriesUntilConnected(t *testing.T) {
	oldDelay := connTimeDelay
	connTimeDelay = time.Millisecond
	t.Cleanup(func() {
		connTimeDelay = oldDelay
	})

	ctrl := gomock.NewController(t)
	client0 := mock.NewMockNodeClient(ctrl)
	client1 := mock.NewMockNodeClient(ctrl)

	full := &eth.Peers{Peers: make([]*eth.Peer, 2)}
	partial := &eth.Peers{Peers: make([]*eth.Peer, 1)}

	client0.EXPECT().ListPeers(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.Peers, error) {
			return full, nil
		},
	).AnyTimes()

	var calls int
	client1.EXPECT().ListPeers(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.Peers, error) {
			calls++
			if calls < 3 {
				return partial, nil
			}
			return full, nil
		},
	).AnyTimes()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.NoError(t, waitForPeersConnected(ctx, 2, client0, client1))
}

func TestWaitForPeersConnectedReturnsShortfallOnTimeout(t *testing.T) {
	oldDelay := connTimeDelay
	connTimeDelay = time.Millisecond
	t.Cleanup(func() {
		connTimeDelay = oldDelay
	})

	ctrl := gomock.NewController(t)
	client0 := mock.NewMockNodeClient(ctrl)
	client1 := mock.NewMockNodeClient(ctrl)

	full := &eth.Peers{Peers: make([]*eth.Peer, 2)}
	partial := &eth.Peers{Peers: make([]*eth.Peer, 1)}

	client0.EXPECT().ListPeers(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.Peers, error) {
			return full, nil
		},
	).AnyTimes()
	client1.EXPECT().ListPeers(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.Peers, error) {
			return partial, nil
		},
	).AnyTimes()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	err := waitForPeersConnected(ctx, 2, client0, client1)
	require.ErrorContains(t, "unexpected amount of peers", err)
}
