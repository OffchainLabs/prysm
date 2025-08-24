package logging

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/sirupsen/logrus"
)

// DataColumnFields extracts a standard set of fields from a DataColumnSidecar into a logrus.Fields struct
// which can be passed to log.WithFields.
func DataColumnFields(column blocks.RODataColumn) logrus.Fields {
	kzgCommitmentCount := len(column.KzgCommitments)

	return logrus.Fields{
		"slot":               column.Slot(),
		"propIdx":            column.ProposerIndex(),
		"blockRoot":          fmt.Sprintf("%#x", column.BlockRoot())[:8],
		"parentRoot":         fmt.Sprintf("%#x", column.ParentRoot())[:8],
		"kzgCommitmentCount": kzgCommitmentCount,
		"colIdx":             column.Index,
	}
}
