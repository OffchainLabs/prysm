package evaluators

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// BuilderIsActive checks that the builder is indeed producing the respective payloads
var BuilderIsActive = e2etypes.Evaluator{
	Name: "builder_is_active_at_epoch_%d",
	Policy: func(e primitives.Epoch) bool {
		fEpoch := params.BeaconConfig().BellatrixForkEpoch
		return policies.OnwardsNthEpoch(fEpoch)(e)
	},
	Evaluation: builderActive,
}

// maxNonBuilderBlocks is the maximum number of blocks that can be built locally
// instead of by the builder before the test fails. This allows tolerance for
// occasional builder timeouts or failures.
const maxNonBuilderBlocks = 2

func builderActive(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := firstConn(conns)
	client := ethpb.NewNodeClient(conn)
	beaconClient := ethpb.NewBeaconChainClient(conn)
	genesis, err := client.GetGenesis(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get genesis data")
	}
	currSlot := slots.CurrentSlot(genesis.GenesisTime.AsTime())
	currEpoch := slots.ToEpoch(currSlot)
	lowestBound := primitives.Epoch(0)
	if currEpoch >= 1 {
		lowestBound = currEpoch - 1
	}

	if lowestBound < params.BeaconConfig().BellatrixForkEpoch {
		lowestBound = params.BeaconConfig().BellatrixForkEpoch
	}
	emptyRt, err := ssz.TransactionsRoot([][]byte{})
	if err != nil {
		return err
	}

	nonBuilderBlocks := 0
	builderBlocks := 0

	blockCtrs, err := beaconClient.ListBeaconBlocks(context.Background(), &ethpb.ListBlocksRequest{QueryFilter: &ethpb.ListBlocksRequest_Epoch{Epoch: lowestBound}})
	if err != nil {
		return errors.Wrap(err, "failed to get beacon blocks")
	}
	for _, ctr := range blockCtrs.BlockContainers {
		b, err := syncCompatibleBlockFromCtr(ctr)
		if err != nil {
			return errors.Wrapf(err, "block type doesn't exist for block at epoch %d", lowestBound)
		}

		if b.IsNil() {
			return errors.New("nil block provided")
		}
		forkStartSlot, err := slots.EpochStart(params.BeaconConfig().BellatrixForkEpoch)
		if err != nil {
			return err
		}
		if forkStartSlot == b.Block().Slot() || forkStartSlot+1 == b.Block().Slot() || lowestBound <= 1 {
			// Skip fork slot and the next one, as we don't send FCUs yet.
			continue
		}
		execPayload, err := b.Block().Body().Execution()
		if err != nil {
			return err
		}
		txRoot, err := execPayload.TransactionsRoot()
		if err != nil {
			return err
		}
		if [32]byte(txRoot) == emptyRt && string(execPayload.ExtraData()) != "prysm-builder" {
			// If a local payload is built with 0 transactions, builder cannot build a payload with more transactions
			// since they both utilize the same EL.
			continue
		}
		if string(execPayload.ExtraData()) != "prysm-builder" {
			nonBuilderBlocks++
			continue
		}
		builderBlocks++
		if execPayload.GasLimit() == 0 {
			return errors.Errorf("%s block with slot %d has a gas limit of 0, when it should be in the 30M range", version.String(b.Version()), b.Block().Slot())
		}
	}
	if lowestBound == currEpoch {
		if nonBuilderBlocks > maxNonBuilderBlocks {
			return errors.Errorf("too many non-builder blocks: %d (max allowed: %d), builder blocks: %d", nonBuilderBlocks, maxNonBuilderBlocks, builderBlocks)
		}
		return nil
	}
	blockCtrs, err = beaconClient.ListBeaconBlocks(context.Background(), &ethpb.ListBlocksRequest{QueryFilter: &ethpb.ListBlocksRequest_Epoch{Epoch: currEpoch}})
	if err != nil {
		return errors.Wrap(err, "failed to get validator participation")
	}
	for _, ctr := range blockCtrs.BlockContainers {
		b, err := syncCompatibleBlockFromCtr(ctr)
		if err != nil {
			return errors.Wrapf(err, "block type doesn't exist for block at epoch %d", lowestBound)
		}
		if b.IsNil() {
			return errors.New("nil block provided")
		}
		forkStartSlot, err := slots.EpochStart(params.BeaconConfig().BellatrixForkEpoch)
		if err != nil {
			return err
		}
		if forkStartSlot == b.Block().Slot() || forkStartSlot+1 == b.Block().Slot() || lowestBound <= 1 {
			// Skip fork slot and the next one, as we don't send FCUs yet.
			continue
		}
		execPayload, err := b.Block().Body().Execution()
		if err != nil {
			return err
		}
		txRoot, err := execPayload.TransactionsRoot()
		if err != nil {
			return err
		}
		if [32]byte(txRoot) == emptyRt && string(execPayload.ExtraData()) != "prysm-builder" {
			// If a local payload is built with 0 transactions, builder cannot build a payload with more transactions
			// since they both utilize the same EL.
			continue
		}
		if string(execPayload.ExtraData()) != "prysm-builder" {
			nonBuilderBlocks++
			continue
		}
		builderBlocks++
		if execPayload.GasLimit() == 0 {
			return errors.Errorf("%s block with slot %d has a gas limit of 0, when it should be in the 30M range", version.String(b.Version()), b.Block().Slot())
		}
	}
	if nonBuilderBlocks > maxNonBuilderBlocks {
		return errors.Errorf("too many non-builder blocks: %d (max allowed: %d), builder blocks: %d", nonBuilderBlocks, maxNonBuilderBlocks, builderBlocks)
	}
	return nil
}
