package light_client_test

import (
	"reflect"
	"testing"

	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	lightClient "github.com/OffchainLabs/prysm/v7/beacon-chain/light-client"
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
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/protobuf/proto"
)

// Altair starts after genesis, so the Altair fork epoch has a block before it.
var backfillForkEpochs = map[int]primitives.Epoch{
	version.Altair:    2,
	version.Bellatrix: 6,
	version.Capella:   10,
	version.Deneb:     14,
	version.Electra:   18,
	version.Fulu:      22,
	version.Gloas:     26,
}

// Written out, not taken from the code under test.
const (
	syncAggregateGindex      = 24
	syncAggregateGindexGloas = 355
)

func setupBackfillForks(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = backfillForkEpochs[version.Altair]
	cfg.BellatrixForkEpoch = backfillForkEpochs[version.Bellatrix]
	cfg.CapellaForkEpoch = backfillForkEpochs[version.Capella]
	cfg.DenebForkEpoch = backfillForkEpochs[version.Deneb]
	cfg.ElectraForkEpoch = backfillForkEpochs[version.Electra]
	cfg.FuluForkEpoch = backfillForkEpochs[version.Fulu]
	cfg.GloasForkEpoch = backfillForkEpochs[version.Gloas]
	params.OverrideBeaconConfig(cfg)
}

func epochStart(t *testing.T, epoch primitives.Epoch) primitives.Slot {
	start, err := slots.EpochStart(epoch)
	require.NoError(t, err)
	return start
}

// chainBlock is a block and its post-state.
type chainBlock struct {
	blk interfaces.ReadOnlySignedBeaconBlock
	st  state.BeaconState
}

func (c chainBlock) slot() primitives.Slot {
	return c.blk.Block().Slot()
}

func (c chainBlock) root(t *testing.T) [32]byte {
	root, err := c.blk.Block().HashTreeRoot()
	require.NoError(t, err)
	return root
}

func (c chainBlock) bodyRoot(t *testing.T) [32]byte {
	root, err := c.blk.Block().Body().HashTreeRoot()
	require.NoError(t, err)
	return root
}

func (c chainBlock) stateRoot(t *testing.T) [32]byte {
	root, err := c.st.HashTreeRoot(t.Context())
	require.NoError(t, err)
	return root
}

func (c chainBlock) finalizedRootProof(t *testing.T) *lightClient.FinalizedRootProof {
	e, err := lightClient.NewFinalizedRootProofFromBeaconState(t.Context(), c.st)
	require.NoError(t, err)
	return e
}

func (c chainBlock) currentSyncCommitteeProof(t *testing.T) *lightClient.CurrentSyncCommitteeProof {
	e, err := lightClient.NewCurrentSyncCommitteeProofFromBeaconState(t.Context(), c.st)
	require.NoError(t, err)
	return e
}

// newChain builds linked blocks at the given slots, each with its own post-state and the fork
// of its slot. The first block links to parentRoot.
func newChain(t *testing.T, parentRoot [32]byte, slotList ...primitives.Slot) []chainBlock {
	chain := make([]chainBlock, len(slotList))
	for i, slot := range slotList {
		v := slots.ToForkVersion(slot)
		seed := byte(slot + 1)
		st := newChainState(t, v, slot, seed)
		stateRoot, err := st.HashTreeRoot(t.Context())
		require.NoError(t, err)
		blk := newChainBlock(t, v, slot, parentRoot, stateRoot, seed)
		chain[i] = chainBlock{blk: blk, st: st}
		parentRoot = chain[i].root(t)
	}
	return chain
}

