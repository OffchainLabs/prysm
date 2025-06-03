package util

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	ckzg4844 "github.com/ethereum/c-kzg-4844/v2/bindings/go"
)

type (
	DataColumnParams struct {
		Slot           primitives.Slot
		ColumnIndex    uint64
		KzgCommitments [][]byte
		DataColumn     []byte // A whole data cell will be filled with the content of one item of this slice.
	}

	DataColumnsParamsByRoot map[[fieldparams.RootLength]byte][]DataColumnParams
)

func CreateTestVerifiedRoDataColumnSidecars(t *testing.T, dataColumnParamsByBlockRoot DataColumnsParamsByRoot) ([]blocks.RODataColumn, []blocks.VerifiedRODataColumn) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	count := 0
	for _, indices := range dataColumnParamsByBlockRoot {
		count += len(indices)
	}

	verifiedRoDataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, count)
	rodataColumnSidecars := make([]blocks.RODataColumn, 0, count)
	for blockRoot, params := range dataColumnParamsByBlockRoot {
		for _, param := range params {
			dataColumn := make([][]byte, 0, len(param.DataColumn))
			for _, value := range param.DataColumn {
				cell := make([]byte, ckzg4844.BytesPerCell)
				for i := range ckzg4844.BytesPerCell {
					cell[i] = value
				}
				dataColumn = append(dataColumn, cell)
			}

			kzgCommitmentsInclusionProof := make([][]byte, 4)
			for i := range kzgCommitmentsInclusionProof {
				kzgCommitmentsInclusionProof[i] = make([]byte, 32)
			}

			dataColumnSidecar := &ethpb.DataColumnSidecar{
				Index:                        param.ColumnIndex,
				KzgCommitments:               param.KzgCommitments,
				Column:                       dataColumn,
				KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
				SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
					Header: &ethpb.BeaconBlockHeader{
						Slot:       param.Slot,
						ParentRoot: make([]byte, fieldparams.RootLength),
						StateRoot:  make([]byte, fieldparams.RootLength),
						BodyRoot:   make([]byte, fieldparams.RootLength),
					},
					Signature: make([]byte, fieldparams.BLSSignatureLength),
				},
			}

			roDataColumnSidecar, err := blocks.NewRODataColumnWithRoot(dataColumnSidecar, blockRoot)
			if err != nil {
				t.Fatal(err)
			}

			rodataColumnSidecars = append(rodataColumnSidecars, roDataColumnSidecar)

			verifiedRoDataColumnSidecar := blocks.NewVerifiedRODataColumn(roDataColumnSidecar)
			verifiedRoDataColumnSidecars = append(verifiedRoDataColumnSidecars, verifiedRoDataColumnSidecar)
		}
	}

	return rodataColumnSidecars, verifiedRoDataColumnSidecars
}
