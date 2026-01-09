package genesis

import "errors"

var ErrFilePathUnset = errors.New("path to genesis data directory is not set")
var ErrGenesisStateNotInitialized = errors.New("genesis state has not been initialized")
var ErrNotGenesisStateFile = errors.New("file is not a genesis state file")
var ErrGenesisFileNotFound = errors.New("genesis state file not found")