func newChainState(t *testing.T, v int, slot primitives.Slot, seed byte) state.BeaconState {
	var st state.BeaconState
	var err error
	switch v {
	case version.Phase0:
		st, err = util.NewBeaconState()
	case version.Altair:
		st, err = util.NewBeaconStateAltair()
	case version.Bellatrix:
		st, err = util.NewBeaconStateBellatrix()
	case version.Capella:
		st, err = util.NewBeaconStateCapella()
	case version.Deneb:
		st, err = util.NewBeaconStateDeneb()
	case version.Electra:
		st, err = util.NewBeaconStateElectra()
	case version.Fulu:
		st, err = util.NewBeaconStateFulu()
	case version.Gloas:
		st, err = util.NewBeaconStateGloas()
	default:
		t.Fatalf("no state fixture for %s", version.String(v))
	}
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(slot))
	require.NoError(t, st.SetFinalizedCheckpoint(&pb.Checkpoint{
		Epoch: slots.ToEpoch(slot),
		Root:  bytesutil.PadTo([]byte{seed, 0xf1}, fieldparams.RootLength),
	}))
	if v >= version.Altair {
		pubkeys := make([][]byte, fieldparams.SyncCommitteeLength)
		for i := range pubkeys {
			pubkeys[i] = bytesutil.PadTo([]byte{seed, byte(i)}, fieldparams.BLSPubkeyLength)
		}
		require.NoError(t, st.SetCurrentSyncCommittee(&pb.SyncCommittee{
			Pubkeys:         pubkeys,
			AggregatePubkey: bytesutil.PadTo([]byte{seed}, fieldparams.BLSPubkeyLength),
		}))
	}
	return st
}

func newChainBlock(t *testing.T, v int, slot primitives.Slot, parentRoot, stateRoot [32]byte, seed byte) interfaces.ReadOnlySignedBeaconBlock {
	var m proto.Message
	bits := make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	bits[seed%fieldparams.SyncAggregateSyncCommitteeBytesLength] = 1
	signature := bytesutil.PadTo([]byte{seed, 0x51}, fieldparams.BLSSignatureLength)
	aggregate := &pb.SyncAggregate{SyncCommitteeBits: bits, SyncCommitteeSignature: signature}
	blockHash := bytesutil.PadTo([]byte{seed, 0xe1}, fieldparams.RootLength)

	switch v {
	case version.Phase0:
		b := util.NewBeaconBlock()
		b.Block.Slot, b.Block.ProposerIndex = slot, primitives.ValidatorIndex(seed)
		b.Block.ParentRoot, b.Block.StateRoot = parentRoot[:], stateRoot[:]
		m = b
	case version.Altair:
		b := util.NewBeaconBlockAltair()
		b.Block.Slot, b.Block.ProposerIndex = slot, primitives.ValidatorIndex(seed)
		b.Block.ParentRoot, b.Block.StateRoot = parentRoot[:], stateRoot[:]
		b.Block.Body.SyncAggregate = aggregate
		m = b
	case version.Bellatrix:
		b := util.NewBeaconBlockBellatrix()
		b.Block.Slot, b.Block.ProposerIndex = slot, primitives.ValidatorIndex(seed)
		b.Block.ParentRoot, b.Block.StateRoot = parentRoot[:], stateRoot[:]
		b.Block.Body.SyncAggregate = aggregate
		m = b
	case version.Capella:
		b := util.NewBeaconBlockCapella()
		b.Block.Slot, b.Block.ProposerIndex = slot, primitives.ValidatorIndex(seed)
		b.Block.ParentRoot, b.Block.StateRoot = parentRoot[:], stateRoot[:]
		b.Block.Body.SyncAggregate = aggregate
		b.Block.Body.ExecutionPayload.BlockHash = blockHash
		m = b
	case version.Deneb:
		b := util.NewBeaconBlockDeneb()
		b.Block.Slot, b.Block.ProposerIndex = slot, primitives.ValidatorIndex(seed)
		b.Block.ParentRoot, b.Block.StateRoot = parentRoot[:], stateRoot[:]
		b.Block.Body.SyncAggregate = aggregate
		b.Block.Body.ExecutionPayload.BlockHash = blockHash
		m = b
	case version.Electra:
		b := util.NewBeaconBlockElectra()
		b.Block.Slot, b.Block.ProposerIndex = slot, primitives.ValidatorIndex(seed)
		b.Block.ParentRoot, b.Block.StateRoot = parentRoot[:], stateRoot[:]
		b.Block.Body.SyncAggregate = aggregate
		b.Block.Body.ExecutionPayload.BlockHash = blockHash
		m = b
	case version.Fulu:
		b := util.NewBeaconBlockFulu()
		b.Block.Slot, b.Block.ProposerIndex = slot, primitives.ValidatorIndex(seed)
		b.Block.ParentRoot, b.Block.StateRoot = parentRoot[:], stateRoot[:]
		b.Block.Body.SyncAggregate = aggregate
		b.Block.Body.ExecutionPayload.BlockHash = blockHash
		m = b
	case version.Gloas:
		b := util.NewBeaconBlockGloas()
		b.Block.Slot, b.Block.ProposerIndex = slot, primitives.ValidatorIndex(seed)
		b.Block.ParentRoot, b.Block.StateRoot = parentRoot[:], stateRoot[:]
		b.Block.Body.SyncAggregate = aggregate
		b.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockHash = blockHash
		m = b
	default:
		t.Fatalf("no block fixture for %s", version.String(v))
	}
	blk, err := blocks.NewSignedBeaconBlock(m)
	require.NoError(t, err)
	return blk
}

