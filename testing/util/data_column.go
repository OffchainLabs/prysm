package util

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

type (
	// DataColumnParam is a struct that holds parameters for creating test RODataColumn and VerifiedRODataColumn sidecars.
	DataColumnParam struct {
		Index                        uint64
		Column                       [][]byte
		KzgCommitments               [][]byte
		KzgProofs                    [][]byte
		KzgCommitmentsInclusionProof [][]byte

		// Part of the beacon block header.
		Slot          primitives.Slot
		ProposerIndex primitives.ValidatorIndex
		ParentRoot    []byte
		StateRoot     []byte
		BodyRoot      []byte
	}
)

// CreateTestVerifiedRoDataColumnSidecars creates test RODataColumn and VerifiedRODataColumn sidecars for testing purposes.
func CreateTestVerifiedRoDataColumnSidecars(t *testing.T, params []DataColumnParam) ([]blocks.RODataColumn, []blocks.VerifiedRODataColumn) {
	const (
		kzgCommitmentsInclusionProofSize = 4
		proofSize                        = 32
	)

	count := len(params)
	verifiedRoDataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, count)
	rodataColumnSidecars := make([]blocks.RODataColumn, 0, count)

	for _, param := range params {
		var parentRoot, stateRoot, bodyRoot [fieldparams.RootLength]byte
		copy(parentRoot[:], param.ParentRoot)
		copy(stateRoot[:], param.StateRoot)
		copy(bodyRoot[:], param.BodyRoot)

		column := make([][]byte, 0, len(param.Column))
		for _, cell := range param.Column {
			var completeCell [kzg.BytesPerCell]byte
			copy(completeCell[:], cell)
			column = append(column, completeCell[:])
		}

		kzgCommitmentsInclusionProof := make([][]byte, 0, kzgCommitmentsInclusionProofSize)
		for range kzgCommitmentsInclusionProofSize {
			kzgCommitmentsInclusionProof = append(kzgCommitmentsInclusionProof, make([]byte, proofSize))
		}

		for i, proof := range param.KzgCommitmentsInclusionProof {
			copy(kzgCommitmentsInclusionProof[i], proof)
		}

		dataColumnSidecar := &ethpb.DataColumnSidecar{
			Index:          param.Index,
			Column:         column,
			KzgCommitments: param.KzgCommitments,
			KzgProofs:      param.KzgProofs,
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					Slot:          param.Slot,
					ProposerIndex: param.ProposerIndex,
					ParentRoot:    parentRoot[:],
					StateRoot:     stateRoot[:],
					BodyRoot:      bodyRoot[:],
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
			KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
		}

		roDataColumnSidecar, err := blocks.NewRODataColumn(dataColumnSidecar)
		if err != nil {
			t.Fatal(err)
		}

		rodataColumnSidecars = append(rodataColumnSidecars, roDataColumnSidecar)

		verifiedRoDataColumnSidecar := blocks.NewVerifiedRODataColumn(roDataColumnSidecar)
		verifiedRoDataColumnSidecars = append(verifiedRoDataColumnSidecars, verifiedRoDataColumnSidecar)
	}

	return rodataColumnSidecars, verifiedRoDataColumnSidecars
}
