package eth_test

import (
	"encoding/binary"
	"fmt"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type backfillSSZ interface {
	proto.Message
	SizeSSZ() int
	MarshalSSZ() ([]byte, error)
	UnmarshalSSZ([]byte) error
	HashTreeRoot() ([32]byte, error)
}

type backfillFixture struct {
	message backfillSSZ
	size    int
	root    [32]byte
}

var backfillForks = []struct {
	name           string
	committeeDepth int
	finalityDepth  int
}{
	{"Altair", 5, 6}, // also Bellatrix.
	{"Capella", 5, 6},
	{"Deneb", 5, 6},
	{"Electra", 6, 7}, // also Fulu
	{"Gloas", 11, 9},
}

func TestLightClientBackfillSSZ(t *testing.T) {
	for _, gloas := range []bool{false, true} {
		for _, empty := range []bool{false, true} {
			t.Run(fmt.Sprintf("block/gloas=%t/empty=%t", gloas, empty), func(t *testing.T) {
				checkBackfillSSZ(t, backfillBlockFixture(t, gloas, empty))
			})
		}
	}
	for _, fork := range backfillForks {
		t.Run(fork.name, func(t *testing.T) {
			for _, committee := range []bool{false, true} {
				t.Run(fmt.Sprintf("committee=%t", committee), func(t *testing.T) {
					bootstrap := backfillBootstrapFixture(t, fork.name, fork.committeeDepth, committee)
					t.Run("bootstrap", func(t *testing.T) {
						checkBackfillSSZ(t, bootstrap)
					})
					t.Run("epoch", func(t *testing.T) {
						checkBackfillSSZ(t, backfillEpochFixture(t, fork.name, fork.finalityDepth, bootstrap))
					})
				})
			}
		})
	}
}

func checkBackfillSSZ(t *testing.T, fixture backfillFixture) {
	t.Helper()
	encoded, err := fixture.message.MarshalSSZ()
	require.NoError(t, err)
	require.Equal(t, fixture.size, len(encoded))
	require.Equal(t, fixture.size, fixture.message.SizeSSZ())
	root, err := fixture.message.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, fixture.root, root, "root must match ordinary SSZ container merkleization")

	decoded := fixture.message.ProtoReflect().New().Interface().(backfillSSZ)
	require.NoError(t, decoded.UnmarshalSSZ(encoded))
	require.Equal(t, true, proto.Equal(fixture.message, decoded))
	reencoded, err := decoded.MarshalSSZ()
	require.NoError(t, err)
	require.DeepEqual(t, encoded, reencoded)
	decodedRoot, err := decoded.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, fixture.root, decodedRoot)

	t.Run("truncated", func(t *testing.T) {
		fresh := fixture.message.ProtoReflect().New().Interface().(backfillSSZ)
		require.NotNil(t, fresh.UnmarshalSSZ(encoded[:1]))
	})
	t.Run("invalid_lengths", func(t *testing.T) {
		checkBackfillInvalidLengths(t, fixture.message)
	})

	fields := fixture.message.ProtoReflect().Descriptor().Fields()
	offsetPosition := 0
	if field := fields.ByName("block_data"); field != nil {
		blocks := fixture.message.ProtoReflect().Get(field).List()
		block := blocks.Get(0).Message().Interface().(backfillSSZ)
		offsetPosition = 8 + 112 + fieldparams.SlotsPerEpoch*block.SizeSSZ()
	} else if fields.ByName("current_sync_committee") == nil {
		t.Run("trailing_bytes", func(t *testing.T) {
			fresh := fixture.message.ProtoReflect().New().Interface().(backfillSSZ)
			require.NotNil(t, fresh.UnmarshalSSZ(append(append([]byte(nil), encoded...), 0)))
		})
		return
	}
	t.Run("invalid_offset", func(t *testing.T) {
		invalid := append([]byte(nil), encoded...)
		binary.LittleEndian.PutUint32(invalid[offsetPosition:], 0)
		fresh := fixture.message.ProtoReflect().New().Interface().(backfillSSZ)
		require.NotNil(t, fresh.UnmarshalSSZ(invalid))
	})
}

