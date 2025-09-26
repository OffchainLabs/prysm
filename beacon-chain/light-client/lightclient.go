package light_client

import (
	"bytes"
	"context"
	"fmt"
	"reflect"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	consensus_types "github.com/OffchainLabs/prysm/v6/consensus-types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	light_client "github.com/OffchainLabs/prysm/v6/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

const ErrNotEnoughSyncCommitteeBits = "sync committee bits count is less than required"

func NewLightClientFinalityUpdateFromBeaconState(
	ctx context.Context,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
	attestedState state.BeaconState,
	attestedBlock interfaces.ReadOnlySignedBeaconBlock,
	finalizedBlock interfaces.ReadOnlySignedBeaconBlock) (interfaces.LightClientFinalityUpdate, error) {
	update, err := NewLightClientUpdateFromBeaconState(ctx, state, block, attestedState, attestedBlock, finalizedBlock)
	if err != nil {
		return nil, err
	}

	return light_client.NewFinalityUpdateFromUpdate(update)
}

func NewLightClientOptimisticUpdateFromBeaconState(
	ctx context.Context,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
	attestedState state.BeaconState,
	attestedBlock interfaces.ReadOnlySignedBeaconBlock) (interfaces.LightClientOptimisticUpdate, error) {
	update, err := NewLightClientUpdateFromBeaconState(ctx, state, block, attestedState, attestedBlock, nil)
	if err != nil {
		return nil, err
	}

	return light_client.NewOptimisticUpdateFromUpdate(update)
}

// To form a LightClientUpdate, the following historical states and blocks are needed:
//   - state: the post state of any block with a post-Altair parent block
//   - block: the corresponding block
//   - attested_state: the post state of attested_block
//   - attested_block: the block referred to by block.parent_root
//   - finalized_block: the block referred to by attested_state.finalized_checkpoint.root,
//     if locally available (may be unavailable, e.g., when using checkpoint sync, or if it was pruned locally)
func NewLightClientUpdateFromBeaconState(
	ctx context.Context,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
	attestedState state.BeaconState,
	attestedBlock interfaces.ReadOnlySignedBeaconBlock,
	finalizedBlock interfaces.ReadOnlySignedBeaconBlock) (interfaces.LightClientUpdate, error) {
	// assert compute_epoch_at_slot(attested_state.slot) >= ALTAIR_FORK_EPOCH
	attestedEpoch := slots.ToEpoch(attestedState.Slot())
	if attestedEpoch < params.BeaconConfig().AltairForkEpoch {
		return nil, fmt.Errorf("invalid attested epoch %d", attestedEpoch)
	}

	// assert sum(block.message.body.sync_aggregate.sync_committee_bits) >= MIN_SYNC_COMMITTEE_PARTICIPANTS
	syncAggregate, err := block.Block().Body().SyncAggregate()
	if err != nil {
		return nil, errors.Wrap(err, "could not get sync aggregate")
	}
	if syncAggregate.SyncCommitteeBits.Count() < params.BeaconConfig().MinSyncCommitteeParticipants {
		return nil, fmt.Errorf(
			"%s (got %d, need %d)",
			ErrNotEnoughSyncCommitteeBits,
			syncAggregate.SyncCommitteeBits.Count(),
			params.BeaconConfig().MinSyncCommitteeParticipants,
		)
	}

	// assert state.slot == state.latest_block_header.slot
	if state.Slot() != state.LatestBlockHeader().Slot {
		return nil, fmt.Errorf("state slot %d not equal to latest block header slot %d", state.Slot(), state.LatestBlockHeader().Slot)
	}

	// assert hash_tree_root(header) == hash_tree_root(block.message)
	header := state.LatestBlockHeader()
	stateRoot, err := state.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get state root")
	}
	header.StateRoot = stateRoot[:]
	headerRoot, err := header.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get header root")
	}
	blockRoot, err := block.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get block root")
	}
	if headerRoot != blockRoot {
		return nil, fmt.Errorf("header root %#x not equal to block root %#x", headerRoot, blockRoot)
	}

	// update_signature_period = compute_sync_committee_period(compute_epoch_at_slot(block.message.slot))
	updateSignaturePeriod := slots.SyncCommitteePeriod(slots.ToEpoch(block.Block().Slot()))

	// assert attested_state.slot == attested_state.latest_block_header.slot
	if attestedState.Slot() != attestedState.LatestBlockHeader().Slot {
		return nil, fmt.Errorf(
			"attested state slot %d not equal to attested latest block header slot %d",
			attestedState.Slot(),
			attestedState.LatestBlockHeader().Slot,
		)
	}

	// attested_header = attested_state.latest_block_header.copy()
	attestedHeader := attestedState.LatestBlockHeader()

	// attested_header.state_root = hash_tree_root(attested_state)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get attested state root")
	}
	attestedHeader.StateRoot = attestedStateRoot[:]

	// assert hash_tree_root(attested_header) == block.message.parent_root
	attestedHeaderRoot, err := attestedHeader.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get attested header root")
	}
	attestedBlockRoot, err := attestedBlock.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get attested block root")
	}
	// assert hash_tree_root(attested_header) == hash_tree_root(attested_block.message) == block.message.parent_root
	if attestedHeaderRoot != block.Block().ParentRoot() || attestedHeaderRoot != attestedBlockRoot {
		return nil, fmt.Errorf(
			"attested header root %#x not equal to block parent root %#x or attested block root %#x",
			attestedHeaderRoot,
			block.Block().ParentRoot(),
			attestedBlockRoot,
		)
	}

	// update_attested_period = compute_sync_committee_period_at_slot(attested_block.message.slot)
	updateAttestedPeriod := slots.SyncCommitteePeriod(slots.ToEpoch(attestedBlock.Block().Slot()))

	// update = LightClientUpdate()
	result, err := CreateDefaultLightClientUpdate(attestedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "could not create default light client update")
	}

	// update.attested_header = block_to_light_client_header(attested_block)
	attestedLightClientHeader, err := BlockToLightClientHeader(ctx, attestedBlock.Version(), attestedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "could not get attested light client header")
	}
	if err = result.SetAttestedHeader(attestedLightClientHeader); err != nil {
		return nil, errors.Wrap(err, "could not set attested header")
	}

	// if update_attested_period == update_signature_period
	if updateAttestedPeriod == updateSignaturePeriod {
		// update.next_sync_committee = attested_state.next_sync_committee
		tempNextSyncCommittee, err := attestedState.NextSyncCommittee()
		if err != nil {
			return nil, errors.Wrap(err, "could not get next sync committee")
		}
		nextSyncCommittee := &pb.SyncCommittee{
			Pubkeys:         tempNextSyncCommittee.Pubkeys,
			AggregatePubkey: tempNextSyncCommittee.AggregatePubkey,
		}
		result.SetNextSyncCommittee(nextSyncCommittee)

		// update.next_sync_committee_branch = NextSyncCommitteeBranch(
		//     compute_merkle_proof(attested_state, next_sync_committee_gindex_at_slot(attested_state.slot)))
		nextSyncCommitteeBranch, err := attestedState.NextSyncCommitteeProof(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not get next sync committee proof")
		}
		if attestedBlock.Version() >= version.Electra {
			if err = result.SetNextSyncCommitteeBranch(nextSyncCommitteeBranch); err != nil {
				return nil, errors.Wrap(err, "could not set next sync committee branch")
			}
		} else if err = result.SetNextSyncCommitteeBranch(nextSyncCommitteeBranch); err != nil {
			return nil, errors.Wrap(err, "could not set next sync committee branch")
		}
	}

	// if finalized_block is not None
	if finalizedBlock != nil && !finalizedBlock.IsNil() {
		// if finalized_block.message.slot != GENESIS_SLOT
		if finalizedBlock.Block().Slot() != 0 {
			// update.finalized_header = block_to_light_client_header(finalized_block)
			finalizedLightClientHeader, err := BlockToLightClientHeader(ctx, attestedBlock.Version(), finalizedBlock)
			if err != nil {
				return nil, errors.Wrap(err, "could not get finalized light client header")
			}
			if err = result.SetFinalizedHeader(finalizedLightClientHeader); err != nil {
				return nil, errors.Wrap(err, "could not set finalized header")
			}
		} else {
			// assert attested_state.finalized_checkpoint.root == Bytes32()
			if !bytes.Equal(attestedState.FinalizedCheckpoint().Root, make([]byte, 32)) {
				return nil, fmt.Errorf("invalid finalized header root %v", attestedState.FinalizedCheckpoint().Root)
			}
		}

		// update.finality_branch = FinalityBranch(
		//     compute_merkle_proof(attested_state, finalized_root_gindex_at_slot(attested_state.slot)))
		finalityBranch, err := attestedState.FinalizedRootProof(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not get finalized root proof")
		}
		if err = result.SetFinalityBranch(finalityBranch); err != nil {
			return nil, errors.Wrap(err, "could not set finality branch")
		}
	}

	// update.sync_aggregate = block.message.body.sync_aggregate
	result.SetSyncAggregate(&pb.SyncAggregate{
		SyncCommitteeBits:      syncAggregate.SyncCommitteeBits,
		SyncCommitteeSignature: syncAggregate.SyncCommitteeSignature,
	})

	// update.signature_slot = block.message.slot
	result.SetSignatureSlot(block.Block().Slot())

	return result, nil
}

