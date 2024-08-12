package logging

import (
	"fmt"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/sirupsen/logrus"
)

// DataColumnFields extracts a standard set of fields from a DataColumnSidecar into a logrus.Fields struct
// which can be passed to log.WithFields.
func DataColumnFields(column blocks.RODataColumn) logrus.Fields {
	return logrus.Fields{
		"slot":           column.Slot(),
		"proposerIndex":  column.ProposerIndex(),
		"blockRoot":      fmt.Sprintf("%#x", column.BlockRoot()),
		"parentRoot":     fmt.Sprintf("%#x", column.ParentRoot()),
		"kzgCommitments": fmt.Sprintf("%#x", column.KzgCommitments),
		"index":          column.ColumnIndex,
	}
}

// BlockFieldsFromColumn extracts the set of fields from a given DataColumnSidecar which are shared by the block and
// all other sidecars for the block.
func BlockFieldsFromColumn(column blocks.RODataColumn) logrus.Fields {
	return logrus.Fields{
		"slot":          column.Slot(),
		"proposerIndex": column.ProposerIndex(),
		"blockRoot":     fmt.Sprintf("%#x", column.BlockRoot()),
		"parentRoot":    fmt.Sprintf("%#x", column.ParentRoot()),
	}
}
