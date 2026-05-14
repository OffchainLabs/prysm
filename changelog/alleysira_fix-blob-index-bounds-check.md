### Fixed

- Add bounds check for blob index to prevent out-of-range access and return `ErrIncorrectBlobIndex` instead of panic.