func CreateDefaultLightClientUpdate(attestedBlock interfaces.ReadOnlySignedBeaconBlock) (interfaces.LightClientUpdate, error) {
	syncCommitteeSize := params.BeaconConfig().SyncCommitteeSize
	pubKeys := make([][]byte, syncCommitteeSize)
	for i := uint64(0); i < syncCommitteeSize; i++ {
		pubKeys[i] = make([]byte, fieldparams.BLSPubkeyLength)
	}
	nextSyncCommittee := &pb.SyncCommittee{
		Pubkeys:         pubKeys,
		AggregatePubkey: make([]byte, fieldparams.BLSPubkeyLength),
	}

	var nextSyncCommitteeBranch [][]byte
	if attestedBlock.Version() >= version.Electra {
		nextSyncCommitteeBranch = make([][]byte, fieldparams.SyncCommitteeBranchDepthElectra)
	} else {
		nextSyncCommitteeBranch = make([][]byte, fieldparams.SyncCommitteeBranchDepth)
	}
	for i := 0; i < len(nextSyncCommitteeBranch); i++ {
		nextSyncCommitteeBranch[i] = make([]byte, fieldparams.RootLength)
	}

	executionBranch := make([][]byte, fieldparams.ExecutionBranchDepth)
	for i := 0; i < fieldparams.ExecutionBranchDepth; i++ {
		executionBranch[i] = make([]byte, 32)
	}

	var finalityBranch [][]byte
	if attestedBlock.Version() >= version.Electra {
		finalityBranch = make([][]byte, fieldparams.FinalityBranchDepthElectra)
	} else {
		finalityBranch = make([][]byte, fieldparams.FinalityBranchDepth)
	}
	for i := 0; i < len(finalityBranch); i++ {
		finalityBranch[i] = make([]byte, 32)
	}

	var m proto.Message
	switch attestedBlock.Version() {
	case version.Altair, version.Bellatrix:
		m = &pb.LightClientUpdateAltair{
			AttestedHeader: &pb.LightClientHeaderAltair{
				Beacon: &pb.BeaconBlockHeader{
					Slot:       attestedBlock.Block().Slot(),
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
				},
			},
			NextSyncCommittee:       nextSyncCommittee,
			NextSyncCommitteeBranch: nextSyncCommitteeBranch,
			FinalityBranch:          finalityBranch,
			FinalizedHeader: &pb.LightClientHeaderAltair{
				Beacon: &pb.BeaconBlockHeader{
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
				},
			},
			SyncAggregate: &pb.SyncAggregate{
				SyncCommitteeBits:      make([]byte, 64),
				SyncCommitteeSignature: make([]byte, 96),
			},
		}
	case version.Capella:
		m = &pb.LightClientUpdateCapella{
			AttestedHeader: &pb.LightClientHeaderCapella{
				Beacon: &pb.BeaconBlockHeader{
					Slot:       attestedBlock.Block().Slot(),
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
				},
				Execution: &enginev1.ExecutionPayloadHeaderCapella{
					ParentHash:       make([]byte, fieldparams.RootLength),
					FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
					StateRoot:        make([]byte, fieldparams.RootLength),
					ReceiptsRoot:     make([]byte, fieldparams.RootLength),
					LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
					PrevRandao:       make([]byte, fieldparams.RootLength),
					ExtraData:        make([]byte, 0),
					BaseFeePerGas:    make([]byte, fieldparams.RootLength),
					BlockHash:        make([]byte, fieldparams.RootLength),
					TransactionsRoot: make([]byte, fieldparams.RootLength),
					WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
				},
				ExecutionBranch: executionBranch,
			},
			NextSyncCommittee:       nextSyncCommittee,
			NextSyncCommitteeBranch: nextSyncCommitteeBranch,
			FinalityBranch:          finalityBranch,
			FinalizedHeader: &pb.LightClientHeaderCapella{
				Beacon: &pb.BeaconBlockHeader{
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
				},
				Execution: &enginev1.ExecutionPayloadHeaderCapella{
					ParentHash:       make([]byte, fieldparams.RootLength),
					FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
					StateRoot:        make([]byte, fieldparams.RootLength),
					ReceiptsRoot:     make([]byte, fieldparams.RootLength),
					LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
					PrevRandao:       make([]byte, fieldparams.RootLength),
					ExtraData:        make([]byte, 0),
					BaseFeePerGas:    make([]byte, fieldparams.RootLength),
					BlockHash:        make([]byte, fieldparams.RootLength),
					TransactionsRoot: make([]byte, fieldparams.RootLength),
					WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
				},
				ExecutionBranch: executionBranch,
			},
			SyncAggregate: &pb.SyncAggregate{
				SyncCommitteeBits:      make([]byte, 64),
				SyncCommitteeSignature: make([]byte, 96),
			},
		}
	case version.Deneb:
		m = &pb.LightClientUpdateDeneb{
			AttestedHeader: &pb.LightClientHeaderDeneb{
				Beacon: &pb.BeaconBlockHeader{
					Slot:       attestedBlock.Block().Slot(),
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
				},
				Execution: &enginev1.ExecutionPayloadHeaderDeneb{
					ParentHash:       make([]byte, fieldparams.RootLength),
					FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
					StateRoot:        make([]byte, fieldparams.RootLength),
					ReceiptsRoot:     make([]byte, fieldparams.RootLength),
					LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
					PrevRandao:       make([]byte, fieldparams.RootLength),
					ExtraData:        make([]byte, 0),
					BaseFeePerGas:    make([]byte, fieldparams.RootLength),
					BlockHash:        make([]byte, fieldparams.RootLength),
					TransactionsRoot: make([]byte, fieldparams.RootLength),
					WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
					GasLimit:         0,
					GasUsed:          0,
				},
				ExecutionBranch: executionBranch,
			},
			NextSyncCommittee:       nextSyncCommittee,
			NextSyncCommitteeBranch: nextSyncCommitteeBranch,
			FinalityBranch:          finalityBranch,
			FinalizedHeader: &pb.LightClientHeaderDeneb{
				Beacon: &pb.BeaconBlockHeader{
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
				},
				Execution: &enginev1.ExecutionPayloadHeaderDeneb{
					ParentHash:       make([]byte, fieldparams.RootLength),
					FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
					StateRoot:        make([]byte, fieldparams.RootLength),
					ReceiptsRoot:     make([]byte, fieldparams.RootLength),
					LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
					PrevRandao:       make([]byte, fieldparams.RootLength),
					ExtraData:        make([]byte, 0),
					BaseFeePerGas:    make([]byte, fieldparams.RootLength),
					BlockHash:        make([]byte, fieldparams.RootLength),
					TransactionsRoot: make([]byte, fieldparams.RootLength),
					WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
					GasLimit:         0,
					GasUsed:          0,
				},
				ExecutionBranch: executionBranch,
			},
			SyncAggregate: &pb.SyncAggregate{
				SyncCommitteeBits:      make([]byte, 64),
				SyncCommitteeSignature: make([]byte, 96),
			},
		}
	case version.Electra, version.Fulu:
		m = &pb.LightClientUpdateElectra{
			AttestedHeader: &pb.LightClientHeaderDeneb{
				Beacon: &pb.BeaconBlockHeader{
					Slot:       attestedBlock.Block().Slot(),
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
				},
				Execution: &enginev1.ExecutionPayloadHeaderDeneb{
					ParentHash:       make([]byte, fieldparams.RootLength),
					FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
					StateRoot:        make([]byte, fieldparams.RootLength),
					ReceiptsRoot:     make([]byte, fieldparams.RootLength),
					LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
					PrevRandao:       make([]byte, fieldparams.RootLength),
					ExtraData:        make([]byte, 0),
					BaseFeePerGas:    make([]byte, fieldparams.RootLength),
					BlockHash:        make([]byte, fieldparams.RootLength),
					TransactionsRoot: make([]byte, fieldparams.RootLength),
					WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
					GasLimit:         0,
					GasUsed:          0,
				},
				ExecutionBranch: executionBranch,
			},
			NextSyncCommittee:       nextSyncCommittee,
			NextSyncCommitteeBranch: nextSyncCommitteeBranch,
			FinalityBranch:          finalityBranch,
			FinalizedHeader: &pb.LightClientHeaderDeneb{
				Beacon: &pb.BeaconBlockHeader{
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
				},
				Execution: &enginev1.ExecutionPayloadHeaderDeneb{
					ParentHash:       make([]byte, fieldparams.RootLength),
					FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
					StateRoot:        make([]byte, fieldparams.RootLength),
					ReceiptsRoot:     make([]byte, fieldparams.RootLength),
					LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
					PrevRandao:       make([]byte, fieldparams.RootLength),
					ExtraData:        make([]byte, 0),
					BaseFeePerGas:    make([]byte, fieldparams.RootLength),
					BlockHash:        make([]byte, fieldparams.RootLength),
					TransactionsRoot: make([]byte, fieldparams.RootLength),
					WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
					GasLimit:         0,
					GasUsed:          0,
				},
				ExecutionBranch: executionBranch,
			},
			SyncAggregate: &pb.SyncAggregate{
				SyncCommitteeBits:      make([]byte, 64),
				SyncCommitteeSignature: make([]byte, 96),
			},
		}
	default:
		return nil, errors.Errorf("unsupported beacon chain version %s", version.String(attestedBlock.Version()))
	}

	return light_client.NewWrappedUpdate(m)
}

