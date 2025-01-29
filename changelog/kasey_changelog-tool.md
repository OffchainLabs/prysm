### Added

- Added an error field to log `Finished building block`. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14696)
- Implemented a new `EmptyExecutionPayloadHeader` function. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14713)
- Added proper gas limit check for header from the builder. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14707)
- `Finished building block`: Display error only if not nil. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14722)
- Added light client feature flag check to RPC handlers. [PR](https://github.com/prysmaticlabs/prysm/pull/14736)
- Added support to update target and max blob count to different values per hard fork config. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14678)
- Log before blob filesystem cache warm-up. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14735)
- New design for the attestation pool. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14324)
- Add field param placeholder for Electra blob target and max to pass spec tests. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14733)
- Light client: Add better error handling. [PR](https://github.com/prysmaticlabs/prysm/pull/14749)
- Add EIP-7691: Blob throughput increase. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14750)
- Trace IDONTWANT Messages in Pubsub. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14778)
- Add Fulu fork boilerplate. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14771)
- DB optimization for saving light client bootstraps (save unique sync committees only)
- Separate type for unaggregated network attestations. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14659)


### Changed

- Process light client finality updates only for new finalized epochs instead of doing it for every block. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14713)
- Refactor subnets subscriptions. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14711)
- Refactor RPC handlers subscriptions. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14732)
- Go deps upgrade, from `ioutil` to `io`. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14737)
- Move successfully registered validator(s) on builder log to debug. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14735)
- Update some test files to use `crypto/rand` instead of `math/rand`. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14747)
- Re-organize the content of the `*.proto` files (No functional change). [[PR]](https://github.com/prysmaticlabs/prysm/pull/14755)
- SSZ files generation: Remove the `// Hash: ...` header.[[PR]](https://github.com/prysmaticlabs/prysm/pull/14760)
- Updated Electra spec definition for `process_epoch`. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14768)
- Update our `go-libp2p-pubsub` dependency. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14770)
- Re-organize the content of files to ease the creation of a new fork boilerplate. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14761)
- Updated spec definition electra `process_registry_updates`. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14767)
- Fixed Metadata errors for peers connected via QUIC. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14776)
- Updated spec definitions for `process_slashings` in godocs. Simplified `ProcessSlashings` API. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14766)
- Update spec tests to v1.5.0-beta.0. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14788)
- Process light client finality updates only for new finalized epochs instead of doing it for every block. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14718)
- Update blobs by rpc topics from V2 to V1. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14785)

### Fixed

- Added check to prevent nil pointer deference or out of bounds array access when validating the BLSToExecutionChange on an impossibly nil validator. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14705)
- EIP-7691: Ensure new blobs subnets are subscribed on epoch in advance. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14759)
- Fix kzg commitment inclusion proof depth minimal value. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14787)

### Removed

- Cleanup ProcessSlashings method to remove unnecessary argument. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14762)
- Remove `/proto/eth/v2` directory. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14765)

### Security

- go version upgrade to 1.22.10 for CVE CVE-2024-34156. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14729)
- Update golang.org/x/crypto to v0.31.0 to address CVE-2024-45337. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14777)
- Update golang.org/x/net to v0.33.0 to address CVE-2024-45338. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14780)
