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
