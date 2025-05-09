package verification

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	ckzg4844 "github.com/ethereum/c-kzg-4844/v2/bindings/go"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
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

// FakeVerifyForTest can be used by tests that need a VerifiedROBlob but don't want to do all the
// expensive set up to perform full validation.
func FakeVerifyForTest(t *testing.T, b blocks.ROBlob) blocks.VerifiedROBlob {
	// log so that t is truly required
	t.Log("producing fake VerifiedROBlob for a test")
	return blocks.NewVerifiedROBlob(b)
}

// FakeVerifySliceForTest can be used by tests that need a []VerifiedROBlob but don't want to do all the
// expensive set up to perform full validation.
func FakeVerifySliceForTest(t *testing.T, b []blocks.ROBlob) []blocks.VerifiedROBlob {
	// log so that t is truly required
	t.Log("producing fake []VerifiedROBlob for a test")
	vbs := make([]blocks.VerifiedROBlob, len(b))
	for i := range b {
		vbs[i] = blocks.NewVerifiedROBlob(b[i])
	}
	return vbs
}

// FakeVerifyDataColumnForTest can be used by tests that need a VerifiedRODataColumn but don't want to do all the
// expensive set up to perform full validation.
func FakeVerifyDataColumnForTest(t *testing.T, b blocks.RODataColumn) blocks.VerifiedRODataColumn {
	// log so that t is truly required
	t.Log("producing fake VerifiedRODataColumn for a test")
	return blocks.NewVerifiedRODataColumn(b)
}

// FakeVerifyDataColumnSliceForTest can be used by tests that need a []VerifiedRODataColumn but don't want to do all the
// expensive set up to perform full validation.
func FakeVerifyDataColumnSliceForTest(t *testing.T, dcs []blocks.RODataColumn) []blocks.VerifiedRODataColumn {
	// Log so that `t`` is truly required.
	t.Log("producing fake []VerifiedRODataColumn for a test")

	vcs := make([]blocks.VerifiedRODataColumn, 0, len(dcs))
	for _, dc := range dcs {
		vcs = append(vcs, blocks.NewVerifiedRODataColumn(dc))
	}

	return vcs
}

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