func blocksOf(chain []chainBlock) []interfaces.ReadOnlySignedBeaconBlock {
	out := make([]interfaces.ReadOnlySignedBeaconBlock, len(chain))
	for i, c := range chain {
		out[i] = c.blk
	}
	return out
}

// lightClientEpochData is the getters every LightClientEpochData type has.
type lightClientEpochData interface {
	light_client.EpochData
	GetParentBlockHeader() *pb.BeaconBlockHeader
	GetFinalizedRoot() []byte
	GetFinalityBranch() [][]byte
}

// lightClientBootstrapData is the getters every LightClientBootstrapData type has.
type lightClientBootstrapData interface {
	GetCurrentSyncCommittee() []*pb.SyncCommittee
	GetCurrentSyncCommitteeBranch() [][]byte
}

func epochDataOf(t *testing.T, rec light_client.EpochData) lightClientEpochData {
	e, ok := rec.(lightClientEpochData)
	require.Equal(t, true, ok, "unexpected record type %T", rec)
	return e
}

// blockDataOf returns block_data as the pre-Gloas type; the Gloas type has the same fields.
func blockDataOf(t *testing.T, rec light_client.EpochData) []*pb.LightClientBlockData {
	switch r := rec.(type) {
	case *pb.LightClientEpochDataAltair:
		return r.BlockData
	case *pb.LightClientEpochDataCapella:
		return r.BlockData
	case *pb.LightClientEpochDataDeneb:
		return r.BlockData
	case *pb.LightClientEpochDataElectra:
		return r.BlockData
	case *pb.LightClientEpochDataGloas:
		out := make([]*pb.LightClientBlockData, len(r.BlockData))
		for i, d := range r.BlockData {
			out[i] = &pb.LightClientBlockData{
				ProposerIndex:              d.ProposerIndex,
				StateRoot:                  d.StateRoot,
				SyncCommitteeBits:          d.SyncCommitteeBits,
				SyncCommitteeSignatureRoot: d.SyncCommitteeSignatureRoot,
				SyncAggregateBranch:        d.SyncAggregateBranch,
			}
		}
		return out
	default:
		t.Fatalf("unexpected record type %T", rec)
		return nil
	}
}

func bootstrapDataOf(t *testing.T, rec light_client.EpochData) lightClientBootstrapData {
	switch r := rec.(type) {
	case *pb.LightClientEpochDataAltair:
		return r.BootstrapData
	case *pb.LightClientEpochDataCapella:
		return r.BootstrapData
	case *pb.LightClientEpochDataDeneb:
		return r.BootstrapData
	case *pb.LightClientEpochDataElectra:
		return r.BootstrapData
	case *pb.LightClientEpochDataGloas:
		return r.BootstrapData
	default:
		t.Fatalf("unexpected record type %T", rec)
		return nil
	}
}

// executionOf returns the execution block hash and execution_branch of bootstrap_data: the
// payload header's block hash, or the bid's parent block hash in Gloas. Altair has neither.
func executionOf(t *testing.T, rec light_client.EpochData) ([]byte, [][]byte) {
	switch r := rec.(type) {
	case *pb.LightClientEpochDataAltair:
		return nil, nil
	case *pb.LightClientEpochDataCapella:
		return r.BootstrapData.Execution.BlockHash, r.BootstrapData.ExecutionBranch
	case *pb.LightClientEpochDataDeneb:
		return r.BootstrapData.Execution.BlockHash, r.BootstrapData.ExecutionBranch
	case *pb.LightClientEpochDataElectra:
		return r.BootstrapData.Execution.BlockHash, r.BootstrapData.ExecutionBranch
	case *pb.LightClientEpochDataGloas:
		return r.BootstrapData.ExecutionBlockHash, r.BootstrapData.ExecutionBranch
	default:
		t.Fatalf("unexpected record type %T", rec)
		return nil, nil
	}
}

