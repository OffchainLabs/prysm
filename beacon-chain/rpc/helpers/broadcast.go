package helpers

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// DataColumnBroadcastFunc is a function for broadcasting data column sidecars.
type DataColumnBroadcastFunc func(root [32]byte, subnet uint64, sidecar *ethpb.DataColumnSidecar) error

// BlobBroadcastFunc is a function for broadcasting blob sidecars.
type BlobBroadcastFunc func(ctx context.Context, index uint64, sidecar *ethpb.BlobSidecar) error

// DataColumnOption configures options for broadcasting data column sidecars.
type DataColumnOption func(*dataColumnOptions)

type dataColumnOptions struct {
	onReceiveDataColumns   func(columns []blocks.VerifiedRODataColumn) error
	onDataColumnsProcessed func(columns []blocks.VerifiedRODataColumn)
}

// WithDataColumnReceiver sets the callback for receiving data columns.
func WithDataColumnReceiver(fn func(columns []blocks.VerifiedRODataColumn) error) DataColumnOption {
	return func(o *dataColumnOptions) {
		o.onReceiveDataColumns = fn
	}
}

// WithDataColumnProcessedCallback sets the callback to be called after data columns are processed.
func WithDataColumnProcessedCallback(fn func(columns []blocks.VerifiedRODataColumn)) DataColumnOption {
	return func(o *dataColumnOptions) {
		o.onDataColumnsProcessed = fn
	}
}

// BroadcastDataColumnSidecars broadcasts data column sidecars concurrently and optionally receives them.
func BroadcastDataColumnSidecars(
	ctx context.Context,
	sidecars []*ethpb.DataColumnSidecar,
	root [32]byte,
	broadcastFunc DataColumnBroadcastFunc,
	options ...DataColumnOption,
) error {
	opts := &dataColumnOptions{}
	for _, opt := range options {
		opt(opts)
	}

	verifiedRODataColumns := make([]blocks.VerifiedRODataColumn, 0, len(sidecars))

	// Create verified RO data columns
	for _, sd := range sidecars {
		roDataColumn, err := blocks.NewRODataColumnWithRoot(sd, root)
		if err != nil {
			return errors.Wrap(err, "new read-only data column with root")
		}

		// We build this block ourselves, so we can upgrade the read only data column sidecar into a verified one.
		verifiedRODataColumn := blocks.NewVerifiedRODataColumn(roDataColumn)
		verifiedRODataColumns = append(verifiedRODataColumns, verifiedRODataColumn)
	}

	// Broadcast data columns concurrently
	eg, _ := errgroup.WithContext(ctx)
	for _, sd := range sidecars {
		// Copy the iteration instance to a local variable to give each go-routine its own copy to play with.
		// See https://golang.org/doc/faq#closures_and_goroutines for more details.
		sidecar := sd
		eg.Go(func() error {
			// Compute the subnet index based on the column index.
			subnet := peerdas.ComputeSubnetForDataColumnSidecar(sidecar.Index)

			if err := broadcastFunc(root, subnet, sidecar); err != nil {
				return errors.Wrap(err, "broadcast data column")
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return errors.Wrap(err, "wait for data columns to be broadcasted")
	}

	// Optionally receive the data columns
	if opts.onReceiveDataColumns != nil {
		if err := opts.onReceiveDataColumns(verifiedRODataColumns); err != nil {
			return errors.Wrap(err, "receive data column")
		}
	}

	// Call the callback if provided
	if opts.onDataColumnsProcessed != nil {
		opts.onDataColumnsProcessed(verifiedRODataColumns)
	}

	return nil
}

// BlobOption configures options for broadcasting blob sidecars.
type BlobOption func(*blobOptions)

type blobOptions struct {
	onReceiveBlob     func(ctx context.Context, blob blocks.VerifiedROBlob) error
	onBlobProcessed   func(blob blocks.VerifiedROBlob)
	onCheckForkchoice func(root [32]byte) bool
}

// WithBlobReceiver sets the callback for receiving blobs.
func WithBlobReceiver(fn func(ctx context.Context, blob blocks.VerifiedROBlob) error) BlobOption {
	return func(o *blobOptions) {
		o.onReceiveBlob = fn
	}
}

// WithBlobProcessedCallback sets the callback to be called after a blob is processed.
func WithBlobProcessedCallback(fn func(blob blocks.VerifiedROBlob)) BlobOption {
	return func(o *blobOptions) {
		o.onBlobProcessed = fn
	}
}

// BroadcastBlobSidecars broadcasts blob sidecars concurrently and optionally receives them.
func BroadcastBlobSidecars(
	ctx context.Context,
	sidecars []*ethpb.BlobSidecar,
	root [32]byte,
	broadcastFunc BlobBroadcastFunc,
	options ...BlobOption,
) error {
	opts := &blobOptions{}
	for _, opt := range options {
		opt(opts)
	}

	eg, eCtx := errgroup.WithContext(ctx)
	for i, sc := range sidecars {
		// Copy the iteration instance to a local variable to give each go-routine its own copy to play with.
		// See https://golang.org/doc/faq#closures_and_goroutines for more details.
		subIdx := i
		sCar := sc
		eg.Go(func() error {
			if err := broadcastFunc(eCtx, uint64(subIdx), sCar); err != nil {
				return errors.Wrap(err, fmt.Sprintf("broadcast blob failed for index %d", subIdx))
			}

			// Optionally receive the blob
			if opts.onReceiveBlob != nil {
				readOnlySc, err := blocks.NewROBlobWithRoot(sCar, root)
				if err != nil {
					return errors.Wrap(err, "ROBlob creation failed")
				}
				verifiedBlob := blocks.NewVerifiedROBlob(readOnlySc)
				if err := opts.onReceiveBlob(ctx, verifiedBlob); err != nil {
					return errors.Wrap(err, "receive blob failed")
				}

				// Call the callback if provided
				if opts.onBlobProcessed != nil {
					opts.onBlobProcessed(verifiedBlob)
				}
			}

			return nil
		})
	}
	return eg.Wait()
}
