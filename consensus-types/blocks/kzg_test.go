package blocks

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/prysmaticlabs/gohashtree"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/container/trie"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func Test_MerkleProofKZGCommitment_Altair(t *testing.T) {
	kzgs := make([][]byte, 3)
	kzgs[0] = make([]byte, 48)
	_, err := rand.Read(kzgs[0])
	require.NoError(t, err)
	kzgs[1] = make([]byte, 48)
	_, err = rand.Read(kzgs[1])
	require.NoError(t, err)
	kzgs[2] = make([]byte, 48)
	_, err = rand.Read(kzgs[2])
	require.NoError(t, err)
	pbBody := &ethpb.BeaconBlockBodyAltair{}

	body, err := NewBeaconBlockBody(pbBody)
	require.NoError(t, err)
	_, err = MerkleProofKZGCommitment(body, 0)
	require.ErrorIs(t, errUnsupportedBeaconBlockBody, err)
}

func Test_MerkleProofKZGCommitment(t *testing.T) {
	kzgs := make([][]byte, 3)
	kzgs[0] = make([]byte, 48)
	_, err := rand.Read(kzgs[0])
	require.NoError(t, err)
	kzgs[1] = make([]byte, 48)
	_, err = rand.Read(kzgs[1])
	require.NoError(t, err)
	kzgs[2] = make([]byte, 48)
	_, err = rand.Read(kzgs[2])
	require.NoError(t, err)
	pbBody := &ethpb.BeaconBlockBodyDeneb{
		SyncAggregate: &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		},
		ExecutionPayload: &enginev1.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			ExtraData:     make([]byte, 0),
		},
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
		BlobKzgCommitments: kzgs,
	}

	body, err := NewBeaconBlockBody(pbBody)
	require.NoError(t, err)
	index := 1
	_, err = MerkleProofKZGCommitment(body, 10)
	require.ErrorIs(t, errInvalidIndex, err)
	proof, err := MerkleProofKZGCommitment(body, index)
	require.NoError(t, err)

	// Test the logic of topProof in MerkleProofKZGCommitment.
	commitmentsRoot, err := getBlobKzgCommitmentsRoot(kzgs)
	require.NoError(t, err)
	bodyMembersRoots, err := topLevelRoots(body)
	require.NoError(t, err, "Failed to get top level roots")
	bodySparse, err := trie.GenerateTrieFromItems(
		bodyMembersRoots,
		logBodyLength,
	)
	require.NoError(t, err, "Failed to generate trie from member roots")
	require.Equal(t, bodyLength, bodySparse.NumOfItems())
	topProof, err := bodySparse.MerkleProof(kzgPosition)
	require.NoError(t, err, "Failed to generate Merkle proof")
	require.DeepEqual(t,
		topProof[:len(topProof)-1],
		proof[fieldparams.LogMaxBlobCommitments+1:],
	)

	root, err := body.HashTreeRoot()
	require.NoError(t, err)
	// Partially verify if the commitments root is in the body root.
	// Proof of the commitment length is not needed.
	require.Equal(t, true, trie.VerifyMerkleProof(root[:], commitmentsRoot[:], kzgPosition, topProof[:len(topProof)-1]))

	chunk := makeChunk(kzgs[index])
	gohashtree.HashChunks(chunk, chunk)
	require.Equal(t, true, trie.VerifyMerkleProof(root[:], chunk[0][:], uint64(index+KZGOffset), proof))
}

// This test explains the calculation of the KZG commitment root's Merkle index
// in the Body's Merkle tree based on the index of the KZG commitment list in the Body.
func Test_KZGRootIndex(t *testing.T) {
	// Level of the KZG commitment root's parent.
	kzgParentRootLevel, err := ceilLog2(kzgPosition)
	require.NoError(t, err)
	// Merkle index of the KZG commitment root's parent.
	// The parent's left child is the KZG commitment root,
	// and its right child is the KZG commitment size.
	kzgParentRootIndex := kzgPosition + (1 << kzgParentRootLevel)
	// The KZG commitment root is the left child of its parent.
	// Its Merkle index is the double of its parent's Merkle index.
	require.Equal(t, 2*kzgParentRootIndex, kzgRootIndex)
}