func recordType(v int) light_client.EpochData {
	switch v {
	case version.Altair, version.Bellatrix:
		return &pb.LightClientEpochDataAltair{}
	case version.Capella:
		return &pb.LightClientEpochDataCapella{}
	case version.Deneb:
		return &pb.LightClientEpochDataDeneb{}
	case version.Electra, version.Fulu:
		return &pb.LightClientEpochDataElectra{}
	default:
		return &pb.LightClientEpochDataGloas{}
	}
}

func zeroBranch(depth int) [][]byte {
	out := make([][]byte, depth)
	for i := range out {
		out[i] = make([]byte, fieldparams.RootLength)
	}
	return out
}

func expectedHeader(t *testing.T, c chainBlock) *pb.BeaconBlockHeader {
	parentRoot := c.blk.Block().ParentRoot()
	stateRoot := c.blk.Block().StateRoot()
	bodyRoot := c.bodyRoot(t)
	return &pb.BeaconBlockHeader{
		Slot:          c.slot(),
		ProposerIndex: c.blk.Block().ProposerIndex(),
		ParentRoot:    parentRoot[:],
		StateRoot:     stateRoot[:],
		BodyRoot:      bodyRoot[:],
	}
}

// checkBlockData checks that got describes c, with zeros padding nodes before the branch.
func checkBlockData(t *testing.T, got *pb.LightClientBlockData, c chainBlock, zeros int) {
	aggregate, err := c.blk.Block().Body().SyncAggregate()
	require.NoError(t, err)
	signatureRoot, err := ssz.MerkleizeByteSliceSSZ(aggregate.SyncCommitteeSignature)
	require.NoError(t, err)
	aggregateRoot, err := aggregate.HashTreeRoot()
	require.NoError(t, err)
	stateRoot := c.blk.Block().StateRoot()

	require.Equal(t, c.blk.Block().ProposerIndex(), got.ProposerIndex)
	require.DeepEqual(t, stateRoot[:], got.StateRoot)
	require.DeepEqual(t, aggregate.SyncCommitteeBits, got.SyncCommitteeBits)
	require.DeepEqual(t, signatureRoot[:], got.SyncCommitteeSignatureRoot)

	gindex := uint64(syncAggregateGindex)
	if c.blk.Version() >= version.Gloas {
		gindex = syncAggregateGindexGloas
	}
	require.DeepEqual(t, zeroBranch(zeros), got.SyncAggregateBranch[:zeros])
	bodyRoot := c.bodyRoot(t)
	require.Equal(t, true, trie.VerifyMerkleProof(bodyRoot[:], aggregateRoot[:], gindex, got.SyncAggregateBranch[zeros:]))
}

func checkEmptyBlockData(t *testing.T, got *pb.LightClientBlockData, depth int) {
	require.Equal(t, primitives.ValidatorIndex(0), got.ProposerIndex)
	require.DeepEqual(t, make([]byte, fieldparams.RootLength), got.StateRoot)
	require.DeepEqual(t, make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength), []byte(got.SyncCommitteeBits))
	require.DeepEqual(t, make([]byte, fieldparams.RootLength), got.SyncCommitteeSignatureRoot)
	require.DeepEqual(t, zeroBranch(depth), got.SyncAggregateBranch)
}

// checkFinality checks that the finality fields come from c's post-state, with zeros padding
// nodes before the branch.
func checkFinality(t *testing.T, rec light_client.EpochData, c chainBlock, zeros int) {
	got := epochDataOf(t, rec)
	require.DeepEqual(t, c.st.FinalizedCheckpoint().Root, got.GetFinalizedRoot())
	gindex, err := statenative.FinalizedRootGeneralizedIndexForVersion(c.blk.Version())
	require.NoError(t, err)
	stateRoot := c.stateRoot(t)
	branch := got.GetFinalityBranch()
	require.DeepEqual(t, zeroBranch(zeros), branch[:zeros])
	require.Equal(t, true, trie.VerifyMerkleProof(stateRoot[:], got.GetFinalizedRoot(), gindex, branch[zeros:]))
}

