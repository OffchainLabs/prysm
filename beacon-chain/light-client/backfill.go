package light_client

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	statenative "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	light_client "github.com/OffchainLabs/prysm/v7/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// FinalizedRootProof is the finalized checkpoint root and its branch, read from the post-state
// of the block at Slot.
type FinalizedRootProof struct {
	Slot   primitives.Slot
	Root   [fieldparams.RootLength]byte
	Branch [][]byte
}

// CurrentSyncCommitteeProof is the current sync committee root and its branch, read from the
// post-state of the block at Slot. Only the root is kept: the committee is stored per period.
type CurrentSyncCommitteeProof struct {
	Slot          primitives.Slot
	CommitteeRoot [fieldparams.RootLength]byte
	Branch        [][]byte
}

// NewFinalizedRootProofFromBeaconState reads a FinalizedRootProof from st.
func NewFinalizedRootProofFromBeaconState(ctx context.Context, st state.BeaconState) (*FinalizedRootProof, error) {
	if st == nil || st.IsNil() {
		return nil, errors.New("nil state")
	}
	checkpoint := st.FinalizedCheckpoint()
	if checkpoint == nil || len(checkpoint.Root) != fieldparams.RootLength {
		return nil, errors.New("invalid finalized checkpoint")
	}
	branch, err := st.FinalizedRootProof(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get finalized root proof")
	}
	return &FinalizedRootProof{
		Slot:   st.Slot(),
		Root:   bytesutil.ToBytes32(checkpoint.Root),
		Branch: bytesutil.SafeCopy2dBytes(branch),
	}, nil
}

// NewCurrentSyncCommitteeProofFromBeaconState reads a CurrentSyncCommitteeProof from st.
func NewCurrentSyncCommitteeProofFromBeaconState(ctx context.Context, st state.BeaconState) (*CurrentSyncCommitteeProof, error) {
	if st == nil || st.IsNil() {
		return nil, errors.New("nil state")
	}
	committee, err := st.CurrentSyncCommittee()
	if err != nil {
		return nil, errors.Wrap(err, "could not get current sync committee")
	}
	if committee == nil {
		return nil, errors.New("nil current sync committee in state")
	}
	root, err := committee.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not hash current sync committee")
	}
	branch, err := st.CurrentSyncCommitteeProof(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get current sync committee proof")
	}
	return &CurrentSyncCommitteeProof{
		Slot:          st.Slot(),
		CommitteeRoot: root,
		Branch:        bytesutil.SafeCopy2dBytes(branch),
	}, nil
}

