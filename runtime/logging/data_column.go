package logging

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/sirupsen/logrus"
)

// DataColumnFields extracts a standard set of fields from a DataColumnSidecar into a logrus.Fields struct
// which can be passed to log.WithFields.
func DataColumnFields(column blocks.RODataColumn) logrus.Fields {
	fields := logrus.Fields{
		"slot":      column.Slot(),
		"blockRoot": fmt.Sprintf("%#x", column.BlockRoot())[:8],
		"colIdx":    column.Index(),
	}

	if propIdx, err := column.ProposerIndex(); err == nil {
		fields["propIdx"] = propIdx
	}
	if parentRoot, err := column.ParentRoot(); err == nil {
		fields["parentRoot"] = fmt.Sprintf("%#x", parentRoot)[:8]
	}
	if kzgCommitments, err := column.KzgCommitments(); err == nil {
		fields["kzgCommitmentCount"] = len(kzgCommitments)
	}

	return fields
}
