### Removed

- Removed `k8s.io/apimachinery` and `k8s.io/client-go` dependencies (~120 transitive dependencies eliminated)
- Binary size reduced by ~4 MB (~5%) for beacon-chain and validator

### Added

- Custom FIFO cache implementation in `container/fifo` replacing `k8s.io/client-go/tools/cache`