// NewLightClientEpochData builds the record for epoch.
//
//   - parent: latest block at or before start(epoch-1). At the Altair fork epoch, latest block
//     before start(epoch), or nil at genesis.
//   - blks: every block in (start(epoch-1), start(epoch)], ascending.
//   - finalityProof: from the first block in epoch-1, nil if there is none.
//   - committeeProof: from the last block in blks, nil if blks is empty.
//
// Each proof must verify against its block's state root. current_sync_committee is left empty;
// the caller stores the committee per period.
func NewLightClientEpochData(
	ctx context.Context,
	epoch primitives.Epoch,
	parent interfaces.ReadOnlySignedBeaconBlock,
	blks []interfaces.ReadOnlySignedBeaconBlock,
	finalityProof *FinalizedRootProof,
	committeeProof *CurrentSyncCommitteeProof,
) (light_client.EpochData, error) {
	cfg := params.BeaconConfig()
	if epoch < cfg.AltairForkEpoch {
		return nil, fmt.Errorf("epoch %d is before the Altair fork epoch %d", epoch, cfg.AltairForkEpoch)
	}
	start, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, err
	}
	isAltairForkEpoch := epoch == cfg.AltairForkEpoch

	parentHeader, parentRoot, err := makeParentBlockHeader(epoch, start, isAltairForkEpoch, parent)
	if err != nil {
		return nil, err
	}
	if err := validateBlocks(start, isAltairForkEpoch, parentRoot, blks); err != nil {
		return nil, err
	}

	v := slots.ToForkVersion(start)
	var checkpointBlock interfaces.ReadOnlySignedBeaconBlock
	if len(blks) > 0 {
		checkpointBlock = blks[len(blks)-1]
		v = checkpointBlock.Version()
	}
	syncAggregateDepth, syncCommitteeDepth, finalityDepth := branchDepths(v)

	blockData, err := makeBlockData(ctx, start, blks, syncAggregateDepth)
	if err != nil {
		return nil, err
	}

	finalizedRoot, finalityBranch, err := makeFinalizedRootAndProof(start, isAltairForkEpoch, parent, blks, finalityProof, finalityDepth)
	if err != nil {
		return nil, err
	}

	committeeBranch, err := makeCurrentSyncCommitteeBranch(checkpointBlock, committeeProof, syncCommitteeDepth)
	if err != nil {
		return nil, err
	}

	switch v {
	case version.Altair, version.Bellatrix:
		return &pb.LightClientEpochDataAltair{
			Epoch:             epoch,
			ParentBlockHeader: parentHeader,
			BlockData:         blockData,
			BootstrapData:     &pb.LightClientBootstrapDataAltair{CurrentSyncCommitteeBranch: committeeBranch},
			FinalizedRoot:     finalizedRoot,
			FinalityBranch:    finalityBranch,
		}, nil
	case version.Capella:
		header, branch, err := makeExecutionAndProofCapellaOrDefault(ctx, checkpointBlock)
		if err != nil {
			return nil, err
		}
		return &pb.LightClientEpochDataCapella{
			Epoch:             epoch,
			ParentBlockHeader: parentHeader,
			BlockData:         blockData,
			BootstrapData: &pb.LightClientBootstrapDataCapella{
				CurrentSyncCommitteeBranch: committeeBranch,
				Execution:                  header,
				ExecutionBranch:            branch,
			},
			FinalizedRoot:  finalizedRoot,
			FinalityBranch: finalityBranch,
		}, nil
	case version.Deneb:
		header, branch, err := makeExecutionAndProofDenebOrDefault(ctx, checkpointBlock)
		if err != nil {
			return nil, err
		}
		return &pb.LightClientEpochDataDeneb{
			Epoch:             epoch,
			ParentBlockHeader: parentHeader,
			BlockData:         blockData,
			BootstrapData: &pb.LightClientBootstrapDataDeneb{
				CurrentSyncCommitteeBranch: committeeBranch,
				Execution:                  header,
				ExecutionBranch:            branch,
			},
			FinalizedRoot:  finalizedRoot,
			FinalityBranch: finalityBranch,
		}, nil
	case version.Electra, version.Fulu:
		header, branch, err := makeExecutionAndProofDenebOrDefault(ctx, checkpointBlock)
		if err != nil {
			return nil, err
		}
		return &pb.LightClientEpochDataElectra{
			Epoch:             epoch,
			ParentBlockHeader: parentHeader,
			BlockData:         blockData,
			BootstrapData: &pb.LightClientBootstrapDataElectra{
				CurrentSyncCommitteeBranch: committeeBranch,
				Execution:                  header,
				ExecutionBranch:            branch,
			},
			FinalizedRoot:  finalizedRoot,
			FinalityBranch: finalityBranch,
		}, nil
	case version.Gloas:
		blockHash, branch, err := makeExecutionAndProofGloasOrDefault(ctx, checkpointBlock)
		if err != nil {
			return nil, err
		}
		return &pb.LightClientEpochDataGloas{
			Epoch:             epoch,
			ParentBlockHeader: parentHeader,
			BlockData:         toBlockDataGloas(blockData),
			BootstrapData: &pb.LightClientBootstrapDataGloas{
				CurrentSyncCommitteeBranch: committeeBranch,
				ExecutionBlockHash:         blockHash,
				ExecutionBranch:            branch,
			},
			FinalizedRoot:  finalizedRoot,
			FinalityBranch: finalityBranch,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported fork %s for light client epoch data", version.String(v))
	}
}

// makeParentBlockHeader returns parent_block_header and the root blks[0] must link to.
func makeParentBlockHeader(
	epoch primitives.Epoch,
	start primitives.Slot,
	isAltairForkEpoch bool,
	parent interfaces.ReadOnlySignedBeaconBlock,
) (*pb.BeaconBlockHeader, [fieldparams.RootLength]byte, error) {
	if parent == nil {
		if !isAltairForkEpoch || epoch != params.BeaconConfig().GenesisEpoch {
			return nil, [32]byte{}, errors.New("parent block is required")
		}
		// Altair fork epoch at genesis: there is no block before it.
		header := &pb.BeaconBlockHeader{
			ParentRoot: make([]byte, fieldparams.RootLength),
			StateRoot:  make([]byte, fieldparams.RootLength),
			BodyRoot:   make([]byte, fieldparams.RootLength),
		}
		return header, [32]byte{}, nil
	}
	if err := blocks.BeaconBlockIsNil(parent); err != nil {
		return nil, [32]byte{}, errors.Wrap(err, "parent block")
	}

	// At most start(epoch-1), or start(epoch)-1 at the Altair fork epoch.
	var latest primitives.Slot
	if isAltairForkEpoch {
		if start == 0 {
			return nil, [32]byte{}, errors.New("parent block must be nil at genesis")
		}
		latest = start - 1
	} else {
		latest = start - fieldparams.SlotsPerEpoch
	}
	if parent.Block().Slot() > latest {
		return nil, [32]byte{}, fmt.Errorf("parent block slot %d is after slot %d", parent.Block().Slot(), latest)
	}

	header, err := makeBeaconBlockHeader(parent)
	if err != nil {
		return nil, [32]byte{}, errors.Wrap(err, "could not make parent block header")
	}
	root, err := parent.Block().HashTreeRoot()
	if err != nil {
		return nil, [32]byte{}, errors.Wrap(err, "could not get parent block root")
	}
	return header, root, nil
}

