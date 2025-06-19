package genesis

import "errors"

// ErrGenesisStorage wraps any error that results from interacting with the genesis storage filesystem.
var ErrGenesisStorage = errors.New("error interacting with genesis storage filesystem")

var ErrFilePathUnset = errors.New("path to genesis data directory is not set")

var ErrGenesisStateNotInitialized = errors.New("genesis state has not been initialized")

var ErrNotGenesisStateFile = errors.New("file is not a genesis state file")

var ErrGenesisFileNotFound = errors.New("genesis state file not found")
