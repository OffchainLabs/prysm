### Added
- New option to select an alternate blob storage layout. Rather than a flat directory with a subdir for each block root, a multi-level scheme is used to organize blobs by epoch/slot/root, enabling leaner syscalls, indexing and pruning.