// ceilLog2 returns the smallest integer greater than or equal to
// the base-2 logarithm of x.
func ceilLog2(x uint32) (uint32, error) {
	if x == 0 {
		return 0, errors.New("log2(0) is undefined")
	}
	var y uint32
	if (x & (x - 1)) == 0 {
		y = 0
	} else {
		y = 1
	}
	for x > 1 {
		x >>= 1
		y += 1
	}
	return y, nil
}

func getBlobKzgCommitmentsRoot(commitments [][]byte) ([32]byte, error) {
	commitmentsLeaves := leavesFromCommitments(commitments)
	commitmentsSparse, err := trie.GenerateTrieFromItems(
		commitmentsLeaves,
		fieldparams.LogMaxBlobCommitments,
	)
	if err != nil {
		return [32]byte{}, err
	}
	return commitmentsSparse.HashTreeRoot()
}

func Benchmark_MerkleProofKZGCommitment(b *testing.B) {
	kzgs := make([][]byte, 3)
	kzgs[0] = make([]byte, 48)
	_, err := rand.Read(kzgs[0])
	require.NoError(b, err)
	kzgs[1] = make([]byte, 48)
	_, err = rand.Read(kzgs[1])
	require.NoError(b, err)
	kzgs[2] = make([]byte, 48)
	_, err = rand.Read(kzgs[2])
	require.NoError(b, err)
	pbBody := &ethpb.BeaconBlockBodyDeneb{
		SyncAggregate: &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		},
		ExecutionPayload: &enginev1.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			ExtraData:     make([]byte, 0),
		},
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
		BlobKzgCommitments: kzgs,
	}

	body, err := NewBeaconBlockBody(pbBody)
	require.NoError(b, err)
	index := 1
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := MerkleProofKZGCommitment(body, index)
		require.NoError(b, err)
	}
}

func Test_VerifyKZGInclusionProof(t *testing.T) {
	kzgs := make([][]byte, 3)
	kzgs[0] = make([]byte, 48)
	_, err := rand.Read(kzgs[0])
	require.NoError(t, err)
	kzgs[1] = make([]byte, 48)
	_, err = rand.Read(kzgs[1])
	require.NoError(t, err)
	kzgs[2] = make([]byte, 48)
	_, err = rand.Read(kzgs[2])
	require.NoError(t, err)
	pbBody := &ethpb.BeaconBlockBodyDeneb{
		SyncAggregate: &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		},
		ExecutionPayload: &enginev1.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			ExtraData:     make([]byte, 0),
		},
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
		BlobKzgCommitments: kzgs,
	}

	body, err := NewBeaconBlockBody(pbBody)
	require.NoError(t, err)
	root, err := body.HashTreeRoot()
	require.NoError(t, err)
	index := 1
	proof, err := MerkleProofKZGCommitment(body, index)
	require.NoError(t, err)

	header := &ethpb.BeaconBlockHeader{
		BodyRoot:   root[:],
		ParentRoot: make([]byte, 32),
		StateRoot:  make([]byte, 32),
	}
	signedHeader := &ethpb.SignedBeaconBlockHeader{
		Header:    header,
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
	sidecar := &ethpb.BlobSidecar{
		Index:                    uint64(index),
		KzgCommitment:            kzgs[index],
		CommitmentInclusionProof: proof,
		SignedBlockHeader:        signedHeader,
	}
	blob, err := NewROBlob(sidecar)
	require.NoError(t, err)
	require.NoError(t, VerifyKZGInclusionProof(blob))
	proof[2] = make([]byte, 32)
	require.ErrorIs(t, errInvalidInclusionProof, VerifyKZGInclusionProof(blob))
}