func checkBackfillInvalidLengths(t *testing.T, message backfillSSZ) {
	t.Helper()
	fields := message.ProtoReflect().Descriptor().Fields()
	check := func(t *testing.T, change func(protoreflect.Message)) {
		t.Helper()
		invalid := proto.Clone(message).(backfillSSZ)
		change(invalid.ProtoReflect())
		_, err := invalid.MarshalSSZ()
		require.NotNil(t, err, "marshal must reject an invalid SSZ length")
		_, err = invalid.HashTreeRoot()
		require.NotNil(t, err, "hashing must reject an invalid SSZ length")
	}
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		switch {
		case field.IsList():
			t.Run(string(field.Name()), func(t *testing.T) {
				if field.Name() == "current_sync_committee" {
					t.Run("two_committees", func(t *testing.T) {
						check(t, func(m protoreflect.Message) {
							list := m.Mutable(field).List()
							for list.Len() < 2 {
								list.Append(protoreflect.ValueOfMessage(genSyncCommittee().ProtoReflect()))
							}
						})
					})
					return
				}
				t.Run("short_vector", func(t *testing.T) {
					check(t, func(m protoreflect.Message) {
						list := m.Mutable(field).List()
						list.Truncate(list.Len() - 1)
					})
				})
				t.Run("long_vector", func(t *testing.T) {
					check(t, func(m protoreflect.Message) {
						list := m.Mutable(field).List()
						list.Append(list.Get(0))
					})
				})
				if field.Kind() == protoreflect.BytesKind {
					for _, length := range []int{31, 33} {
						t.Run(fmt.Sprintf("root_bytes=%d", length), func(t *testing.T) {
							check(t, func(m protoreflect.Message) {
								m.Mutable(field).List().Set(0, protoreflect.ValueOfBytes(make([]byte, length)))
							})
						})
					}
				}
			})
		case field.Kind() == protoreflect.BytesKind:
			length := len(message.ProtoReflect().Get(field).Bytes())
			for _, invalidLength := range []int{length - 1, length + 1} {
				t.Run(fmt.Sprintf("%s/bytes=%d", field.Name(), invalidLength), func(t *testing.T) {
					check(t, func(m protoreflect.Message) {
						m.Set(field, protoreflect.ValueOfBytes(make([]byte, invalidLength)))
					})
				})
			}
		}
	}
}

func backfillBlockFixture(t *testing.T, gloas, empty bool) backfillFixture {
	t.Helper()
	depth := 4
	if gloas {
		depth = 8
	}
	index := uint64(1337)
	stateRoot := genBytes(32, 2)
	bits := genBytes(fieldparams.SyncAggregateSyncCommitteeBytesLength, 3)
	signatureRoot, err := ssz.MerkleizeByteSliceSSZ(genBytes(96, 4))
	require.NoError(t, err)
	branch := genBranch(depth, 5)
	if empty {
		index = 0
		clear(stateRoot)
		clear(bits)
		signatureRoot = [32]byte{}
		branch = genBranch(depth, 0)
	}
	var message backfillSSZ = &ethpb.LightClientBlockData{
		ProposerIndex: primitives.ValidatorIndex(index), StateRoot: stateRoot,
		SyncCommitteeBits: bits, SyncCommitteeSignatureRoot: signatureRoot[:], SyncAggregateBranch: branch,
	}
	if gloas {
		message = &ethpb.LightClientBlockDataGloas{
			ProposerIndex: primitives.ValidatorIndex(index), StateRoot: stateRoot,
			SyncCommitteeBits: bits, SyncCommitteeSignatureRoot: signatureRoot[:], SyncAggregateBranch: branch,
		}
	}
	bitsRoot, err := ssz.MerkleizeByteSliceSSZ(bits)
	require.NoError(t, err)
	roots := [][32]byte{
		ssz.Uint64Root(index), [32]byte(stateRoot), bitsRoot, signatureRoot, branchVectorRoot(branch),
	}
	return backfillFixture{
		message: message,
		size:    8 + 32 + len(bits) + 32 + depth*32,
		root:    ssz.MerkleizeVector(roots, uint64(len(roots))),
	}
}

