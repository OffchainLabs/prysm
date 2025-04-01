package filesystem

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

type mockLayout struct {
	pruneBeforeFunc func(primitives.Epoch) (*pruneSummary, error)
}

var _ fsLayout = &mockLayout{}

func (m *mockLayout) name() string {
	return "mock"
}

func (*mockLayout) dir(_ blobIdent) string {
	return ""
}

func (*mockLayout) blockParentDirs(id blobIdent) []string {
	return []string{}
}

func (*mockLayout) sszPath(_ blobIdent) string {
	return ""
}

func (*mockLayout) partPath(_ blobIdent, _ string) string {
	return ""
}

func (*mockLayout) iterateIdents(_ primitives.Epoch) (*identIterator, error) {
	return nil, nil
}

func (*mockLayout) ident(_ [32]byte, _ uint64) (blobIdent, error) {
	return blobIdent{}, nil
}

func (*mockLayout) dirIdent(_ [32]byte) (blobIdent, error) {
	return blobIdent{}, nil
}

func (*mockLayout) summary(_ [32]byte) BlobStorageSummary {
	return BlobStorageSummary{}
}

func (*mockLayout) notify(blobIdent) error {
	return nil
}

func (m *mockLayout) pruneBefore(before primitives.Epoch) (*pruneSummary, error) {
	return m.pruneBeforeFunc(before)
}

func (*mockLayout) remove(ident blobIdent) (int, error) {
	return 0, nil
}

var _ fsLayout = &mockLayout{}

func TestCleaner(t *testing.T) {
	l := &periodicEpochLayout{}
	p := l.periodDir(11235813)
	e := l.epochDir(11235813)
	dc := newDirCleaner()
	dc.add(p)
	require.Equal(t, 2, dc.maxDepth)
	dc.add(e)
	require.Equal(t, 3, dc.maxDepth)
}
