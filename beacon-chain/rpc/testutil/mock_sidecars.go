package testutil

import ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"

// CreateDataColumnSidecar generates a filled dummy data column sidecar
func CreateDataColumnSidecar(index uint64, data []byte) *ethpb.DataColumnSidecar {
	return &ethpb.DataColumnSidecar{
		Index:  index,
		Column: [][]byte{data},
		SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
			Header: &ethpb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: 1,
				ParentRoot:    make([]byte, 32),
				StateRoot:     make([]byte, 32),
				BodyRoot:      make([]byte, 32),
			},
			Signature: make([]byte, 96),
		},
		KzgCommitments:               [][]byte{make([]byte, 48)},
		KzgProofs:                    [][]byte{make([]byte, 48)},
		KzgCommitmentsInclusionProof: [][]byte{make([]byte, 32)},
	}
}

// CreateBlobSidecar generates a filled dummy data blob sidecar
func CreateBlobSidecar(index uint64, blob []byte) *ethpb.BlobSidecar {
	return &ethpb.BlobSidecar{
		Index: index,
		Blob:  blob,
		SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
			Header: &ethpb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: 1,
				ParentRoot:    make([]byte, 32),
				StateRoot:     make([]byte, 32),
				BodyRoot:      make([]byte, 32),
			},
			Signature: make([]byte, 96),
		},
		KzgCommitment: make([]byte, 48),
		KzgProof:      make([]byte, 48),
	}
}
