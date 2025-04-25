package evaluators

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v6/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

var streamDeadline = 1 * time.Minute
var startingFork = version.Phase0

// AltairForkTransition ensures that the Altair hard fork has occurred successfully.
var AltairForkTransition = types.Evaluator{
	Name: "altair_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		altair := policies.OnEpoch(params.BeaconConfig().AltairForkEpoch)
		// TODO (11750): modify policies to take an end to end config
		if startingFork == version.Phase0 {
			return altair(e)
		}
		return false
	},
	Evaluation: altairForkOccurs,
}

// BellatrixForkTransition ensures that the Bellatrix hard fork has occurred successfully.
var BellatrixForkTransition = types.Evaluator{
	Name: "bellatrix_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		fEpoch := params.BeaconConfig().BellatrixForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: bellatrixForkOccurs,
}

// CapellaForkTransition ensures that the Capella hard fork has occurred successfully.
var CapellaForkTransition = types.Evaluator{
	Name: "capella_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		fEpoch := params.BeaconConfig().CapellaForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: capellaForkOccurs,
}

// DenebForkTransition ensures that the Deneb hard fork has occurred successfully
var DenebForkTransition = types.Evaluator{
	Name: "deneb_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		fEpoch := params.BeaconConfig().DenebForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: denebForkOccurs,
}

// ElectraForkTransition ensures that the electra hard fork has occurred successfully
var ElectraForkTransition = types.Evaluator{
	Name: "electra_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		fEpoch := params.BeaconConfig().ElectraForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: electraForkOccurs,
}

func altairForkOccurs(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := ethpb.NewBeaconNodeValidatorClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()
	stream, err := client.StreamBlocksAltair(ctx, &ethpb.StreamBlocksRequest{VerifiedOnly: true})
	if err != nil {
		return errors.Wrap(err, "failed to get stream")
	}
	fSlot, err := slots.EpochStart(params.BeaconConfig().AltairForkEpoch)
	if err != nil {
		return err
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return errors.New("context canceled prematurely")
	}
	res, err := stream.Recv()
	if err != nil {
		return err
	}
	if res == nil || res.Block == nil {
		return errors.New("nil block returned by beacon node")
	}
	if res.GetPhase0Block() == nil && res.GetAltairBlock() == nil {
		return errors.New("nil block returned by beacon node")
	}
	if res.GetPhase0Block() != nil {
		return errors.New("phase 0 block returned after altair fork has occurred")
	}
	blk, err := blocks.NewSignedBeaconBlock(res.GetAltairBlock())
	if err != nil {
		return err
	}
	if err := blocks.BeaconBlockIsNil(blk); err != nil {
		return err
	}
	if blk.Block().Slot() < fSlot {
		return errors.Errorf("wanted a block >= %d but received %d", fSlot, blk.Block().Slot())
	}
	return nil
}

func bellatrixForkOccurs(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := ethpb.NewBeaconNodeValidatorClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()
	stream, err := client.StreamBlocksAltair(ctx, &ethpb.StreamBlocksRequest{VerifiedOnly: true})
	if err != nil {
		return errors.Wrap(err, "failed to get stream")
	}
	fSlot, err := slots.EpochStart(params.BeaconConfig().BellatrixForkEpoch)
	if err != nil {
		return err
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return errors.New("context canceled prematurely")
	}
	res, err := stream.Recv()
	if err != nil {
		return err
	}
	if res == nil || res.Block == nil {
		return errors.New("nil block returned by beacon node")
	}
	if res.GetPhase0Block() == nil && res.GetAltairBlock() == nil && res.GetBellatrixBlock() == nil {
		return errors.New("nil block returned by beacon node")
	}
	if res.GetPhase0Block() != nil {
		return errors.New("phase 0 block returned after bellatrix fork has occurred")
	}
	if res.GetAltairBlock() != nil {
		return errors.New("altair block returned after bellatrix fork has occurred")
	}
	blk, err := blocks.NewSignedBeaconBlock(res.GetBellatrixBlock())
	if err != nil {
		return err
	}
	if err := blocks.BeaconBlockIsNil(blk); err != nil {
		return err
	}
	if blk.Block().Slot() < fSlot {
		return errors.Errorf("wanted a block >= %d but received %d", fSlot, blk.Block().Slot())
	}
	return nil
}