func backfillBootstrapFixture(t *testing.T, fork string, depth int, includeCommittee bool) backfillFixture {
	t.Helper()
	var committees []*ethpb.SyncCommittee
	var committeeRoot [32]byte
	committeeSize := 0
	if includeCommittee {
		committee := genSyncCommittee()
		committees = []*ethpb.SyncCommittee{committee}
		pubkeyRoots := make([][32]byte, len(committee.Pubkeys))
		for i, key := range committee.Pubkeys {
			root, err := ssz.MerkleizeByteSliceSSZ(key)
			require.NoError(t, err)
			pubkeyRoots[i] = root
		}
		aggregatePubkeyRoot, err := ssz.MerkleizeByteSliceSSZ(committee.AggregatePubkey)
		require.NoError(t, err)
		committeeRoot = ssz.MerkleizeVector([][32]byte{
			ssz.MerkleizeVector(pubkeyRoots, uint64(len(pubkeyRoots))), aggregatePubkeyRoot,
		}, 2)
		committeeSize = (fieldparams.SyncCommitteeLength + 1) * 48
	}
	branch := genBranch(depth, 6)
	committeeLength := ssz.Uint64Root(uint64(len(committees)))
	roots := [][32]byte{
		ssz.MixInLength(committeeRoot, committeeLength[:]),
		branchVectorRoot(branch),
	}
	size := 4 + depth*32 + committeeSize
	var message, execution backfillSSZ
	var executionBranch [][]byte
	switch fork {
	case "Altair":
		message = &ethpb.LightClientBootstrapDataAltair{CurrentSyncCommittee: committees, CurrentSyncCommitteeBranch: branch}
	case "Gloas":
		hash, proof := genBytes(32, 7), genBranch(11, 8)
		message = &ethpb.LightClientBootstrapDataGloas{
			CurrentSyncCommittee: committees, CurrentSyncCommitteeBranch: branch,
			ExecutionBlockHash: hash, ExecutionBranch: proof,
		}
		size += 32 + 11*32
		roots = append(roots, [32]byte(hash), branchVectorRoot(proof))
	case "Capella":
		executionBranch = genBranch(4, 8)
		header := util.HydrateBlindedBeaconBlockBodyCapella(&ethpb.BlindedBeaconBlockBodyCapella{}).ExecutionPayloadHeader
		header.BlockHash, header.ExtraData, header.GasUsed = genBytes(32, 7), []byte{9, 10, 11}, 1234
		execution = header
		message = &ethpb.LightClientBootstrapDataCapella{
			CurrentSyncCommittee: committees, CurrentSyncCommitteeBranch: branch, Execution: header, ExecutionBranch: executionBranch,
		}
		size += 568 + len(header.ExtraData)
	case "Deneb", "Electra":
		executionBranch = genBranch(4, 8)
		header := util.HydrateBlindedBeaconBlockBodyDeneb(&ethpb.BlindedBeaconBlockBodyDeneb{}).ExecutionPayloadHeader
		header.BlockHash, header.ExtraData, header.GasUsed = genBytes(32, 7), []byte{9, 10, 11}, 1234
		header.BlobGasUsed, header.ExcessBlobGas = 131072, 262144
		execution = header
		switch fork {
		case "Deneb":
			message = &ethpb.LightClientBootstrapDataDeneb{
				CurrentSyncCommittee: committees, CurrentSyncCommitteeBranch: branch, Execution: header, ExecutionBranch: executionBranch,
			}
		case "Electra":
			message = &ethpb.LightClientBootstrapDataElectra{
				CurrentSyncCommittee: committees, CurrentSyncCommitteeBranch: branch, Execution: header, ExecutionBranch: executionBranch,
			}
		}
		size += 584 + len(header.ExtraData)
	default:
		t.Fatalf("unsupported backfill bootstrap fork %q", fork)
	}
	if execution != nil {
		executionRoot, err := execution.HashTreeRoot()
		require.NoError(t, err)
		roots = append(roots, executionRoot, branchVectorRoot(executionBranch))
		size += 4 + 4*32 // Execution offset and proof.
	}
	return backfillFixture{message: message, size: size, root: ssz.MerkleizeVector(roots, uint64(len(roots)))}
}