// validateBlocks checks that blks is an ascending chain linked to parentRoot, inside
// (start(epoch-1), start], with each block of the fork its slot says.
func validateBlocks(
	start primitives.Slot,
	isAltairForkEpoch bool,
	parentRoot [fieldparams.RootLength]byte,
	blks []interfaces.ReadOnlySignedBeaconBlock,
) error {
	for i, blk := range blks {
		if err := blocks.BeaconBlockIsNil(blk); err != nil {
			return errors.Wrapf(err, "block %d", i)
		}
		slot := blk.Block().Slot()
		if slot > start {
			return fmt.Errorf("block at slot %d is after the start slot %d", slot, start)
		}
		if start-slot >= fieldparams.SlotsPerEpoch {
			return fmt.Errorf("block at slot %d is at or before the start slot of the previous epoch", slot)
		}
		if isAltairForkEpoch && slot != start {
			return fmt.Errorf("block at slot %d: only the start slot %d is allowed at the Altair fork epoch", slot, start)
		}
		if i > 0 && slot <= blks[i-1].Block().Slot() {
			return fmt.Errorf("blocks are not ascending at slot %d", slot)
		}
		if want := slots.ToForkVersion(slot); blk.Version() != want {
			return fmt.Errorf("block at slot %d is %s, want %s", slot, version.String(blk.Version()), version.String(want))
		}
		if blk.Block().ParentRoot() != parentRoot {
			return fmt.Errorf("block at slot %d does not link to the previous block", slot)
		}
		root, err := blk.Block().HashTreeRoot()
		if err != nil {
			return errors.Wrapf(err, "could not get root of block at slot %d", slot)
		}
		parentRoot = root
	}
	return nil
}

// branchDepths returns the branch lengths of fork v's record type.
func branchDepths(v int) (syncAggregate, syncCommittee, finality int) {
	switch {
	case v >= version.Gloas:
		return fieldparams.SyncAggregateBranchDepthGloas, fieldparams.SyncCommitteeBranchDepthGloas, fieldparams.FinalityBranchDepthGloas
	case v >= version.Electra:
		return fieldparams.SyncAggregateBranchDepth, fieldparams.SyncCommitteeBranchDepthElectra, fieldparams.FinalityBranchDepthElectra
	default:
		return fieldparams.SyncAggregateBranchDepth, fieldparams.SyncCommitteeBranchDepth, fieldparams.FinalityBranchDepth
	}
}

// normalizeMerkleBranch is the spec's normalize_merkle_branch: it pads an older fork's branch
// with leading zero nodes to depth.
func normalizeMerkleBranch(name string, branch [][]byte, depth int) ([][]byte, error) {
	if len(branch) > depth {
		return nil, fmt.Errorf("%s branch has %d nodes, more than the %d of the record's fork", name, len(branch), depth)
	}
	out := make([][]byte, depth)
	extra := depth - len(branch)
	for i := range extra {
		out[i] = make([]byte, fieldparams.RootLength)
	}
	for i, node := range branch {
		if len(node) != fieldparams.RootLength {
			return nil, fmt.Errorf("%s branch node %d has length %d, want %d", name, i, len(node), fieldparams.RootLength)
		}
		out[extra+i] = bytesutil.SafeCopyBytes(node)
	}
	return out, nil
}

func zeroBranch(depth int) [][]byte {
	out := make([][]byte, depth)
	for i := range out {
		out[i] = make([]byte, fieldparams.RootLength)
	}
	return out
}

