### Changed

- `getVerifyingState`: Optimize state replay for concurrent data column verification using singleflight deduplication.
- `getVerifyingState`: Check state caches before entering the expensive singleflight state replay.