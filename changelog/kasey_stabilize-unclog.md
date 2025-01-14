### Changed

- Updated geth to 1.14~. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14351)
- E2e tests start from bellatrix. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14351)

### Removed

- Remove `/memsize/` pprof endpoint as it will no longer be supported in go 1.23. [[PR]](https://github.com/prysmaticlabs/prysm/pull/14351)

### Ignored

- switches unclog from using a flaky github artifact to using a stable release asset.
- Adds changelog entries that were missing from the branch switching prysm over to unclog.
