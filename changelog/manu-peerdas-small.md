### Added 
- `verifyBlobCommitmentCount`: Print max allowed blob count in error message.


### Ignored
- `TestPersist`: Use `fieldparams.RootLength` instead of `32`.
- `TestDataColumnSidecarsByRootReq_Marshal`: Remove blank line.
- `ConvertPeerIDToNodeID`: Improve readability by using one line per field.

### Changed
- `parseIndices`: Return `[]int` instead of `[]uint64`.

