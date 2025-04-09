package verification

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
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