// checkBootstrap checks that bootstrap_data comes from checkpoint block c.
func checkBootstrap(t *testing.T, rec light_client.EpochData, c chainBlock) {
	got := bootstrapDataOf(t, rec)
	require.Equal(t, 0, len(got.GetCurrentSyncCommittee()))
	committee, err := c.st.CurrentSyncCommittee()
	require.NoError(t, err)
	committeeRoot, err := committee.HashTreeRoot()
	require.NoError(t, err)
	gindex, err := statenative.CurrentSyncCommitteeGeneralizedIndexForVersion(c.blk.Version())
	require.NoError(t, err)
	stateRoot := c.stateRoot(t)
	require.Equal(t, true, trie.VerifyMerkleProof(stateRoot[:], committeeRoot[:], gindex, got.GetCurrentSyncCommitteeBranch()))

	executionHash, _ := executionOf(t, rec)
	switch v := c.blk.Version(); {
	case v >= version.Gloas:
		bid, err := c.blk.Block().Body().SignedExecutionPayloadBid()
		require.NoError(t, err)
		require.DeepEqual(t, bid.Message.ParentBlockHash, executionHash)
	case v >= version.Capella:
		payload, err := c.blk.Block().Body().Execution()
		require.NoError(t, err)
		require.DeepEqual(t, payload.BlockHash(), executionHash)
	}
}

func TestNewLightClientEpochData_EveryFork(t *testing.T) {
	setupBackfillForks(t)
	for v, forkEpoch := range backfillForkEpochs {
		t.Run(version.String(v), func(t *testing.T) {
			// Two epochs in, so the parent and the blocks are all in fork v.
			epoch := forkEpoch + 2
			start := epochStart(t, epoch)
			prevStart := start - fieldparams.SlotsPerEpoch
			slotList := make([]primitives.Slot, 0, fieldparams.SlotsPerEpoch+1)
			for s := prevStart; s <= start; s++ {
				slotList = append(slotList, s)
			}
			chain := newChain(t, [32]byte{1}, slotList...)
			parent, blks := chain[0], chain[1:]
			checkpointBlock := blks[len(blks)-1]

			rec, err := lightClient.NewLightClientEpochData(t.Context(), epoch, parent.blk, blocksOf(blks), parent.finalizedRootProof(t), checkpointBlock.currentSyncCommitteeProof(t))
			require.NoError(t, err)
			require.Equal(t, reflect.TypeOf(recordType(v)), reflect.TypeOf(rec))
			got, err := light_client.EpochDataVersion(rec)
			require.NoError(t, err)
			require.Equal(t, v, got)

			require.Equal(t, epoch, rec.GetEpoch())
			require.DeepEqual(t, expectedHeader(t, parent), epochDataOf(t, rec).GetParentBlockHeader())
			blockData := blockDataOf(t, rec)
			require.Equal(t, fieldparams.SlotsPerEpoch, len(blockData))
			for i, c := range blks {
				checkBlockData(t, blockData[i], c, 0)
			}
			checkBootstrap(t, rec, checkpointBlock)
			checkFinality(t, rec, parent, 0)
		})
	}
}

func TestNewLightClientEpochData_EmptySlots(t *testing.T) {
	setupBackfillForks(t)
	epoch := backfillForkEpochs[version.Deneb] + 2
	start := epochStart(t, epoch)
	prevStart := start - fieldparams.SlotsPerEpoch

	// Both start slots are empty.
	chain := newChain(t, [32]byte{1}, prevStart-7, prevStart+3, prevStart+10, start-1)
	parent, blks := chain[0], chain[1:]
	first, checkpointBlock := blks[0], blks[len(blks)-1]

	rec, err := lightClient.NewLightClientEpochData(t.Context(), epoch, parent.blk, blocksOf(blks), first.finalizedRootProof(t), checkpointBlock.currentSyncCommitteeProof(t))
	require.NoError(t, err)

	require.DeepEqual(t, expectedHeader(t, parent), epochDataOf(t, rec).GetParentBlockHeader())
	// block_data[i] is slot prevStart+1+i.
	filled := map[int]chainBlock{2: blks[0], 9: blks[1], 30: blks[2]}
	for i, bd := range blockDataOf(t, rec) {
		if c, ok := filled[i]; ok {
			checkBlockData(t, bd, c, 0)
		} else {
			checkEmptyBlockData(t, bd, fieldparams.SyncAggregateBranchDepth)
		}
	}
	checkBootstrap(t, rec, checkpointBlock)
	// The parent is before epoch-1, so finality comes from blks[0].
	checkFinality(t, rec, first, 0)
}

