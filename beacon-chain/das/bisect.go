package das

import (
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
