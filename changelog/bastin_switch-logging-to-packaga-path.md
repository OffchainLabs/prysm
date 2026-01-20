### Changed

- Added a log.go file for every important package with a logger variable containing a `package` field set to the package
  path.
- Added a CI check to ensure every important package has a log.go file with the correct `package` field.
- Changed the log formatter to use this `package` field instead of the previous `prefix` field.