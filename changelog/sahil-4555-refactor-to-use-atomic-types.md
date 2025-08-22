### Added

The old code uses clunky atomic operations that are hard to read and easy to mess up. The new code uses Go's simpler atomic types that do the same job but with cleaner, safer syntax. this makes less likely to have threading bugs when handling multiple operations at once.
