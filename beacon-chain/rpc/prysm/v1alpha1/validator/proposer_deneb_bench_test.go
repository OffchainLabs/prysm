package validator

import (
	"errors"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// BuildBlobSidecarsOriginal is the original implementation for comparison
func BuildBlobSidecarsOriginal(blk interfaces.SignedBeaconBlock, blobs [][]byte, kzgProofs [][]byte) ([]*ethpb.BlobSidecar, error) {
	if blk.Version() < version.Deneb {
		return nil, nil // No blobs before deneb.
	}
	commits, err := blk.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, err
	}
	cLen := len(commits)
	if cLen != len(blobs) || cLen != len(kzgProofs) {
		return nil, errors.New("blob KZG commitments don't match number of blobs or KZG proofs")
	}
	blobSidecars := make([]*ethpb.BlobSidecar, cLen)
	header, err := blk.Header()
	if err != nil {
		return nil, err
	}
	body := blk.Block().Body()
	for i := range blobSidecars {
		proof, err := blocks.MerkleProofKZGCommitment(body, i)
		if err != nil {
			return nil, err
		}
		blobSidecars[i] = &ethpb.BlobSidecar{
			Index:                    uint64(i),
			Blob:                     blobs[i],
			KzgCommitment:            commits[i],
			KzgProof:                 kzgProofs[i],
			SignedBlockHeader:        header,
			CommitmentInclusionProof: proof,
		}
	}
	return blobSidecars, nil
}

func setupBenchmarkData(b *testing.B, numBlobs int) (interfaces.SignedBeaconBlock, [][]byte, [][]byte) {
	b.Helper()

	// Create KZG commitments
	kzgCommitments := make([][]byte, numBlobs)
	for i := 0; i < numBlobs; i++ {
		kzgCommitments[i] = bytesutil.PadTo([]byte{byte(i)}, 48)
	}

	// Create block with KZG commitments
	blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockDeneb())
	if err != nil {
		b.Fatal(err)
	}
	if err := blk.SetBlobKzgCommitments(kzgCommitments); err != nil {
		b.Fatal(err)
	}

	// Create blobs
	blobs := make([][]byte, numBlobs)
	for i := 0; i < numBlobs; i++ {
		blobs[i] = make([]byte, fieldparams.BlobLength)
		// Add some variation to the blob data
		blobs[i][0] = byte(i)
	}

	// Create KZG proofs
	proof, err := hexutil.Decode("0xb4021b0de10f743893d4f71e1bf830c019e832958efd6795baf2f83b8699a9eccc5dc99015d8d4d8ec370d0cc333c06a")
	if err != nil {
		b.Fatal(err)
	}
	kzgProofs := make([][]byte, numBlobs)
	for i := 0; i < numBlobs; i++ {
		kzgProofs[i] = proof
	}

	return blk, blobs, kzgProofs
}

func BenchmarkBuildBlobSidecars_Original_1Blob(b *testing.B) {
	blk, blobs, kzgProofs := setupBenchmarkData(b, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := BuildBlobSidecarsOriginal(blk, blobs, kzgProofs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildBlobSidecars_Optimized_1Blob(b *testing.B) {
	blk, blobs, kzgProofs := setupBenchmarkData(b, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := BuildBlobSidecars(blk, blobs, kzgProofs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildBlobSidecars_Original_2Blobs(b *testing.B) {
	blk, blobs, kzgProofs := setupBenchmarkData(b, 2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := BuildBlobSidecarsOriginal(blk, blobs, kzgProofs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildBlobSidecars_Optimized_3Blobs(b *testing.B) {
	blk, blobs, kzgProofs := setupBenchmarkData(b, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := BuildBlobSidecars(blk, blobs, kzgProofs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildBlobSidecars_Original_3Blobs(b *testing.B) {
	blk, blobs, kzgProofs := setupBenchmarkData(b, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := BuildBlobSidecarsOriginal(blk, blobs, kzgProofs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildBlobSidecars_Optimized_4Blobs(b *testing.B) {
	blk, blobs, kzgProofs := setupBenchmarkData(b, 4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := BuildBlobSidecars(blk, blobs, kzgProofs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildBlobSidecars_Original_9Blobs(b *testing.B) {
	blk, blobs, kzgProofs := setupBenchmarkData(b, 9)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := BuildBlobSidecarsOriginal(blk, blobs, kzgProofs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildBlobSidecars_Optimized_9Blobs(b *testing.B) {
	blk, blobs, kzgProofs := setupBenchmarkData(b, 9)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := BuildBlobSidecars(blk, blobs, kzgProofs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark the individual components to understand where the improvements come from
func BenchmarkMerkleProofKZGCommitment_Original(b *testing.B) {
	blk, _, _ := setupBenchmarkData(b, 4)
	body := blk.Block().Body()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 4; j++ {
			_, err := blocks.MerkleProofKZGCommitment(body, j)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkMerkleProofKZGCommitment_Optimized(b *testing.B) {
	blk, _, _ := setupBenchmarkData(b, 4)
	body := blk.Block().Body()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Pre-compute components once
		components, err := blocks.PrecomputeMerkleProofComponents(body)
		if err != nil {
			b.Fatal(err)
		}

		// Generate proofs for each index
		for j := 0; j < 4; j++ {
			_, err := blocks.MerkleProofKZGCommitmentFromComponents(components, j)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