// makeBlockData returns block_data. Entry i is slot start-SLOTS_PER_EPOCH+1+i; empty slots stay
// default.
func makeBlockData(
	ctx context.Context,
	start primitives.Slot,
	blks []interfaces.ReadOnlySignedBeaconBlock,
	depth int,
) ([]*pb.LightClientBlockData, error) {
	data := make([]*pb.LightClientBlockData, fieldparams.SlotsPerEpoch)
	for i := range data {
		data[i] = &pb.LightClientBlockData{
			StateRoot:                  make([]byte, fieldparams.RootLength),
			SyncCommitteeBits:          make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignatureRoot: make([]byte, fieldparams.RootLength),
			SyncAggregateBranch:        zeroBranch(depth),
		}
	}
	for _, blk := range blks {
		slot := blk.Block().Slot()
		i := fieldparams.SlotsPerEpoch - 1 - int(start-slot)

		aggregate, err := blk.Block().Body().SyncAggregate()
		if err != nil {
			return nil, errors.Wrapf(err, "could not get sync aggregate of block at slot %d", slot)
		}
		if aggregate == nil || len(aggregate.SyncCommitteeBits) != fieldparams.SyncAggregateSyncCommitteeBytesLength ||
			len(aggregate.SyncCommitteeSignature) != fieldparams.BLSSignatureLength {
			return nil, fmt.Errorf("block at slot %d has an invalid sync aggregate", slot)
		}
		signatureRoot, err := ssz.MerkleizeByteSliceSSZ(aggregate.SyncCommitteeSignature)
		if err != nil {
			return nil, errors.Wrapf(err, "could not hash sync committee signature of block at slot %d", slot)
		}
		proof, err := blocks.SyncAggregateProof(ctx, blk.Block())
		if err != nil {
			return nil, errors.Wrapf(err, "could not get sync aggregate proof of block at slot %d", slot)
		}
		branch, err := normalizeMerkleBranch("sync aggregate", proof, depth)
		if err != nil {
			return nil, err
		}
		stateRoot := blk.Block().StateRoot()
		data[i] = &pb.LightClientBlockData{
			ProposerIndex:              blk.Block().ProposerIndex(),
			StateRoot:                  stateRoot[:],
			SyncCommitteeBits:          bytesutil.SafeCopyBytes(aggregate.SyncCommitteeBits),
			SyncCommitteeSignatureRoot: signatureRoot[:],
			SyncAggregateBranch:        branch,
		}
	}
	return data, nil
}

// toBlockDataGloas converts block_data to the Gloas type. The fields are the same; only the
// branch depth differs, and makeBlockData already used the Gloas depth.
func toBlockDataGloas(data []*pb.LightClientBlockData) []*pb.LightClientBlockDataGloas {
	out := make([]*pb.LightClientBlockDataGloas, len(data))
	for i, d := range data {
		out[i] = &pb.LightClientBlockDataGloas{
			ProposerIndex:              d.ProposerIndex,
			StateRoot:                  d.StateRoot,
			SyncCommitteeBits:          d.SyncCommitteeBits,
			SyncCommitteeSignatureRoot: d.SyncCommitteeSignatureRoot,
			SyncAggregateBranch:        d.SyncAggregateBranch,
		}
	}
	return out
}

// makeFinalizedRootAndProof returns finalized_root and finality_branch from the first block in
// epoch-1, or defaults when there is none or epoch is the Altair fork epoch.
func makeFinalizedRootAndProof(
	start primitives.Slot,
	isAltairForkEpoch bool,
	parent interfaces.ReadOnlySignedBeaconBlock,
	blks []interfaces.ReadOnlySignedBeaconBlock,
	proof *FinalizedRootProof,
	depth int,
) ([]byte, [][]byte, error) {
	// parent is nil only when isAltairForkEpoch is true.
	var firstBlock interfaces.ReadOnlySignedBeaconBlock
	if !isAltairForkEpoch {
		if parent.Block().Slot() == start-fieldparams.SlotsPerEpoch {
			firstBlock = parent
		} else if len(blks) > 0 && blks[0].Block().Slot() < start {
			firstBlock = blks[0]
		}
	}
	if firstBlock == nil {
		if proof != nil {
			return nil, nil, errors.New("finalized root proof given, but no block in the previous epoch to take it from")
		}
		return make([]byte, fieldparams.RootLength), zeroBranch(depth), nil
	}

	slot := firstBlock.Block().Slot()
	if proof == nil {
		return nil, nil, fmt.Errorf("finalized root proof from the post-state of slot %d is required", slot)
	}
	if proof.Slot != slot {
		return nil, nil, fmt.Errorf("finalized root proof is from slot %d, want slot %d", proof.Slot, slot)
	}
	gindex, err := statenative.FinalizedRootGeneralizedIndexForVersion(firstBlock.Version())
	if err != nil {
		return nil, nil, err
	}
	stateRoot := firstBlock.Block().StateRoot()
	if !trie.VerifyMerkleProof(stateRoot[:], proof.Root[:], gindex, proof.Branch) {
		return nil, nil, fmt.Errorf("finalized root proof does not verify against the state root of slot %d", slot)
	}
	branch, err := normalizeMerkleBranch("finality", proof.Branch, depth)
	if err != nil {
		return nil, nil, err
	}
	return bytesutil.SafeCopyBytes(proof.Root[:]), branch, nil
}

