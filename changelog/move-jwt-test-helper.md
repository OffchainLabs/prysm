### Changed

- Moved the test-only JWT helper `createTokenString` out of production code and into `*_test.go` to avoid shipping unused logic and dependencies in `validator/rpc`.
