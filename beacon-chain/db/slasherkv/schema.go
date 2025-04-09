package slasherkv

// The schema will define how to store and retrieve data from the db.
// we can prefix or suffix certain values such as `block` with attributes
// for prefix-wide scans across the underlying BoltDB buckets when filtering data.
// For example, we might store attestations as shard + attestation_root -> attestation, making
// it easy to scan for keys that have a certain shard number as a prefix and return those
// corresponding attestations.
var (

	// key: (encoded) ValidatorIndex
	// value: (encoded) Epoch
	attestedEpochsByValidator = []byte("attested-epochs-by-validator")

	// key: attestation SigningRoot
	// value: (encoded + compressed) IndexedAttestation
	attestationRecordsBucket = []byte("attestation-records")

	// key: (encoded) Target Epoch + (encoded) ValidatorIndex
	// value: attestation SigningRoot
	attestationDataRootsBucket = []byte("attestation-data-roots")

	// key: Slot+ValidatorIndex
	// value: (encoded) SignedBlockHeaderWrapper
	proposalRecordsBucket = []byte("proposal-records")
	slasherChunksBucket   = []byte("slasher-chunks")
)
