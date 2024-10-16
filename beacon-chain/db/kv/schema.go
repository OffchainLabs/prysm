package kv

// The schema will define how to store and retrieve data from the db.
// we can prefix or suffix certain values such as `block` with attributes
// for prefix-wide scans across the underlying BoltDB buckets when filtering data.
// For example, we might store attestations as shard + attestation_root -> attestation, making
// it easy to scan for keys that have a certain shard number as a prefix and return those
// corresponding attestations.
var (
	blocksBucket                   = []byte("blocks")
	stateBucket                    = []byte("state")
	stateSummaryBucket             = []byte("state-summary")
	chainMetadataBucket            = []byte("chain-metadata")
	checkpointBucket               = []byte("check-point")
	powchainBucket                 = []byte("powchain")
	stateValidatorsBucket          = []byte("state-validators")
	feeRecipientBucket             = []byte("fee-recipient")
	registrationBucket             = []byte("registration")
	executionPayloadEnvelopeBucket = []byte("execution-payload-envelope")

	// Light Client Updates Bucket
	lightClientUpdatesBucket = []byte("light-client-updates")

	// Deprecated: This bucket was migrated in PR 6461. Do not use, except for migrations.
	slotsHasObjectBucket = []byte("slots-has-objects")
	// Deprecated: This bucket was migrated in PR 6461. Do not use, except for migrations.
	archivedRootBucket = []byte("archived-index-root")

	// Key indices buckets.
	blockParentRootIndicesBucket   = []byte("block-parent-root-indices")
	blockSlotIndicesBucket         = []byte("block-slot-indices")
	stateSlotIndicesBucket         = []byte("state-slot-indices")
	finalizedBlockRootsIndexBucket = []byte("finalized-block-roots-index")
	blockRootValidatorHashesBucket = []byte("block-root-validator-hashes")

	// Specific item keys.
	headBlockRootKey           = []byte("head-root")
	genesisBlockRootKey        = []byte("genesis-root")
	depositContractAddressKey  = []byte("deposit-contract")
	justifiedCheckpointKey     = []byte("justified-checkpoint")
	finalizedCheckpointKey     = []byte("finalized-checkpoint")
	powchainDataKey            = []byte("powchain-data")
	lastValidatedCheckpointKey = []byte("last-validated-checkpoint")

	// Below keys are used to identify objects are to be fork compatible.
	// Objects that are only compatible with specific forks should be prefixed with such keys.
	altairKey                  = []byte("altair")
	bellatrixKey               = []byte("merge")
	bellatrixBlindKey          = []byte("blind-bellatrix")
	capellaKey                 = []byte("capella")
	capellaBlindKey            = []byte("blind-capella")
	saveBlindedBeaconBlocksKey = []byte("save-blinded-beacon-blocks")
	denebKey                   = []byte("deneb")
	denebBlindKey              = []byte("blind-deneb")
	electraKey                 = []byte("electra")
	electraBlindKey            = []byte("blind-electra")
	epbsKey                    = []byte("epbs")

	// block root included in the beacon state used by weak subjectivity initial sync
	originCheckpointBlockRootKey = []byte("origin-checkpoint-block-root")
	// tracking data about an ongoing backfill
	backfillStatusKey = []byte("backfill-status")

	// Deprecated: This index key was migrated in PR 6461. Do not use, except for migrations.
	lastArchivedIndexKey = []byte("last-archived")
	// Deprecated: This index key was migrated in PR 6461. Do not use, except for migrations.
	savedStateSlotsKey = []byte("saved-state-slots")

	// Migrations
	migrationsBucket = []byte("migrations")
)
