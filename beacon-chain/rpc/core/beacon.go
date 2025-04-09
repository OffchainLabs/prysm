package core

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	consensusblocks "github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// Retrieve chain head information from the DB and the current beacon state.
func (s *Service) ChainHead(ctx context.Context) (*ethpb.ChainHead, *RpcError) {
	headBlock, err := s.HeadFetcher.HeadBlock(ctx)
	if err != nil {
		return nil, &RpcError{
			Err:    errors.Wrapf(err, "could not get head block"),
			Reason: Internal,
		}
	}
	if err := consensusblocks.BeaconBlockIsNil(headBlock); err != nil {
		return nil, &RpcError{
			Err:    errors.Wrapf(err, "head block of chain was nil"),
			Reason: NotFound,
		}
	}
	optimisticStatus, err := s.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		return nil, &RpcError{
			Err:    errors.Wrapf(err, "could not get optimistic status"),
			Reason: Internal,
		}
	}
	headBlockRoot, err := headBlock.Block().HashTreeRoot()
	if err != nil {
		return nil, &RpcError{
			Err:    errors.Wrapf(err, "could not get head block root"),
			Reason: Internal,
		}
	}

	validGenesis := false
	validateCP := func(cp *ethpb.Checkpoint, name string) error {
		if bytesutil.ToBytes32(cp.Root) == params.BeaconConfig().ZeroHash && cp.Epoch == 0 {
			if validGenesis {
				return nil
			}
			// Retrieve genesis block in the event we have genesis checkpoints.
			genBlock, err := s.BeaconDB.GenesisBlock(ctx)
			if err != nil || consensusblocks.BeaconBlockIsNil(genBlock) != nil {
				return errors.New("could not get genesis block")
			}
			validGenesis = true
			return nil
		}
		b, err := s.BeaconDB.Block(ctx, bytesutil.ToBytes32(cp.Root))
		if err != nil {
			return errors.Errorf("could not get %s block: %v", name, err)
		}
		if err := consensusblocks.BeaconBlockIsNil(b); err != nil {
			return errors.Errorf("could not get %s block: %v", name, err)
		}
		return nil
	}

	finalizedCheckpoint := s.FinalizedFetcher.FinalizedCheckpt()
	if err := validateCP(finalizedCheckpoint, "finalized"); err != nil {
		return nil, &RpcError{
			Err:    errors.Wrap(err, "could not get finalized checkpoint"),
			Reason: Internal,
		}
	}

	justifiedCheckpoint := s.FinalizedFetcher.CurrentJustifiedCheckpt()
	if err := validateCP(justifiedCheckpoint, "justified"); err != nil {
		return nil, &RpcError{
			Err:    errors.Wrap(err, "could not get current justified checkpoint"),
			Reason: Internal,
		}
	}

	prevJustifiedCheckpoint := s.FinalizedFetcher.PreviousJustifiedCheckpt()
	if err := validateCP(prevJustifiedCheckpoint, "prev justified"); err != nil {
		return nil, &RpcError{
			Err:    errors.Wrap(err, "could not get previous justified checkpoint"),
			Reason: Internal,
		}
	}

	fSlot, err := slots.EpochStart(finalizedCheckpoint.Epoch)
	if err != nil {
		return nil, &RpcError{
			Err:    errors.Wrapf(err, "could not get epoch start slot from finalized checkpoint epoch"),
			Reason: Internal,
		}
	}
	jSlot, err := slots.EpochStart(justifiedCheckpoint.Epoch)
	if err != nil {
		return nil, &RpcError{
			Err:    errors.Wrapf(err, "could not get epoch start slot from justified checkpoint epoch"),
			Reason: Internal,
		}
	}
	pjSlot, err := slots.EpochStart(prevJustifiedCheckpoint.Epoch)
	if err != nil {
		return nil, &RpcError{
			Err:    errors.Wrapf(err, "could not get epoch start slot from prev justified checkpoint epoch"),
			Reason: Internal,
		}
	}
	return &ethpb.ChainHead{
		HeadSlot:                   headBlock.Block().Slot(),
		HeadEpoch:                  slots.ToEpoch(headBlock.Block().Slot()),
		HeadBlockRoot:              headBlockRoot[:],
		FinalizedSlot:              fSlot,
		FinalizedEpoch:             finalizedCheckpoint.Epoch,
		FinalizedBlockRoot:         finalizedCheckpoint.Root,
		JustifiedSlot:              jSlot,
		JustifiedEpoch:             justifiedCheckpoint.Epoch,
		JustifiedBlockRoot:         justifiedCheckpoint.Root,
		PreviousJustifiedSlot:      pjSlot,
		PreviousJustifiedEpoch:     prevJustifiedCheckpoint.Epoch,
		PreviousJustifiedBlockRoot: prevJustifiedCheckpoint.Root,
		OptimisticStatus:           optimisticStatus,
	}, nil
}