func ComputeTransactionsRoot(payload interfaces.ExecutionData) ([]byte, error) {
	transactionsRoot, err := payload.TransactionsRoot()
	if errors.Is(err, consensus_types.ErrUnsupportedField) {
		transactions, err := payload.Transactions()
		if err != nil {
			return nil, errors.Wrap(err, "could not get transactions")
		}
		transactionsRootArray, err := ssz.TransactionsRoot(transactions)
		if err != nil {
			return nil, errors.Wrap(err, "could not get transactions root")
		}
		transactionsRoot = transactionsRootArray[:]
	} else if err != nil {
		return nil, errors.Wrap(err, "could not get transactions root")
	}
	return transactionsRoot, nil
}

func ComputeWithdrawalsRoot(payload interfaces.ExecutionData) ([]byte, error) {
	withdrawalsRoot, err := payload.WithdrawalsRoot()
	if errors.Is(err, consensus_types.ErrUnsupportedField) {
		withdrawals, err := payload.Withdrawals()
		if err != nil {
			return nil, errors.Wrap(err, "could not get withdrawals")
		}
		withdrawalsRootArray, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
		if err != nil {
			return nil, errors.Wrap(err, "could not get withdrawals root")
		}
		withdrawalsRoot = withdrawalsRootArray[:]
	} else if err != nil {
		return nil, errors.Wrap(err, "could not get withdrawals root")
	}
	return withdrawalsRoot, nil
}

