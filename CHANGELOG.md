# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project adheres to Semantic Versioning.

## [Unreleased](https://github.com/prysmaticlabs/prysm/compare/v5.1.2...HEAD)

### Added

- Electra EIP6110: Queue deposit [pr](https://github.com/prysmaticlabs/prysm/pull/14430)
- Add Bellatrix tests for light client functions.
- Add Discovery Rebooter Feature.
- Added GetBlockAttestationsV2 endpoint.
- Light client support: Consensus types for Electra
- Added SubmitPoolAttesterSlashingV2 endpoint.
- Added SubmitAggregateAndProofsRequestV2 endpoint.
- Updated the `beacon-chain/monitor` package to Electra. [PR](https://github.com/prysmaticlabs/prysm/pull/14562)

### Changed

- Electra EIP6110: Queue deposit requests changes from consensus spec pr #3818
- reversed the boolean return on `BatchVerifyDepositsSignatures`, from need verification, to all keys successfully verified
- Fix `engine_exchangeCapabilities` implementation.
- Updated the default `scrape-interval` in `Client-stats` to 2 minutes to accommodate Beaconcha.in API rate limits.
- Switch to compounding when consolidating with source==target.
- Revert block db save when saving state fails.
- Return false from HasBlock if the block is being synced. 
- Cleanup forkchoice on failed insertions.
- Use read only validator for core processing to avoid unnecessary copying.
- Use ROBlock across block processing pipeline.
- Added missing Eth-Consensus-Version headers to GetBlockAttestationsV2 and GetAttesterSlashingsV2 endpoints.

### Deprecated

- `/eth/v1alpha1/validator/activation/stream` grpc wait for activation stream is deprecated. [pr](https://github.com/prysmaticlabs/prysm/pull/14514)

### Removed

- Removed finalized validator index cache, no longer needed.

### Fixed

- Fixed mesh size by appending `gParams.Dhi = gossipSubDhi`
- Fix skipping partial withdrawals count.
- wait for the async StreamEvent writer to exit before leaving the http handler, avoiding race condition panics [pr](https://github.com/prysmaticlabs/prysm/pull/14557)
- Certain deb files were returning a 404 which made building new docker images without an existing
  cache impossible. This has been fixed with updates to rules_oci and bazel-lib.
- Fixed an issue where the length check between block body KZG commitments and the existing cache from the database was incompatible.

### Security

## [v5.1.2](https://github.com/prysmaticlabs/prysm/compare/v5.1.1...v5.1.2) - 2024-10-16 

This is a hotfix release with one change. 

Prysm v5.1.1 contains an updated implementation of the beacon api streaming events endpoint. This
new implementation contains a bug that can cause a panic in certain conditions. The issue is
difficult to reproduce reliably and we are still trying to determine the root cause, but in the
meantime we are issuing a patch that recovers from the panic to prevent the node from crashing.

This only impacts the v5.1.1 release beacon api event stream endpoints. This endpoint is used by the
prysm REST mode validator (a feature which requires the validator to be configured to use the beacon
api intead of prysm's stock grpc endpoints) or accessory software that connects to the events api,
like https://github.com/ethpandaops/ethereum-metrics-exporter

### Fixed 

- Recover from panics when writing the event stream [#14545](https://github.com/prysmaticlabs/prysm/pull/14545)

## [v5.1.1](https://github.com/prysmaticlabs/prysm/compare/v5.1.0...v5.1.1) - 2024-10-15

This release has a number of features and improvements. Most notably, the feature flag 
`--enable-experimental-state` has been flipped to "opt out" via `--disable-experimental-state`. 
The experimental state management design has shown significant improvements in memory usage at
runtime. Updates to libp2p's gossipsub have some bandwidith stability improvements with support for
IDONTWANT control messages. 

The gRPC gateway has been deprecated from Prysm in this release. If you need JSON data, consider the
standardized beacon-APIs. 

Updating to this release is recommended at your convenience.

### Added

- Aggregate and proof committee validation for Electra.
- More tests for electra field generation.
- Light client support: Implement `ComputeFieldRootsForBlockBody`.
- Light client support: Add light client database changes.
- Light client support: Implement capella and deneb changes.
- Light client support: Implement `BlockToLightClientHeader` function.
- Light client support: Consensus types.
- GetBeaconStateV2: add Electra case.
- Implement [consensus-specs/3875](https://github.com/ethereum/consensus-specs/pull/3875).
- Tests to ensure sepolia config matches the official upstream yaml.
- `engine_newPayloadV4`,`engine_getPayloadV4` used for electra payload communication with execution client.  [pr](https://github.com/prysmaticlabs/prysm/pull/14492)
- HTTP endpoint for PublishBlobs.
- GetBlockV2, GetBlindedBlock, ProduceBlockV2, ProduceBlockV3: add Electra case.
- Add Electra support and tests for light client functions.
- fastssz version bump (better error messages).
- SSE implementation that sheds stuck clients. [pr](https://github.com/prysmaticlabs/prysm/pull/14413)
- Added GetPoolAttesterSlashingsV2 endpoint.
- Use engine API get-blobs for block subscriber to reduce block import latency and potentially reduce bandwidth.

### Changed

- Electra: Updated interop genesis generator to support Electra.
- `getLocalPayload` has been refactored to enable work in ePBS branch.
- `TestNodeServer_GetPeer` and `TestNodeServer_ListPeers` test flakes resolved by iterating the whole peer list to find
  a match rather than taking the first peer in the map.
- Passing spectests v1.5.0-alpha.4 and v1.5.0-alpha.5.
- Beacon chain now asserts that the external builder block uses the expected gas limit.
- Electra: Add electra objects to beacon API.
- Electra: Updated block publishing beacon APIs to support Electra.
- "Submitted builder validator registration settings for custom builders" log message moved to debug level.
- config: Genesis validator root is now hardcoded in params.BeaconConfig()
- `grpc-gateway-host` is renamed to http-host. The old name can still be used as an alias.
- `grpc-gateway-port` is renamed to http-port. The old name can still be used as an alias.
- `grpc-gateway-corsdomain` is renamed to http-cors-domain. The old name can still be used as an alias.
- `api-timeout` is changed from int flag to duration flag, default value updated.
- Light client support: abstracted out the light client headers with different versions.
- `ApplyToEveryValidator` has been changed to prevent misuse bugs, it takes a closure that takes a `ReadOnlyValidator` and returns a raw pointer to a `Validator`. 
- Removed gorilla mux library and replaced it with net/http updates in go 1.22.
- Clean up `ProposeBlock` for validator client to reduce cognitive scoring and enable further changes.
- Updated k8s-io/client-go to v0.30.4 and k8s-io/apimachinery to v0.30.4
- Migrated tracing library from opencensus to opentelemetry for both the beacon node and validator.
- Refactored light client code to make it more readable and make future PRs easier.
- Update light client helper functions to reference `dev` branch of CL specs
- Updated Libp2p Dependencies to allow prysm to use gossipsub v1.2 .
- Updated Sepolia bootnodes.
- Make committee aware packing the default by deprecating `--enable-committee-aware-packing`.
- Moved `ConvertKzgCommitmentToVersionedHash` to the `primitives` package.
- Updated correlation penalty for EIP-7251. 

### Deprecated
- `--disable-grpc-gateway` flag is deprecated due to grpc gateway removal.
- `--enable-experimental-state` flag is deprecated. This feature is now on by default. Opt-out with `--disable-experimental-state`.

### Removed

- Removed gRPC Gateway.
- Removed unused blobs bundle cache.
- Removed consolidation signing domain from params. The Electra design changed such that EL handles consolidation signature verification.
- Remove engine_getPayloadBodiesBy{Hash|Range}V2

### Fixed

- Fixed early release of read lock in BeaconState.getValidatorIndex.
- Electra: resolve inconsistencies with validator committee index validation.
- Electra: build blocks with blobs.
- E2E: fixed gas limit at genesis
- Light client support: use LightClientHeader instead of BeaconBlockHeader.
- validator registration log changed to debug, and the frequency of validator registration calls are reduced
- Core: Fix process effective balance update to safe copy validator for Electra.
- `== nil` checks before calling `IsNil()` on interfaces to prevent panics.
- Core: Fixed slash processing causing extra hashing.
- Core: Fixed extra allocations when processing slashings.
- remove unneeded container in blob sidecar ssz response
- Light client support: create finalized header based on finalizedBlock's version, not attestedBlock.
- Light client support: fix light client attested header execution fields' wrong version bug.
- Testing: added custom matcher for better push settings testing.
- Registered `GetDepositSnapshot` Beacon API endpoint.

### Security

No notable security updates.

## [v5.1.0](https://github.com/prysmaticlabs/prysm/compare/v5.0.4...v5.1.0) - 2024-08-20

This release contains 171 new changes and many of these are related to Electra! Along side the Electra changes, there
are nearly 100 changes related to bug fixes, feature additions, and other improvements to Prysm. Updating to this
release is recommended at your convenience.

⚠️ Deprecation Notice: Removal of gRPC Gateway and Gateway Flag Renaming ⚠️

In an upcoming release, we will be deprecating the gRPC gateway and renaming several associated flags. This change will
result in the removal of access to several internal APIs via REST, though the gRPC endpoints will remain unaffected. We
strongly encourage systems to transition to using the beacon API endpoints moving forward. Please refer to PR for more
details.

### Added

- Electra work
- Fork-specific consensus-types interfaces
- Fuzz ssz roundtrip marshalling, cloner fuzzing
- Add support for multiple beacon nodes in the REST API
- Add middleware for Content-Type and Accept headers
- Add debug logs for proposer settings
- Add tracing to beacon api package
- Add support for persistent validator keys when using remote signer. --validators-external-signer-public-keys and
  --validators-external-signer-key-file See the docs page for more info.
- Add AggregateKeyFromIndices to beacon state to reduce memory usage when processing attestations
- Add GetIndividualVotes endpoint
- Implement is_better_update for light client
- HTTP endpoint for GetValidatorParticipation
- HTTP endpoint for GetChainHead
- HTTP endpoint for GetValidatorActiveSetChanges
- Check locally for min-bid and min-bid-difference

### Changed

- Refactored slasher operations to their logical order
- Refactored Gwei and Wei types from math to primitives package.
- Unwrap payload bid from ExecutionData
- Change ZeroWei to a func to avoid shared ptr
- Updated go-libp2p to v0.35.2 and go-libp2p-pubsub to v0.11.0
- Use genesis block root in epoch 1 for attester duties
- Cleanup validator client code
- Old attestations log moved to debug. "Attestation is too old to broadcast, discarding it"
- Modify ProcessEpoch not to return the state as a returned value
- Updated go-bitfield to latest release
- Use go ticker instead of timer
- process_registry_updates no longer makes a full copy of the validator set
- Validator client processes sync committee roll separately
- Use vote pointers in forkchoice to reduce memory churn
- Avoid Cloning When Creating a New Gossip Message
- Proposer filters invalid attestation signatures
- Validator now pushes proposer settings every slot
- Get all beacon committees at once
- Committee-aware attestation packing

### Deprecated

- `--enable-debug-rpc-endpoints` is deprecated and debug rpc points are on by default.

### Removed

- Removed fork specific getter functions (i.e. PbCapellaBlock, PbDenebBlock, etc)

### Fixed

- Fixed debug log "upgraded stake to $fork" to only log on upgrades instead of every state transition
- Fixed nil block panic in API
- Fixed mockgen script
- Do not fail to build block when block value is unknown
- Fix prysmctl TUI when more than 20 validators were listed
- Revert peer backoff changes from. This was causing some sync committee performance issues.
- Increased attestation seen cache expiration to two epochs
- Fixed slasher db disk usage leak
- fix: Multiple network flags should prevent the BN to start
- Correctly handle empty payload from GetValidatorPerformance requests
- Fix Event stream with carriage return support
- Fix panic on empty block result in REST API
- engine_getPayloadBodiesByRangeV1 - fix, adding hexutil encoding on request parameters
- Use sync committee period instead of epoch in `createLightClientUpdate`

### Security

- Go version updated to 1.22

## [v5.0.4](https://github.com/prysmaticlabs/prysm/compare/v5.0.3...v5.0.4) - 2024-07-21

This release has many wonderful bug fixes and improvements. Some highlights include p2p peer fix for windows users,
beacon API fix for retrieving blobs older than the minimum blob retention period, and improvements to initial sync by
avoiding redundant blob downloads.

Updating to this release is recommended at your earliest convenience, especially for windows users.

### Added

- Beacon-api: broadcast blobs in the event of seen block
- P2P: Add QUIC support

### Changed

- Use slices package for various slice operations
- Initsync skip local blobs
- Use read only validators in Beacon API
- Return syncing status when node is optimistic
- Upgrade the Beacon API e2e evaluator
- Don't return error that can be internally handled
- Allow consistent auth token for validator apis
- Change example.org DNS record
- Simplify prune invalid by reusing existing fork choice store call
- use [32]byte keys in the filesystem cache
- Update Libp2p Dependencies
- Parallelize Broadcasting And Processing Each Blob
- Substantial VC cleanup
- Only log error when aggregator check fails
- Update Libp2p Dependencies
- Change Attestation Log To Debug
- update codegen dep and cleanup organization

### Deprecated

- Remove eip4881 flag (--disable-eip-4881)

### Removed

- Remove the Goerli/Prater support
- Remove unused IsViableForCheckpoint
- Remove unused validator map copy method

### Fixed

- Various typos and other cosmetic fixes
- Send correct state root with finalized event stream
- Extend Broadcast Window For Attestations
- Beacon API: Use retention period when fetching blobs
- Backfill throttling
- Use correct port for health check in Beacon API e2e evaluator
- Do not remove blobs DB in slasher.
- use time.NewTimer() to avoid possible memory leaks
- paranoid underflow protection without error handling
- Fix CommitteeAssignments to not return every validator
- Fix dependent root retrival genesis case
- Restrict Dials From Discovery
- Always close cache warm chan to prevent blocking
- Keep only the latest value in the health channel

### Security

- Bump golang.org/x/net from 0.21.0 to 0.23.0

## [v5.0.3](https://github.com/prysmaticlabs/prysm/compare/v5.0.2...v5.0.3) - 2024-04-04

Prysm v5.0.3 is a small patch release with some nice additions and bug fixes. Updating to this release is recommended
for users on v5.0.0 or v5.0.1. There aren't many changes since last week's v5.0.2 so upgrading is not strictly required,
but there are still improvements in this release so update if you can!

### Added

- Testing: spec test coverage tool
- Add bid value metrics
- prysmctl: Command-line interface for visualizing min/max span bucket
- Explicit Peering Agreement implementation

### Changed

- Utilize next slot cache in block rewards rpc
- validator: Call GetGenesis only once when using beacon API
- Simplify ValidateAttestationTime
- Various typo / commentary improvements
- Change goodbye message from rate limited peer to debug verbosity
- Bump libp2p to v0.33.1
- Fill in missing debug logs for blob p2p IGNORE/REJECT

### Fixed

- Remove check for duplicates in pending attestation queue
- Repair finalized index issue
- Maximize Peer Capacity When Syncing
- Reject Empty Bundles

### Security

No security updates in this release.

## [v5.0.2](https://github.com/prysmaticlabs/prysm/compare/v5.0.1...v5.0.2) - 2024-03-27

This release has many optimizations, UX improvements, and bug fixes. Due to the number of important bug fixes and
optimizations, we encourage all operators to update to v5.0.2 at their earliest convenience.

In this release, there is a notable change to the default value of --local-block-value-boost from 0 to 10. This means
that the default behavior of using the builder API / mev-boost requires the builder bid to be 10% better than your local
block profit. If you want to preserve the existing behavior, set --local-block-value-boost=0.

### Added

- API: Add support for sync committee selections
- blobs: call fsync between part file write and rename (feature flag --blob-save-fsync)
- Implement EIP-3076 minimal slashing protection, using a filesystem database (feature flag
  --enable-minimal-slashing-protection)
- Save invalid block to temp --save-invalid-block-temp
- Compute unrealized checkpoints with pcli
- Add gossip blob sidecar verification ms metric
- Backfill min slot flag (feature flag --backfill-oldest-slot)
- adds a metric to track blob sig cache lookups
- Keymanager APIs - get,post,delete graffiti
- Set default LocalBlockValueBoost to 10
- Add bid value metrics
- REST VC metrics

### Changed

- Normalized checkpoint logs
- Normalize filesystem/blob logs
- Updated gomock libraries
- Use Max Request Limit in Initial Sync
- Do not Persist Startup State
- Normalize backfill logs/errors
- Unify log fields
- Do Not Compute Block Root Again
- Optimize Adding Dirty Indices
- Use a Validator Reader When Computing Unrealized Balances
- Copy Validator Field Trie
- Do not log zero sync committee messages
- small cleanup on functions: use slots.PrevSlot
- Set the log level for running on <network> as INFO.
- Employ Dynamic Cache Sizes
- VC: Improve logging in case of fatal error
- refactoring how proposer settings load into validator client
- Spectest: Unskip Merkle Proof test
- Improve logging.
- Check Unrealized Justification Balances In Spectests
- Optimize SubscribeCommitteeSubnets VC action
- Clean up unreachable code; use new(big.Int) instead of big.NewInt(0)
- Update bazel, rules_go, gazelle, and go versions
- replace receive slot with event stream
- New gossip cache size
- Use headstate for recent checkpoints
- Update spec test to official 1.4.0
- Additional tests for KZG commitments
- Enable Configurable Mplex Timeouts
- Optimize SubmitAggregateSelectionProof VC action
- Re-design TestStartDiscV5_DiscoverPeersWithSubnets test
- Add da waited time to sync block log
- add log message if in da check at slot end
- Log da block root in hex
- Log the slot and blockroot when we deadline waiting for blobs
- Modify the algorithm of updateFinalizedBlockRoots
- Rename payloadattribute Timestamps to Timestamp
- Optimize GetDuties VC action
- docker: Add bazel target for building docker tarball
- Utilize next slot cache in block rewards rpc
- Spec test coverage report
- Refactor batch verifier for sharing across packages

### Removed

- Remove unused bolt buckets
- config: Remove DOMAIN_BLOB_SIDECAR.
- Remove unused deneb code
- Clean up: remove some unused beacon state protos
- Cleaned up code in the sync package
- P2P: Simplify code

### Fixed

- Slasher: Reduce surrounding/surrounded attestations processing time
- Fix blob batch verifier pointer receiver
- db/blobs: Check non-zero data is written to disk
- avoid part path collisions with mem addr entropy
- Download checkpoint sync origin blobs in init-sync
- bazel: Update aspect-build/bazel-lib to v2.5.0
- move setting route handlers to registration from start
- Downgrade Level DB to Stable Version
- Fix failed reorg log
- Fix Data Race in Epoch Boundary
- exit blob fetching for cp block if outside retention
- Do not check parent weight on early FCU
- Fix VC DB conversion when no proposer settings is defined and add Experimental flag in the
  --enable-minimal-slashing-protection help.
- keymanager api: lowercase statuses
- Fix unrealized justification
- fix race condition when pinging peers
- Fix/race receive block
- Blob verification spectest
- Ignore Pubsub Messages Hitting Context Deadlines
- Use justified checkpoint from head state to build attestation
- only update head at 10 seconds when validating
- Use correct gossip validation time
- fix 1-worker underflow; lower default batch size
- handle special case of batch size=1
- Always Set Inprogress Boolean In Cache
- Builder APIs: adding headers to post endpoint
- Rename mispelled variable
- allow blob by root within da period
- Rewrite Pruning Implementation To Handle EIP 7045
- Set default fee recipient if tracked val fails
- validator client on rest mode has an inappropriate context deadline for events
- validator client should set beacon API endpoint in configurations
- Fix get validator endpoint for empty query parameters
- Expand Our TTL for our Message ID Cache
- fix some typos
- fix handling of goodbye messages for limited peers
- create the log file along with its parent directory if not present
- Call GetGenesis only once

### Security

- Go version has been updated from 1.21.6 to 1.21.8.

## [v5.0.1](https://github.com/prysmaticlabs/prysm/compare/v5.0.0...v5.0.1) - 2024-03-08

This minor patch release has some nice improvements over the recent v5.0.0 for Deneb. We have minimized this patch
release to include only low risk and valuable fixes or features ahead of the upcoming network upgrade on March 13th.

Deneb is scheduled for mainnet epoch 269568 on March 13, 2024 at 01:55:35pm UTC. All operators MUST update their Prysm
software to v5.0.0 or later before the upgrade in order to continue following the blockchain.

### Added

- A new flag to ensure that blobs are flushed to disk via fsync immediately after write. --blob-save-fsync

### Changed

- Enforce a lower maximum batch limit value to prevent annoying peers
- Download blobs for checkpoint sync block before starting sync
- Set justified epoch to the finalized epoch in Goerli to unstuck some Prysm nodes on Goerli

### Fixed

- Data race in epoch boundary cache
- "Failed reorg" log was misplaced
- Do not check parent weights on early fork choice update calls
- Compute unrealized justification with slashed validators
- Missing libxml dependency

### Security

Prysm version v5.0.0 or later is required to maintain participation in the network after the Deneb upgrade.

## [v5.0.0](https://github.com/prysmaticlabs/prysm/compare/v4.2.1...v5.0.0)

Behold the Prysm v5 release with official support for Deneb on Ethereum mainnet!

Deneb is scheduled for mainnet epoch 269568 on March 13, 2024 at 01:55:35pm UTC. All operators MUST update their Prysm
software to v5.0.0 or later before the upgrade in order to continue following the blockchain.

This release brings improvements to the backfill functionality of the beacon node to support backfilling blobs. If
running a beacon node with checkpoint sync, we encourage you to test the backfilling functionality and share your
feedback. Run with backfill enabled using the flag --enable-experimental-backfill.

Known Issues

- --backfill-batch-size with a value of 1 or less breaks backfill.
- Validator client on v4.2.0 or older uses some API methods that are incompatible with beacon node v5. Ensure that you
  have updated the beacon node and validator client to v4.2.1 and then upgrade to v5 or update both processes at the
  same time to minimize downtime.

### Added

- Support beacon_committee_selections
- /eth/v1/beacon/deposit_snapshot
- Docker images now have coreutils pre-installed
- da_waited_time_milliseconds tracks total time waiting for data availablity check in ReceiveBlock
- blob_written, blob_disk_count, blob_disk_bytes new metrics for tracking blobs on disk
- Backfill supports blob backfilling
- Add mainnet deneb fork epoch config

### Changed

- --clear-db and --force-clear-db flags now remove blobs as well as beaconchain.db
- EIP-4881 is now on by default.
- Updates filtering logic to match spec
- Verbose signature verification is now on by default
- gossip_block_arrival_milliseconds and gossip_block_verification_milliseconds measure in
- milliseconds instead of nanoseconds
- aggregate_attestations_t1 histogram buckets have been updated
- Reduce lookahead period from 8 to 4. This reduces block batch sizes during sync to account for
- larger blocks in deneb.
- Update gohashtree to v0.0.4-beta
- Various logging improvements
- Improved operations during syncing
- Backfill starts after initial-sync is complete

### Deprecated

The following flags have been removed entirely:

- --enable-reorg-late-blocks
- --disable-vectorized-htr
- --aggregate-parallel
- --build-block-parallel
- --enable-registration-cache, disable-gossip-batch-aggregation
- --safe-slots-to-import-optimistically
- --show-deposit-data

### Removed

- Prysm gRPC slasher endpoints are removed
- Remove /eth/v1/debug/beacon/states/{state_id}
- Prysm gRPC endpoints that were marked as deprecated in v4 have been removed
- Remove /eth/v1/beacon/blocks/{block_id}

### Fixed

- Return unaggregated if no aggregated attestations available in GetAggregateAttestation
- Fix JWT auth checks in certain API endpoints used by the web UI
- Return consensus block value in wei units
- Minor fixes in protobuf files
- Fix 500 error when requesting blobs from a block without blobs
- Handle cases were EL client is syncing and unable to provide payloads
- /eth/v1/beacon/blob_sidecars/{block_id} correctly returns an error when invalid indices are requested
- Fix head state fetch when proposing a failed reorg
- Fix data race in background forkchoice update call
- Correctly return "unavailable" response to peers requesting batches before the node completes
- backfill.
- Many significant improvements and fixes to the prysm slasher
- Fixed slashing gossip checks, improves peer scores for slasher peers
- Log warning if attempting to exit more than 5 validators at a time
- Do not cache inactive public keys
- Validator exits prints testnet URLs
- Fix pending block/blob zero peer edge case
- Check non-zero blob data is written to disk
- Avoid blob partial filepath collisions with mem addr entropy

### Security

v5.0.0 of Prysm is required to maintain participation in the network after the Deneb upgrade.

## [v4.2.1](https://github.com/prysmaticlabs/prysm/compare/v4.2.0...v4.2.1) - 2024-01-29

Welcome to Prysm Release v4.2.1! This release is highly recommended for stakers and node operators, possibly being the
final update before V5.

⚠️ This release will cause failures on Goerli, Sepolia and Holeski testnets, when running on certain older CPUs without
AVX support (eg Celeron) after the Deneb fork. This is not an issue for mainnet.

### Added

- Linter: Wastedassign linter enabled to improve code quality.
- API Enhancements:
    - Added payload return in Wei for /eth/v3/validator/blocks.
    - Added Holesky Deneb Epoch for better epoch management.
- Testing Enhancements:
    - Clear cache in tests of core helpers to ensure test reliability.
    - Added Debug State Transition Method for improved debugging.
    - Backfilling test: Enabled backfill in E2E tests for more comprehensive coverage.
- API Updates: Re-enabled jwt on keymanager API for enhanced security.
- Logging Improvements: Enhanced block by root log for better traceability.
- Validator Client Improvements:
    - Added Spans to Core Validator Methods for enhanced monitoring.
    - Improved readability in validator client code for better maintenance (various commits).

### Changed

- Optimizations and Refinements:
    - Lowered resource usage in certain processes for efficiency.
    - Moved blob rpc validation closer to peer read for optimized processing.
    - Cleaned up validate beacon block code for clarity and efficiency.
    - Updated Sepolia Deneb fork epoch for alignment with network changes.
    - Changed blob latency metrics to milliseconds for more precise measurement.
    - Altered getLegacyDatabaseLocation message for better clarity.
    - Improved wait for activation method for enhanced performance.
    - Capitalized Aggregated Unaggregated Attestations Log for consistency.
    - Modified HistoricalRoots usage for accuracy.
    - Adjusted checking of attribute emptiness for efficiency.
- Database Management:
    - Moved --db-backup-output-dir as a deprecated flag for database management simplification.
    - Added the Ability to Defragment the Beacon State for improved database performance.
- Dependency Update: Bumped quic-go version from 0.39.3 to 0.39.4 for up-to-date dependencies.

### Removed

- Removed debug setting highest slot log to clean up the logging process.
- Deleted invalid blob at block processing for data integrity.

### Fixed

- Bug Fixes:
    - Fixed off by one error for improved accuracy.
    - Resolved small typo in error messages for clarity.
    - Addressed minor issue in blsToExecChange validator for better validation.
    - Corrected blobsidecar json tag for commitment inclusion proof.
    - Fixed ssz post-requests content type check.
    - Resolved issue with port logging in bootnode.
- Test Fixes: Re-enabled Slasher E2E Test for more comprehensive testing.

### Security

No security issues in this release.

## [v4.2.0](https://github.com/prysmaticlabs/prysm/compare/v4.1.1...v4.2.0) - 2024-01-11

Happy new year! We have an incredibly exciting release to kick off the new year. This release is **strongly recommended
** for all operators to update as it has many bug fixes, security patches, and features that will improve the Prysm
experience on mainnet. This release has so many wonderful changes that we've deviated from our normal release notes
format to aptly categorize the changes.

### Highlights

#### Upgrading / Downgrading Validators

There are some API changes bundled in this release that require you to upgrade or downgrade in particular order. If the
validator is updated before the beacon node, it will see repeated 404 errors at start up until the beacon node is
updated as it uses a new API endpoint introduced in v4.2.0.

:arrow_up_small:  **Upgrading**: Upgrade the beacon node, then the validator.
:arrow_down_small: **Downgrading**: Downgrade the validator to v4.1.1 then downgrade the beacon node.

#### Deneb Goerli Support

This release adds in full support for the upcoming deneb hard fork on goerli next week on January 17th.

#### Networking Parameter Changes

This release increases the default peer count to 70 from 45. The reason this is done is so that node's running
with default peer counts can perform their validator duties as expected. Users who want to use the old peer count
can add in `--p2p-max-peers=45` as a flag.

#### Profile Guided Optimization

This release has binaries built using PGO, for more information on how it works feel free to look
here: https://tip.golang.org/doc/pgo .
This allows the go compiler to build more optimized Prysm binaries using production profiles and workloads.

#### ARM Supported Docker Images

Our docker images now support amd64 and arm64 architecture! This long awaited feature is finally here for Apple Silicon
and Raspberry Pi users.

### Deneb

#### Core

- Use ROForkchoice in blob verifier
- Add Goerli Deneb Fork Epoch
- Use deneb key for deneb state in saveStatesEfficientInternal
- Initialize Inactivity Scores Correctly
- Excluse DA wait time for chain processing time
- Initialize sig cache for verification.Initializer
- Verify roblobs
- KZG Commitment inclusion proof verifier
- Merkle Proofs of KZG commitments
- Add RO blob sidecar
- Check blob index duplication for blob notifier
- Remove sidecars with invalid proofs
- Proposer: better handling of blobs bundle
- Update proposer RPC to new blob sidecar format
- Implement Slot-Dependent Caching for Blobs Bundle
- Verified roblobs

#### Networking

- Check sidecar index in BlobSidecarsByRoot response
- Use proposer index cache for blob verification
- VerifiedROBlobs in initial-sync
- Reordered blob validation
- Initialize blob storage for initial sync service
- Use verified blob for gossip checks
- Update broadcast method to use `BlobSidecar` instead of `SingedBlobSidecar`
- Remove pending blobs queue
- Reject Blob Sidecar Incorrect Index
- Check return and request lengths for blob sidecar by root
- Fix blob sidecar subnet check
- Add pending blobs queue for missing parent block
- Verify blobs that arrived from by root request
- Reject blobs with invalid parent
- Add more blob and block checks for by range
- Exit early if blob by root request is empty
- Request missing blobs while processing pending queue
- Check blob exists before requesting from peer
- Passing block as arugment for sidecar validation

#### Blob Management

- Remove old blob types
- minimize syscalls in pruning routine
- Prune dangling blob
- Use Afero Walk for Pruning Blob
- Initialize blob storage without pruning
- Fix batch pruning errors
- Blob filesystem add pruning during blob write
- Blob filesystem add pruning at startup
- Ensure partial blob is deleted if there's an error
- Split blob pruning into two funcs
- Use functional options for `--blob-retention-epochs`
- Blob filesystem: delete blobs
- Fix Blob Storage Path
- Add blob getters
- Blob filesystem: Save Blobs
- Blob filesystem: prune blobs
- blobstorage: Improve mkdirall error

#### Beacon-API

- Add rpc trigger for blob sidecar event
- Do not skip mev boost in `v3` block production endpoint
- Beacon APIs: re enabling blob events
- Beacon API: update Deneb endpoints after removing blob signing
- Beacon API: fix get blob returns 500 instead of empty
- Fix bug in Beacon API getBlobs
- Fix blob_sidecar SSE payload
- fix(beacon-chain/rpc): blob_sidecar event stream handler
- Improvements to `produceBlockV3`
- Deneb: Produce Block V3 - adding consensus block value

#### Validator Client

- Validator client: remove blob signing
- Deneb - web3signer

#### Testing

- Enable Deneb For E2E Scenario Tests
- Activate deneb in E2E
- Deneb E2E

#### Miscellaneous

- Update blob pruning log
- Fix total pruned metric + add to logging
- Check kzg commitment count from builder
- Add error wrapping to blob initialization errors
- Blob filesystem metrics
- Check builder header kzg commitment
- Add more color to sending blob by range req log
- Move pruning log to after retention check
- Enhance Pruning Logs
- Rename Blob retention epoch flag
- Check that blobs count is correct when unblinding
- Log blob's kzg commmitment at sync
- Replace MAX_BLOB_EPOCHS usages with more accurate terms
- Fix comment of `BlobSidecarsBySlot`

### Core Prysm Work(Non-Deneb)

#### Core Protocol

- Only process blocks which haven't been processed
- Initialize exec payload fields and enforce order
- Add nil check for head in IsOptimistic
- Unlock forkchoice store if attribute is empty
- Make Aggregating In Parallel The Permanent Default
- Break out several helpers from `postBlockProcess`
- Don't hardcode 4 seconds in forkchoice
- Simplify fcu 4
- Remove the getPayloadAttribute call from updateForkchoiceWithExecution
- Simplify fcu 2
- Remove getPayloadAttributes from FCU call
- Simplify fcu 1
- Remove unsafe proposer indices cache
- Rewrite `ProposeBlock` endpoint
- Remove blind field from block type
- update shuffling caches before calling FCU on epoch boundaries
- Return SignedBeaconBlock from ReadOnlySignedBeaconBlock.Copy
- Use advanced epoch cache when preparing proposals
- refactor Payload Id caches
- Use block value correctly when proposing a block
- use different keys for the proposer indices cache
- Use a cache of one entry to build attestation
- Remove signed block requirement from no-verify functions
- Allow requests for old target roots
- Remove Redundant Hash Computation in Cache
- Fix FFG LMD Consistency Check (Option 2)
- Verify lmd without ancestor
- Track target in forkchoice
- Return early from ReceiveBlock if already sycned

#### Builder

- Adding builder boost factor to get block v3
- Builder API: Fix max field check on toProto function
- Add sanity checks for bundle from builder
- Update Prysm Proposer end points for Builder API
- Builder API: remove blinded blob sidecar
- Allow validators registration batching on Builder API `/eth/v1/builder/validators`

#### State-Management

- Add Detailed Multi Value Metrics
- Optimize Multivalue Slice For Trie Recomputation
- Fix Multivalue Slice Deadlock
- Set Better Slice Capacities in the State

#### Networking

- Refactor Network Config Into Main Config
- Handle potential error from newBlockRangeBatcher
- Clean Up Goodbye Stream Errors
- Support New Subnet Backbone
- Increase Networking Defaults
- Bump Up Gossip Queue Size
- Improve Gossipsub Rejection Metric
- Add Gossipsub Queue Flag
- Fix Deadlock With Subscriber Checker
- Add Additional Pubsub Metrics
- Verify Block Signatures On Insertion Into Pending Queue
- Enhance Validation for Block by Root RPC Requests
- Add a helper for max request block
- Fix Pending Queue Deadline Bug
- Add context deadline for pending queue's receive block
- Fix Pending Queue Expiration Bug
- sync only up to previous epoch on phase 1
- Use correct context for sendBatchRootRequest
- Refactor Pending Block Queue Logic in Sync Package
- Check block exists in pending queue before requesting from peer
- Set Verbosity of Goodbye Logs to Trace
- use read only head state

#### Beacon-API

_Most of the PRs here involve shifting our http endpoints to using vanilla http handlers(without the API middleware)._

- http endpoint cleanup
- Revert "REST VC: Subscribe to Beacon API events "
- proposer and attester slashing sse
- REST VC: Subscribe to Beacon API events
- Simplify error handling for JsonRestHandler
- Update block publishing to 2.4.2 spec
- Use `SkipMevBoost` properly during block production
- Handle HTTP 404 Not Found in `SubmitAggregateAndProof`
- beacon-chain/rpc: use BalanceAtIndex instead of Balances to reduce memory copy
- HTTP endpoints cleanup
- APIs: reusing grpc cors middleware for rest
- Beacon API: routes unit test
- Remove API Middleware
- HTTP validator API: beacon and account endpoints
- REST VC: Use POST to fetch validators
- HTTP handler for Beacon API events
- Move weak subjectivity endpoint to HTTP
- Handle non-JSON responses from Beacon API
- POST version of GetValidators and GetValidatorBalances
- [2/5] light client http api
- HTTP validator API: wallet endpoints
- HTTP Validator API: slashing protection import and export
- Config HTTP endpoints
- Return 404 from `eth/v1/beacon/headers` when there are no blocks
- Pool slashings HTTP endpoints
- Validator HTTP endpoints
- Debug HTTP endpoints
- HTTP validator API: health endpoints
- HTTP Validator API:  `/eth/v1/keystores`
- Allow unknown fields in Beacon API responses
- HTTP state endpoints
- HTTP Validator API: `/eth/v1/validator/{pubkey}/feerecipient`
- HTTP Validator API: `/eth/v1/validator/{pubkey}/gas_limit`
- HTTP VALIDATOR API: remote keymanager api `/eth/v1/remotekeys`
- rpc/apimiddleware: Test all paths can be created
- HTTP Beacon APIs for blocks
- HTTP VALIDATOR API: `/eth/v1/validator/{pubkey}/voluntary_exit`
- HTTP Beacon APIs: 3 state endpoints
- HTTP Beacon APIs for node
- HTTP API: `/eth/v1/beacon/pool/bls_to_execution_changes`
- Register sync subnet when fetching sync committee duties through Beacon API

#### Validator Client

- Refactor validator client help.
- `--validatorS-registration-batch-size` (add `s`)
- Validator client: Always use the `--datadir` value.
- Hook to slot stream instead of block stream on the VC
- CLI: fixing account import ux bugs
- `filterAndCacheActiveKeys`: Stop filtering out exiting validators
- Gracefully handle unknown validator index in the REST VC
- Don't fetch duties for unknown keys
- Fix Domain Data Caching
- Add `--jwt-id` flag
- Make Prysm VC compatible with the version `v5.3.0` of the slashing protections interchange tests.
- Fix handling POST requests in the REST VC
- Better error handling in REST VC
- Fix block proposals in the REST validator client
- CLEANUP: validator exit prompt
- integrate validator count endpoint in validator client

#### Build/CI Work

- Bazel 7.0.0
- Sort static analyzers, add more, fix violations
- For golangci-lint, enable all by default
- Enable mirror linter and fix findings
- Enable usestdlibvars linter and fix findings
- Fix docker image version strings in CI
- fixing sa4006
- Enable errname linter and fix findings
- Remove rules_docker, make multiarch images canonical
- Fix staticcheck violations
- Add staticchecks to bazel builds
- CI: Add merge queue events trigger for github workflows
- Update bazel and other CI improvements
- bazel: Run buildifier, general cleanup
- pgo: Enable pgo behind release flag
- pgo: remove default pprof profile
- zig: Update zig to recent main branch commit
- Enable profile guided optimization for beacon-chain
- Refactor Exported Names to Follow Golang Best Practices
- Update rules_go and gazelle to 0.42 & 0.33 (latest releases)
- Fix image deps

#### Dependency Updates

- Update go to 1.21.6
- Update Our Golang Crypto Library
- Update libp2p/go-libp2p-asn-util to v0.4.1
- Update Libp2p To v0.32.1 and Go to v1.21.5
- Bump google.golang.org/grpc from 1.53.0 to 1.56.3
- Update go to 1.20.10

#### Testing

- Enable Profiling for Long Running E2E Runs
- Fetch Goroutine Traces in E2E
- Fix Up Builder Evaluator
- Increase Blob Batch Parameters in E2E
- Uncomment e2e flakiness
- Update spectests to 1.4.0-beta.5
- Test improvement TestValidateVoluntaryExit_ValidExit
- Simplify post-evaluation in Beacon API evaluator
- Run Evaluator In the Middle Of An Epoch
- Simplify Beacon API evaluator
- Fix Optimistic Sync Evaluator
- Add test helpers to produce commitments and proofs
- Redesign of Beacon API evaluator
- Drop Transaction Count for Transaction Generator
- Add concurrency test for getting attestation state
- Add `construct_generic_block_test` to build file
- Implement Merkle proof spectests
- Remove `/node/peers/{peer_id}` from Beacon API evaluator
- Update spectest and changed minimal preset for field elements
- Better Beacon API evaluator part 1
- beacon-chain/blockchain: fix some datarace in go test
- beacon-node/rpc: fix go test datarace
- Fix Builder Testing For Multiclient Runs
- Fill state attestations
- beacon-chain/sync: fix some datarace in go test
- beacon-chain/execution: fix a data race in testcase
- Add state not found test case

#### Feature Updates

- Make New Engine Methods The Permanent Default
- Make Reorging Of Late Blocks The Permanent Default

#### Miscellaneous

- Update teku's bootnode
- fix metric for exited validator
- Fix typos
- Replace validator count with validator indices in update fee recipient log
- Log value of local payload when proposing
- Small encoding fixes on logs and http error code change
- typo fix
- Fix error string generation for missing commitments
- Increase buffer of events channel
- Fix missing testnet versions. Issue
- Update README.md
- Only run metrics for canonical blocks
- Relax file permissions check on existing directories
- forkchoice.Getter wrapper with locking wrappers
- Initialize cancellable root context in main.go
- Fix forkchoice pkg's comments grammar
- lock RecentBlockSlot
- Comment typo
- Optimize `ReplayBlocks` for Zero Diff
- Remove default value of circuit breaker flags
- Fix Withdrawals
- Remove no-op cancel func
- Update Terms of Service
- fix head slot in log
- DEPRECTATION: Remove exchange transition configuration call
- fix segmentation fork when Capella for epoch is MaxUint64
- Return Error Gracefully When Removing 4881 Flag
- Add zero length check on indices during NextSyncCommitteeIndices
- Replace Empty Slice Literals with Nil Slices
- Refactor Error String Formatting According to Go Best Practices
- Fix redundant type converstion
- docs: fix typo
- Add Clarification To Sync Committee Cache
- Fix typos
- remove bad comment
- Remove confusing comment
- Log when sending FCU with payload attributes
- Fix Withdrawals Marshalling
- beacon-chain/execution: no need to reread and unmarshal the eth1Data twice

## [v4.1.1](https://github.com/prysmaticlabs/prysm/compare/v4.1.0...v4.1.1) - 2023-10-24

This patch release includes two cherry-picked changes from the develop branch to resolve critical issues that affect a
small set of users.

### Fixed

- Fix improperly registered REST API endpoint for validators using Prysm's REST API with an external builder
- Fix deadlock when using --enable-experimental-state feature

### Security

No security issues in thsi release.

## [v4.1.0](https://github.com/prysmaticlabs/prysm/compare/v4.0.8...v4.1.0) - 2023-08-22

- **Fundamental Deneb Support**: This release lays the foundation for Deneb support, although features like backwards
  syncing and filesystem-based blob storage are planned for Q4 2024.
- **Multi-Value Slices for Beacon State**: Implemented multi-value slices to reduce the memory footprint and optimize
  certain processing paths. This data structure allows for storing values shared between state instances more
  efficiently. This feature is controller by the `--enable-experimental-state` flag.
- **EIP-4881 Deposit Tree**: Integrated the EIP-4881 Deposit Tree into Prysm to optimize runtime block processing and
  production. This feature is controlled by a flag: `--enable-eip-4881`
- **BLST version 0.3.11**: Introduced a significant improvement to the portable build's performance. The portable build
  now features runtime detection, automatically enabling optimized code paths if your CPU supports it.
- **Multiarch Containers Preview Available**: multiarch (:wave: arm64 support :wave:) containers will be offered for
  preview at the following locations:
    - Beacon Chain: [gcr.io/prylabs-dev/prysm/beacon-chain:v4.1.0](gcr.io/prylabs-dev/prysm/beacon-chain:v4.1.0)
    - Validator: [gcr.io/prylabs-dev/prysm/validator:v4.1.0](gcr.io/prylabs-dev/prysm/validator:v4.1.0)
    - Please note that in the next cycle, we will exclusively use these containers at the canonical URLs.

### Added

#### EIP-4844:

##### Core:

- **Deneb State & Block Types**: New state and block types added specifically for Deneb.
- **Deneb Protobufs**: Protocol Buffers designed exclusively for Deneb.
- **Deneb Engine API**: Specialized API endpoints for Deneb.
- **Deneb Config/Params**: Deneb-specific configurations and parameters from the deneb-integration branch.

##### Blob Management:

- **Blob Retention Epoch Period**: Configurable retention periods for blobs.
- **Blob Arrival Gossip Metric**: Metrics for blob arrivals via gossip protocol.
- **Blob Merge Function**: Functionality to merge and validate saved/new blobs.
- **Blob Channel**: A channel dedicated to blob processing.
- **Save Blobs to DB**: Feature to save blobs to the database for subscribers.

##### Logging and Validation:

- **Logging for Blob Sidecar**: Improved logging functionalities for Blob Sidecar.
- **Blob Commitment Count Logging**: Introduced logging for blob commitment counts.
- **Blob Validation**: A feature to validate blobs.

##### Additional Features and Tests:

- **Deneb Changes & Blobs to Builder**: Deneb-specific changes and blob functionality added to the builder.
- **Deneb Blob Sidecar Events**: Blob sidecar events added as part of the Deneb release.
- **KZG Commitments**: Functionality to copy KZG commitments when using the builder block.
- **Deneb Validator Beacon APIs**: New REST APIs specifically for the Deneb release.
- **Deneb Tests**: Test cases specific to the Deneb version.
- **PublishBlockV2 for Deneb**: The `publishblockv2` endpoint implemented specifically for Deneb.
- **Builder Override & Builder Flow for Deneb**: An override for the builder and a new RPC to handle the builder flow in
  Deneb.
- **SSZ Detection for Deneb**: SSZ detection capabilities added for Deneb.
- **Validator Signing for Deneb**: Validators can now sign Deneb blocks.
- **Deneb Upgrade Function**: A function to handle the upgrade to Deneb.

#### Rest of EIPs

- **EIP-4788**: Added support for Beacon block root in the EVM.
- **EIP-7044** and **EIP-7045**: Implemented support for Perpetually Valid Signed Voluntary Exits and increased the max
  attestation inclusion slot.

#### Beacon API:

*Note: All Beacon API work is related with moving endpoints into pure HTTP handlers. This is NOT new functionality.*

##### Endpoints moved to HTTP:

- `/eth/v1/beacon/blocks` and `/eth/v1/beacon/blinded_blocks`.
- `/eth/v1/beacon/states/{state_id}/committees`.
- `/eth/v1/config/deposit_contract`.
- `/eth/v1/beacon/pool/sync_committees`.
- `/eth/v1/beacon/states/{state_id}/validators`, `/eth/v1/beacon/states/{state_id}/validators/{validator_id}`
  and `/eth/v1/beacon/states/{state_id}/validator_balances`.
- `/eth/v1/validator/duties/attester/{epoch}`, `/eth/v1/validator/duties/proposer/{epoch}`
  and `/eth/v1/validator/duties/sync/{epoch}`.
- `/eth/v1/validator/register_validator`.
- `/eth/v1/validator/prepare_beacon_proposer`.
- `/eth/v1/beacon/headers`.
- `/eth/v1/beacon/blocks/{block_id}/root`.
- `/eth/v1/validator/attestation_data`.
- `/eth/v1/validator/sync_committee_contribution`.
- `/eth/v1/beacon/genesis` and `/eth/v1/beacon/states/{state_id}/finality_checkpoints`.
- `/eth/v1/node/syncing`.
- `/eth/v1/beacon/pool/voluntary_exits`.
- `/eth/v1/beacon/headers/{block_id}` and `/eth/v1/validator/liveness/{epoch}`.

##### Miscellaneous:

- **Comma-Separated Query Params**: Support for comma-separated query parameters added to Beacon API.
- **Middleware for Query Params**: Middleware introduced for handling comma-separated query parameters.
- **Content-Type Header**: Compliance improved by adding Content-Type header to VC POST requests.
- **Node Version**: REST-based node version endpoint implemented.

#### Other additions

##### Protocol:

- **Multi-Value Slice for Beacon State**: Enhanced the beacon state by utilizing a multi-value slice.
- **EIP-4881 Deposit Tree**: EIP-4881 Deposit Tree integrated into Prysm, controlled by a feature flag.
- **New Engine Methods**: New engine methods set as the default.
- **Light Client Sync Protocol**: Initiation of a 5-part light client sync protocol.
- **Block Commitment Checks**: Functionality to reject blocks with excessive commitments added.

##### State Management:

- **Alloc More Items**: Modified beacon-node/state to allocate an additional item during appends.
- **GetParentBlockHash Helper**: Refactoring of `getLocalPayloadAndBlobs` with a new helper function for fetching parent
  block hashes.
- **RW Lock for Duties**: Read-Write lock mechanism introduced for managing validator duties.

##### Build and CI/CD Improvements:

- **Manual Build Tag**: A "manual" build tag introduced to expedite CI build times.
- **Multiarch Docker Containers**: Support for multiple architectures in Docker containers added.

##### Testing:

- **Init-Sync DA Tests**: Tests for initial sync Data Availability (DA) included.
- **Fuzz List Timeout**: Github workflow for fuzz testing now includes a timeout setting.
- **Go Fuzzing Workflow**: New Github workflow for Go fuzzing on a cron schedule.

##### Logging and Monitoring:

- **FFG-LMD Consistency Logging**: Enhanced logging for Finality Gadget LMD (FFG-LMD) consistency.
- **Validator Count Endpoint**: New endpoint to count the number of validators.

##### User Interface and Web:

- **Web UI Release**: Prysm Web UI v2.0.4 released with unspecified updates and improvements.

##### Testnet support:

- **Holesky Support**: Support for Holesky decompositions integrated into the codebase.

##### Error Handling and Responses:

- **Validation Error in ForkchoiceUpdatedResponse**: Included validation errors in fork choice update responses.
- **Wrapped Invalid Block Error**: Improved error handling for cases where an invalid block error is wrapped..

### Changed

#### General:

- **Skip MEV-Boost Flag**: Updated `GetBlock` RPC to utilize `skip mev-boost` flag.
- **Portable Version of BLST**: Transitioned to portable BLST version as default.
- **Teku Mainnet Bootnodes**: Refreshed Teku mainnet bootnodes ENRs.
- **Geth Version Updates**: Elevated geth to version v1.13.1 for additional stability and features.
- **Parallel Block Building**: Deprecated sequential block building path

#### Deneb-Specific Changes:

- **Deneb Spectests Release**: Upgraded to Deneb spectests v1.4.0-beta.2-hotfix.
- **Deneb API and Builder Cleanup**: Conducted clean-up activities for Deneb-specific API and builder.
- **Deneb Block Versioning**: Introduced changes related to Deneb produce block version 3.
- **Deneb Database Methods**: Adapted database methods to accommodate Deneb.
- **Unused Code Removal**: Eliminated an unused function and pending blobs queue.
- **Blob Sidecar Syncing**: Altered behavior when value is 0.

#### Code Cleanup and Refactor:

- **API Types Cleanup**: Reorganized API types for improved readability.
- **Geth Client Headers**: Simplified code for setting geth client headers.
- **Bug Report Template**: Revised requirements for more clarity.

#### Flags and Configuration:

- **Safe Slots to Import Flag**: Deprecated this flag for standard alignment.
- **Holesky Config**: Revised the Holesky configuration for new genesis.

#### Logging:

- **Genesis State Warning**: Will log a warning if the genesis state size is under 1KB.
- **Debug Log Removal**: Excised debug logs for cleaner output.

#### Miscellaneous:

- **First Aggregation Timing**: Default setting for first aggregation is 7 seconds post-genesis.
- **Pointer Usage**: Modified execution chain to use pointers, reducing copy operations.

#### Dependency Updates:

- **Go Version Update**: Updated to Go version 1.20.7.
- **Go Version Update**: Updated to Go version 1.20.9 for better security.
- **Various Dependencies**: Updated multiple dependencies including Geth, Bazel, rules_go, Gazelle, BLST, and go-libp2p.

### Removed

- **Remote Slashing Protection**: Eliminated the remote slashing protection feature.
- **Go-Playground/Validator**: Removed the go-playground/validator dependency from the Beacon API.
- **Revert Cache Proposer ID**: Reverted the caching of proposer ID on GetProposerDuties.
- **Go-Playground/Validator**: Removed go-playground/validator from Beacon API.
- **Reverted Cache Proposer ID**: Reversed the change that cached proposer ID on GetProposerDuties.
- **Cache Proposer ID**: Reversed the functionality that cached proposer ID on GetProposerDuties.
- **Quadratic Loops in Exiting**: Eliminated quadratic loops that occurred during voluntary exits, improving
  performance.
- **Deprecated Go Embed Rules**: Removed deprecated `go_embed` rules from rules_go, to stay up-to-date with best
  practices.
- **Alpine Images**: Removed Alpine images from the Prysm project.

### Fixed

#### Deneb-Specific Bug Fixes:

- **Deneb Builder Bid HTR**: Fixed an issue related to HashTreeRoot (HTR) in Deneb builder bid.
- **PBV2 Condition**: Corrected conditions related to PBV2.
- **Route Handler and Cleanup**: Updated the route handler and performed minor cleanups.
- **Devnet6 Interop Issues**: Resolved interoperability issues specific to Devnet6.
- **Sepolia Version**: Updated the version information for the Sepolia testnet.
- **No Blob Bundle Handling**: Rectified the handling when no blob bundle exists.
- **Blob Sidecar Prefix**: Corrected the database prefix used for blob sidecars.
- **Blob Retrieval Error**: Added specific error handling for blob retrieval from the database.
- **Blob Sidecar Count**: Adjusted metrics for accurate blob sidecar count.
- **Sync/RPC Blob Usage**: Rectified blob usage when requesting a block by root in Sync/RPC.

#### Cache Fixes:

- **Don't Prune Proposer ID Cache**: Fixed a loop erroneously pruning the proposer ID cache.
- **LastRoot Adjustment**: Altered `LastRoot` to return the head root.
- **Last Canonical Root**: Modified forkchoice to return the last canonical root of the epoch.

#### Block Processing fixes:

- **Block Validation**: Fixed an issue where blocks were incorrectly marked as bad during validation.
- **Churn Limit Helpers**: Improved churn limit calculations through refactoring.
- **Churn with 0 Exits**: Rectified a bug that calculated churn even when there were 0 exits.
- **Proposer Duties Sorting**: Resolved sorting issues in proposer duties.
- **Duplicate Block Processing**: Eliminated redundant block processing.

#### Error Handling and Logging:

- **RpcError from Core Service**: Ensured that `RpcError` is returned from core services.
- **Unhandled Error**: Enhanced error management by handling previously unhandled errors.
- **Error Handling**: Wrapped `ctx.Err` for improved error handling.
- **Attestation Error**: Optimized error management in attestation processing.

#### Test and Build Fixes:

- **Racy Tests in Blockchain**: Resolved race conditions in blockchain tests.
- **TestService_ReceiveBlock**: Modified `TestService_ReceiveBlock` to work as expected.
- **Build Issue with @com_github_ethereum_c_kzg_4844**: Resolved build issues related to this specific library.
- **Fuzz Testing**: Addressed fuzz testing issues in the `origin/deneb-integration`
- **Long-Running E2E Tests**: Fixed issues that were causing the end-to-end tests to run for an extended period.

#### Additional Fixes:

- **Public Key Copies During Aggregation**: Optimized to avoid unnecessary public key copies during aggregation.
- **Epoch Participations**: Fixed the setting of current and previous epoch participations.
- **Verify Attestations**: Resolved an attestation verification issue in proposer logic.
- **Empty JSON/YAML Files**: Fixed an issue where `prysmctl` was writing empty configuration files.
- **Generic Fixes**: Addressed various unspecified issues.
- **Phase0 Block Parsing**: Resolved parsing issues in phase0 blocks on submit.
- **Hex Handling**: Upgraded the hex handling in various modules.
- **Initial Sync PreProcessing**: Resolved an issue affecting the initial sync preprocessing.

### Security

No security updates in this release.

## [v4.0.8](https://github.com/prysmaticlabs/prysm/compare/v4.0.7...v4.0.8) - 2023-08-22

Welcome to Prysm Release v4.0.8! This release is recommended. Highlights:

- Parallel hashing of validator entries in the beacon state. This results in a faster hash tree root. ~3x reduction
- Parallel validations of consensus and execution checks. This results in a faster block verification
- Aggregate parallel is now the default. This results in faster attestation aggregation time if a node is subscribed to
  multiple beacon attestation subnets. ~3x reduction
- Better process block epoch boundary cache usages and bug fixes
- Beacon-API endpoints optimizations and bug fixes

### Added

- Optimization: parallelize hashing for validator entries in beacon state
- Optimization: parallelize consensus & execution validation when processing beacon block
- Optimization: integrate LRU cache (above) for validator public keys
- Cache: threadsafe LRU with non-blocking reads for concurrent readers
- PCLI: add deserialization time in benchmark
- PCLI: add allocation data To benchmark
- Beacon-API: GetSyncCommitteeRewards endpoint
- Beacon-API: SSZ responses for the Publishblockv2
- Beacon-API client: use GetValidatorPerformance
- Spec tests: mainnet withdrawals and bls spec tests
- Spec tests: random and fork transition spec tests
- Spec tests execution payload operation tests
- Metric: block gossip arrival time
- Metric: state regen duration
- Metric: validator is in the next sync committee
- New data structure: multi-value slice

### Changed

- Build: update Go version to 1.20.6
- Build: update hermetic_cc_toolchain
- Optimization: aggregate parallel is now default
- Optimization: do not perform full copies for metrics reporting
- Optimization: use GetPayloadBodies in Execution Engine Client
- Optimization: better nil check for reading validator
- Optimization: better cache update at epoch boundary
- Optimization: improve InnerShuffleList for shuffling
- Optimization: remove span for converting to indexed attestation`
- Beacon-API: optimize GetValidatorPerformance as POST
- Beacon-API: optimize /eth/v1/validator/aggregate_attestation
- Beacon-API: optimize /eth/v1/validator/contribution_and_proofs
- Beacon-API: optimize /eth/v1/validator/aggregate_and_proofs
- Beacon-API: use struct in beacon-chain/rpc/core to store dependencies
- Beacon-API: set CoreService in beaconv1alpha1.Server
- Beacon-API: use BlockProcessed event in certain endpoints
- Syncing: exit sync early with 0 peers to sync
- Cache: only call epoch boundary processing on canonical blocks
- Build: update server-side events dependency
- Refactor: slot tickers with intervals
- Logging: shift Error Logs To Debug
- Logging: clean up attestation routine logs

### Fixed

- Cache: update shuffling caches at epoch boundary
- Cache: committee cache correctly for epoch + 1
- Cache: use the correct context for UpdateCommitteeCache
- Cache: proposer-settings edge case for activating validators
- Cache: prevent the public key cache from overwhelming runtime
- Sync: correctly set optimistic status in the head when syncing
- Sync: use last optimistic status on batch
- Flag: adds local boost flag to main/usage
- Beacon-API: correct header for get block and get blinded block calls
- Beacon-API: GetValidatorPerformance endpoint
- Beacon-API: return correct historical roots in Capella state
- Beacon-API: use the correct root in consensus validation
- Prysm API: size of SyncCommitteeBits
- Mev-boost: builder gas limit fix default to 0 in some cases
- PCLI: benchmark deserialize without clone and init trie
- PCLI: state trie for HTR duration
- Metric: adding fix pending validators balance
- Metric: effective balance for unknown/pending validators
- Comment: comments when receiving block
- Comment: cleanups to blockchain pkg

### Security

No security updates in this release.

## [v4.0.7](https://github.com/prysmaticlabs/prysm/compare/v4.0.6...v4.0.7) - 2023-07-13

Welcome to the v4.0.7 release of Prysm! This recommended release contains many essential optimizations since v4.0.6.

Highlights:

- The validator proposal time for slot 0 has been reduced by 800ms. Writeup and PR
- The attestation aggregation time has been reduced by 400ms—roughly 75% with all subnets subscribed. Flag
  --aggregate-parallel. PR. This is only useful if running more than a dozen validator keys. The more subnets your node
  subscribe to, the more useful.
- The usage of fork choice lock has been reduced and optimized, significantly reducing block processing time. This
  results in a higher proposal and attest rate. PR
- The block proposal path has been optimized with more efficient copies and a better pruning algorithm for pending
  deposits. PR and PR
- Validator Registration cache is enabled by default, this affects users who have used webui along with mevboost. Please
  review PR for details.

Note: We remind our users that there are two versions of the cryptographic library BLST, one is "portable" and less
performant, and another is "non-portable" or "modern" and more performant. Most users would want to use the second one.
You can set the environment variable USE_PRYSM_MODERN=true when using prysm.sh. The released docker images are using the
non-portable version by default.

### Added

- Optimize multiple validator status query
- Track optimistic status on head
- Get attestation rewards API end point
- Expected withdrawals API
- Validator voluntary exit endpoint
- Aggregate atts using fixed pool of go routines
- Use the incoming payload status instead of calling forkchoice
- Add hermetic_cc_toolchain for a hermetic cc toolchain
- Cache next epoch proposers at epoch boundary
- Optimize Validator Roots Computation
- Log Finalized Deposit Insertion
- Move consensus and execution validation outside of onBlock
- Add metric for ReceiveBlock
- Prune Pending Deposits on Finalization
- GetValidatorPerformance http endpoint
- Block proposal copy Bytes Alternatively
- Append Dynamic Adding Trusted Peer Apis

### Changed

- Do not validate merge transition block after Capella
- Metric for balance displayed for public keys without validator indexes
- Set blst_modern=true to be the bazel default build
- Rename payloadHash to lastValidHash in setOptimisticToInvalid
- Clarify sync committee message validation
- Checkpoint sync ux
- Registration Cache by default

### Removed

- Disable nil payloadid log on relayers flags
- Remove unneeded helper
- Remove forkchoice call from notify new payload

### Fixed

- Late block task wait for initial sync
- Log the right block number
- Fix for keystore field name to align with EIP2335
- Fix epoch participation parsing for API
- Spec checker, ensure file does not exit or error
- Uint256 parsing for builder API
- Fuzz target for execution payload
- Contribution doc typo
- Unit test TestFieldTrie_NativeState_fieldConvertersNative
- Typo on beacon-chain/node/node.go
- Remove single bit aggregation for aggregator
- Deflake cloners_test.go
- Use diff context to update proposer cache background
- Update protobuf and protobuf deps
- Run ineffassign for all code
- Increase validator client startup proposer settings deadline
- Correct log level for 'Could not send a chunked response'
- Rrune invalid blocks during initial sync
- Handle Epoch Boundary Misses
- Bump google.golang.org/grpc from 1.40.0 to 1.53.0
- Fix bls signature batch unit test
- Fix Context Cancellation for insertFinalizedDeposits
- Lock before saving the poststate to db

### Security

No security updates in this release.

## [v4.0.6](https://github.com/prysmaticlabs/prysm/compare/v4.0.5...v4.0.6) - 2023-07-15

Welcome to v4.0.6 release of Prysm! This recommended release contains many essential optimizations since v4.0.5. Notable
highlights:

Better handling of state field trie under late block scenario. This improves the next slot proposer's proposed time
Better utilization of next slot cache under various conditions

**Important read:**

1.) We use this opportunity to remind you that two different implementations of the underlying cryptographic library
BLST exist.

- portable: supports every CPU made in the modern era
- non-portable: more performant but requires your CPU to support special instructions

Most users will want to use the "non-portable" version since most CPUs support these instructions. Our docker builds are
now non-portable by default. Most users will benefit from the performance improvements. You can run with the "portable"
versions if your CPU is old or unsupported. For binary distributions and to maintain backward compatibility with older
versions of prysm.sh or prysm.bat, users that want to benefit from the non-portable performance improvements need to add
an environment variable, like so: USE_PRYSM_MODERN=true prysm.sh beacon-chain prefix, or download the "non-portable"
version of the binaries from the github repo.

2.) A peering bug that led to nodes losing peers gradually and eventually needing a restart has been patched. Nodes
previously affected by it can remove the --disable-resource-manager flag from v4.0.6 onwards.

### Added

- Copy state field tries for late block
- Utilize next slot cache correctly under late block scenario
- Epoch boundary uses next slot cache
- Beacon API broadcast_validation to block publishing
- Appropriate Size for the P2P Attestation Queue
- Flag --disable-resource-manager to disable resource manager for libp2p
- Beacon RPC start and end block building time logs
- Prysmctl: output proposer settings
- Libp2p patch
- Handle trusted peers for libp2p
- Spec test v1.4.0-alpha.1

### Changed

- Use fork-choice store to validate sync message faster
- Proposer RPc unblind block workflow
- Restore flag disable-peer-scorer
- Validator import logs improvement
- Optimize zero hash comparisons in forkchoice
- Check peer threshold is met before giving up on context deadline
- Cleanup of proposer payload ID cache
- Clean up set execution data for proposer RPC
- Update Libp2p to v0.27.5
- Always Favour Yamux for Multiplexing
- Ignore Phase0 Blocks For Monitor
- Move hash tree root to after block broadcast
- Use next slot cache for sync committee
- Log validation time for blocks
- Change update duties to handle all validators exited check
- Ignore late message log

### Removed

- SubmitblindBlock context timeout
- Defer state feed In propose block

### Fixed

- Sandwich attack on honest reorgs
- Missing config yamls for specific domains
- Release lock before panic for feed
- Return 500 in `/eth/v1/node/peers` interface
- Checkpoint sync uses correct slot

### Security

No security updates in this release.

## [v4.0.5](https://github.com/prysmaticlabs/prysm/compare/v4.0.4...v4.0.5) - 2023-05-22

Welcome to v4.0.5 release of Prysm! This release contains many important improvements and bug fixes since v4.0.4,
including significant improvements to attestation aggregation. See @potuz's
notes [here](https://hackmd.io/TtyFurRJRKuklG3n8lMO9Q). This release is **strongly** recommended for all users.

Note: The released docker images are using the portable version of the blst cryptography library. The Prysm team will
release docker images with the non-portable blst library as the default image. In the meantime, you can compile docker
images with blst non-portable locally with the `--define=blst_modern=true` bazel flag, use the "-modern-" assets
attached to releases, or set environment varaible USE_PRYSM_MODERN=true when using prysm.sh.

### Added

- Added epoch and root to "not a checkpt in forkchoice" log message
- Added cappella support for eth1voting tool
- Persist validator proposer settings in the validator db.
- Add flag to disable p2p resource management. This flag is for debugging purposes and should not be used in production
  for extended periods of time. Use this flag if you are experiencing significant peering issues.
  --disable-resource-manager

### Changed

- Improved slot ticker for attestation aggregation
- Parallel block production enabled by default. Opt out with --disable-build-block-parallel if issues are suspected with
  this feature.
- Improve attestation aggregation by not using max cover on unaggregated attestations and not checking subgroup of
  previously validated signatures.
- Improve sync message processing by using forkchoice

### Fixed

- Fixed --slasher flag.
- Fixed state migration for capella / bellatrix
- Fix deadlock when using --monitor-indices

### Security

No security updates in this release.

## [v4.0.4](https://github.com/prysmaticlabs/prysm/compare/v4.0.3...v4.0.4) - 2023-05-15

Welcome to v4.0.4 release of Prysm! This is the first full release following the recent mainnet issues and it is very
important that all stakers update to this release as soon as possible.

Aside from the critical fixes for mainnet, this release contains a number of new features and other fixes since v4.0.3.

### Added

- Feature to build consensus and execution blocks in parallel. This feature has shown a noticeable reduction (~200ms) in
  block proposal times. Enable with --build-block-parallel
- An in memory cache for validator registration can be enabled with --enable-registration-cache. See PR description
  before enabling.
- Added new linters
- Improved tracing data for builder pipeline
- Improved withdrawal phrasing in validator withdrawal tooling
- Improved blinded block error message
- Added test for future slot tolerance
- Pre-populate bls pubkey cache
- Builder API support in E2E tests

### Changed

- Updated spectests to v1.3
- Cleanup duplicated code
- Updated method signature for UnrealizedJustifiedPayloadBlockHash()
- Updated k8s.io/client-go to 0.20.0
- Removed unused method argument
- Refactored / moved some errors to different package
- Update next slot cache at an earlier point in block processing
- Use next slot cache for payload attribute
- Cleanup keymanager mock
- Update to go 1.20
- Modify InsertFinalizedDeposits signature to return an error
- Improved statefeed initialization
- Use v1alpha1 server in block production
- Updated go generated files
- Typo corrections

### Fixed

- Fixed e2e tx fuzzer nilerr lint issue
- Fixed status for pending validators with multiple deposits
- Use gwei in builder value evaluation
- Return correct error when failing to unmarshal genesis state
- Avoid double state copy in latestAncestor call
- Fix mock v1alpha1 server
- Fix committee race test
- Fix flaky validator tests
- Log correctly when the forkchoice head changed
- Filter inactive keys from mev-boost / builder API validator registration
- Save attestation to cache when calling SubmitAttestation in beacon API
- Avoid panic on nil broadcast object
- Fix initialization race
- Properly close subnet iterator
- ⚠️ Ignore untimely attestations
- Fix inverted metric
- ⚠️ Save to checkpoint cache if next state cache hits

### Security

This release contains some important fixes that improve the resiliency of Ethereum Consensus Layer.
See https://github.com/prysmaticlabs/prysm/pull/12387 and https://github.com/prysmaticlabs/prysm/pull/12398.

## [v4.0.3](https://github.com/prysmaticlabs/prysm/compare/v4.0.2...v4.0.3) - 2023-04-20

### Added

- Add REST API endpoint for beacon chain client's GetChainHead
- Add prepare-all-payloads flag
- support modifying genesis.json for capella
- Add support for engine_exchangeCapabilities
- prysmctl: Add support for writing signed validator exits to disk

### Changed

- Enable misspell linter & fix findings

### Fixed

- Fix Panic In Builder Service
- prysmctl using the same genesis func as e2e
- Check that Builder Is Configured
- Correctly use Gwei to compare builder bid value
- Fix Broken Dependency
- Deflake TestWaitForActivation_AccountsChanged
- Fix Attester Slashing Validation In Gossip
- Keymanager fixes for bad file writes
- windows: Fix build after PR 12293

### Security

No security updates in this release.

## [v4.0.2](https://github.com/prysmaticlabs/prysm/compare/v4.0.1...v4.0.2) - 2023-04-12

This release fixes a critical bug on Prysm interacting with mev-boost / relayer. You MUST upgrade to this release if you
run Prysm with mev boost and relayer, or you will be missing block proposals during the first days after the Shapella
fork while the block has bls-to-exec changes.
Post-mortem that describes this incident will be provided by the end of the week.

One of this release's main optimizations is revamping the next slot cache. It has been upgraded to be more performant
across edge case re-org scenarios. This can help with the bad head attestation vote.

Minor fixes in this release address a bug that affected certain large operators querying RPC endpoints. This bug caused
unexpected behavior and may have impacted the performance of affected operators. To resolve this issue, we have included
a patch that ensures proper functionality when querying RPC endpoints.

### Added

- CLI: New beacon node flag local-block-value-boost that allows the local block value to be multiplied by the boost
  value
- Smart caching for square root computation
- Beacon-API: Implemented Block rewards endpoint
- Beacon-API client: Implemented GetSyncStatus endpoint
- Beacon-API client: Implemented GetGenesis endpoint
- Beacon-API client: Implemented ListValidators endpoint

### Changed

- Block processing: Optimize next slot cache
- Execution-API: Used unrealized justified block hash for FCU call
- CLI: Improved voluntary exit confirmation prompt
- Unit test: Unskip API tests
- End to end test: Misc improvements
- Build: Build tag to exclude mainnet genesis from prysmctl
- Dependency: Update go-ethereum to v1.11.3
- Dependency: Update lighthouse to v4.0.1

### Fixed

- Builder: Unblind beacon block correctly with bls-to-exec changes
- Block construction: Default to local payload on error correctly
- Block construction: Default to local payload on nil value correctly
- Block processing: Fallback in update head on error
- Block processing: Add orphaned operations to the appropriate pool
- Prysm-API: Fix Deadlock in StreamChainHead
- Beacon-API: Get header error, nil summary returned from the DB
- Beacon-API: Broadcast correct slashing object

### Security

No security updates in this release.

## [v4.0.1](https://github.com/prysmaticlabs/prysm/compare/v4.0.0...v4.0.1)

This is a reissue of v4.0.0. See https://github.com/prysmaticlabs/prysm/issues/12201 for more information.

## [v4.0.0](https://github.com/prysmaticlabs/prysm/compare/v3.2.2...v4.0.0)

### Added

- Config: set mainnet capella epoch
- Validator: enable proposer to reorg late block
- Metric: bls-to-exec count in the operation pool
- Metric: pubsub metrics racer
- Metric: add late block metric
- Engine-API: Implement GetPayloadBodies
- Beacon-API: Implement GetPayloadAttribute SSE
- Prysm CLI: add experimental flags to dev mode
- Prysmctl utility: add eth1data to genesis state
- Spec test: EIP4881 spec compliance tests
- Spec test: forkchoice lock to fix flaskyness

### Changed

- Prysm: upgrade v3 to v4
- Prysm: apply goimports to generated files
- Validator: lower builder circuit breaker thresholds to 5 missed slots per epoch and updates off by 1
- Validator: reorg late block by default
- Forkchoice: cleanups
- Forkchoice: remove bouncing attack fix and strength equivocation discarding
- Forkchoice: call FCU at 4s mark if there's no new head
- Forkchoice: better locking on calls to retrieving ancestor root
- Forkchoice: stricker visibility for blockchain package access
- Block processing: optimizing validator balance retrieval by using epoch boundary cache
- Block processing: reduce FCU calls
- Block processing: increase attempted reorgs at the correct spot
- Block processing: remove duplicated bls to exec message pruning
- Block processing: skip hash tree root state when checking optimistic mode
- Prysm-API: mark GetChainHead deprecated
- Logging: add late block logs
- Logging: enhancements and clean ups
- Build: fix bazel remote cache upload
- Build: update cross compile toolchains
- Build: only build non-test targets in hack/update-go-pbs.sh
- Build: update rules_go to v0.38.1 and go_version to 1.19.7
- Build: replace bazel pkg_tar rule with canonical @rules_pkg pkg_tar
- Build: update bazel to 6.1.0
- Libp2p: updated to latest version
- Libp2p: make peer scorer permanent default
- Test: disable e2e slasher test
- CLI: derecate the following flags

### Deprecated

The following flags have been deprecated.

- disable-peer-scorer
- disable-vectorized-htr
- disable-gossip-batch-aggregation

### Removed

- Prsym remote signer
- CLI: Prater feature flag
- CLI: Deprecated flags
- Unit test: unused beacon chain altair mocks
- Validator REST API: unused endpoints

The following flags have been removed.

- http-web3provider
- enable-db-backup-webhook
- bolt-mmap-initial-size
- disable-discv5
- enable-reorg-late-blocks
- disable-attesting-history-db-cache
- enable-vectorized-htr
- enable-peer-scorer
- enable-forkchoice-doubly-linked-tree
- enable-back-pull
- enable-duty-count-down
- head-sync
- enable-gossip-batch-aggregation
- enable-larger-gossip-history
- fallback-web3provider
- disable-native-state
- enable-only-blinded-beacon-blocks
- ropsten
- interop-genesis-state
- experimental-enable-boundary-checks
- disable-back-pull
- disable-forkchoice-doubly-linked-tree

### Fixed

- Validator: startup deadline
- Prysmctl: withdrawals fork checking logic
- End-to-end test: fix flakes
- End-to-end test: fix altair transition
- Unit test: fix error message in

### Security

This release is required to participate in the Capella upgrade.

## [v3.2.2](https://github.com/prysmaticlabs/prysm/compare/v3.2.2...v3.2.1) - 2023-05-10

Gm! ☀️ We are excited to announce our release for upgrading Goerli testnet to Shanghai / Capella! 🚀

This release is MANDATORY for Goerli testnet. You must upgrade your Prysm beacon node and validator client to this
release before Shapella hard fork time epoch=162304 or UTC=14/03/2023, 10:25:36 pm.

This release is a low-priority for the mainnet.
This release is the same commit as v3.2.2-rc.3. If you are already running v3.2.2-rc.3, then you do not need to update
your client.

### Added

- Capella fork epoch
- Validator client REST implementation GetFeeRecipientByPubKey
- New end-to-end test for post-attester duties

### Changed

- Storing blind beacon block by default for new Prysm Database
- Raise the max grpc message size to a very large value by default
- Update rules docker to v0.25.0
- Update distroless base images
- Update protoc-gen-go-cast to suppress tool output
- Update deps for Capella
- Remove gRPC fallback client from validator REST API
- Prysmctl now verifies capella fork for bls to exec message change
- Core block processing cleanup
- Better locking design around forkchoice store
- Core process sync aggregate function returns reward amount
- Use Epoch boundary cache to retrieve balances
- Misc end-to-end test improvements and fixes
- Add slot number to proposal error log

### Deprecated

- Deprecate flag --interop-genesis-state

### Removed

- Remove Ropsten testnet config and feature flag

### Security

This release is required for Goerli to upgrade to Capella.

## [v3.2.1](https://github.com/prysmaticlabs/prysm/compare/v3.2.0...v3.2.1) - 2023-02-13

We are excited to announce the release of Prysm v3.2.1 🎉

This is the first release to support Capella / Shanghai. The Sepolia testnet Capella upgrade time is currently set to
2/28/2023, 4:04:48 AM UTC. The Goerli testnet and Mainnet upgrade times are still yet to be determined. In Summary:

- This is a mandatory upgrade for Sepolia nodes and validators
- This is a recommended upgrade for Goerli and Mainnet nodes and validators

There are some known issues with this release.

- mev-boost, relayer, and builder support for Capella upgrade are built in but still need to be tested. Given the lack
  of testing infrastructure, none of the clients could test this for withdrawals testnet. There may be hiccups when
  using mev-boost on the Capella upgraded testnets.

### Added

- Capella Withdrawal support
- Add Capella fork epoch for Sepolia
- Various Validator client REST implementations (Part of EPF)
- Various Beacon API additions
- Cache Fork Digest Computation to save compute
- Beacon node can bootstrap from non-genesis state (i.e bellatrix state)
- Refactor bytesutil, add support for go1.20 slice to array conversions
- Add Span information for attestation record save request
- Matric addition
- Identify invalid signature within batch verification
- Support for getting consensus values from beacon config
- EIP-4881: Spec implementation
- Test helper to generate valid bls-to-exec message
- Spec tests v1.3.0 rc.2

### Changed

- Prysm CLI utility support for exit
- Beacon API improvement
- Prysm API get block RPC
- Prysm API cleanups
- Block processing cleanup,
- Forkchoice logging improvements
- Syncing logging improvement
- Validator client set event improvement for readability and error handling
- Engine API implementation cleanups
- End to end test improvements
- Prysm CLI withdrawal ux improvement
- Better log for the block that never became head

### Removed

- Remove cache lookup and lock request for database boltdb transaction

### Fixed

- Beacon API
- Use the correct attribute if there's a payload ID cache miss
- Call FCU with an attribute on non-head block
- Sparse merkle trie bug fix
- Waiting For Bandwidth Issue While Syncing
- State Fetcher to retrieve correct epoch
- Exit properly with terminal block hash
- PrepareBeaconProposer API duplicating validator indexes when not persisted in DB
- Multiclient end-to-end
- Deep source warnings

### Security

There are no security updates in this release.

## [v3.2.0](https://github.com/prysmaticlabs/prysm/compare/v3.1.2...v3.2.0) - 2022-12-16

This release contains a number of great features and improvements as well as progress towards the upcoming Capella
upgrade. This release also includes some API changes which are reflected in the minor version bump. If you are using
mev-boost, you will need to update your prysm client to v3.2.0 before updating your mev-boost instance in the future.
See [flashbots/mev-boost#404](https://github.com/flashbots/mev-boost/issues/404) for more details.

### Added

- Support for non-english mnemonic phrases in wallet creation.
- Exit validator without confirmation prompt using --force-exit flag
- Progress on Capella and eip-4844 upgrades
- Added randao json endpoint. /eth/v1/beacon/states/{state_id}/randao
- Added liveness endpoint /eth/v1/validator/liveness/{epoch}
- Progress on adding json-api support for prysm validator
- Prysmctl can now generate genesis.ssz for forks after phase0.

### Changed

- --chain-config-file now throws an error if used concurrently with --network flag.
- Added more histogram metrics for block arrival latency times block_arrival_latency_milliseconds
- Priority queue RetrieveByKey now uses read lock instead of write lock
- Use custom types for certain ethclient requests. Fixes an issue when using prysm on gnosis chain.
- Updted forkchoice endpoint /eth/v1/debug/forkchoice (was /eth/v1/debug/beacon/forkchoice)
- Include empty fields in builder json client.
- Computing committee assignments for slots older than the oldest historical root in the beacon state is now forbidden

### Removed

- Deprecated protoarray tests have been removed

### Fixed

- Unlock pending block queue if there is any error on inserting a block
- Prysmctl generate-genesis yaml file now uses the correct format
- ENR serialization now correctly serializes some inputs that did not work previously
- Use finalized block hash if a payload ID cache miss occurs
- prysm.sh now works correctly with Mac M1 chips (it downloads darwin-arm64 binaries)
- Use the correct block root for block events api
- Users running a VPN should be able to make p2p dials.
- Several minor typos and code cleanups

### Security

- Go is updated to 1.19.4.

## [v3.1.2](https://github.com/prysmaticlabs/prysm/compare/v3.1.1...v3.1.2) - 2022-10-27

### Added

- Timestamp field to forkchoice node json responses
- Further tests to non-trivial functions of the builder service
- Support for VotedFraction in forkchoice
- Metrics for reorg distance and depths
- Support for optimistic sync spectests
- CLI flag for customizing engine endpoint timeout --engine-endpoint-timeout-seconds
- Support for lodestar identification in p2p monitoring
- --enable-full-ssz-data-logging to display debug ssz data on gossip messages that fail validation
- Progress on capella and withdrawals support
- Validator exit can be performed from prysmctl
- Blinded block support through the json API

### Changed

- Refactoring / cleanup of keymanager
- Refactoring / improvements in initial sync
- Forkchoice hardening
- Improved log warnings when fee recipient is not set
- Changed ready for merge log frequency to 1 minute
- Move log Unable to cache headers for execution client votes to debug
- Rename field in invalid pruned blocks log
- Validate checkpoint slot
- Return an error if marshaling invalid Uint256
- Fallback to uncached getPayload if timeout
- Update bazel to 5.3.0
- godocs cleanup and other cleanups
- Forkchoice track highest received root
- Metrics updated block arrival time histograms
- Log error and continue when proposer boost roots are missing
- Do not return on error during on_tick
- Do not return on error after update head
- Update default RPC HTTP timeout to 30s
- Improved fee recipient UX.
- Produce block skips mev-boost
- Builder getPayload timeout set to 3s
- Make stategen aware of forkchoice
- Increase verbosity of warning to error when new head cannot be determined when receiving an attestation
- Provide justified balances to forkchoice
- Update head continues without attestations
- Migrate historical states in another goroutine to avoid blocking block execution
- Made API middleware structs public
- Updated web UI to v2.0.2
- Default value for --block-batch-limit-burst-factor changed from 10 to 2.
- Vendored leaky bucket implementation with minor modifications

### Deprecated

- --disable-native-state flag and associated feature

### Removed

- Unused WithTimeout for builder client
- Optimistic sync candidate check
- Cleans up proto states
- Protoarray implementation of forkchoice

### Fixed

- Block fields to return a fixed sized array rather than slice
- Lost cancel in validator runner
- Release held lock on error
- Properly submit blinded blocks
- Unwanted wrapper of gRPC status errors
- Sync tests fixed and updated spectests to 1.2.0
- Prevent timeTillDuty from reporting a negative value
- Don't mark /healthz as unhealthy when mev-boost relayer is down
- Proposer index cache and slot is used for GetProposerDuties
- Properly retrieve values for validator monitoring flag from cli
- Fee recipient fixes and persistence
- Handle panic when rpc client is not yet initialized
- Improved comments and error messages
- SSL support for multiple gRPC endpoints
- Addressed some tool feedback and code complaints
- Handle unaggregated attestations in the event feed
- Prune / expire payload ID cache entries when using beacon json API
- Payload ID cache may have missed on skip slots due to incorrect key computation

### Security

- Libp2p updated to v0.22.0

## [v3.1.1](https://github.com/prysmaticlabs/prysm/compare/v3.1.0...v3.1.1) - 2022-09-09

This is another highly recommended release. It contains a forkchoice pruning fix and a gossipsub optimization. It is
recommended to upgrade to this release before the Merge next week, which is currently tracking for Wed Sept
14 (https://bordel.wtf/). Happy staking! See you on the other side!

### Fixed

- Fix memory leaks in fork choice store which leads to node becoming slower
- Improve connectivity and solves issues connecting with peers

### Security

No security updates in this release.

## [v3.1.0](https://github.com/prysmaticlabs/prysm/compare/v3.1.0...v3.0.0) - 2022-09-05

Updating to this release is highly recommended as it contains several important fixes and features for the merge. You
must be using Prysm v3 or later before Bellatrix activates on September 6th.

**Important docs links**

- [How to prepare for the merge](https://docs.prylabs.network/docs/prepare-for-merge)
- [How to check merge readiness status](https://docs.prylabs.network/docs/monitoring/checking-status)

### Added

- Add time until next duty in epoch logs for validator
- Builder API: Added support for deleting gas limit endpoint
- Added debug endpoint GetForkChoice for doubly-linked-tree
- Added support for engine API headers. --execution-headers=key=value
- New merge specific metrics. See

### Changed

- Deposit cache now returns shallow copy of deposits
- Updated go-ethereum dependency to v1.10.23
- Updated LLVM compiler version to 13.0.1
- Builder API: filter 0 bid and empty tx root responses
- Allow attestations/blocks to be received by beacon node when the nodes only optimistically synced
- Add depth and distance to CommonAncestorRoot reorg object
- Allocate slice array to expected length in several methods
- Updated lighthouse to version v3 in E2E runner
- Improved handling of execution client errors
- Updated web3signer version in E2E runner
- Improved error messages for db unmarshalling failures in ancestor state lookup
- Only updated finalized checkpoints in database if its more recent than previous checkpoint

### Removed

- Dead / unused code delete

### Fixed

- Fixed improper wrapping of certain errors
- Only log fee recipient message if changed
- Simplify ListAttestations RPC method fixes
- Fix several RPC methods to be aware of the appropriate fork
- Fixed encoding issue with builder API register validator method. fixes
- Improved blinded block handling in API. fixes
- Fixed IPC path for windows users
- Fix proposal of blinded blocks
- Prysm no longer crashes on start up if builder endpoint is not available

### Security

There are no security updates in this release.

## [v3.0.0](https://github.com/prysmaticlabs/prysm/compare/v3.0.0...v2.1.4) 2022-08-22

### Added

- Passing spectests v1.2.0-rc.3
- prysmctl: Generate genesis state via prysmctl testnet generate-genesis [command options] [arguments...]
- Keymanager: Add support for setting the gas limit via API.
- Merge: Mainnet merge epoch and TTD defined!
- Validator: Added expected wait time for pending validator activation in log message.
- Go: Prysm now uses proper versioning suffix v3 for this release. GoDocs and downstream users can now import prysm as
  expected for go projects.
- Builder API: Register validator via HTTP REST Beacon API endpoint /eth/v1/validator/register_validator
- Cross compilation support for Mac ARM64 chips (Mac M1, M2)

### Changed

- **Require an execution client** `--execution-endpoint=...`. The default value has changed to `localhost:8551` and you
  must use the jwt flag `--jwt-secret=...`. Review [the docs](https://docs.prylabs.network/docs/prepare-for-merge) for
  more information
- `--http-web3provider` has been renamed to `--execution-endpoint`. Please update your configuration
  as `--http-web3provider` will be removed in a future release.
- Insert attestations into forkchoice sooner
- Builder API: `gas_limit` changed from int to string to support JSON / YAML configs. `--suggested-gas-limit` changed
  from int to string.
- Fork choice: Improved handling of double locks / deadlocks
- Lower libp2p log level
- Improved re-org logs with additional metadata
- Improved error messages found by semgrep
- Prysm Web UI updated to release v2.0.1
- Protobuf message renaming (non-breaking changes)
- Enabled feature to use gohashtree by default. Disable with `--disable-vectorized-htr`
- Enabled fork choice doubly linked tree feature by default. Disable with `--disable-forkchoice-doubly-linked-tree`
- Remote signer: Renamed some field names to better represent block types (non-breaking changes for gRPC users, possibly
  breaking change for JSON API users)
- Builder API: require header and payload root match.
- Improved responses for json-rpc requests batching when using blinded beacon blocks.
- Builder API: Improved error messages
- Builder API: Issue warning when validator expects builder ready beacon node, but beacon node is not configured with a
  relay.
- Execution API: Improved payload ID to handle reorg scenarios

### Deprecated

- Several features have been promoted to stable or removed. The following flags are now deprecated and will be removed
  in a future
  release. `--enable-db-backup-webhook`, `--bolt-mmap-initial-size`, `--disable-discv5`, `--disable-attesting-history-db-cache`, `--enable-vectorized-htr`, `--enable-peer-scorer`, `--enable-forkchoice-doubly-linked-tree`, `--enable-duty-count-down`, `--head-sync`, `--enable-gossip-batch-aggregateion`, `--enable-larger-gossip-history`, `--fallback-web3provider`, `--use-check-point-cache`.
- Several beacon API endpoints marked as deprecated

### Removed

- Logging: Removed phase0 fields from validator performance log messages
- Deprecated slasher protos have been removed
- Deprecated beacon API endpoints
  removed: `GetBeaconState`, `ProduceBlock`, `ListForkChoiceHeads`, `ListBlocks`, `SubmitValidatorRegistration`, `GetBlock`, `ProposeBlock`
- API: Forkchoice method `GetForkChoice` has been removed.
- All previously deprecated feature flags have been
  removed. `--enable-active-balance-cache`, `--correctly-prune-canonical-atts`, `--correctly-insert-orphaned-atts`, `--enable-next-slot-state-cache`, `--enable-batch-gossip-verification`, `--enable-get-block-optimizations`, `--enable-balance-trie-computation`, `--disable-next-slot-state-cache`, `--attestation-aggregation-strategy`, `--attestation-aggregation-force-opt-maxcover`, `--pyrmont`, `--disable-get-block-optimizations`, `--disable-proposer-atts-selection-using-max-cover`, `--disable-optimized-balance-update`, `--disable-active-balance-cache`, `--disable-balance-trie-computation`, `--disable-batch-gossip-verification`, `--disable-correctly-prune-canonical-atts`, `--disable-correctly-insert-orphaned-atts`, `--enable-native-state`, `--enable-peer-scorer`, `--enable-gossip-batch-aggregation`, `--experimental-disable-boundry-checks`
- Validator Web API: Removed unused ImportAccounts and DeleteAccounts rpc options

### Fixed

- Keymanager API: Status enum values are now returned as lowercase strings.
- Misc builder API fixes
- API: Fix GetBlock to return canonical block
- Cache: Fix cache overwrite policy for bellatrix proposer payload ID cache.
- Fixed string slice flags with file based configuration

### Security

- Upgrade your Prysm beacon node and validator before the merge!

## [v2.1.4](https://github.com/prysmaticlabs/prysm/compare/v2.1.4...v2.1.3) - 2022-08-10

As we prepare our `v3` mainnet release for [The Merge](https://ethereum.org/en/upgrades/merge/), `v2.1.4` marks the end
of the `v2` era. Node operators and validators are **highly encouraged** to upgrade to release `v2.1.4` - many bug fixes
and improvements have been included in preparation for The Merge. `v3` will contain breaking changes, and will be
released within the next few weeks. Using `v2.1.4` in the meantime will give you access to a more streamlined user
experience. See our [v2.1.4 doc](https://docs.prylabs.network/docs/vnext/214-rc) to learn how to use v2.1.4 to run a
Merge-ready configuration on the Goerli-Prater network pair.

### Added

- Sepolia testnet configs `--sepolia`
- Goerli as an alias to Prater and testnet configs `--prater` or `--goerli`
- Fee recipient API for key manager
- YML config flag support for web3 signer
- Validator registration API for web3 signer
- JSON tcontent type with optional metadata
- Flashbots MEV boost support
- Store blind block (i.e block with payload header) instead of full block (i.e. block with payload) for storage
  efficiency (currently only available when the `enable-only-blinded-beacon-blocks` feature flag is enabled)
- Pcli utility support to print blinded block
- New Web v2.0 release into Prysm

### Changed

- Native state improvement is enabled by default
- Use native blocks instead of protobuf blocks
- Peer scorer is enabled by default
- Enable fastssz to use vectorized HTR hash algorithm improvement
- Forkchoice store refactor and cleanups
- Update libp2p library dependency
- RPC proposer duty is now allowed next epoch query
- Do not print traces with `log.withError(err)`
- Testnets are running with pre-defined feature flags

### Removed

- Deprecate Step Parameter from our Block By Range Requests

### Fixed

- Ignore nil forkchoice node when saving orphaned atts
- Sync: better handling of missing state summary in DB
- Validator: creates invalid terminal block using the same timestamp as payload
- P2P: uses incorrect goodbye codes
- P2p: defaults Incorrectly to using Mplex, which results in losing Teku peers
- Disable returning future state for API
- Eth1 connection API panic

### Security

There are no security updates in this release.

## [v2.1.3](https://github.com/prysmaticlabs/prysm/compare/v2.1.2...v2.1.3) - 2022-07-06

### Added

- Many fuzz test additions
- Support bellatrix blocks with web3signer
- Support for the Sepolia testnet with `--terminal-total-difficulty-override 17000000000000000`. The override flag is
  required in this release.
- Support for the Ropsten testnet. No override flag required
- JSON API allows SSZ-serialized blocks in `publishBlock`
- JSON API allows SSZ-serialized blocks in `publishBlindedBlock`
- JSON API allows SSZ-serialized requests in `produceBlockV2` and `produceBlindedBlock`
- Progress towards Builder API and MEV boost support (not ready for testing in this release)
- Support for `DOMAIN_APPLICATION_MARK` configuration
- Ignore subset aggregates if a better aggregate has been seen already
- Reinsertion of reorg'd attestations
- Command `beacon-chain generate-auth-secret` to assist with generating a hex encoded secret for engine API
- Return optimistic status to `ChainHead` related grpc service
- TTD log and prometheus metric
- Panda ascii art banner for the merge!

### Changed

- Improvements to forkchoice
- Invalid checksummed (or no checksum) addresses used for fee recipient will log a warning. fixes,
- Use cache backed `getBlock` method in several places of blockchain package
- Reduced log frequency of "beacon node doesn't have a parent in db with root" error
- Improved nil checks for state management
- Enhanced debug logs for p2p block validation
- Many helpful refactoring and cosmetic changes
- Move WARN level message about weak subjectivity sync and improve message content
- Handle connection closing for web3/eth1 nil connection
- Testing improvements
- E2E test improvements
- Increase file descriptor limit up to the maximum by default
- Improved classification of "bad blocks"
- Updated engine API error code handling
- Improved "Synced new block" message to include minimal information based on the log verbosity.
- Add nil checks for nil finalized checkpoints
- Change weak subjectivity sync to use the most recent finalized state rather than the oldest state within the current
  period.
- Ensure a finalized root can't be all zeros
- Improved db lookup of HighestSlotBlocksBelow to start from the end of the index rather than the beginning.
- Improved packing of state balances for hashtreeroot
- Improved field trie recomputation

### Removed

- Removed handling of `INVALID_TERMINAL_BLOCK` response from engine API

### Fixed

- `/eth/v1/beacon/blinded_blocks` JSON API endpoint
- SSZ handling of JSON API payloads
- Config registry fixes
- Withdrawal epoch overflows
- Race condition with blockchain service Head()
- Race condition with validator's highest valid slot accessor
- Do not update cache with the result of a cancelled request
- `validator_index` should be a string integer rather than a number integer per spec.
- Use timestamp heuristic to determine deposits to process rather than simple calculation of follow distance
- Return `IsOptimistic` in `ValidateSync` responses

### Security

There are no security updates in this release.

## [v2.1.2](https://github.com/prysmaticlabs/prysm/compare/v2.1.1...v2.1.2) - 2022-05-16

### Added

- Update forkchoice head before produce block
- Support for blst modern builds on linux amd64
- [Beacon API support](ethereum/beacon-APIs#194) for blinded block
- Proposer index and graffiti fields in Received block debug log for verbosity
- Forkchoice removes equivocating votes for weight accounting

### Changed

- Updated to Go [1.18](https://github.com/golang/go/releases/tag/go1.18)
- Updated go-libp2p to [v0.18.0](https://github.com/libp2p/go-libp2p/releases/tag/v0.18.0)
- Updated beacon API's Postman collection to 2.2.0
- Moved eth2-types into Prysm for cleaner consolidation of consensus types

### Removed

- Prymont testnet support
- Flag `disable-proposer-atts-selection-using-max-cover` which disables defaulting max cover strategy for proposer
  selecting attestations
- Flag `disable-get-block-optimizations` which disables optimization with beacon block construction
- Flag `disable-optimized-balance-update"` which disables optimized effective balance update
- Flag `disable-active-balance-cache` which disables active balance cache
- Flag `disable-balance-trie-computation` which disables balance trie optimization for hash tree root
- Flag `disable-batch-gossip-verification` which disables batch gossip verification
- Flag `disable-correctly-insert-orphaned-atts` which disables the fix for orphaned attestations insertion

### Fixed

- `end block roots don't match` bug which caused beacon node down time
- Doppelganger off by 1 bug which introduced some false-positive
- Fee recipient warning log is only disabled after Bellatrix fork epoch

### Security

There are no security updates in this release.

## [v2.1.1](https://github.com/prysmaticlabs/prysm/compare/v2.1.0...v2.1.1) - 2022-05-03

This patch release includes 3 cherry picked fixes for regressions found in v2.1.0.

View the full changelist from v2.1.0: https://github.com/prysmaticlabs/prysm/compare/v2.1.0...v2.1.1

If upgrading from v2.0.6, please review
the [full changelist](https://github.com/prysmaticlabs/prysm/compare/v2.0.6...v2.1.1) of both v2.1.0 and v2.1.1.

This release is required for users on v2.1.0 and recommended for anyone on v2.0.6.

The following known issues exist in v2.1.0 and also exist in this release.

- Erroneous warning message in validator client when bellatrix fee recipient is unset. This is a cosmetic message and
  does not affect run time behavior in Phase0/Altair.
- In Bellatrix/Kiln: Fee recipient flags may not work as expected. See for a fix and more details.

### Fixed

- Doppelganger false positives may have caused a failure to start in the validator client.
- Connections to execution layer clients were not properly cleaned up and lead to resource leaks when using ipc.
- Initial sync (or resync when beacon node falls out of sync) could lead to a panic.

### Security

There are no security updates in this release.

## [v2.1.0](https://github.com/prysmaticlabs/prysm/compare/v2.0.6...v2.1.0) - 2022-04-26

There are two known issues with this release:

- Erroneous warning message in validator client when bellatrix fee recipient is unset. This is a cosmetic message and
  does not affect run time behavior in Phase0/Altair.
- In Bellatrix/Kiln: Fee recipient flags may not work as expected. See for a fix and more details.

### Added

- Web3Signer support. See the [documentation](https://docs.prylabs.network/docs/next/wallet/web3signer) for more
  details.
- Bellatrix support. See [kiln testnet instructions](https://hackmd.io/OqIoTiQvS9KOIataIFksBQ?view)
- Weak subjectivity sync / checkpoint sync. This is an experimental feature and may have unintended side effects for
  certain operators serving historical data. See
  the [documentation](https://docs.prylabs.network/docs/next/prysm-usage/checkpoint-sync) for more details.
- A faster build of blst for beacon chain on linux amd64. Use the environment variable `USE_PRYSM_MODERN=true` with
  prysm.sh, use the "modern" binary, or bazel build with `--define=blst_modern=true`.
- Vectorized sha256. This may have performance improvements with use of the new flag `--enable-vectorized-htr`.
- A new forkchoice structure that uses a doubly linked tree implementation. Try this feature with the
  flag `--enable-forkchoice-doubly-linked-tree`
- Fork choice proposer boost is implemented and enabled by default. See PR description for more details.

### Changed

- **Flag Default Change** The default value for `--http-web3provider` is now `localhost:8545`. Previously was empty
  string.
- Updated spectest compliance to v1.1.10.
- Updated to bazel 5.0.0
- Gossip peer scorer is now part of the `--dev` flag.

### Removed

- Removed released feature for next slot cache. `--disable-next-slot-state-cache` flag has been deprecated and removed.

### Fixed

Too many bug fixes and improvements to mention all of them. See
the [full changelist](https://github.com/prysmaticlabs/prysm/compare/v2.0.6...v2.1.0)

### Security

There are no security updates in this release.

## [v2.0.6](https://github.com/prysmaticlabs/prysm/compare/v2.0.5...v2.0.6) 2022-01-31

### Added

- Bellatrix/Merge progress
- Light client support merkle proof retrieval for beacon state finalized root and sync committees
- Web3Signer support (work in progress)
- Implement state management with native go structs (work in progress)
- Added static analysis for mutex lock management
- Add endpoint to query eth1 connections
- Batch gossipsub verification enabled
- Get block optimizations enabled
- Batch decompression for signatures
- Balance trie feature enabled

### Changed

- Use build time constants for field lengths.
- Monitoring service logging improvements / cleanup
- Renamed state v3 import alias
- Spec tests passing at tag 1.1.8
- Bazel version updated to 4.2.2
- Renamed github.com/eth2-clients -> github.com/eth-clients
- p2p reduce memory allocation in gossip digest calculation
- Allow comma separated formatting for event topics in API requests
- Update builder image from buster to bullseye
- Renaming "merge" to "bellatrix"
- Refactoring / code dedupication / general clean up
- Update libp2p
- Reduce state copy in state upgrades
- Deduplicate sync committee messages from pool before retrieval

### Removed

- tools/deployContract: removed k8s specific logic

### Fixed

- Sync committee API endpoint can now be queried for future epochs
- Initialize merkle layers and recompute dirty fields in beacon state proofs
- Fixed data race in API calls

### Security

- Clean variable filepaths in validator wallet back up commands, e2e tests, and other tooling (gosec G304)

## [v2.0.5](https://github.com/prysmaticlabs/prysm/compare/v2.0.4...v2.0.5) - 2021-12-13

### Added

- Implement import keystores standard API
- Added more fields to "Processed attestation aggregation" log
- Incremental changes to support The Merge hardfork
- Implement validator monitoring service in beacon chain node via flag `--monitor-indices`.
- Added validator log to display "aggregated since launch" every 5 epochs.
- Add HTTP client wrapper for interfacing with remote signer See
- Update web UI to version v1.0.2.

### Changed

- Refactor beacon state to allow for a single cached hasher
- Default config name to "devnet" when not provided in the config yaml.
- Alter erroneously capitalized error messages
- Bump spec tests to version v1.1.6
- Improvements to Doppelganger check
- Improvements to "grpc client connected" log.
- Update libp2p to v0.15.1
- Resolve several checks from deepsource
- Update go-ethereum to v1.10.13
- Update some flags from signed integer flags to unsigned flags.
- Filter errored keys from slashing protection history in standard API.
- Ensure slashing protection exports and key manager api work according to spec
- Improve memory performance by properly allocating slice size
- Typos fix
- Remove unused imports
- Use cashed finalized state when pruning deposits
- Significant slasher improvements
- Various code cleanups
- Standard API improvements for keymanager API
- Use safe sub64 for safer math
- Fix CORS in middleware API
- Add more fields to remote signer request object
- Refactoring to support checkpoint or genesis origin.

### Deprecated

Please be advised that Prysm's package path naming will change in the next release. If you are a downstream user of
Prysm (i.e. import prysm libraries into your project) then you may be impacted. Please see
issue https://github.com/prysmaticlabs/prysm/issues/10006.

### Fixed

- Allow API requests for next sync committee.
- Check sync status before performing a voluntary exit.
- Fixed issue where historical requests for validator balances would time out by removing the 30s timeout limitation.
- Add missing ssz spec tests

### Security

- Add justifications to gosec security finding suppression.

## [v2.0.4](https://github.com/prysmaticlabs/prysm/compare/v2.0.3...v2.0.4) - 2021-11-29

### Added

- Several changes for The Merge
- More monitoring functionality for blocks and sync committees

### Changed

- Improvements to block proposal computation when packing deposits.
- Renaming SignatureSet -> SignatureBatch

### Deprecated

### Fixed

- Revert PR [9830](https://github.com/prysmaticlabs/prysm/pull/9830) to remove performance regression. See:
  issue [9935](https://github.com/prysmaticlabs/prysm/issues/9935)

### Security

No security updates in this release.

## [v2.0.3](https://github.com/prysmaticlabs/prysm/compare/v2.0.2...v2.0.3) - 2021-11-22

This release also includes a major update to the web UI. Please review the v1 web UI
notes [here](https://github.com/prysmaticlabs/prysm-web-ui/releases/tag/v1.0.0)

### Added

- Web v1 released
- Updated Beacon API to v2.1.0
- Add validation of keystores via validator client RPC endpoint to support new web UI
- GitHub actions: errcheck and gosimple lint
- Event API support for `contribution_and_proof` and `voluntar_exit` events.
- Validator key management standard API schema and some implementation
- Add helpers for The Merge fork epoch calculation
- Add cli overrides for certain constants for The Merge
- Add beacon block and state structs for The Merge
- Validator monitoring improvements
- Cache deposits to improve deposit selection/processing
- Emit warning upon empty validator slashing protection export
- Add balance field trie cache and optimized hash trie root operations. `--enable-balance-trie-computation`

### Changed

- Updated to spectests v1.1.5
- Refactor web authentication
- Added uint64 overflow protection
- Sync committee pool returns empty slice instead of nil on cache miss
- Improved description of datadir flag
- Simplied web password requirements
- Web JWT tokens no longer expire.
- Updated keymanager protos
- Watch and update jwt secret when auth token file updated on disk.
- Update web based slashing protection export from POST to GET
- Reuse helpers to validate fully populated objects.
- Rename interop-cold-start to deterministic-genesis
- Validate password on RPC create wallet request
- Refactor for weak subjectivity sync implementation
- Update naming for Atlair previous epoch attester
- Remove duplicate MerkleizeTrieLeaves method.
- Add explict error for validator flag checks on out of bound positions
- Simplify method to check if the beacon chain client should update the justified epoch value.
- Rename web UI performance endpoint to "summary"
- Refactor powchain service to be more functional
- Use math.MaxUint64
- Share / reused finalized state on prysm start up services
- Refactor slashing protection history code packages
- Improve RNG commentary
- Use next slot cache in more areas of the application
- Improve context aware p2p peer scoring loops
- Various code clean up
- Prevent redundant processing of blocks from pending queue
- Enable Altair tests on e2e against prior release client
- Use lazy state balance cache

### Deprecated

- Web UI login has been replaced.
- Web UI bar graph removed.

### Removed

- Prysmatic Labs' [go-ethereum fork](https://github.com/prysmaticlabs/bazel-go-ethereum) removed from build tooling.
  Upstream go-ethereum is now used with familiar go.mod tooling.
- Removed duplicate aggergation validation p2p pipelines.
- Metrics calculation removed extra condition
- Removed superflous errors from peer scoring parameters registration

### Fixed

- Allow submitting sync committee subscriptions for next period
- Ignore validators without committee assignment when fetching attester duties
- Return "version" field for ssz blocks in beacon API
- Fixed bazel build transitions for dbg builds. Allows IDEs to hook into debugger again.
- Fixed case where GetDuties RPC endpoint might return a false positive for sync committee selection for validators that
  have no deposited yet
- Fixed validator exits in v1 method, broadcast correct object
- Fix Altair individual votes endpoint
- Validator performance calculations fixed
- Return correct response from key management api service
- Check empty genesis validators root on slashing protection data export
- Fix stategen with genesis state.
- Fixed multiple typos
- Fix genesis state registration in interop mode
- Fix network flags in slashing protection export

### Security

- Added another encryption key to security.txt.

## [v2.0.2](https://github.com/prysmaticlabs/prysm/compare/v2.0.1...v2.0.2) - 2021-10-18

### Added

- Optimizations to block proposals. Enabled with `--enable-get-block-optimizations`.
  See [issue 8943](https://github.com/prysmaticlabs/prysm/issues/8943)
  and [issue 9708](https://github.com/prysmaticlabs/prysm/issues/9708) before enabling.
- Beacon Standard API: register v1alpha2 endpoints

### Changed

- Beacon Standard API: Improved sync error messages
- Beacon Standard API: Omit validators without sync duties
- Beacon Standard API: Return errors for unknown state/block versions
- Spec alignment: Passing spec vectors at v1.1.2
- Logs: Improved "synced block.."
- Bazel: updated to v4.2.1
- E2E: more strict participation checks
- Eth1data: Reduce disk i/o saving interval

### Deprecated

- ⚠️ v2 Remote slashing protection server disabled for now ⚠️

### Fixed

- Beacon Standard API: fetch sync committee duties for current and next period's epoch
- Beacon Standard API: remove special treatment to graffiti in block results
- Beacon Standard API: fix epoch calculation in sync committee duties
- Doppelganger: Fix false positives
- UI: Validator gRPC gateway health endpoint fixed

### Security

- Spec alignment: Update Eth2FastAggregateVerify to match spec
- Helpers: enforce stronger slice index checks
- Deposit Trie: Handle impossible non-power of 2 trie leaves
- UI: Add security headers

## [v2.0.1](https://github.com/prysmaticlabs/prysm/compare/v2.0.0...v2.0.1) - 2021-10-06

### Fixed

- Updated libp2p transport library to stop metrics logging errors on windows.
- Prysm's web UI assets serve properly
- Eth2 api returns full validator balance rather than effective balance
- Slashing protection service registered properly in validator.

### Security

We've updated the Prysm base docker images to a more recent build.

## [v2.0.0](https://github.com/prysmaticlabs/prysm/compare/v1.4.4...v2.0.0)

This release is the largest release of Prysm to date. v2.0.0 includes support for the upcoming Altair hard fork on the
mainnet Ethereum Beacon Chain.
This release consists
of [380 changes](https://github.com/prysmaticlabs/prysm/compare/v1.4.4...f7845afa575963302116e673d400d2ab421252ac) to
support Altair, improve performance of phase0 beacon nodes, and various bug fixes from v1.4.4.

### Upgrading From v1

Please update your beacon node to v2.0.0 prior to updating your validator. The beacon node can serve requests to a
v1.4.4 validator, however a v2.0.0 validator will not start against a v1.4.4 beacon node. If you're operating a highly
available beacon chain service, ensure that all of your beacon nodes are updated to v2.0.0 before starting the upgrade
on your validators.

### Added

- Full Altair
  support. [Learn more about Altair.](https://github.com/ethereum/annotated-spec/blob/8473024d745a3a2b8a84535d57773a8e86b66c9a/altair/beacon-chain.md)
- Added bootnodes from the Nimbus team.
- Revamped slasher implementation. The slasher functionality is no longer a standalone binary. Slasher functionality is
  available from the beacon node with the `--slasher` flag. Note: Running the slasher has considerably increased
  resource requirements. Be sure to review the latest documentation before enabling this feature. This feature is
  experimental.
- Support for standard JSON API in the beacon node. Prysm validators continue to use Prysm's API.
- Configurable subnet peer requirements. Increased minimum desired peers per subnet from 4 to 6. This can be modified
  with `--minimum-peers-per-subnet` in the beacon node..
- Support for go build on darwin_arm64 devices (Mac M1 chips). Cross compiling for darwin_arm64 is not yet supported..
- Batch verification of pubsub objects. This should improve pubsub processing performance on multithreaded machines.
- Improved attestation pruning. This feature should improve block proposer performance and overall network attestation
  inclusion rates. Opt-out with `--disable-correctly-prune-canonical-atts` in the beacon node.
- Active balance cache to improve epoch processing. Opt-out with `--disable-active-balance-cache`
- Experimental database improvements to reduce history state entry space usage in the beaconchain.db. This functionality
  can be permanently enabled with the flag `--enable-historical-state-representation`. Enabling this feature can realize
  a 25% improvement in space utilization for the average user , while 70 -80% for power users(archival node operators).
  Note: once this feature is toggled on, it modifies the structure of the database with a migration and cannot be rolled
  back. This feature is experimental and should only be used in non-serving beacon nodes in case of database corruption
  or other critical issue.

#### New Metrics

**Beacon chain node**

| Metric                                           | Description                                                                                           | References |
| ------------------------------------------------ | ----------------------------------------------------------------------------------------------------- | ---------- |
| `p2p_message_ignored_validation_total`           | Count of messages that were ignored in validation                                                     |            |
| `beacon_current_active_validators`               | Current total active validators                                                                       |            |
| `beacon_processed_deposits_total`                | Total number of deposits processed                                                                    |            |
| `sync_head_state_miss`                           | The number of sync head state requests that are not present in the cache                              |            |
| `sync_head_state_hit`                            | The number of sync head state requests that are present in the cache                                  |            |
| `total_effective_balance_cache_miss`             | The number of get requests that are not present in the cache                                          |            |
| `total_effective_balance_cache_hit`              | The number of get requests that are present in the cache                                              |            |
| `sync_committee_index_cache_miss_total`          | The number of committee requests that aren't present in the sync committee index cache                |            |
| `sync_committee_index_cache_hit_total`           | The number of committee requests that are present in the sync committee index cache                   |            |
| `next_slot_cache_hit`                            | The number of cache hits on the next slot state cache                                                 |            |
| `next_slot_cache_miss`                           | The number of cache misses on the next slot state cache                                               |            |
| `validator_entry_cache_hit_total`                | The number of cache hits on the validator entry cache                                                 |            |
| `validator_entry_cache_miss_total`               | The number of cache misses on the validator entry cache                                               |            |
| `validator_entry_cache_delete_total`             | The number of cache deletes on the validator entry cache                                              |            |
| `saved_sync_committee_message_total`             | The number of saved sync committee message total                                                      |            |
| `saved_sync_committee_contribution_total`        | The number of saved sync committee contribution total                                                 |            |
| `libp2p_peers`                                   | Tracks the total number of libp2p peers                                                               |            |
| `p2p_status_message_missing`                     | The number of attempts the connection handler rejects a peer for a missing status message             |            |
| `p2p_sync_committee_subnet_recovered_broadcasts` | The number of sync committee messages that were attempted to be broadcast with no peers on the subnet |            |
| `p2p_sync_committee_subnet_attempted_broadcasts` | The number of sync committees that were attempted to be broadcast                                     |            |
| `p2p_subscribed_topic_peer_total`                | The number of peers subscribed to topics that a host node is also subscribed to                       |            |
| `saved_orphaned_att_total`                       | Count the number of times an orphaned attestation is saved                                            |            |

### Changed

- Much refactoring of "util" packages into more canonical packages. Please review Prysm package structure and godocs.
- Altair object keys in beacon-chain/db/kv are prefixed with "altair". BeaconBlocks and BeaconStates are the only
  objects affected by database key changes for Altair. This affects any third party tooling directly querying Prysm's
  beaconchain.db.
- Updated Teku bootnodes.
- Updated Lighthouse bootnodes.
- End to end testing now collects jaeger spans
- Improvements to experimental peer quality scoring. This feature is only enabled with `--enable-peer-scorer`.
- Validator performance logging behavior has changed in Altair. Post-Altair hardfork has the following changes:
  Inclusion distance and inclusion slots will no longer be displayed. Correctly voted target will only be true if also
  included within 32 slots. Correctly voted head will only be true if the attestation was included in the next slot.
  Correctly voted source will only be true if attestation is included within 5 slots. Inactivity score will be
  displayed.
- Increased pubsub message queue size from 256 to 600 to support larger networks and higher message volume.
- The default attestation aggregation changed to the improved optimized max cover algorithm.
- Prysm is passing spectests at v1.1.0 (latest available release).
- `--subscribe-all-subnets` will subscribe to all attestation subnets and sync subnets in post-altair hard fork.
- "eth2" is now an illegal term. If you say it or type it then something bad might happen.
- Improved cache hit ratio for validator entry cache.
- Reduced memory overhead during database migrations.
- Improvements to beacon state writes to database.

#### Changed Metrics

**Beacon chain node**
| Metric                | Old Name             | Description                                          | References |
| --------------------- | -------------------- | ---------------------------------------------------- | ---------- |
| `beacon_reorgs_total` | `beacon_reorg_total` | Count the number of times a beacon chain has a reorg |            |

### Deprecated

These flags are hidden from the help text and no longer modify the behavior of Prysm. These flags should be removed from
user runtime configuration as the flags will eventually be removed entirely and Prysm will fail to start if a deleted or
unknown flag is provided.

- `--enable-active-balance-cache`
- `--correctly-prune-canonical-atts`
- `--correctly-insert-orphaned-atts`
- `--enable-next-slot-state-cache`

### Removed

Note: Removed flags will block starting up with an error "flag provided but not defined:".
Please check that you are not using any of the removed flags in this section!

- Prysm's standalone slasher application (cmd/slasher) has been fully removed. Use the `--slasher` flag with a beacon
  chain node for full slasher functionality.
- `--disable-blst` (beacon node and validator). [blst](https://github.com/supranational/blst) is the only BLS library
  offered for Prysm.
- `--disable-sync-backtracking` and `--enable-sync-backtracking` (beacon node). This feature has been released for some
  time. See.
- `--diable-pruning-deposit-proofs` (beacon node). This feature has been released for some time. See.
- `--disable-eth1-data-majority-vote` (beacon node). This feature is no longer in use in Prysm. See,.
- `--proposer-atts-selection-using-max-cover` (beacon node). This feature has been released for some time. See.
- `--update-head-timely` (beacon node). This feature was released in v1.4.4. See.
- `--enable-optimized-balance-update` (beacon node). This feature was released in v1.4.4. See.
- Kafka support is no longer available in the beacon node. This functionality was never fully completed and did not
  fulfill many desirable use cases. This removed the flag `--kafka-url` (beacon node). See.
- Removed tools/faucet. Use the faucet
  in [prysmaticlabs/periphery](https://github.com/prysmaticlabs/periphery/tree/c2ac600882c37fc0f2a81b0508039124fb6bcf47/eth-faucet)
  if operating a testnet faucet server.
- Tooling for prior testnet contracts has been removed. Any of the old testnet contracts with `drain()` function have
  been removed as well.
- Toledo tesnet config is removed.
- Removed --eth-api-port (beacon node). All APIs interactions have been moved to --grpc-gateway-port. See.

### Fixed

- Database lock contention improved in block database operations.
- JSON API now returns an error when unknown fields are provided.
- Correctly return `epoch_transition` field in `head` JSON API events stream.
- Various fixes in standard JSON API
- Finalize deposits before initializing the beacon node. This may improve missed proposals
- JSON API returns header "Content-Length" 0 when returning an empty JSON object.
- Initial sync fixed when there is a very long period of missing blocks.
- Fixed log statement when a web3 endpoint failover occurs.
- Windows prysm.bat is fixed

### Security

- You MUST update to v2.0.0 or later release before epoch 74240 or your client will fork off from the rest of the
  network.
- Prysm's JWT library has been updated to a maintained version of the previous JWT library. JWTs are only used in the
  UI.

Please review our newly
updated [security reporting policy](https://github.com/prysmaticlabs/prysm/blob/develop/SECURITY.md).

- Fix subcommands such as validator accounts list

### Security

There are no security updates in this release.

# Older than v2.0.0

For changelog history for releases older than v2.0.0, please refer to https://github.com/prysmaticlabs/prysm/releases