func TestNewLightClientEpochData_NoBlocks(t *testing.T) {
	setupBackfillForks(t)
	// No blocks at the Electra fork epoch: the record is Electra and the parent's Deneb finality
	// branch is padded.
	epoch := backfillForkEpochs[version.Electra]
	start := epochStart(t, epoch)
	parent := newChain(t, [32]byte{1}, start-fieldparams.SlotsPerEpoch)[0]

	rec, err := lightClient.NewLightClientEpochData(t.Context(), epoch, parent.blk, nil, parent.finalizedRootProof(t), nil)
	require.NoError(t, err)
	require.Equal(t, reflect.TypeOf(&pb.LightClientEpochDataElectra{}), reflect.TypeOf(rec))
	got, err := light_client.EpochDataVersion(rec)
	require.NoError(t, err)
	require.Equal(t, version.Electra, got)

	require.DeepEqual(t, expectedHeader(t, parent), epochDataOf(t, rec).GetParentBlockHeader())
	for _, bd := range blockDataOf(t, rec) {
		checkEmptyBlockData(t, bd, fieldparams.SyncAggregateBranchDepth)
	}
	bootstrap := bootstrapDataOf(t, rec)
	require.Equal(t, 0, len(bootstrap.GetCurrentSyncCommittee()))
	require.DeepEqual(t, zeroBranch(fieldparams.SyncCommitteeBranchDepthElectra), bootstrap.GetCurrentSyncCommitteeBranch())
	executionHash, executionBranch := executionOf(t, rec)
	require.DeepEqual(t, make([]byte, fieldparams.RootLength), executionHash)
	require.DeepEqual(t, zeroBranch(fieldparams.ExecutionBranchDepth), executionBranch)
	checkFinality(t, rec, parent, fieldparams.FinalityBranchDepthElectra-fieldparams.FinalityBranchDepth)

	_, err = lightClient.NewLightClientEpochData(t.Context(), epoch, parent.blk, nil, parent.finalizedRootProof(t), parent.currentSyncCommitteeProof(t))
	require.ErrorContains(t, "there are no blocks", err)
}

func TestNewLightClientEpochData_ForkBoundary(t *testing.T) {
	setupBackfillForks(t)
	tests := []struct {
		name               string
		fork               int
		syncAggregateZeros int
		finalityZeros      int
	}{
		{"Deneb to Electra", version.Electra, 0, fieldparams.FinalityBranchDepthElectra - fieldparams.FinalityBranchDepth},
		{"Fulu to Gloas", version.Gloas, 4, fieldparams.FinalityBranchDepthGloas - fieldparams.FinalityBranchDepthElectra},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			epoch := backfillForkEpochs[tt.fork]
			start := epochStart(t, epoch)
			prevStart := start - fieldparams.SlotsPerEpoch
			// The parent and blks[0] are in the old fork; the checkpoint block is the new fork's first.
			chain := newChain(t, [32]byte{1}, prevStart, prevStart+1, start)
			parent, blks := chain[0], chain[1:]
			checkpointBlock := blks[1]
			require.Equal(t, tt.fork, checkpointBlock.blk.Version())
			require.Equal(t, tt.fork-1, blks[0].blk.Version())

			rec, err := lightClient.NewLightClientEpochData(t.Context(), epoch, parent.blk, blocksOf(blks), parent.finalizedRootProof(t), checkpointBlock.currentSyncCommitteeProof(t))
			require.NoError(t, err)
			require.Equal(t, reflect.TypeOf(recordType(tt.fork)), reflect.TypeOf(rec))

			blockData := blockDataOf(t, rec)
			checkBlockData(t, blockData[0], blks[0], tt.syncAggregateZeros)
			checkBlockData(t, blockData[fieldparams.SlotsPerEpoch-1], checkpointBlock, 0)
			checkBootstrap(t, rec, checkpointBlock)
			checkFinality(t, rec, parent, tt.finalityZeros)
		})
	}
}

