# PeerDAS Sync Implementation Review

## Overview
This PR implements data column syncing for PeerDAS (Peer Data Availability Sampling) in the Fulu upgrade. The changes span across multiple areas of the beacon chain sync system.

## Review Areas

### 1. Core Data Column Sync Infrastructure
**Files:**
- `beacon-chain/sync/data_column_sidecars.go` (NEW - 772 lines)
- `beacon-chain/sync/data_column_sidecars_test.go` (NEW - 219 lines)

**Items to Review:**
- New `DataColumnSidecarsParams` struct and related types
- Data column fetching and validation logic
- Peer scoring and rate limiting for data columns
- Integration with existing sync infrastructure
- Error handling and retry mechanisms
- Memory and resource management

### 2. Initial Sync Enhancements
**Files:**
- `beacon-chain/sync/initial-sync/blocks_fetcher.go` (212 lines changed)
- `beacon-chain/sync/initial-sync/blocks_fetcher_test.go` (164 lines changed)
- `beacon-chain/sync/initial-sync/blocks_fetcher_utils.go` (24 lines changed)
- `beacon-chain/sync/initial-sync/round_robin.go` (192 lines changed)
- `beacon-chain/sync/initial-sync/round_robin_test.go` (174 lines changed)
- `beacon-chain/sync/initial-sync/service.go` (171 lines changed)
- `beacon-chain/sync/initial-sync/service_test.go` (153 lines changed)

**Items to Review:**
- Integration of data column fetching into initial sync process
- Changes to block fetcher to handle data columns
- Round robin sync modifications for PeerDAS
- Service-level changes for data column support
- Test coverage for new data column sync functionality
- Performance implications of additional sync data

### 3. RPC and Peer Communication
**Files:**
- `beacon-chain/sync/rpc_beacon_blocks_by_root.go` (131 lines changed)
- `beacon-chain/sync/rpc_data_column_sidecars_by_root.go` (4 lines changed)
- `beacon-chain/sync/rpc_data_column_sidecars_by_root_test.go` (4 lines changed)
- `beacon-chain/sync/rpc_send_request.go` (42 lines changed)
- `beacon-chain/sync/rpc_send_request_test.go` (25 lines changed)

**Items to Review:**
- RPC method modifications for data column requests
- Request/response handling for data column sidecars
- Peer communication protocol changes
- Rate limiting and request validation
- Error handling in RPC layer

### 4. Data Availability System (DAS)
**Files:**
- `beacon-chain/das/availability_blobs.go` (19 lines changed)
- `beacon-chain/das/availability_blobs_test.go` (26 lines changed)
- `beacon-chain/das/availability_columns.go` (213 lines removed)
- `beacon-chain/das/availability_columns_test.go` (313 lines removed)
- `beacon-chain/das/iface.go` (10 lines changed)
- `beacon-chain/das/mock.go` (9 lines changed)
- `beacon-chain/das/BUILD.bazel` (5 lines changed)

**Items to Review:**
- Removal of old availability_columns implementation
- Interface changes and their impact
- Mock updates for testing
- Integration with new sync system
- Backward compatibility considerations

### 5. Storage Layer Changes
**Files:**
- `beacon-chain/db/filesystem/data_column.go` (8 lines added)
- `beacon-chain/db/filesystem/data_column_cache.go` (8 lines removed)
- `beacon-chain/db/filesystem/mock.go` (11 lines removed)
- `beacon-chain/db/iface/interface.go` (21 lines changed)

**Items to Review:**
- Database interface changes for data columns
- Filesystem storage modifications
- Cache management updates
- Mock implementation changes
- Data persistence and retrieval patterns

### 6. Block Processing Integration
**Files:**
- `beacon-chain/blockchain/process_block.go` (25 lines changed)
- `beacon-chain/sync/pending_blocks_queue.go` (171 lines changed)

**Items to Review:**
- Integration of data column processing in block processing pipeline
- Pending blocks queue modifications for data columns
- Block validation with data column requirements
- Queue management and ordering
- Error handling and rollback scenarios

### 7. Consensus Types and Protocol
**Files:**
- `consensus-types/blocks/proto.go` (4 lines changed)
- `consensus-types/blocks/roblock.go` (11 lines changed)
- `consensus-types/blocks/rodatacolumn.go` (18 lines changed)

**Items to Review:**
- Consensus type modifications for data columns
- Read-only block interface changes
- Data column type definitions and methods
- Serialization and deserialization updates
- Type safety and validation

### 8. Verification and Validation
**Files:**
- `beacon-chain/verification/data_column.go` (9 lines added)
- `beacon-chain/sync/verify/blob.go` (4 lines changed)

**Items to Review:**
- Data column verification logic
- Integration with existing blob verification
- Cryptographic validation requirements
- Performance of verification operations
- Error handling in verification pipeline

### 9. Configuration and Testing
**Files:**
- `cmd/beacon-chain/flags/config.go` (2 lines changed)
- `beacon-chain/core/helpers/sync_committee_test.go` (2 lines added)
- `beacon-chain/p2p/peers/scorers/gossip_scorer_test.go` (2 lines changed)

**Items to Review:**
- Configuration flag changes
- Test infrastructure updates
- Peer scoring modifications
- Integration test coverage
- End-to-end testing scenarios

## Priority Review Order
1. **High Priority:** Core Data Column Sync Infrastructure (Area 1)
2. **High Priority:** Initial Sync Enhancements (Area 2) 
3. **Medium Priority:** Data Availability System Changes (Area 4)
4. **Medium Priority:** RPC and Peer Communication (Area 3)
5. **Medium Priority:** Block Processing Integration (Area 6)
6. **Low Priority:** Storage Layer Changes (Area 5)
7. **Low Priority:** Consensus Types and Protocol (Area 7)
8. **Low Priority:** Verification and Validation (Area 8)
9. **Low Priority:** Configuration and Testing (Area 9)