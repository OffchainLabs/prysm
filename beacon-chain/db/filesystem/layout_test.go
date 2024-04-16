package filesystem

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

type mockLayout struct {
	pruneBeforeFunc func(primitives.Epoch) (*pruneSummary, error)
}

func (*mockLayout) dir(_ blobIdent) string {
	return ""
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
