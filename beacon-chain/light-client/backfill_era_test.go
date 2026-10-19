package light_client_test

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	lightClient "github.com/OffchainLabs/prysm/v7/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	statenative "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/era"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/golang/snappy"
	"google.golang.org/protobuf/proto"
)

// eraFile builds an era file whose state is at stateSlot. The collector never reads the state of
// the block and next files, so callers may pass placeholder bytes for them.
func eraFile(t *testing.T, stateSlot primitives.Slot, blks []interfaces.ReadOnlySignedBeaconBlock, stateSSZ []byte) *era.Reader {
	record := func(b []byte, typ [2]byte, data []byte) []byte {
		b = append(b, typ[:]...)
		b = binary.LittleEndian.AppendUint32(b, uint32(len(data)))
		return append(append(b, 0, 0), data...)
	}
	compress := func(data []byte) []byte {
		var buf bytes.Buffer
		w := snappy.NewBufferedWriter(&buf)
		_, err := w.Write(data)
		require.NoError(t, err)
		require.NoError(t, w.Close())
		return buf.Bytes()
	}
	index := func(b []byte, start primitives.Slot, offsets []int) []byte {
		data := binary.LittleEndian.AppendUint64(nil, uint64(start))
		for _, off := range offsets {
			var rel int64
			if off != 0 {
				rel = int64(off - len(b))
			}
			data = binary.LittleEndian.AppendUint64(data, uint64(rel))
		}
		return record(b, [2]byte{0x69, 0x32}, binary.LittleEndian.AppendUint64(data, uint64(len(offsets))))
	}

	perEra := params.BeaconConfig().SlotsPerHistoricalRoot
	start := stateSlot - perEra
	b := record(nil, [2]byte{0x65, 0x32}, nil)
	offsets := make([]int, perEra)
	for _, blk := range blks {
		enc, err := blk.MarshalSSZ()
		require.NoError(t, err)
		offsets[blk.Block().Slot()-start] = len(b)
		b = record(b, [2]byte{0x01, 0x00}, compress(enc))
	}
	stateOffset := len(b)
	b = record(b, [2]byte{0x02, 0x00}, compress(stateSSZ))
	b = index(b, start, offsets)
	b = index(b, stateSlot, []int{stateOffset})

	r, err := era.NewReader(bytes.NewReader(b), int64(len(b)))
	require.NoError(t, err)
	return r
}

// eraChain is a short chain laid out as three era files.
type eraChain struct {
	startFile, blockFile, nextFile *era.Reader
	start                          primitives.Slot
	blks                           map[primitives.Slot]interfaces.ReadOnlySignedBeaconBlock
}

// newEraChain builds the chain. Era 0 holds one block at slot 1. Era 1 starts at slot S and holds
// blocks at the given ascending offsets from S; offset 0 is a block at S. The next era holds
// blocks at nextOffsets from its first slot. The chain starts in Altair when the Altair fork epoch
// is 0, else in Phase0.
func newEraChain(t *testing.T, offsets, nextOffsets []primitives.Slot) *eraChain {
	ctx := t.Context()
	cfg := params.BeaconConfig()
	S := cfg.SlotsPerHistoricalRoot

	var st state.BeaconState
	var keys []bls.SecretKey
	if cfg.AltairForkEpoch == 0 {
		st, keys = util.DeterministicGenesisStateAltair(t, 64)
		committee, err := altair.NextSyncCommittee(ctx, st)
		require.NoError(t, err)
		require.NoError(t, st.SetCurrentSyncCommittee(committee))
		// A next committee that differs from the current one, so a mix-up between them shows.
		next := proto.Clone(committee).(*pb.SyncCommittee)
		slices.Reverse(next.Pubkeys)
		require.NoError(t, st.SetNextSyncCommittee(next))
		require.NoError(t, st.SetFork(&pb.Fork{PreviousVersion: cfg.GenesisForkVersion, CurrentVersion: cfg.AltairForkVersion}))
	} else {
		st, keys = util.DeterministicGenesisState(t, 64)
	}

	gen := func(slot primitives.Slot) proto.Message {
		var m proto.Message
		var err error
		if slots.ToForkVersion(slot) == version.Phase0 {
			m, err = util.GenerateFullBlock(st, keys, &util.BlockGenConfig{}, slot)
		} else {
			m, err = util.GenerateFullBlockAltair(st, keys, &util.BlockGenConfig{}, slot)
		}
		require.NoError(t, err)
		return m
	}
	apply := func(slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
		blk, err := blocks.NewSignedBeaconBlock(gen(slot))
		require.NoError(t, err)
		_, st, err = transition.ExecuteStateTransitionNoVerifyAnySig(ctx, st, blk)
		require.NoError(t, err)
		return blk
	}

	// Jump to slot S-1 instead of processing 8190 slots. The block at slot 1 then needs the state
	// root that the next slot writes into its header, so the next block links to it.
	b1 := gen(1)
	blk1, err := blocks.NewSignedBeaconBlock(b1)
	require.NoError(t, err)
	_, st, err = transition.ExecuteStateTransitionNoVerifyAnySig(ctx, st, blk1)
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(S-1))
	root, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	switch b := b1.(type) {
	case *pb.SignedBeaconBlock:
		b.Block.StateRoot = root[:]
	case *pb.SignedBeaconBlockAltair:
		b.Block.StateRoot = root[:]
	}
	blk1, err = blocks.NewSignedBeaconBlock(b1)
	require.NoError(t, err)

	c := &eraChain{start: S, blks: map[primitives.Slot]interfaces.ReadOnlySignedBeaconBlock{}}
	var eraBlocks, nextBlocks []interfaces.ReadOnlySignedBeaconBlock
	var startSSZ []byte
	if len(offsets) > 0 && offsets[0] == 0 {
		blk := apply(S)
		c.blks[S] = blk
		eraBlocks = append(eraBlocks, blk)
		offsets = offsets[1:]
		startSSZ, err = st.MarshalSSZ()
	} else {
		var atS state.BeaconState
		atS, err = transition.ProcessSlots(ctx, st.Copy(), S)
		require.NoError(t, err)
		startSSZ, err = atS.MarshalSSZ()
	}
	require.NoError(t, err)
	for _, off := range offsets {
		blk := apply(S + off)
		c.blks[S+off] = blk
		eraBlocks = append(eraBlocks, blk)
	}
	for _, off := range nextOffsets {
		blk := apply(2*S + off)
		c.blks[2*S+off] = blk
		nextBlocks = append(nextBlocks, blk)
	}

	c.startFile = eraFile(t, S, []interfaces.ReadOnlySignedBeaconBlock{blk1}, startSSZ)
	c.blockFile = eraFile(t, 2*S, eraBlocks, []byte{0})
	c.nextFile = eraFile(t, 3*S, nextBlocks, []byte{0})
	return c
}

