### Changed

- Updated go to go1.24.0.
- Updated gosec to v2.22.1 and golangci to v1.64.5.
- Updated github.com/trailofbits/go-mutexasserts.
- Updated rules_go to cf3c3af34bd869b864f5f2b98e2f41c2b220d6c9 to support go1.24.0.

### Fixed

- Fixed use of deprecated rand.Seed. 
- Fixed build issue with SszGen where the go binary was not present in the $PATH.
