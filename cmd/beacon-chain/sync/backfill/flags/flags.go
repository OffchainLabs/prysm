package flags

import (
	"github.com/urfave/cli/v2"
)

var (
	backfillBatchSizeName   = "backfill-batch-size"
	backfillWorkerCountName = "backfill-worker-count"

	// EnableExperimentalBackfill enables backfill for checkpoint synced nodes.
	// This flag will be removed once backfill is enabled by default.
	EnableExperimentalBackfill = &cli.BoolFlag{
		Name: "enable-experimental-backfill",
		Usage: "Backfill is still experimental at this time. " +
			"It will only be enabled if this flag is specified and the node was started using checkpoint sync.",
	}
	// BackfillBatchSize allows users to tune block backfill request sizes to maximize network utilization
	// at the cost of higher memory.
	BackfillBatchSize = &cli.Uint64Flag{
		Name: backfillBatchSizeName,
		Usage: "Number of blocks per backfill batch. " +
			"A larger number will request more blocks at once from peers, but also consume more system memory to " +
			"hold batches in memory during processing. This has a multiplicative effect with " + backfillWorkerCountName + ".",
		Value: 32,
	}
	// BackfillWorkerCount allows users to tune the number of concurrent backfill batches to download, to maximize
	// network utilization at the cost of higher memory.
	BackfillWorkerCount = &cli.IntFlag{
		Name: backfillWorkerCountName,
		Usage: "Number of concurrent backfill batch requests. " +
			"A larger number will better utilize network resources, up to a system-dependent limit, but will also " +
			"consume more system memory to hold batches in memory during processing. Multiply by " + backfillBatchSizeName + " and " +
			"average block size (~2MB before deneb) to find the right number for your system. " +
			"This has a multiplicative effect with " + backfillBatchSizeName + ".",
		Value: 2,
	}
	BackfillOldestSlot = &cli.Uint64Flag{
		Name: "backfill-oldest-slot",
		Usage: "Specifies the oldest slot that backfill should download. " +
			"If this value is greater than current_slot - MIN_EPOCHS_FOR_BLOCK_REQUESTS, it will be ignored with a warning log.",
	}
)
