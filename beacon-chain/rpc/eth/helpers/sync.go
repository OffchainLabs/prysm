package helpers

import (
	"bytes"
	"context"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// IsOptimistic checks whether the beacon state's block is optimistic.
func IsOptimistic(
	ctx context.Context,
	stateId []byte,
	optimisticModeFetcher blockchain.OptimisticModeFetcher,
	stateFetcher lookup.Stater,
	chainInfo blockchain.ChainInfoFetcher,
	database db.ReadOnlyDatabase,
) (bool, error) {
	stateIdString := strings.ToLower(string(stateId))
	switch stateIdString {
	case "head":
		return optimisticModeFetcher.IsOptimistic(ctx)
	case "genesis":
		return false, nil
	case "finalized":
		fcp := chainInfo.FinalizedCheckpt()
		if fcp == nil {
			return true, errors.New("received nil finalized checkpoint")
		}
		// Special genesis case in the event our checkpoint root is a zerohash.
		if bytes.Equal(fcp.Root, params.BeaconConfig().ZeroHash[:]) {
			return false, nil
		}
		return optimisticModeFetcher.IsOptimisticForRoot(ctx, bytesutil.ToBytes32(fcp.Root))
	case "justified":
		jcp := chainInfo.CurrentJustifiedCheckpt()
		if jcp == nil {
			return true, errors.New("received nil justified checkpoint")
		}
		// Special genesis case in the event our checkpoint root is a zerohash.
		if bytes.Equal(jcp.Root, params.BeaconConfig().ZeroHash[:]) {
			return false, nil
		}
		return optimisticModeFetcher.IsOptimisticForRoot(ctx, bytesutil.ToBytes32(jcp.Root))
	default:
		if bytesutil.IsHex(stateId) {
			id, err := hexutil.Decode(stateIdString)
			if err != nil {
				e := lookup.NewStateIdParseError(err)
				return false, &e
			}
			return isStateRootOptimistic(ctx, id, optimisticModeFetcher, stateFetcher, chainInfo, database)
		} else if len(stateId) == 32 {
			return isStateRootOptimistic(ctx, stateId, optimisticModeFetcher, stateFetcher, chainInfo, database)
		} else {
			optimistic, err := optimisticModeFetcher.IsOptimistic(ctx)
			if err != nil {
				return true, errors.Wrap(err, "could not check optimistic status")
			}
			if !optimistic {
				return false, nil
			}
			slotNumber, parseErr := strconv.ParseUint(stateIdString, 10, 64)
			if parseErr != nil {
				// ID format does not match any valid options.
				e := lookup.NewStateIdParseError(parseErr)
				return true, &e
			}
			fcp := chainInfo.FinalizedCheckpt()
			if fcp == nil {
				return true, errors.New("received nil finalized checkpoint")
			}
			finalizedSlot, err := slots.EpochStart(fcp.Epoch)
			if err != nil {
				return true, errors.Wrap(err, "could not get head state's finalized slot")
			}
			lastValidatedCheckpoint, err := database.LastValidatedCheckpoint(ctx)
			if err != nil {
				return true, errors.Wrap(err, "could not get last validated checkpoint")
			}
			validatedSlot, err := slots.EpochStart(lastValidatedCheckpoint.Epoch)
			if err != nil {
				return true, errors.Wrap(err, "could not get last validated slot")
			}
			if primitives.Slot(slotNumber) <= validatedSlot {
				return false, nil
			}
			// if the finalized checkpoint is higher than the last
			// validated checkpoint, we are syncing and have synced
			// a finalization being optimistic
			if validatedSlot < finalizedSlot {
				return true, nil
			}
			if primitives.Slot(slotNumber) == chainInfo.HeadSlot() {
				// We know the head is optimistic because we checked it above.
				return true, nil
			}
			headRoot, err := chainInfo.HeadRoot(ctx)
			if err != nil {
				return true, errors.Wrap(err, "could not get head root")
			}
			r, err := chainInfo.Ancestor(ctx, headRoot, primitives.Slot(slotNumber))
			if err != nil {
				return true, errors.Wrap(err, "could not get ancestor root")
			}
			return optimisticModeFetcher.IsOptimisticForRoot(ctx, bytesutil.ToBytes32(r))
		}
	}
}

func isStateRootOptimistic(
	ctx context.Context,
	stateId []byte,
	optimisticModeFetcher blockchain.OptimisticModeFetcher,
	stateFetcher lookup.Stater,
	chainInfo blockchain.ChainInfoFetcher,
	database db.ReadOnlyDatabase,
) (bool, error) {
	st, err := stateFetcher.State(ctx, stateId)
	if err != nil {
		return true, lookup.NewFetchStateError(err)
	}
	if st.Slot() == chainInfo.HeadSlot() {
		return optimisticModeFetcher.IsOptimistic(ctx)
	}
	has, roots, err := database.BlockRootsBySlot(ctx, st.Slot())
	if err != nil {
		return true, errors.Wrapf(err, "could not get block roots for slot %d", st.Slot())
	}
	if !has {
		return true, lookup.NewBlockNotFoundError("no block roots returned from the database")
	}
	for _, r := range roots {
		b, err := database.Block(ctx, r)
		if err != nil {
			return true, errors.Wrapf(err, "could not obtain block")
		}
		if bytesutil.ToBytes32(stateId) != b.Block().StateRoot() {
			continue
		}
		return optimisticModeFetcher.IsOptimisticForRoot(ctx, r)
	}
	// No block matching requested state root, return true.
	return true, nil
}
