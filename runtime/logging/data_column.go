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

	// Fulu sidecars carry proposer index, parent root, and KZG commitments
	// directly. Gloas sidecars don't have these fields.
	if !column.IsGloas() {
		propIdx, _ := column.ProposerIndex()
		fields["propIdx"] = propIdx
		parentRoot, _ := column.ParentRoot()
		fields["parentRoot"] = fmt.Sprintf("%#x", parentRoot)[:8]
		kzgCommitments, _ := column.KzgCommitments()
		fields["kzgCommitmentCount"] = len(kzgCommitments)
	}

	return fields
}