func BlockToLightClientHeader(
	ctx context.Context,
	attestedBlockVersion int, // this is the version that the light client header should be in, based on the attested block.
	block interfaces.ReadOnlySignedBeaconBlock, // this block is either the attested block, or the finalized block. in case of the latter, we might need to upgrade it to the attested block's version.
) (interfaces.LightClientHeader, error) {
	if block.Version() > attestedBlockVersion {
		return nil, errors.Errorf("block version %s is greater than attested block version %s", version.String(block.Version()), version.String(attestedBlockVersion))
	}

	beacon, err := makeBeaconBlockHeader(block)
	if err != nil {
		return nil, errors.Wrap(err, "could not make beacon block header")
	}

	var m proto.Message
	switch attestedBlockVersion {
	case version.Altair, version.Bellatrix:
		m = &pb.LightClientHeaderAltair{
			Beacon: beacon,
		}
	case version.Capella:
		payloadHeader, payloadProof, err := makeExecutionAndProofCapella(ctx, block)
		if err != nil {
			return nil, errors.Wrap(err, "could not make execution payload header and proof")
		}
		m = &pb.LightClientHeaderCapella{
			Beacon:          beacon,
			Execution:       payloadHeader,
			ExecutionBranch: payloadProof,
		}
	case version.Deneb, version.Electra, version.Fulu:
		payloadHeader, payloadProof, err := makeExecutionAndProofDeneb(ctx, block)
		if err != nil {
			return nil, errors.Wrap(err, "could not make execution payload header and proof")
		}
		m = &pb.LightClientHeaderDeneb{
			Beacon:          beacon,
			Execution:       payloadHeader,
			ExecutionBranch: payloadProof,
		}
	default:
		return nil, fmt.Errorf("unsupported attested block version %s", version.String(attestedBlockVersion))
	}

	return light_client.NewWrappedHeader(m)
}

