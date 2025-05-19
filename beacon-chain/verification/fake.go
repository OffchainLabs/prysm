package verification

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
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