func setupEraConfig(t *testing.T, altairForkEpoch primitives.Epoch) {
	params.SetupTestConfigCleanup(t)
	// Start from mainnet, not the active config: other tests in this package leave changes in it.
	cfg := params.MainnetConfig().Copy()
	cfg.AltairForkEpoch = altairForkEpoch
	params.OverrideBeaconConfig(cfg)
}

// checkCommittee checks committee against rec's committee branch and the state root of its
// checkpoint block.
func checkCommittee(t *testing.T, committee *pb.SyncCommittee, rec *pb.LightClientEpochDataAltair, checkpoint interfaces.ReadOnlySignedBeaconBlock) {
	require.NotNil(t, committee)
	root, err := committee.HashTreeRoot()
	require.NoError(t, err)
	gindex, err := statenative.CurrentSyncCommitteeGeneralizedIndexForVersion(version.Altair)
	require.NoError(t, err)
	stateRoot := checkpoint.Block().StateRoot()
	require.Equal(t, true, trie.VerifyMerkleProof(stateRoot[:], root[:], gindex, rec.BootstrapData.CurrentSyncCommitteeBranch))
}

func TestNewLightClientEpochDataFromEra(t *testing.T) {
	setupEraConfig(t, 0)
	spe := params.BeaconConfig().SlotsPerEpoch
	// Era 1 is epochs 256 to 511. Blocks (as offsets from its first slot S, which is empty):
	//   3, 31   epoch 256: first block 3 is the finality source of record 257
	//   40      epoch 257: its start slot is empty
	//   64      the start slot of epoch 258: checkpoint of record 258, finality source of 259
	//           epoch 259 has no block
	//   130     epoch 260
	c := newEraChain(t, []primitives.Slot{3, 31, 40, 64, 130}, []primitives.Slot{0})
	S := c.start
	firstEpoch := primitives.Epoch(S/spe) + 1

	recs, committees, err := lightClient.NewLightClientEpochDataFromEra(t.Context(), c.startFile, c.blockFile, nil)
	require.NoError(t, err)
	require.Equal(t, 255, len(recs))
	for i, r := range recs {
		require.Equal(t, firstEpoch+primitives.Epoch(i), r.GetEpoch())
	}

	rec := func(epoch primitives.Epoch) *pb.LightClientEpochDataAltair {
		r, ok := recs[epoch-firstEpoch].(*pb.LightClientEpochDataAltair)
		require.Equal(t, true, ok)
		return r
	}
	// block_data[i] is slot start(epoch-1)+1+i.
	index := func(epoch primitives.Epoch, slot primitives.Slot) int {
		return int(slot - (primitives.Slot(epoch-1)*spe + 1))
	}
	filled := func(r *pb.LightClientEpochDataAltair) []int {
		var out []int
		for i, bd := range r.BlockData {
			if !bytes.Equal(bd.StateRoot, make([]byte, fieldparams.RootLength)) {
				out = append(out, i)
			}
		}
		return out
	}

	// The committee of period 1 comes from the checkpoint block of record 257.
	require.Equal(t, 1, len(committees))
	checkCommittee(t, committees[1], rec(257), c.blks[S+31])

	// Record 257: parent is the block at slot 1 in the start file, because slot S is empty.
	r := rec(257)
	require.Equal(t, primitives.Slot(1), r.ParentBlockHeader.Slot)
	require.DeepEqual(t, []int{index(257, S+3), index(257, S+31)}, filled(r))

	// Record 258: the block at its start slot is its checkpoint block.
	r = rec(258)
	require.Equal(t, S+31, r.ParentBlockHeader.Slot)
	require.DeepEqual(t, []int{index(258, S+40), index(258, S+64)}, filled(r))

	// Record 259: no blocks, and the parent sits at start(258), so it is the finality source.
	r = rec(259)
	require.Equal(t, S+64, r.ParentBlockHeader.Slot)
	require.Equal(t, 0, len(filled(r)))

	// Record 260: epoch 259 has no block, so there is no finality source.
	r = rec(260)
	require.DeepEqual(t, make([]byte, fieldparams.RootLength), r.FinalizedRoot)

	// Record 261 holds the last block of the era; later records are empty.
	require.DeepEqual(t, []int{index(261, S+130)}, filled(rec(261)))
	require.Equal(t, 0, len(filled(rec(511))))

	// With the next file, the record of the last epoch is built from its first block.
	recs, committees, err = lightClient.NewLightClientEpochDataFromEra(t.Context(), c.startFile, c.blockFile, c.nextFile)
	require.NoError(t, err)
	require.Equal(t, 256, len(recs))
	last, ok := recs[255].(*pb.LightClientEpochDataAltair)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.Epoch(512), last.Epoch)
	require.DeepEqual(t, []int{fieldparams.SlotsPerEpoch - 1}, filled(last))
	// The block at 2S starts period 2, after the committees rotate.
	require.Equal(t, 2, len(committees))
	checkCommittee(t, committees[2], last, c.blks[2*S])
}