func HasRelevantSyncCommittee(update interfaces.LightClientUpdate) (bool, error) {
	if update.Version() >= version.Electra {
		branch, err := update.NextSyncCommitteeBranchElectra()
		if err != nil {
			return false, err
		}
		return !reflect.DeepEqual(branch, interfaces.LightClientSyncCommitteeBranchElectra{}), nil
	}
	branch, err := update.NextSyncCommitteeBranch()
	if err != nil {
		return false, err
	}
	return !reflect.DeepEqual(branch, interfaces.LightClientSyncCommitteeBranch{}), nil
}

func HasFinality(update interfaces.LightClientUpdate) (bool, error) {
	if update.Version() >= version.Electra {
		b, err := update.FinalityBranchElectra()
		if err != nil {
			return false, err
		}
		return !reflect.DeepEqual(b, interfaces.LightClientFinalityBranchElectra{}), nil
	}

	b, err := update.FinalityBranch()
	if err != nil {
		return false, err
	}
	return !reflect.DeepEqual(b, interfaces.LightClientFinalityBranch{}), nil
}

func IsBetterUpdate(newUpdate, oldUpdate interfaces.LightClientUpdate) (bool, error) {
	if oldUpdate == nil || oldUpdate.IsNil() {
		return true, nil
	}

	maxActiveParticipants := newUpdate.SyncAggregate().SyncCommitteeBits.Len()
	newNumActiveParticipants := newUpdate.SyncAggregate().SyncCommitteeBits.Count()
	oldNumActiveParticipants := oldUpdate.SyncAggregate().SyncCommitteeBits.Count()
	newHasSupermajority := newNumActiveParticipants*3 >= maxActiveParticipants*2
	oldHasSupermajority := oldNumActiveParticipants*3 >= maxActiveParticipants*2

	if newHasSupermajority != oldHasSupermajority {
		return newHasSupermajority, nil
	}
	if !newHasSupermajority && newNumActiveParticipants != oldNumActiveParticipants {
		return newNumActiveParticipants > oldNumActiveParticipants, nil
	}

	newUpdateAttestedHeaderBeacon := newUpdate.AttestedHeader().Beacon()
	oldUpdateAttestedHeaderBeacon := oldUpdate.AttestedHeader().Beacon()

	// Compare presence of relevant sync committee
	newHasRelevantSyncCommittee, err := HasRelevantSyncCommittee(newUpdate)
	if err != nil {
		return false, err
	}
	newHasRelevantSyncCommittee = newHasRelevantSyncCommittee &&
		(slots.SyncCommitteePeriod(slots.ToEpoch(newUpdateAttestedHeaderBeacon.Slot)) == slots.SyncCommitteePeriod(slots.ToEpoch(newUpdate.SignatureSlot())))
	oldHasRelevantSyncCommittee, err := HasRelevantSyncCommittee(oldUpdate)
	if err != nil {
		return false, err
	}
	oldHasRelevantSyncCommittee = oldHasRelevantSyncCommittee &&
		(slots.SyncCommitteePeriod(slots.ToEpoch(oldUpdateAttestedHeaderBeacon.Slot)) == slots.SyncCommitteePeriod(slots.ToEpoch(oldUpdate.SignatureSlot())))

	if newHasRelevantSyncCommittee != oldHasRelevantSyncCommittee {
		return newHasRelevantSyncCommittee, nil
	}

	// Compare indication of any finality
	newHasFinality, err := HasFinality(newUpdate)
	if err != nil {
		return false, err
	}
	oldHasFinality, err := HasFinality(oldUpdate)
	if err != nil {
		return false, err
	}
	if newHasFinality != oldHasFinality {
		return newHasFinality, nil
	}

	newUpdateFinalizedHeaderBeacon := newUpdate.FinalizedHeader().Beacon()
	oldUpdateFinalizedHeaderBeacon := oldUpdate.FinalizedHeader().Beacon()

	// Compare sync committee finality
	if newHasFinality {
		newHasSyncCommitteeFinality :=
			slots.SyncCommitteePeriod(slots.ToEpoch(newUpdateFinalizedHeaderBeacon.Slot)) ==
				slots.SyncCommitteePeriod(slots.ToEpoch(newUpdateAttestedHeaderBeacon.Slot))
		oldHasSyncCommitteeFinality :=
			slots.SyncCommitteePeriod(slots.ToEpoch(oldUpdateFinalizedHeaderBeacon.Slot)) ==
				slots.SyncCommitteePeriod(slots.ToEpoch(oldUpdateAttestedHeaderBeacon.Slot))

		if newHasSyncCommitteeFinality != oldHasSyncCommitteeFinality {
			return newHasSyncCommitteeFinality, nil
		}
	}

	// Tiebreaker 1: Sync committee participation beyond supermajority
	if newNumActiveParticipants != oldNumActiveParticipants {
		return newNumActiveParticipants > oldNumActiveParticipants, nil
	}

	// Tiebreaker 2: Prefer older data (fewer changes to best)
	if newUpdateAttestedHeaderBeacon.Slot != oldUpdateAttestedHeaderBeacon.Slot {
		return newUpdateAttestedHeaderBeacon.Slot < oldUpdateAttestedHeaderBeacon.Slot, nil
	}

	return newUpdate.SignatureSlot() < oldUpdate.SignatureSlot(), nil
}

