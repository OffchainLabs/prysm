package evaluators

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	mock "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestValidatorsVoteWithTheMajoritySortsBlocksBySlot(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mock.NewMockBeaconChainClient(ctrl)
	ec := e2etypes.NewEvaluationContext(nil)
	vote := []byte{0xaa}

	client.EXPECT().GetChainHead(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*ethpb.ChainHead, error) {
			return &ethpb.ChainHead{HeadEpoch: 1}, nil
		},
	)
	client.EXPECT().ListBeaconBlocks(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *ethpb.ListBlocksRequest, ...grpc.CallOption) (*ethpb.ListBeaconBlocksResponse, error) {
			return &ethpb.ListBeaconBlocksResponse{BlockContainers: []*ethpb.BeaconBlockContainer{
				phase0BlockContainer(4, vote),
				phase0BlockContainer(5, vote),
				phase0BlockContainer(6, vote),
				phase0BlockContainer(1, vote),
			}}, nil
		},
	)

	require.NoError(t, validatorsVoteWithTheMajorityForClient(ec, client))
	require.Equal(t, true, string(ec.ExpectedEth1DataVote) == string(vote))
}

func TestSelectVoluntaryExitCandidatesBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		keys     [][48]byte
		exited   map[[48]byte]primitives.Epoch
		reserved map[[48]byte]bool
		limit    int
		wantNil  bool
		want     []primitives.ValidatorIndex
	}{
		{
			name:    "returns nil for zero limit",
			keys:    [][48]byte{{1}},
			limit:   0,
			wantNil: true,
		},
		{
			name:    "returns nil for negative limit",
			keys:    [][48]byte{{1}},
			limit:   -1,
			wantNil: true,
		},
		{
			name:  "returns empty candidates for empty keys",
			limit: 1,
			want:  []primitives.ValidatorIndex{},
		},
		{
			name:  "returns fewer eligible candidates than limit",
			keys:  [][48]byte{{1}, {2}},
			limit: 3,
			want:  []primitives.ValidatorIndex{0, 1},
		},
		{
			name:   "skips exited keys",
			keys:   [][48]byte{{1}, {2}},
			exited: map[[48]byte]primitives.Epoch{{1}: 1},
			limit:  1,
			want:   []primitives.ValidatorIndex{1},
		},
		{
			name:     "falls back when all keys are reserved",
			keys:     [][48]byte{{1}, {2}},
			reserved: map[[48]byte]bool{{1}: true, {2}: true},
			limit:    2,
			want:     []primitives.ValidatorIndex{0, 1},
		},
		{
			name:     "prefers unreserved keys before reserved keys",
			keys:     [][48]byte{{1}, {2}, {3}},
			exited:   map[[48]byte]primitives.Epoch{{1}: 1},
			reserved: map[[48]byte]bool{{2}: true},
			limit:    2,
			want:     []primitives.ValidatorIndex{2, 1},
		},
		{
			name:   "selects first and last indices",
			keys:   [][48]byte{{1}, {2}, {3}},
			exited: map[[48]byte]primitives.Epoch{{2}: 1},
			limit:  2,
			want:   []primitives.ValidatorIndex{0, 2},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidates := selectVoluntaryExitCandidates(test.keys, test.exited, test.reserved, test.limit)
			if test.wantNil {
				require.Equal(t, true, candidates == nil)
				return
			}
			require.DeepEqual(t, test.want, candidates)
		})
	}
}

func TestEnsureVoluntaryExitSubmitted(t *testing.T) {
	tests := []struct {
		name      string
		count     int
		wantError string
	}{
		{
			name:      "returns an error when no exits are submitted",
			wantError: "no eligible validators available for voluntary exit",
		},
		{
			name:  "allows one submitted exit",
			count: 1,
		},
		{
			name:  "allows multiple submitted exits",
			count: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ensureVoluntaryExitSubmitted(test.count)
			if test.wantError != "" {
				require.ErrorContains(t, test.wantError, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func phase0BlockContainer(slot primitives.Slot, vote []byte) *ethpb.BeaconBlockContainer {
	return &ethpb.BeaconBlockContainer{
		Block: &ethpb.BeaconBlockContainer_Phase0Block{
			Phase0Block: &ethpb.SignedBeaconBlock{
				Block: &ethpb.BeaconBlock{
					Slot: slot,
					Body: &ethpb.BeaconBlockBody{
						Eth1Data: &ethpb.Eth1Data{BlockHash: vote},
					},
				},
			},
		},
	}
}
