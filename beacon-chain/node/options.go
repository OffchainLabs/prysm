package node

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/builder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v6/config/params"
)

// Option for beacon node configuration.
type Option func(bn *BeaconNode) error

// WithBlockchainFlagOptions includes functional options for the blockchain service related to CLI flags.
func WithBlockchainFlagOptions(opts []blockchain.Option) Option {
	return func(bn *BeaconNode) error {
		bn.serviceFlagOpts.blockchainFlagOpts = opts
		return nil
	}
}

// WithExecutionChainOptions includes functional options for the execution chain service related to CLI flags.
func WithExecutionChainOptions(opts []execution.Option) Option {
	return func(bn *BeaconNode) error {
		bn.serviceFlagOpts.executionChainFlagOpts = opts
		return nil
	}
}

// WithBuilderFlagOptions includes functional options for the builder service related to CLI flags.
func WithBuilderFlagOptions(opts []builder.Option) Option {
	return func(bn *BeaconNode) error {
		bn.serviceFlagOpts.builderOpts = opts
		return nil
	}
}

// WithBlobStorage sets the BlobStorage backend for the BeaconNode
func WithBlobStorage(bs *filesystem.BlobStorage) Option {
	return func(bn *BeaconNode) error {
		bn.BlobStorage = bs
		return nil
	}
}

// WithBlobStorageOptions appends 1 or more filesystem.BlobStorageOption on the beacon node,
// to be used when initializing blob storage.
func WithBlobStorageOptions(opt ...filesystem.BlobStorageOption) Option {
	return func(bn *BeaconNode) error {
		bn.BlobStorageOptions = append(bn.BlobStorageOptions, opt...)
		return nil
	}
}

func WithConfigOptions(opt ...params.Option) Option {
	return func(bn *BeaconNode) error {
		bn.ConfigOptions = append(bn.ConfigOptions, opt...)
		return nil
	}
}

// WithDataColumnStorage sets the DataColumnStorage backend for the BeaconNode
func WithDataColumnStorage(bs *filesystem.DataColumnStorage) Option {
	return func(bn *BeaconNode) error {
		bn.DataColumnStorage = bs
		return nil
	}
}

// WithDataColumnStorageOptions appends 1 or more filesystem.DataColumnStorageOption on the beacon node,
// to be used when initializing data column storage.
func WithDataColumnStorageOptions(opt ...filesystem.DataColumnStorageOption) Option {
	return func(bn *BeaconNode) error {
		bn.DataColumnStorageOptions = append(bn.DataColumnStorageOptions, opt...)
		return nil
	}
}