func NewLightClientBootstrapFromBeaconState(
	ctx context.Context,
	currentSlot primitives.Slot,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
) (interfaces.LightClientBootstrap, error) {
	// assert compute_epoch_at_slot(state.slot) >= ALTAIR_FORK_EPOCH
	if slots.ToEpoch(state.Slot()) < params.BeaconConfig().AltairForkEpoch {
		return nil, fmt.Errorf("light client bootstrap is not supported before Altair, invalid slot %d", state.Slot())
	}

	// assert state.slot == state.latest_block_header.slot
	latestBlockHeader := state.LatestBlockHeader()
	if state.Slot() != latestBlockHeader.Slot {
		return nil, fmt.Errorf("state slot %d not equal to latest block header slot %d", state.Slot(), latestBlockHeader.Slot)
	}

	// header.state_root = hash_tree_root(state)
	stateRoot, err := state.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get state root")
	}
	latestBlockHeader.StateRoot = stateRoot[:]

	// assert hash_tree_root(header) == hash_tree_root(block.message)
	latestBlockHeaderRoot, err := latestBlockHeader.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get latest block header root")
	}
	beaconBlockRoot, err := block.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get block root")
	}
	if latestBlockHeaderRoot != beaconBlockRoot {
		return nil, fmt.Errorf("latest block header root %#x not equal to block root %#x", latestBlockHeaderRoot, beaconBlockRoot)
	}

	bootstrap, err := createDefaultLightClientBootstrap(currentSlot)
	if err != nil {
		return nil, errors.Wrap(err, "could not create default light client bootstrap")
	}

	lightClientHeader, err := BlockToLightClientHeader(ctx, state.Version(), block)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert block to light client header")
	}

	err = bootstrap.SetHeader(lightClientHeader)
	if err != nil {
		return nil, errors.Wrap(err, "could not set header")
	}

	currentSyncCommittee, err := state.CurrentSyncCommittee()
	if err != nil {
		return nil, errors.Wrap(err, "could not get current sync committee")
	}

	err = bootstrap.SetCurrentSyncCommittee(currentSyncCommittee)
	if err != nil {
		return nil, errors.Wrap(err, "could not set current sync committee")
	}

	currentSyncCommitteeProof, err := state.CurrentSyncCommitteeProof(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get current sync committee proof")
	}

	err = bootstrap.SetCurrentSyncCommitteeBranch(currentSyncCommitteeProof)
	if err != nil {
		return nil, errors.Wrap(err, "could not set current sync committee proof")
	}

	return bootstrap, nil
}