func TestNewLightClientEpochData_AltairForkEpoch(t *testing.T) {
	setupBackfillForks(t)
	epoch := backfillForkEpochs[version.Altair]
	start := epochStart(t, epoch)

	t.Run("after genesis", func(t *testing.T) {
		// The parent is the last Phase0 block; the only other block is at the start slot.
		chain := newChain(t, [32]byte{1}, start-1, start)
		parent, checkpointBlock := chain[0], chain[1]
		require.Equal(t, version.Phase0, parent.blk.Version())

		rec, err := lightClient.NewLightClientEpochData(t.Context(), epoch, parent.blk, blocksOf(chain[1:]), nil, checkpointBlock.currentSyncCommitteeProof(t))
		require.NoError(t, err)
		require.Equal(t, reflect.TypeOf(&pb.LightClientEpochDataAltair{}), reflect.TypeOf(rec))

		e := epochDataOf(t, rec)
		require.DeepEqual(t, expectedHeader(t, parent), e.GetParentBlockHeader())
		blockData := blockDataOf(t, rec)
		for _, bd := range blockData[:fieldparams.SlotsPerEpoch-1] {
			checkEmptyBlockData(t, bd, fieldparams.SyncAggregateBranchDepth)
		}
		checkBlockData(t, blockData[fieldparams.SlotsPerEpoch-1], checkpointBlock, 0)
		checkBootstrap(t, rec, checkpointBlock)
		require.DeepEqual(t, make([]byte, fieldparams.RootLength), e.GetFinalizedRoot())
		require.DeepEqual(t, zeroBranch(fieldparams.FinalityBranchDepth), e.GetFinalityBranch())

		_, err = lightClient.NewLightClientEpochData(t.Context(), epoch, parent.blk, blocksOf(chain[1:]), parent.finalizedRootProof(t), checkpointBlock.currentSyncCommitteeProof(t))
		require.ErrorContains(t, "no block in the previous epoch", err)

		earlier := newChain(t, [32]byte{1}, start-2, start-1, start)
		_, err = lightClient.NewLightClientEpochData(t.Context(), epoch, earlier[0].blk, blocksOf(earlier[1:]), nil, earlier[2].currentSyncCommitteeProof(t))
		require.ErrorContains(t, "only the start slot", err)
	})

	t.Run("at genesis", func(t *testing.T) {
		cfg := params.BeaconConfig().Copy()
		cfg.AltairForkEpoch = 0
		params.OverrideBeaconConfig(cfg)
		genesis := newChain(t, [32]byte{}, 0)[0]

		rec, err := lightClient.NewLightClientEpochData(t.Context(), 0, nil, blocksOf([]chainBlock{genesis}), nil, genesis.currentSyncCommitteeProof(t))
		require.NoError(t, err)
		require.DeepEqual(t, &pb.BeaconBlockHeader{
			ParentRoot: make([]byte, fieldparams.RootLength),
			StateRoot:  make([]byte, fieldparams.RootLength),
			BodyRoot:   make([]byte, fieldparams.RootLength),
		}, epochDataOf(t, rec).GetParentBlockHeader())
		checkBlockData(t, blockDataOf(t, rec)[fieldparams.SlotsPerEpoch-1], genesis, 0)

		_, err = lightClient.NewLightClientEpochData(t.Context(), 0, genesis.blk, nil, nil, nil)
		require.ErrorContains(t, "must be nil at genesis", err)
	})
}