func TestNewLightClientEpochDataFromEra_BlockAtS(t *testing.T) {
	setupEraConfig(t, 0)
	c := newEraChain(t, []primitives.Slot{0, 3}, nil)
	recs, _, err := lightClient.NewLightClientEpochDataFromEra(t.Context(), c.startFile, c.blockFile, nil)
	require.NoError(t, err)
	// The block at S is the parent of record 257 and the first block of epoch 256, so its
	// finality proof comes from the start file's state.
	r, ok := recs[0].(*pb.LightClientEpochDataAltair)
	require.Equal(t, true, ok)
	require.Equal(t, c.start, r.ParentBlockHeader.Slot)
	require.DeepNotEqual(t, zeroBranch(len(r.FinalityBranch)), r.FinalityBranch)
}

func TestNewLightClientEpochDataFromEra_CommitteePeriod(t *testing.T) {
	setupEraConfig(t, 0)
	// The next era has no block at its first slot, so the checkpoint block of record 512 is the
	// block at S+8191, in period 1.
	c := newEraChain(t, []primitives.Slot{3, params.BeaconConfig().SlotsPerHistoricalRoot - 1}, nil)
	recs, committees, err := lightClient.NewLightClientEpochDataFromEra(t.Context(), c.startFile, c.blockFile, c.nextFile)
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(512), recs[len(recs)-1].GetEpoch())
	require.Equal(t, 1, len(committees))
	require.NotNil(t, committees[1])
}

func TestNewLightClientEpochDataFromEra_AltairInsideEra(t *testing.T) {
	setupEraConfig(t, 260)
	spe := params.BeaconConfig().SlotsPerEpoch
	at := func(epoch primitives.Epoch, plus primitives.Slot) primitives.Slot {
		return primitives.Slot(epoch-256)*spe + plus
	}
	// Phase0 blocks before epoch 260, Altair blocks from its start slot on.
	c := newEraChain(t, []primitives.Slot{3, 40, at(259, 4), at(260, 0), at(260, 9), at(261, 0), at(262, 3)}, nil)
	recs, _, err := lightClient.NewLightClientEpochDataFromEra(t.Context(), c.startFile, c.blockFile, nil)
	require.NoError(t, err)
	// Records up to the Altair fork epoch are skipped.
	require.Equal(t, primitives.Epoch(261), recs[0].GetEpoch())
	require.Equal(t, 511-260, len(recs))
}