// makeCurrentSyncCommitteeBranch returns current_sync_committee_branch of the checkpoint block,
// or a default branch when there is none.
func makeCurrentSyncCommitteeBranch(
	checkpointBlock interfaces.ReadOnlySignedBeaconBlock,
	proof *CurrentSyncCommitteeProof,
	depth int,
) ([][]byte, error) {
	if checkpointBlock == nil {
		if proof != nil {
			return nil, errors.New("current sync committee proof given, but there are no blocks")
		}
		return zeroBranch(depth), nil
	}

	slot := checkpointBlock.Block().Slot()
	if proof == nil {
		return nil, fmt.Errorf("current sync committee proof from the post-state of slot %d is required", slot)
	}
	if proof.Slot != slot {
		return nil, fmt.Errorf("current sync committee proof is from slot %d, want slot %d", proof.Slot, slot)
	}
	gindex, err := statenative.CurrentSyncCommitteeGeneralizedIndexForVersion(checkpointBlock.Version())
	if err != nil {
		return nil, err
	}
	stateRoot := checkpointBlock.Block().StateRoot()
	if !trie.VerifyMerkleProof(stateRoot[:], proof.CommitteeRoot[:], gindex, proof.Branch) {
		return nil, fmt.Errorf("current sync committee proof does not verify against the state root of slot %d", slot)
	}
	return normalizeMerkleBranch("sync committee", proof.Branch, depth)
}

// makeExecutionAndProofCapellaOrDefault is makeExecutionAndProofCapella, or the default header
// and branch when there is no checkpoint block.
func makeExecutionAndProofCapellaOrDefault(ctx context.Context, checkpointBlock interfaces.ReadOnlySignedBeaconBlock) (*enginev1.ExecutionPayloadHeaderCapella, [][]byte, error) {
	if checkpointBlock == nil {
		p, err := execution.EmptyExecutionPayloadHeader(version.Capella)
		if err != nil {
			return nil, nil, err
		}
		header, ok := p.(*enginev1.ExecutionPayloadHeaderCapella)
		if !ok {
			return nil, nil, fmt.Errorf("payload header type %T is not %T", p, header)
		}
		return header, emptyPayloadProof(), nil
	}
	return makeExecutionAndProofCapella(ctx, checkpointBlock)
}

// makeExecutionAndProofDenebOrDefault is makeExecutionAndProofDeneb, or the default header and
// branch when there is no checkpoint block.
func makeExecutionAndProofDenebOrDefault(ctx context.Context, checkpointBlock interfaces.ReadOnlySignedBeaconBlock) (*enginev1.ExecutionPayloadHeaderDeneb, [][]byte, error) {
	if checkpointBlock == nil {
		p, err := execution.EmptyExecutionPayloadHeader(version.Deneb)
		if err != nil {
			return nil, nil, err
		}
		header, ok := p.(*enginev1.ExecutionPayloadHeaderDeneb)
		if !ok {
			return nil, nil, fmt.Errorf("payload header type %T is not %T", p, header)
		}
		return header, emptyPayloadProof(), nil
	}
	return makeExecutionAndProofDeneb(ctx, checkpointBlock)
}

// makeExecutionAndProofGloasOrDefault returns the bid's parent block hash and its branch, or
// defaults when there is no checkpoint block.
func makeExecutionAndProofGloasOrDefault(ctx context.Context, checkpointBlock interfaces.ReadOnlySignedBeaconBlock) ([]byte, [][]byte, error) {
	if checkpointBlock == nil {
		return make([]byte, fieldparams.RootLength), zeroBranch(fieldparams.ExecutionBranchDepthGloas), nil
	}
	bid, err := checkpointBlock.Block().Body().SignedExecutionPayloadBid()
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get execution payload bid")
	}
	if bid == nil || bid.Message == nil || len(bid.Message.ParentBlockHash) != fieldparams.RootLength {
		return nil, nil, errors.New("invalid execution payload bid")
	}
	proof, err := blocks.ExecutionBlockHashProof(ctx, checkpointBlock.Block())
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get execution block hash proof")
	}
	return bytesutil.SafeCopyBytes(bid.Message.ParentBlockHash), proof, nil
}