func TestNewLightClientEpochData_Rejects(t *testing.T) {
	setupBackfillForks(t)
	epoch := backfillForkEpochs[version.Electra] + 2
	start := epochStart(t, epoch)
	prevStart := start - fieldparams.SlotsPerEpoch
	chain := newChain(t, [32]byte{1}, prevStart, prevStart+1, prevStart+2, start)
	parent, blks := chain[0], chain[1:]
	checkpointBlock := blks[len(blks)-1]
	finalityProof, committeeProof := parent.finalizedRootProof(t), checkpointBlock.currentSyncCommitteeProof(t)

	// A block whose type does not match its slot's fork.
	denebAtElectraSlot := newChainBlock(t, version.Deneb, prevStart+1, parent.root(t), blks[0].stateRoot(t), 9)

	wrongFinalityProof := *finalityProof
	wrongFinalityProof.Root[0] ^= 1
	wrongCommitteeProof := *committeeProof
	wrongCommitteeProof.CommitteeRoot[0] ^= 1

	type input struct {
		epoch          primitives.Epoch
		parent         interfaces.ReadOnlySignedBeaconBlock
		blks           []interfaces.ReadOnlySignedBeaconBlock
		finalityProof  *lightClient.FinalizedRootProof
		committeeProof *lightClient.CurrentSyncCommitteeProof
	}
	valid := input{epoch, parent.blk, blocksOf(blks), finalityProof, committeeProof}
	_, err := lightClient.NewLightClientEpochData(t.Context(), valid.epoch, valid.parent, valid.blks, valid.finalityProof, valid.committeeProof)
	require.NoError(t, err)

	tests := []struct {
		name string
		in   func(in input) input
		want string
	}{
		{"epoch before Altair", func(in input) input { in.epoch = 1; return in }, "before the Altair fork epoch"},
		{"nil parent", func(in input) input { in.parent = nil; return in }, "parent block is required"},
		{"parent after the previous start slot", func(in input) input { in.parent = blks[0].blk; return in }, "is after slot"},
		{"block after the start slot", func(in input) input {
			in.blks = append(in.blks, newChain(t, checkpointBlock.root(t), start+1)[0].blk)
			return in
		}, "after the start slot"},
		{"block at the previous start slot", func(in input) input { in.blks = blocksOf(chain); return in }, "at or before the start slot of the previous epoch"},
		{"blocks not ascending", func(in input) input {
			in.blks = []interfaces.ReadOnlySignedBeaconBlock{blks[0].blk, blks[0].blk}
			return in
		}, "not ascending"},
		{"chain break", func(in input) input { in.blks = in.blks[1:]; return in }, "does not link"},
		{"block type does not match its slot", func(in input) input {
			in.blks = []interfaces.ReadOnlySignedBeaconBlock{denebAtElectraSlot}
			return in
		}, "is deneb, want electra"},
		{"finalized root proof missing", func(in input) input { in.finalityProof = nil; return in }, "finalized root proof from the post-state of slot"},
		{"finalized root proof from the wrong slot", func(in input) input { in.finalityProof = blks[0].finalizedRootProof(t); return in }, "finalized root proof is from slot"},
		{"finalized root proof does not verify", func(in input) input { in.finalityProof = &wrongFinalityProof; return in }, "finalized root proof does not verify"},
		{"committee proof missing", func(in input) input { in.committeeProof = nil; return in }, "current sync committee proof from the post-state of slot"},
		{"committee proof from the wrong slot", func(in input) input { in.committeeProof = blks[0].currentSyncCommitteeProof(t); return in }, "current sync committee proof is from slot"},
		{"committee proof does not verify", func(in input) input { in.committeeProof = &wrongCommitteeProof; return in }, "current sync committee proof does not verify"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in(valid)
			_, err := lightClient.NewLightClientEpochData(t.Context(), in.epoch, in.parent, in.blks, in.finalityProof, in.committeeProof)
			require.ErrorContains(t, tt.want, err)
		})
	}
}

// The store accepts what the builder returns.
func TestNewLightClientEpochData_StoreRoundTrip(t *testing.T) {
	setupBackfillForks(t)
	epoch := backfillForkEpochs[version.Gloas] + 2
	start := epochStart(t, epoch)
	chain := newChain(t, [32]byte{1}, start-fieldparams.SlotsPerEpoch, start-5, start)
	parent, blks := chain[0], chain[1:]

	rec, err := lightClient.NewLightClientEpochData(t.Context(), epoch, parent.blk, blocksOf(blks), parent.finalizedRootProof(t), blks[1].currentSyncCommitteeProof(t))
	require.NoError(t, err)

	db := dbtest.SetupDB(t)
	require.NoError(t, db.SaveLightClientEpochData(t.Context(), []light_client.EpochData{rec}))
	stored, err := db.LightClientEpochData(t.Context(), epoch)
	require.NoError(t, err)
	require.Equal(t, true, proto.Equal(rec, stored))
}
