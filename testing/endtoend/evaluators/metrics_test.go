package evaluators

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	mock "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestCompareHeadSlots(t *testing.T) {
	const tolerance primitives.Slot = 8
	tests := []struct {
		name     string
		heads    []primitives.Slot
		timeSlot primitives.Slot
		wantErr  string
	}{
		{
			name:     "all heads at the clock slot",
			heads:    []primitives.Slot{176, 176},
			timeSlot: 176,
		},
		{
			name:     "current slot block not received yet",
			heads:    []primitives.Slot{175, 175},
			timeSlot: 176,
		},
		{
			name:     "slots skipped network-wide within tolerance",
			heads:    []primitives.Slot{174, 174},
			timeSlot: 176,
		},
		{
			name:     "one node a slot behind the highest head during skipped slots",
			heads:    []primitives.Slot{174, 173},
			timeSlot: 176,
		},
		{
			name:     "skipped slots at the tolerance bound",
			heads:    []primitives.Slot{168},
			timeSlot: 176,
		},
		{
			name:     "node lags the highest head",
			heads:    []primitives.Slot{176, 173},
			timeSlot: 176,
			wantErr:  "node 1 head slot 173 trails wall-clock slot 176 by 3 slots (highest head across nodes 176, skipped-slot tolerance 8)",
		},
		{
			name:     "node lags while the others tolerate skipped slots",
			heads:    []primitives.Slot{174, 171},
			timeSlot: 176,
			wantErr:  "node 1 head slot 171 trails wall-clock slot 176 by 5 slots",
		},
		{
			name:     "chain stalled beyond tolerance",
			heads:    []primitives.Slot{167, 167},
			timeSlot: 176,
			wantErr:  "node 0 head slot 167 trails wall-clock slot 176 by 9 slots",
		},
		{
			name:     "head ahead of the clock",
			heads:    []primitives.Slot{177},
			timeSlot: 176,
			wantErr:  "node 0 head slot 177 is ahead of wall-clock slot 176",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := compareHeadSlots(tt.heads, tt.timeSlot, tolerance)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, tt.wantErr, err)
		})
	}
}

// genesisForClockSlot returns a genesis time that puts the wall clock in the middle of slot.
func genesisForClockSlot(slot primitives.Slot) time.Time {
	d := params.BeaconConfig().SlotDuration()
	return time.Now().Add(-time.Duration(slot)*d - d/2)
}

func chainHeadAt(slot primitives.Slot) *eth.ChainHead {
	return &eth.ChainHead{HeadSlot: slot}
}

func TestWaitForHeadsNearClockRetriesUntilHeadCatchesUp(t *testing.T) {
	oldDelay := connTimeDelay
	connTimeDelay = time.Millisecond
	t.Cleanup(func() {
		connTimeDelay = oldDelay
	})

	ctrl := gomock.NewController(t)
	client0 := mock.NewMockBeaconChainClient(ctrl)
	client1 := mock.NewMockBeaconChainClient(ctrl)

	client0.EXPECT().GetChainHead(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.ChainHead, error) {
			return chainHeadAt(100), nil
		},
	).AnyTimes()

	var calls int
	client1.EXPECT().GetChainHead(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.ChainHead, error) {
			calls++
			if calls < 3 {
				return chainHeadAt(97), nil
			}
			return chainHeadAt(100), nil
		},
	).AnyTimes()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, waitForHeadsNearClock(ctx, genesisForClockSlot(100), 8, client0, client1))
	require.Equal(t, true, calls >= 3, "expected the lagging node to be polled until it caught up")
}

func TestWaitForHeadsNearClockReturnsLastErrorOnTimeout(t *testing.T) {
	oldDelay := connTimeDelay
	connTimeDelay = time.Millisecond
	t.Cleanup(func() {
		connTimeDelay = oldDelay
	})

	ctrl := gomock.NewController(t)
	client0 := mock.NewMockBeaconChainClient(ctrl)
	client1 := mock.NewMockBeaconChainClient(ctrl)

	client0.EXPECT().GetChainHead(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.ChainHead, error) {
			return chainHeadAt(90), nil
		},
	).AnyTimes()
	client1.EXPECT().GetChainHead(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.ChainHead, error) {
			return chainHeadAt(100), nil
		},
	).AnyTimes()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := waitForHeadsNearClock(ctx, genesisForClockSlot(100), 8, client0, client1)
	require.ErrorContains(t, "node 0 head slot 90 trails wall-clock slot 100 by 10 slots", err)
}

func TestWaitForHeadsNearClockAcceptsSkippedSlots(t *testing.T) {
	oldDelay := connTimeDelay
	connTimeDelay = time.Millisecond
	t.Cleanup(func() {
		connTimeDelay = oldDelay
	})

	ctrl := gomock.NewController(t)
	client0 := mock.NewMockBeaconChainClient(ctrl)
	client1 := mock.NewMockBeaconChainClient(ctrl)
	for _, client := range []*mock.MockBeaconChainClient{client0, client1} {
		client.EXPECT().GetChainHead(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*eth.ChainHead, error) {
				return chainHeadAt(98), nil
			},
		).Times(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, waitForHeadsNearClock(ctx, genesisForClockSlot(100), 8, client0, client1))
}
