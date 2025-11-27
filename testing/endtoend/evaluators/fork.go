package evaluators

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

var streamDeadline = 1 * time.Minute

// AltairForkTransition ensures that the Altair hard fork has occurred successfully.
var AltairForkTransition = e2etypes.Evaluator{
	Name: "altair_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Altair
		if e2etypes.GenesisFork() >= version.Altair {
			return false
		}
		altair := policies.OnEpoch(params.BeaconConfig().AltairForkEpoch)
		return altair(e)
	},
	Evaluation: altairForkOccurs,
}

// BellatrixForkTransition ensures that the Bellatrix hard fork has occurred successfully.
var BellatrixForkTransition = e2etypes.Evaluator{
	Name: "bellatrix_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Bellatrix
		if e2etypes.GenesisFork() >= version.Bellatrix {
			return false
		}
		fEpoch := params.BeaconConfig().BellatrixForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: bellatrixForkOccurs,
}

// CapellaForkTransition ensures that the Capella hard fork has occurred successfully.
var CapellaForkTransition = e2etypes.Evaluator{
	Name: "capella_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Capella
		if e2etypes.GenesisFork() >= version.Capella {
			return false
		}
		fEpoch := params.BeaconConfig().CapellaForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: capellaForkOccurs,
}

// DenebForkTransition ensures that the Deneb hard fork has occurred successfully
var DenebForkTransition = e2etypes.Evaluator{
	Name: "deneb_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Deneb
		if e2etypes.GenesisFork() >= version.Deneb {
			return false
		}
		fEpoch := params.BeaconConfig().DenebForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: denebForkOccurs,
}

// ElectraForkTransition ensures that the electra hard fork has occurred successfully
var ElectraForkTransition = e2etypes.Evaluator{
	Name: "electra_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Electra
		if e2etypes.GenesisFork() >= version.Electra {
			return false
		}
		fEpoch := params.BeaconConfig().ElectraForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: electraForkOccurs,
}

func altairForkOccurs(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
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

func bellatrixForkOccurs(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
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

func capellaForkOccurs(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
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

func denebForkOccurs(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
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

func electraForkOccurs(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
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
