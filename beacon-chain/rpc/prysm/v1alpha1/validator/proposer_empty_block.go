package validator

import (
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func getEmptyBlock(slot primitives.Slot) (interfaces.SignedBeaconBlock, error) {
	var sBlk interfaces.SignedBeaconBlock
	var err error
	epoch := slots.ToEpoch(slot)
	switch {
	case epoch >= params.BeaconConfig().FuluForkEpoch:
		sBlk, err = blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockFulu{Block: &ethpb.BeaconBlockFulu{Body: &ethpb.BeaconBlockBodyFulu{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	case epoch >= params.BeaconConfig().ElectraForkEpoch:
		sBlk, err = blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockElectra{Block: &ethpb.BeaconBlockElectra{Body: &ethpb.BeaconBlockBodyElectra{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	case epoch >= params.BeaconConfig().DenebForkEpoch:
		sBlk, err = blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockDeneb{Block: &ethpb.BeaconBlockDeneb{Body: &ethpb.BeaconBlockBodyDeneb{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	case epoch >= params.BeaconConfig().CapellaForkEpoch:
		sBlk, err = blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{Block: &ethpb.BeaconBlockCapella{Body: &ethpb.BeaconBlockBodyCapella{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	case epoch >= params.BeaconConfig().BellatrixForkEpoch:
		sBlk, err = blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockBellatrix{Block: &ethpb.BeaconBlockBellatrix{Body: &ethpb.BeaconBlockBodyBellatrix{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	case epoch >= params.BeaconConfig().AltairForkEpoch:
		sBlk, err = blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockAltair{Block: &ethpb.BeaconBlockAltair{Body: &ethpb.BeaconBlockBodyAltair{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	default:
		sBlk, err = blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlock{Block: &ethpb.BeaconBlock{Body: &ethpb.BeaconBlockBody{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	}
	return sBlk, err
}