func backfillEpochFixture(t *testing.T, fork string, depth int, bootstrap backfillFixture) backfillFixture {
	t.Helper()
	const epoch = 12
	header := &ethpb.BeaconBlockHeader{
		Slot: primitives.Slot((epoch - 1) * fieldparams.SlotsPerEpoch), ProposerIndex: 42,
		ParentRoot: genBytes(32, 10), StateRoot: genBytes(32, 11), BodyRoot: genBytes(32, 12),
	}
	blocks := make([]*ethpb.LightClientBlockData, fieldparams.SlotsPerEpoch)
	gloasBlocks := make([]*ethpb.LightClientBlockDataGloas, fieldparams.SlotsPerEpoch)
	blockRoots := make([][32]byte, fieldparams.SlotsPerEpoch)
	blockSize := 0
	for i := range blockRoots {
		block := backfillBlockFixture(t, fork == "Gloas", i%3 == 0)
		blockRoots[i], blockSize = block.root, block.size
		if fork == "Gloas" {
			gloasBlocks[i] = block.message.(*ethpb.LightClientBlockDataGloas)
		} else {
			blocks[i] = block.message.(*ethpb.LightClientBlockData)
		}
	}
	finalized, branch := genBytes(32, 13), genBranch(depth, 14)
	var message backfillSSZ
	switch fork {
	case "Altair":
		message = &ethpb.LightClientEpochDataAltair{
			Epoch: epoch, ParentBlockHeader: header, BlockData: blocks,
			BootstrapData: bootstrap.message.(*ethpb.LightClientBootstrapDataAltair), FinalizedRoot: finalized, FinalityBranch: branch,
		}
	case "Capella":
		message = &ethpb.LightClientEpochDataCapella{
			Epoch: epoch, ParentBlockHeader: header, BlockData: blocks,
			BootstrapData: bootstrap.message.(*ethpb.LightClientBootstrapDataCapella), FinalizedRoot: finalized, FinalityBranch: branch,
		}
	case "Deneb":
		message = &ethpb.LightClientEpochDataDeneb{
			Epoch: epoch, ParentBlockHeader: header, BlockData: blocks,
			BootstrapData: bootstrap.message.(*ethpb.LightClientBootstrapDataDeneb), FinalizedRoot: finalized, FinalityBranch: branch,
		}
	case "Electra":
		message = &ethpb.LightClientEpochDataElectra{
			Epoch: epoch, ParentBlockHeader: header, BlockData: blocks,
			BootstrapData: bootstrap.message.(*ethpb.LightClientBootstrapDataElectra), FinalizedRoot: finalized, FinalityBranch: branch,
		}
	case "Gloas":
		message = &ethpb.LightClientEpochDataGloas{
			Epoch: epoch, ParentBlockHeader: header, BlockData: gloasBlocks,
			BootstrapData: bootstrap.message.(*ethpb.LightClientBootstrapDataGloas), FinalizedRoot: finalized, FinalityBranch: branch,
		}
	}
	headerRoots := [][32]byte{
		ssz.Uint64Root(uint64(header.Slot)), ssz.Uint64Root(uint64(header.ProposerIndex)),
		[32]byte(header.ParentRoot), [32]byte(header.StateRoot), [32]byte(header.BodyRoot),
	}
	headerRoot := ssz.MerkleizeVector(headerRoots, uint64(len(headerRoots)))
	roots := [][32]byte{
		ssz.Uint64Root(epoch), headerRoot, ssz.MerkleizeVector(blockRoots, uint64(len(blockRoots))), bootstrap.root,
		[32]byte(finalized), branchVectorRoot(branch),
	}
	return backfillFixture{
		message: message,
		size:    8 + 112 + fieldparams.SlotsPerEpoch*blockSize + 4 + bootstrap.size + 32 + depth*32,
		root:    ssz.MerkleizeVector(roots, uint64(len(roots))),
	}
}

func genSyncCommittee() *ethpb.SyncCommittee {
	keys := make([][]byte, fieldparams.SyncCommitteeLength)
	for i := range keys {
		keys[i] = genBytes(48, byte(i%255+1))
	}
	return &ethpb.SyncCommittee{Pubkeys: keys, AggregatePubkey: genBytes(48, 15)}
}

func genBytes(length int, seed byte) []byte {
	out := make([]byte, length)
	if seed != 0 {
		for i := range out {
			out[i] = seed + byte(i)
		}
	}
	return out
}

// genBranch generates synthetic sibling hashes, not a valid inclusion proof.
func genBranch(depth int, seed byte) [][]byte {
	out := make([][]byte, depth)
	for i := range out {
		itemSeed := seed
		if seed != 0 {
			itemSeed += byte(i)
		}
		out[i] = genBytes(32, itemSeed)
	}
	return out
}

// branchVectorRoot returns the SSZ root of the branch vector, not the proven tree root.
func branchVectorRoot(branch [][]byte) [32]byte {
	chunks := make([][32]byte, len(branch))
	for i, root := range branch {
		chunks[i] = [32]byte(root)
	}
	return ssz.MerkleizeVector(chunks, uint64(len(chunks)))
}
