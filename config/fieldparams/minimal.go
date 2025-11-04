//go:build minimal

package field_params

const (
	Preset                                = "minimal"
	BlockRootsLength                      = 64                // SLOTS_PER_HISTORICAL_ROOT
	StateRootsLength                      = 64                // SLOTS_PER_HISTORICAL_ROOT
	RandaoMixesLength                     = 64                // EPOCHS_PER_HISTORICAL_VECTOR
	HistoricalRootsLength                 = 16777216          // HISTORICAL_ROOTS_LIMIT
	ValidatorRegistryLimit                = 1099511627776     // VALIDATOR_REGISTRY_LIMIT
	Eth1DataVotesLength                   = 32                // SLOTS_PER_ETH1_VOTING_PERIOD
	PreviousEpochAttestationsLength       = 1024              // MAX_ATTESTATIONS * SLOTS_PER_EPOCH
	CurrentEpochAttestationsLength        = 1024              // MAX_ATTESTATIONS * SLOTS_PER_EPOCH
	SlashingsLength                       = 64                // EPOCHS_PER_SLASHINGS_VECTOR
	SyncCommitteeLength                   = 32                // SYNC_COMMITTEE_SIZE
	RootLength                            = 32                // RootLength defines the byte length of a Merkle root.
	BLSSignatureLength                    = 96                // BLSSignatureLength defines the byte length of a BLSSignature.
	BLSPubkeyLength                       = 48                // BLSPubkeyLength defines the byte length of a BLSSignature.
	MaxTxsPerPayloadLength                = 1048576           // MaxTxsPerPayloadLength defines the maximum number of transactions that can be included in a payload.
	MaxBytesPerTxLength                   = 1073741824        // MaxBytesPerTxLength defines the maximum number of bytes that can be included in a transaction.
	FeeRecipientLength                    = 20                // FeeRecipientLength defines the byte length of a fee recipient.
	LogsBloomLength                       = 256               // LogsBloomLength defines the byte length of a logs bloom.
	VersionLength                         = 4                 // VersionLength defines the byte length of a fork version number.
	SlotsPerEpoch                         = 8                 // SlotsPerEpoch defines the number of slots per epoch.
	SyncCommitteeAggregationBytesLength   = 1                 // SyncCommitteeAggregationBytesLength defines the sync committee aggregate bytes.
	SyncAggregateSyncCommitteeBytesLength = 4                 // SyncAggregateSyncCommitteeBytesLength defines the length of sync committee bytes in a sync aggregate.
	MaxWithdrawalsPerPayload              = 4                 // MaxWithdrawalsPerPayloadLength defines the maximum number of withdrawals that can be included in a payload.
	MaxBlobCommitmentsPerBlock            = 4096              // MaxBlobCommitmentsPerBlock defines the theoretical limit of blobs can be included in a block.
	LogMaxBlobCommitments                 = 12                // Log_2 of MaxBlobCommitmentsPerBlock
	BlobLength                            = 131072            // BlobLength defines the byte length of a blob.
	BlobSize                              = 131072            // defined to match blob.size in bazel ssz codegen
	BlobSidecarSize                       = 131928            // defined to match blob sidecar size in bazel ssz codegen
	KzgCommitmentSize                     = 48                // KzgCommitmentSize defines the byte length of a KZG commitment.
	KzgCommitmentInclusionProofDepth      = 17                // Merkle proof depth for blob_kzg_commitments list item
	ExecutionBranchDepth                  = 4                 // ExecutionBranchDepth defines the number of leaves in a merkle proof of the execution payload header.
	SyncCommitteeBranchDepth              = 5                 // SyncCommitteeBranchDepth defines the number of leaves in a merkle proof of a sync committee.
	SyncCommitteeBranchDepthElectra       = 6                 // SyncCommitteeBranchDepthElectra defines the number of leaves in a merkle proof of a sync committee.
	FinalityBranchDepth                   = 6                 // FinalityBranchDepth defines the number of leaves in a merkle proof of the finalized checkpoint root.
	FinalityBranchDepthElectra            = 7                 // FinalityBranchDepthElectra defines the number of leaves in a merkle proof of the finalized checkpoint root.
	PendingDepositsLimit                  = 134217728         // Maximum number of pending balance deposits in the beacon state.
	PendingPartialWithdrawalsLimit        = 64                // Maximum number of pending partial withdrawals in the beacon state.
	PendingConsolidationsLimit            = 64                // Maximum number of pending consolidations in the beacon state.
	MaxAttesterSlashingsElectra           = 1                 // Maximum number of attester slashings in a block.
	MaxRandomByte                         = uint64(1<<8 - 1)  // Maximum value for a random value using for proposer and sync committee sampling.
	MaxRandomValueElectra                 = uint64(1<<16 - 1) // Maximum value for a random value using for proposer and sync committee sampling.

	// Introduced in Fulu network upgrade.
	NumberOfColumns = 128 // NumberOfColumns refers to the specified number of data columns that can exist in a network.
	CellsPerBlob    = 64  // CellsPerBlob refers to the number of cells in a (non-extended) blob.
)