func capellaForkOccurs(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := ethpb.NewBeaconNodeValidatorClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()
	stream, err := client.StreamBlocksAltair(ctx, &ethpb.StreamBlocksRequest{VerifiedOnly: true})
	if err != nil {
		return errors.Wrap(err, "failed to get stream")
	}
	fSlot, err := slots.EpochStart(params.BeaconConfig().CapellaForkEpoch)
	if err != nil {
		return err
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return errors.New("context canceled prematurely")
	}
	res, err := stream.Recv()
	if err != nil {
		return err
	}
	if res == nil || res.Block == nil {
		return errors.New("nil block returned by beacon node")
	}

	if res.GetBlock() == nil {
		return errors.New("nil block returned by beacon node")
	}
	if res.GetCapellaBlock() == nil {
		return errors.Errorf("non-capella block returned after the fork with type %T", res.Block)
	}
	blk, err := blocks.NewSignedBeaconBlock(res.GetCapellaBlock())
	if err != nil {
		return err
	}
	if err := blocks.BeaconBlockIsNil(blk); err != nil {
		return err
	}
	if blk.Block().Slot() < fSlot {
		return errors.Errorf("wanted a block at slot >= %d but received %d", fSlot, blk.Block().Slot())
	}
	return nil
}

func denebForkOccurs(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := ethpb.NewBeaconNodeValidatorClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()
	stream, err := client.StreamBlocksAltair(ctx, &ethpb.StreamBlocksRequest{VerifiedOnly: true})
	if err != nil {
		return errors.Wrap(err, "failed to get stream")
	}
	fSlot, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	if err != nil {
		return err
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return errors.New("context canceled prematurely")
	}
	res, err := stream.Recv()
	if err != nil {
		return err
	}
	if res == nil || res.Block == nil {
		return errors.New("nil block returned by beacon node")
	}

	if res.GetBlock() == nil {
		return errors.New("nil block returned by beacon node")
	}
	if res.GetDenebBlock() == nil {
		return errors.Errorf("non-deneb block returned after the fork with type %T", res.Block)
	}
	blk, err := blocks.NewSignedBeaconBlock(res.GetDenebBlock())
	if err != nil {
		return err
	}
	if err := blocks.BeaconBlockIsNil(blk); err != nil {
		return err
	}
	if blk.Block().Slot() < fSlot {
		return errors.Errorf("wanted a block at slot >= %d but received %d", fSlot, blk.Block().Slot())
	}
	return nil
}

func electraForkOccurs(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := ethpb.NewBeaconNodeValidatorClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()
	stream, err := client.StreamBlocksAltair(ctx, &ethpb.StreamBlocksRequest{VerifiedOnly: true})
	if err != nil {
		return errors.Wrap(err, "failed to get stream")
	}
	fSlot, err := slots.EpochStart(params.BeaconConfig().ElectraForkEpoch)
	if err != nil {
		return err
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return errors.New("context canceled prematurely")
	}
	res, err := stream.Recv()
	if err != nil {
		return err
	}
	if res == nil || res.Block == nil {
		return errors.New("nil block returned by beacon node")
	}

	if res.GetBlock() == nil {
		return errors.New("nil block returned by beacon node")
	}
	if res.GetElectraBlock() == nil {
		return errors.Errorf("non-electra block returned after the fork with type %T", res.Block)
	}
	blk, err := blocks.NewSignedBeaconBlock(res.GetElectraBlock())
	if err != nil {
		return err
	}
	if err := blocks.BeaconBlockIsNil(blk); err != nil {
		return err
	}
	if blk.Block().Slot() < fSlot {
		return errors.Errorf("wanted a block at slot >= %d but received %d", fSlot, blk.Block().Slot())
	}
	return nil
}