func UpdateHasSupermajority(syncAggregate *pb.SyncAggregate) bool {
	maxActiveParticipants := syncAggregate.SyncCommitteeBits.Len()
	numActiveParticipants := syncAggregate.SyncCommitteeBits.Count()
	return numActiveParticipants*3 >= maxActiveParticipants*2
}

// IsFinalityUpdateValidForBroadcast checks if a finality update needs to be broadcasted.
// It is also used to check if an incoming gossiped finality update is valid for forwarding and saving.
func IsFinalityUpdateValidForBroadcast(newUpdate, oldUpdate interfaces.LightClientFinalityUpdate) bool {
	if oldUpdate == nil {
		return true
	}
	// The finalized_header.beacon.slot is greater than that of all previously forwarded finality_updates,
	// or it matches the highest previously forwarded slot and also has a sync_aggregate indicating supermajority (> 2/3)
	// sync committee participation while the previously forwarded finality_update for that slot did not indicate supermajority
	newUpdateSlot := newUpdate.FinalizedHeader().Beacon().Slot
	newHasSupermajority := UpdateHasSupermajority(newUpdate.SyncAggregate())

	lastUpdateSlot := oldUpdate.FinalizedHeader().Beacon().Slot
	lastHasSupermajority := UpdateHasSupermajority(oldUpdate.SyncAggregate())

	if newUpdateSlot < lastUpdateSlot {
		return false
	}
	if newUpdateSlot == lastUpdateSlot && (lastHasSupermajority || !newHasSupermajority) {
		return false
	}
	return true
}

