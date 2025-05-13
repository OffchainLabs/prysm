package das

import (
	"fmt"
	"strings"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/pkg/errors"
)

var ErrBisectInconsistent = errors.New("state of bisector inconsistent with columns to bisect")

type Bisector interface {
	Bisect([]blocks.RODataColumn) error
	Next() ([]blocks.RODataColumn, error)
	OnError(error)
	Error() error
}

type columnBatchError struct {
	errors map[[32]byte]error
}

func (e *columnBatchError) add(root [32]byte, err error) {
	e.errors[root] = err
}

func (e *columnBatchError) count() int {
	return len(e.errors)
}

func (e *columnBatchError) Error() string {
	roots := make([]string, 0, len(e.errors))
	for root := range e.errors {
		roots = append(roots, fmt.Sprintf("%#x", root))
	}
	return fmt.Sprintf("column verification error for roots: %s", strings.Join(roots, ","))
}

func (e *columnBatchError) combine(other *columnBatchError) {
	for root, err := range other.errors {
		e.errors[root] = err
	}
}