// IsBetterFinalityUpdate checks if the new finality update is better than the old one for saving.
// This does not concern broadcasting, but rather the decision of whether to save the new update.
// For broadcasting checks, use IsFinalityUpdateValidForBroadcast.
func IsBetterFinalityUpdate(newUpdate, oldUpdate interfaces.LightClientFinalityUpdate) bool {
	if oldUpdate == nil || oldUpdate.IsNil() {
		return true
	}

	// Full nodes SHOULD provide the LightClientFinalityUpdate with the highest attested_header.beacon.slot (if multiple, highest signature_slot)
	newFinalizedSlot := newUpdate.FinalizedHeader().Beacon().Slot
	newAttestedSlot := newUpdate.AttestedHeader().Beacon().Slot

	oldFinalizedSlot := oldUpdate.FinalizedHeader().Beacon().Slot
	oldAttestedSlot := oldUpdate.AttestedHeader().Beacon().Slot

	if newFinalizedSlot < oldFinalizedSlot {
		return false
	}
	if newFinalizedSlot == oldFinalizedSlot {
		if newAttestedSlot < oldAttestedSlot {
			return false
		}
		if newAttestedSlot == oldAttestedSlot && newUpdate.SignatureSlot() <= oldUpdate.SignatureSlot() {
			return false
		}
	}
	return true
}

func IsBetterOptimisticUpdate(newUpdate, oldUpdate interfaces.LightClientOptimisticUpdate) bool {
	if oldUpdate == nil || oldUpdate.IsNil() {
		return true
	}
	// The attested_header.beacon.slot is greater than that of all previously forwarded optimistic updates
	return newUpdate.AttestedHeader().Beacon().Slot > oldUpdate.AttestedHeader().Beacon().Slot
}
