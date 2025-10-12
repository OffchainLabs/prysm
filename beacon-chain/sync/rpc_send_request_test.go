package sync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	p2pTypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/libp2p/go-libp2p/core/network"
)

func TestSendRequest_SendBeaconBlocksByRangeRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pcl := fmt.Sprintf("%s/ssz_snappy", p2p.RPCBlocksByRangeTopicV1)

	t.Run("stream error", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		// Bogus peer doesn't support a given protocol, so stream error is expected.
		bogusPeer := p2ptest.NewTestP2P(t)
		p1.Connect(bogusPeer)

		req := &ethpb.BeaconBlocksByRangeRequest{}
		_, err := SendBeaconBlocksByRangeRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, bogusPeer.PeerID(), req, nil)
		assert.ErrorContains(t, "protocols not supported", err)
	})

	knownBlocks := make([]*ethpb.SignedBeaconBlock, 0)
	genesisBlk := util.NewBeaconBlock()
	genesisBlkRoot, err := genesisBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	parentRoot := genesisBlkRoot
	for i := 0; i < 255; i++ {
		blk := util.NewBeaconBlock()
		blk.Block.Slot = primitives.Slot(i)
		blk.Block.ParentRoot = parentRoot[:]
		knownBlocks = append(knownBlocks, blk)
		parentRoot, err = blk.Block.HashTreeRoot()
		require.NoError(t, err)
	}

	knownBlocksProvider := func(p2pProvider p2p.P2P, processor BeaconBlockProcessor) func(stream network.Stream) {
		return func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()

			req := &ethpb.BeaconBlocksByRangeRequest{}
			assert.NoError(t, p2pProvider.Encoding().DecodeWithMaxLength(stream, req))

			for i := req.StartSlot; i < req.StartSlot.Add(req.Count*req.Step); i += primitives.Slot(req.Step) {
				if processor != nil {
					wsb, err := blocks.NewSignedBeaconBlock(knownBlocks[i])
					require.NoError(t, err)
					if processorErr := processor(wsb); processorErr != nil {
						if errors.Is(processorErr, io.EOF) {
							// Close stream, w/o any errors written.
							return
						}
						_, err := stream.Write([]byte{0x01})
						assert.NoError(t, err)
						msg := p2pTypes.ErrorMessage(processorErr.Error())
						_, err = p2pProvider.Encoding().EncodeWithMaxLength(stream, &msg)
						assert.NoError(t, err)
						return
					}
				}
				if uint64(i) >= uint64(len(knownBlocks)) {
					break
				}
				wsb, err := blocks.NewSignedBeaconBlock(knownBlocks[i])
				require.NoError(t, err)
				err = WriteBlockChunk(stream, startup.NewClock(time.Now(), [32]byte{}), p2pProvider.Encoding(), wsb)
				if err != nil && err.Error() != network.ErrReset.Error() {
					require.NoError(t, err)
				}
			}
		}
	}

	t.Run("no block processor", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		p2.SetStreamHandler(pcl, knownBlocksProvider(p2, nil))

		req := &ethpb.BeaconBlocksByRangeRequest{
			StartSlot: 20,
			Count:     128,
			Step:      1,
		}
		blocks, err := SendBeaconBlocksByRangeRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 128, len(blocks))
	})

	t.Run("has block processor - no errors", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		p2.SetStreamHandler(pcl, knownBlocksProvider(p2, nil))

		// No error from block processor.
		req := &ethpb.BeaconBlocksByRangeRequest{
			StartSlot: 20,
			Count:     128,
			Step:      1,
		}
		blocksFromProcessor := make([]interfaces.ReadOnlySignedBeaconBlock, 0)
		blocks, err := SendBeaconBlocksByRangeRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, func(block interfaces.ReadOnlySignedBeaconBlock) error {
			blocksFromProcessor = append(blocksFromProcessor, block)
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 128, len(blocks))
		assert.DeepEqual(t, blocks, blocksFromProcessor)
	})

	t.Run("has block processor - throw error", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		p2.SetStreamHandler(pcl, knownBlocksProvider(p2, nil))

		// Send error from block processor.
		req := &ethpb.BeaconBlocksByRangeRequest{
			StartSlot: 20,
			Count:     128,
			Step:      1,
		}
		errFromProcessor := errors.New("processor error")
		_, err := SendBeaconBlocksByRangeRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, func(block interfaces.ReadOnlySignedBeaconBlock) error {
			return errFromProcessor
		})
		assert.ErrorContains(t, errFromProcessor.Error(), err)
	})

	t.Run("max request blocks", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		p2.SetStreamHandler(pcl, knownBlocksProvider(p2, nil))

		// No cap on max roots.
		req := &ethpb.BeaconBlocksByRangeRequest{
			StartSlot: 20,
			Count:     128,
			Step:      1,
		}
		blocks, err := SendBeaconBlocksByRangeRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 128, len(blocks))

		// Cap max returned roots.
		cfg := params.BeaconConfig().Copy()
		maxRequestBlocks := cfg.MaxRequestBlocks
		defer func() {
			cfg.MaxRequestBlocks = maxRequestBlocks
			params.OverrideBeaconConfig(cfg)
		}()
		blocks, err = SendBeaconBlocksByRangeRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, func(block interfaces.ReadOnlySignedBeaconBlock) error {
			// Since ssz checks the boundaries, and doesn't normally allow to send requests bigger than
			// the max request size, we are updating max request size dynamically. Even when updated dynamically,
			// no more than max request size of blocks is expected on return.
			cfg.MaxRequestBlocks = 3
			params.OverrideBeaconConfig(cfg)
			return nil
		})
		assert.ErrorContains(t, ErrInvalidFetchedData.Error(), err)
		assert.Equal(t, 0, len(blocks))
	})

	t.Run("process custom error", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		blocksProcessed := 0
		expectedErr := errors.New("some error")
		p2.SetStreamHandler(pcl, knownBlocksProvider(p2, func(block interfaces.ReadOnlySignedBeaconBlock) error {
			if blocksProcessed > 2 {
				return expectedErr
			}
			blocksProcessed++
			return nil
		}))

		req := &ethpb.BeaconBlocksByRangeRequest{
			StartSlot: 20,
			Count:     128,
			Step:      1,
		}
		blocks, err := SendBeaconBlocksByRangeRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, nil)
		assert.ErrorContains(t, expectedErr.Error(), err)
		assert.Equal(t, 0, len(blocks))
	})

	t.Run("blocks out of order: step 1", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)

		// Switch known blocks, so that slots are out of order.
		knownBlocks[30], knownBlocks[31] = knownBlocks[31], knownBlocks[30]
		defer func() {
			knownBlocks[31], knownBlocks[30] = knownBlocks[30], knownBlocks[31]
		}()

		p2.SetStreamHandler(pcl, func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()

			req := &ethpb.BeaconBlocksByRangeRequest{}
			assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))

			for i := req.StartSlot; i < req.StartSlot.Add(req.Count*req.Step); i += primitives.Slot(req.Step) {
				if uint64(i) >= uint64(len(knownBlocks)) {
					break
				}
				wsb, err := blocks.NewSignedBeaconBlock(knownBlocks[i])
				require.NoError(t, err)
				err = WriteBlockChunk(stream, startup.NewClock(time.Now(), [32]byte{}), p2.Encoding(), wsb)
				if err != nil && err.Error() != network.ErrReset.Error() {
					require.NoError(t, err)
				}
			}
		})

		req := &ethpb.BeaconBlocksByRangeRequest{
			StartSlot: 20,
			Count:     128,
			Step:      1,
		}
		blocks, err := SendBeaconBlocksByRangeRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, nil)
		assert.ErrorContains(t, ErrInvalidFetchedData.Error(), err)
		assert.Equal(t, 0, len(blocks))

	})

	t.Run("blocks out of order: step 10", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)

		// Switch known blocks, so that slots are out of order.
		knownBlocks[30], knownBlocks[31] = knownBlocks[31], knownBlocks[30]
		defer func() {
			knownBlocks[31], knownBlocks[30] = knownBlocks[30], knownBlocks[31]
		}()

		p2.SetStreamHandler(pcl, func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()

			req := &ethpb.BeaconBlocksByRangeRequest{}
			assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))

			for i := req.StartSlot; i < req.StartSlot.Add(req.Count*req.Step); i += primitives.Slot(req.Step) {
				if uint64(i) >= uint64(len(knownBlocks)) {
					break
				}
				wsb, err := blocks.NewSignedBeaconBlock(knownBlocks[i])
				require.NoError(t, err)
				err = WriteBlockChunk(stream, startup.NewClock(time.Now(), [32]byte{}), p2.Encoding(), wsb)
				if err != nil && err.Error() != network.ErrReset.Error() {
					require.NoError(t, err)
				}
			}
		})

		req := &ethpb.BeaconBlocksByRangeRequest{
			StartSlot: 20,
			Count:     128,
			Step:      10,
		}
		blocks, err := SendBeaconBlocksByRangeRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, nil)
		assert.ErrorContains(t, ErrInvalidFetchedData.Error(), err)
		assert.Equal(t, 0, len(blocks))

	})
}

func TestSendRequest_SendBeaconBlocksByRootRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pcl := fmt.Sprintf("%s/ssz_snappy", p2p.RPCBlocksByRootTopicV1)

	knownBlocks := make(map[[32]byte]*ethpb.SignedBeaconBlock)
	knownRoots := make([][32]byte, 0)
	for i := 0; i < 5; i++ {
		blk := util.NewBeaconBlock()
		blkRoot, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		knownRoots = append(knownRoots, blkRoot)
		knownBlocks[knownRoots[len(knownRoots)-1]] = blk
	}

	t.Run("stream error", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		// Bogus peer doesn't support a given protocol, so stream error is expected.
		bogusPeer := p2ptest.NewTestP2P(t)
		p1.Connect(bogusPeer)

		req := &p2pTypes.BeaconBlockByRootsReq{}
		_, err := SendBeaconBlocksByRootRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, bogusPeer.PeerID(), req, nil)
		assert.ErrorContains(t, "protocols not supported", err)
	})

	knownBlocksProvider := func(p2pProvider p2p.P2P, processor BeaconBlockProcessor) func(stream network.Stream) {
		return func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()

			req := new(p2pTypes.BeaconBlockByRootsReq)
			assert.NoError(t, p2pProvider.Encoding().DecodeWithMaxLength(stream, req))
			if len(*req) == 0 {
				return
			}
			for _, root := range *req {
				if blk, ok := knownBlocks[root]; ok {
					if processor != nil {
						wsb, err := blocks.NewSignedBeaconBlock(blk)
						require.NoError(t, err)
						if processorErr := processor(wsb); processorErr != nil {
							if errors.Is(processorErr, io.EOF) {
								// Close stream, w/o any errors written.
								return
							}
							_, err := stream.Write([]byte{0x01})
							assert.NoError(t, err)
							msg := p2pTypes.ErrorMessage(processorErr.Error())
							_, err = p2pProvider.Encoding().EncodeWithMaxLength(stream, &msg)
							assert.NoError(t, err)
							return
						}
					}
					_, err := stream.Write([]byte{0x00})
					assert.NoError(t, err, "Could not write to stream")
					_, err = p2pProvider.Encoding().EncodeWithMaxLength(stream, blk)
					assert.NoError(t, err, "Could not send response back")
				}
			}
		}
	}

	t.Run("no block processor", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		p2.SetStreamHandler(pcl, knownBlocksProvider(p2, nil))

		req := &p2pTypes.BeaconBlockByRootsReq{knownRoots[0], knownRoots[1]}
		blocks, err := SendBeaconBlocksByRootRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(blocks))
	})

	t.Run("has block processor - no errors", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		p2.SetStreamHandler(pcl, knownBlocksProvider(p2, nil))

		// No error from block processor.
		req := &p2pTypes.BeaconBlockByRootsReq{knownRoots[0], knownRoots[1]}
		blocksFromProcessor := make([]interfaces.ReadOnlySignedBeaconBlock, 0)
		blocks, err := SendBeaconBlocksByRootRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, func(block interfaces.ReadOnlySignedBeaconBlock) error {
			blocksFromProcessor = append(blocksFromProcessor, block)
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(blocks))
		assert.DeepEqual(t, blocks, blocksFromProcessor)
	})

	t.Run("has block processor - throw error", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		p2.SetStreamHandler(pcl, knownBlocksProvider(p2, nil))

		// Send error from block processor.
		req := &p2pTypes.BeaconBlockByRootsReq{knownRoots[0], knownRoots[1]}
		errFromProcessor := errors.New("processor error")
		_, err := SendBeaconBlocksByRootRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, func(block interfaces.ReadOnlySignedBeaconBlock) error {
			return errFromProcessor
		})
		assert.ErrorContains(t, errFromProcessor.Error(), err)
	})

	t.Run("max request blocks", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		p2.SetStreamHandler(pcl, knownBlocksProvider(p2, nil))

		// No cap on max roots.
		req := &p2pTypes.BeaconBlockByRootsReq{knownRoots[0], knownRoots[1], knownRoots[2], knownRoots[3]}
		clock := startup.NewClock(time.Now(), [32]byte{})
		blocks, err := SendBeaconBlocksByRootRequest(ctx, clock, p1, p2.PeerID(), req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 4, len(blocks))

		// Cap max returned roots.
		cfg := params.BeaconConfig().Copy()
		maxRequestBlocks := cfg.MaxRequestBlocks
		defer func() {
			cfg.MaxRequestBlocks = maxRequestBlocks
			params.OverrideBeaconConfig(cfg)
		}()
		blocks, err = SendBeaconBlocksByRootRequest(ctx, clock, p1, p2.PeerID(), req, func(block interfaces.ReadOnlySignedBeaconBlock) error {
			// Since ssz checks the boundaries, and doesn't normally allow to send requests bigger than
			// the max request size, we are updating max request size dynamically. Even when updated dynamically,
			// no more than max request size of blocks is expected on return.
			cfg.MaxRequestBlocks = 3
			params.OverrideBeaconConfig(cfg)
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 3, len(blocks))
	})

	t.Run("process custom error", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		blocksProcessed := 0
		expectedErr := errors.New("some error")
		p2.SetStreamHandler(pcl, knownBlocksProvider(p2, func(block interfaces.ReadOnlySignedBeaconBlock) error {
			if blocksProcessed > 2 {
				return expectedErr
			}
			blocksProcessed++
			return nil
		}))

		req := &p2pTypes.BeaconBlockByRootsReq{knownRoots[0], knownRoots[1], knownRoots[2], knownRoots[3]}
		blocks, err := SendBeaconBlocksByRootRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, nil)
		assert.ErrorContains(t, expectedErr.Error(), err)
		assert.Equal(t, 0, len(blocks))
	})

	t.Run("process io.EOF error", func(t *testing.T) {
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		blocksProcessed := 0
		expectedErr := io.EOF
		p2.SetStreamHandler(pcl, knownBlocksProvider(p2, func(block interfaces.ReadOnlySignedBeaconBlock) error {
			if blocksProcessed > 2 {
				return expectedErr
			}
			blocksProcessed++
			return nil
		}))

		req := &p2pTypes.BeaconBlockByRootsReq{knownRoots[0], knownRoots[1], knownRoots[2], knownRoots[3]}
		blocks, err := SendBeaconBlocksByRootRequest(ctx, startup.NewClock(time.Now(), [32]byte{}), p1, p2.PeerID(), req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(blocks))
	})
}

func TestBlobValidatorFromRootReq(t *testing.T) {
	rootA := bytesutil.PadTo([]byte("valid"), 32)
	rootB := bytesutil.PadTo([]byte("invalid"), 32)
	header := &ethpb.SignedBeaconBlockHeader{
		Header:    &ethpb.BeaconBlockHeader{Slot: 0},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
	blobSidecarA0 := util.GenerateTestDenebBlobSidecar(t, bytesutil.ToBytes32(rootA), header, 0, []byte{}, make([][]byte, 0))
	blobSidecarA1 := util.GenerateTestDenebBlobSidecar(t, bytesutil.ToBytes32(rootA), header, 1, []byte{}, make([][]byte, 0))
	blobSidecarB0 := util.GenerateTestDenebBlobSidecar(t, bytesutil.ToBytes32(rootB), header, 0, []byte{}, make([][]byte, 0))
	cases := []struct {
		name     string
		ids      []*ethpb.BlobIdentifier
		response []blocks.ROBlob
		err      error
	}{
		{
			name:     "expected",
			ids:      []*ethpb.BlobIdentifier{{BlockRoot: rootA, Index: 0}},
			response: []blocks.ROBlob{blobSidecarA0},
		},
		{
			name:     "wrong root",
			ids:      []*ethpb.BlobIdentifier{{BlockRoot: rootA, Index: 0}},
			response: []blocks.ROBlob{blobSidecarB0},
			err:      errUnrequested,
		},
		{
			name:     "wrong index",
			ids:      []*ethpb.BlobIdentifier{{BlockRoot: rootA, Index: 0}},
			response: []blocks.ROBlob{blobSidecarA1},
			err:      errUnrequested,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := p2pTypes.BlobSidecarsByRootReq(c.ids)
			vf := blobValidatorFromRootReq(&r)
			for _, sc := range c.response {
				err := vf(sc)
				if c.err != nil {
					require.ErrorIs(t, err, c.err)
					return
				}
				require.NoError(t, err)
			}
		})
	}
}

func TestBlobValidatorFromRangeReq(t *testing.T) {
	cases := []struct {
		name         string
		req          *ethpb.BlobSidecarsByRangeRequest
		responseSlot primitives.Slot
		err          error
	}{
		{
			name: "valid - count multi",
			req: &ethpb.BlobSidecarsByRangeRequest{
				StartSlot: 10,
				Count:     10,
			},
			responseSlot: 14,
		},
		{
			name: "valid - count 1",
			req: &ethpb.BlobSidecarsByRangeRequest{
				StartSlot: 10,
				Count:     1,
			},
			responseSlot: 10,
		},
		{
			name: "invalid - before",
			req: &ethpb.BlobSidecarsByRangeRequest{
				StartSlot: 10,
				Count:     1,
			},
			responseSlot: 9,
			err:          errBlobResponseOutOfBounds,
		},
		{
			name: "invalid - after, count 1",
			req: &ethpb.BlobSidecarsByRangeRequest{
				StartSlot: 10,
				Count:     1,
			},
			responseSlot: 11,
			err:          errBlobResponseOutOfBounds,
		},
		{
			name: "invalid - after, multi",
			req: &ethpb.BlobSidecarsByRangeRequest{
				StartSlot: 10,
				Count:     10,
			},
			responseSlot: 23,
			err:          errBlobResponseOutOfBounds,
		},
		{
			name: "invalid - after, at boundary, multi",
			req: &ethpb.BlobSidecarsByRangeRequest{
				StartSlot: 10,
				Count:     10,
			},
			responseSlot: 20,
			err:          errBlobResponseOutOfBounds,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vf := blobValidatorFromRangeReq(c.req)
			header := &ethpb.SignedBeaconBlockHeader{
				Header:    &ethpb.BeaconBlockHeader{Slot: c.responseSlot},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			}
			sc := util.GenerateTestDenebBlobSidecar(t, [32]byte{}, header, 0, []byte{}, make([][]byte, 0))
			err := vf(sc)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSeqBlobValid(t *testing.T) {
	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	one, oneBlobs := generateTestBlockWithSidecars(t, [32]byte{}, ds, 3)
	r1, err := one.Block.HashTreeRoot()
	require.NoError(t, err)
	two, twoBlobs := generateTestBlockWithSidecars(t, r1, ds+1, 3)
	r2, err := two.Block.HashTreeRoot()
	require.NoError(t, err)
	_, oops := generateTestBlockWithSidecars(t, r2, ds, 4)
	oops[1].SignedBlockHeader.Header.ParentRoot = bytesutil.PadTo([]byte("derp"), 32)
	wrongRoot, err := blocks.NewROBlobWithRoot(oops[2].BlobSidecar, bytesutil.ToBytes32([]byte("parentderp")))
	require.NoError(t, err)
	oob := oops[3]
	oob.Index = uint64(params.BeaconConfig().MaxBlobsPerBlock(ds))

	cases := []struct {
		name  string
		seq   []blocks.ROBlob
		err   error
		errAt int
	}{
		{
			name: "all valid",
			seq:  append(append([]blocks.ROBlob{}, oneBlobs...), twoBlobs...),
		},
		{
			name: "idx out of bounds",
			seq:  []blocks.ROBlob{oob},
			err:  errBlobIndexOutOfBounds,
		},
		{
			name: "first index is not zero",
			seq:  []blocks.ROBlob{oneBlobs[1]},
			err:  errChunkResponseIndexNotAsc,
		},
		{
			name: "index out of order, same block",
			seq:  []blocks.ROBlob{oneBlobs[1], oneBlobs[0]},
			err:  errChunkResponseIndexNotAsc,
		},
		{
			name:  "second block starts at idx 1",
			seq:   []blocks.ROBlob{oneBlobs[0], twoBlobs[1]},
			err:   errChunkResponseIndexNotAsc,
			errAt: 1,
		},
		{
			name:  "slots not ascending",
			seq:   append(append([]blocks.ROBlob{}, twoBlobs...), oops...),
			err:   errChunkResponseSlotNotAsc,
			errAt: len(twoBlobs),
		},
		{
			name:  "same slot, different root",
			seq:   []blocks.ROBlob{oops[0], wrongRoot},
			err:   errChunkResponseBlockMismatch,
			errAt: 1,
		},
		{
			name:  "same slot, different parent root",
			seq:   []blocks.ROBlob{oops[0], oops[1]},
			err:   errChunkResponseBlockMismatch,
			errAt: 1,
		},
		{
			name:  "next slot, different parent root",
			seq:   []blocks.ROBlob{oops[0], twoBlobs[0]},
			err:   errChunkResponseParentMismatch,
			errAt: 1,
		},
	}
	for _, c := range cases {
		sbv := newSequentialBlobValidator()
		t.Run(c.name, func(t *testing.T) {
			for i := range c.seq {
				err := sbv(c.seq[i])
				if c.err != nil && i == c.errAt {
					require.ErrorIs(t, err, c.err)
					return
				}
				require.NoError(t, err)
			}
		})
	}
}

func TestSendBlobsByRangeRequest(t *testing.T) {
	topic := fmt.Sprintf("%s/ssz_snappy", p2p.RPCBlobSidecarsByRangeTopicV1)
	ctx := t.Context()

	t.Run("single blob - Deneb", func(t *testing.T) {
		// Setup genesis such that we are currently in deneb.
		s := uint64(util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)) * params.BeaconConfig().SecondsPerSlot
		clock := startup.NewClock(time.Now().Add(-time.Second*time.Duration(s)), [32]byte{})
		ctxByte, err := ContextByteVersionsForValRoot(clock.GenesisValidatorsRoot())
		require.NoError(t, err)
		// Setup peers
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		// Set current slot to a deneb slot.
		slot := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch+1)
		// Create a simple handler that will return a valid response.
		p2.SetStreamHandler(topic, func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()

			req := &ethpb.BlobSidecarsByRangeRequest{}
			assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))
			assert.Equal(t, slot, req.StartSlot)
			assert.Equal(t, uint64(1), req.Count)

			// Create a sequential set of blobs with the appropriate header information.
			var prevRoot [32]byte
			for i := req.StartSlot; i < req.StartSlot+primitives.Slot(req.Count); i++ {
				b := util.HydrateBlobSidecar(&ethpb.BlobSidecar{})
				b.SignedBlockHeader.Header.Slot = i
				b.SignedBlockHeader.Header.ParentRoot = prevRoot[:]
				ro, err := blocks.NewROBlob(b)
				require.NoError(t, err)
				vro := blocks.NewVerifiedROBlob(ro)
				prevRoot = vro.BlockRoot()
				assert.NoError(t, WriteBlobSidecarChunk(stream, clock, p2.Encoding(), vro))
			}
		})
		req := &ethpb.BlobSidecarsByRangeRequest{
			StartSlot: slot,
			Count:     1,
		}

		blobs, err := SendBlobsByRangeRequest(ctx, clock, p1, p2.PeerID(), ctxByte, req)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(blobs))
	})

	t.Run("Deneb - Electra epoch boundary crossing", func(t *testing.T) {
		cfg := params.BeaconConfig()
		cfg.ElectraForkEpoch = cfg.DenebForkEpoch + 1
		undo, err := params.SetActiveWithUndo(cfg)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, undo())
		}()
		// Setup genesis such that we are currently in deneb.
		s := uint64(util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)) * params.BeaconConfig().SecondsPerSlot
		clock := startup.NewClock(time.Now().Add(-time.Second*time.Duration(s)), [32]byte{})
		ctxByte, err := ContextByteVersionsForValRoot(clock.GenesisValidatorsRoot())
		require.NoError(t, err)
		// Setup peers
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)
		// Set current slot to the first slot of the last deneb epoch.
		slot := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
		// Create a simple handler that will return a valid response.
		p2.SetStreamHandler(topic, func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()

			req := &ethpb.BlobSidecarsByRangeRequest{}
			assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))
			assert.Equal(t, slot, req.StartSlot)
			assert.Equal(t, uint64(params.BeaconConfig().SlotsPerEpoch)*3, req.Count)

			// Create a sequential set of blobs with the appropriate header information.
			var prevRoot [32]byte
			for i := req.StartSlot; i < req.StartSlot+primitives.Slot(req.Count); i++ {
				maxBlobsForSlot := cfg.MaxBlobsPerBlock(i)
				parentRoot := prevRoot
				header := util.HydrateSignedBeaconHeader(&ethpb.SignedBeaconBlockHeader{})
				header.Header.Slot = i
				header.Header.ParentRoot = parentRoot[:]
				bRoot, err := header.Header.HashTreeRoot()
				require.NoError(t, err)
				prevRoot = bRoot
				// Send the maximum possible blobs per slot.
				for j := 0; j < maxBlobsForSlot; j++ {
					b := util.HydrateBlobSidecar(&ethpb.BlobSidecar{})
					b.SignedBlockHeader = header
					b.Index = uint64(j)
					ro, err := blocks.NewROBlob(b)
					require.NoError(t, err)
					vro := blocks.NewVerifiedROBlob(ro)
					assert.NoError(t, WriteBlobSidecarChunk(stream, clock, p2.Encoding(), vro))
				}
			}
		})
		req := &ethpb.BlobSidecarsByRangeRequest{
			StartSlot: slot,
			Count:     uint64(params.BeaconConfig().SlotsPerEpoch) * 3,
		}
		maxDenebBlobs := cfg.MaxBlobsPerBlockAtEpoch(cfg.DenebForkEpoch)
		maxElectraBlobs := cfg.MaxBlobsPerBlockAtEpoch(cfg.ElectraForkEpoch)
		totalDenebBlobs := primitives.Slot(maxDenebBlobs) * params.BeaconConfig().SlotsPerEpoch
		totalElectraBlobs := primitives.Slot(maxElectraBlobs) * 2 * params.BeaconConfig().SlotsPerEpoch
		totalExpectedBlobs := totalDenebBlobs + totalElectraBlobs

		blobs, err := SendBlobsByRangeRequest(ctx, clock, p1, p2.PeerID(), ctxByte, req)
		assert.NoError(t, err)
		assert.Equal(t, int(totalExpectedBlobs), len(blobs))
	})

	t.Run("Starting from Electra", func(t *testing.T) {
		cfg := params.BeaconConfig()
		cfg.ElectraForkEpoch = cfg.DenebForkEpoch + 1
		undo, err := params.SetActiveWithUndo(cfg)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, undo())
		}()

		s := uint64(util.SlotAtEpoch(t, params.BeaconConfig().ElectraForkEpoch)) * params.BeaconConfig().SecondsPerSlot
		clock := startup.NewClock(time.Now().Add(-time.Second*time.Duration(s)), [32]byte{})
		ctxByte, err := ContextByteVersionsForValRoot(clock.GenesisValidatorsRoot())
		require.NoError(t, err)
		// Setup peers
		p1 := p2ptest.NewTestP2P(t)
		p2 := p2ptest.NewTestP2P(t)
		p1.Connect(p2)

		slot := util.SlotAtEpoch(t, params.BeaconConfig().ElectraForkEpoch)
		// Create a simple handler that will return a valid response.
		p2.SetStreamHandler(topic, func(stream network.Stream) {
			defer func() {
				assert.NoError(t, stream.Close())
			}()

			req := &ethpb.BlobSidecarsByRangeRequest{}
			assert.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))
			assert.Equal(t, slot, req.StartSlot)
			assert.Equal(t, uint64(params.BeaconConfig().SlotsPerEpoch)*3, req.Count)

			// Create a sequential set of blobs with the appropriate header information.
			var prevRoot [32]byte
			for i := req.StartSlot; i < req.StartSlot+primitives.Slot(req.Count); i++ {
				maxBlobsForSlot := cfg.MaxBlobsPerBlock(i)
				parentRoot := prevRoot
				header := util.HydrateSignedBeaconHeader(&ethpb.SignedBeaconBlockHeader{})
				header.Header.Slot = i
				header.Header.ParentRoot = parentRoot[:]
				bRoot, err := header.Header.HashTreeRoot()
				require.NoError(t, err)
				prevRoot = bRoot
				// Send the maximum possible blobs per slot.
				for j := 0; j < maxBlobsForSlot; j++ {
					b := util.HydrateBlobSidecar(&ethpb.BlobSidecar{})
					b.SignedBlockHeader = header
					b.Index = uint64(j)
					ro, err := blocks.NewROBlob(b)
					require.NoError(t, err)
					vro := blocks.NewVerifiedROBlob(ro)
					assert.NoError(t, WriteBlobSidecarChunk(stream, clock, p2.Encoding(), vro))
				}
			}
		})
		req := &ethpb.BlobSidecarsByRangeRequest{
			StartSlot: slot,
			Count:     uint64(params.BeaconConfig().SlotsPerEpoch) * 3,
		}

		maxElectraBlobs := cfg.MaxBlobsPerBlockAtEpoch(cfg.ElectraForkEpoch)
		totalElectraBlobs := primitives.Slot(maxElectraBlobs) * 3 * params.BeaconConfig().SlotsPerEpoch

		blobs, err := SendBlobsByRangeRequest(ctx, clock, p1, p2.PeerID(), ctxByte, req)
		assert.NoError(t, err)
		assert.Equal(t, int(totalElectraBlobs), len(blobs))
	})
}

func TestErrInvalidFetchedDataDistinction(t *testing.T) {
	require.Equal(t, false, errors.Is(ErrInvalidFetchedData, verification.ErrBlobInvalid))
}

func TestSendDataColumnSidecarsByRangeRequest(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	beaconConfig := params.BeaconConfig()
	beaconConfig.FuluForkEpoch = 0
	params.OverrideBeaconConfig(beaconConfig)
	params.BeaconConfig().InitializeForkSchedule()
	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)
	nilTestCases := []struct {
		name    string
		request *ethpb.DataColumnSidecarsByRangeRequest
	}{
		{
			name:    "nil request",
			request: nil,
		},
		{
			name:    "count is 0",
			request: &ethpb.DataColumnSidecarsByRangeRequest{},
		},
		{
			name:    "columns is nil",
			request: &ethpb.DataColumnSidecarsByRangeRequest{Count: 1},
		},
	}

	for _, tc := range nilTestCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := SendDataColumnSidecarsByRangeRequest(DataColumnSidecarsParams{Ctx: t.Context()}, "", tc.request)
			require.NoError(t, err)
			require.IsNil(t, actual)
		})
	}

	t.Run("too many columns in request", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		beaconConfig := params.BeaconConfig()
		beaconConfig.MaxRequestDataColumnSidecars = 0
		params.OverrideBeaconConfig(beaconConfig)

		request := &ethpb.DataColumnSidecarsByRangeRequest{Count: 1, Columns: []uint64{1, 2, 3}}
		_, err := SendDataColumnSidecarsByRangeRequest(DataColumnSidecarsParams{Ctx: t.Context()}, "", request)
		require.ErrorContains(t, errMaxRequestDataColumnSidecarsExceeded.Error(), err)
	})

	type slotIndex struct {
		Slot  primitives.Slot
		Index uint64
	}

	createSidecar := func(slotIndex slotIndex) *ethpb.DataColumnSidecar {
		const count = 4
		kzgCommitmentsInclusionProof := make([][]byte, 0, count)
		for range count {
			kzgCommitmentsInclusionProof = append(kzgCommitmentsInclusionProof, make([]byte, 32))
		}

		return &ethpb.DataColumnSidecar{
			Index: slotIndex.Index,
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					Slot:       slotIndex.Slot,
					ParentRoot: make([]byte, fieldparams.RootLength),
					StateRoot:  make([]byte, fieldparams.RootLength),
					BodyRoot:   make([]byte, fieldparams.RootLength),
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
			KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
		}
	}

	testCases := []struct {
		name          string
		slotIndices   []slotIndex
		expectedError error
	}{
		{
			name: "too many responses",
			slotIndices: []slotIndex{
				{Slot: 0, Index: 1},
				{Slot: 0, Index: 2},
				{Slot: 0, Index: 3},
				{Slot: 1, Index: 1},
				{Slot: 1, Index: 2},
				{Slot: 1, Index: 3},
				{Slot: 0, Index: 3}, // Duplicate
			},
			expectedError: errMaxResponseDataColumnSidecarsExceeded,
		},
		{
			name: "perfect match",
			slotIndices: []slotIndex{
				{Slot: 0, Index: 1},
				{Slot: 0, Index: 2},
				{Slot: 0, Index: 3},
				{Slot: 1, Index: 1},
				{Slot: 1, Index: 2},
				{Slot: 1, Index: 3},
			},
		},
		{
			name: "few responses than maximum possible",
			slotIndices: []slotIndex{
				{Slot: 0, Index: 1},
				{Slot: 0, Index: 2},
				{Slot: 0, Index: 3},
				{Slot: 1, Index: 1},
				{Slot: 1, Index: 2},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			protocol := fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRangeTopicV1)
			clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})

			p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
			p1.Connect(p2)

			expected := make([]*ethpb.DataColumnSidecar, 0, len(tc.slotIndices))
			for _, slotIndex := range tc.slotIndices {
				sidecar := createSidecar(slotIndex)
				expected = append(expected, sidecar)
			}

			requestSent := &ethpb.DataColumnSidecarsByRangeRequest{
				StartSlot: 0,
				Count:     2,
				Columns:   []uint64{1, 3, 2},
			}

			var wg sync.WaitGroup
			wg.Add(1)

			p2.SetStreamHandler(protocol, func(stream network.Stream) {
				wg.Done()

				requestReceived := new(ethpb.DataColumnSidecarsByRangeRequest)
				err := p2.Encoding().DecodeWithMaxLength(stream, requestReceived)
				assert.NoError(t, err)
				assert.DeepSSZEqual(t, requestSent, requestReceived)

				for _, sidecar := range expected {
					err := WriteDataColumnSidecarChunk(stream, clock, p2.Encoding(), sidecar)
					assert.NoError(t, err)
				}

				err = stream.CloseWrite()
				assert.NoError(t, err)
			})

			parameters := DataColumnSidecarsParams{
				Ctx:    t.Context(),
				Tor:    clock,
				P2P:    p1,
				CtxMap: ctxMap,
			}

			actual, err := SendDataColumnSidecarsByRangeRequest(parameters, p2.PeerID(), requestSent)
			if tc.expectedError != nil {
				require.ErrorContains(t, tc.expectedError.Error(), err)
				if util.WaitTimeout(&wg, time.Second) {
					t.Fatal("Did not receive stream within 1 sec")
				}

				return
			}

			require.Equal(t, len(expected), len(actual))
			for i := range expected {
				require.DeepSSZEqual(t, expected[i], actual[i].DataColumnSidecar)
			}
		})
	}
}

func TestIsSidecarSlotWithinBounds(t *testing.T) {
	request := &ethpb.DataColumnSidecarsByRangeRequest{
		StartSlot: 10,
		Count:     10,
	}

	validator, err := isSidecarSlotWithinBounds(request)
	require.NoError(t, err)

	testCases := []struct {
		name            string
		slot            primitives.Slot
		isErrorExpected bool
	}{
		{
			name:            "too soon",
			slot:            9,
			isErrorExpected: true,
		},
		{
			name:            "too late",
			slot:            20,
			isErrorExpected: true,
		},
		{
			name:            "within bounds",
			slot:            15,
			isErrorExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const count = 4
			kzgCommitmentsInclusionProof := make([][]byte, 0, count)
			for range count {
				kzgCommitmentsInclusionProof = append(kzgCommitmentsInclusionProof, make([]byte, 32))
			}

			sidecarPb := &ethpb.DataColumnSidecar{
				SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
					Header: &ethpb.BeaconBlockHeader{
						Slot:       tc.slot,
						ParentRoot: make([]byte, fieldparams.RootLength),
						StateRoot:  make([]byte, fieldparams.RootLength),
						BodyRoot:   make([]byte, fieldparams.RootLength),
					},
					Signature: make([]byte, fieldparams.BLSSignatureLength),
				},
				KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
			}

			sidecar, err := blocks.NewRODataColumn(sidecarPb)
			require.NoError(t, err)

			err = validator(sidecar)
			if tc.isErrorExpected {
				require.NotNil(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestIsSidecarIndexRequested(t *testing.T) {
	request := &ethpb.DataColumnSidecarsByRangeRequest{
		Columns: []uint64{2, 9, 4},
	}

	validator := isSidecarIndexRequested(request)

	testCases := []struct {
		name            string
		index           uint64
		isErrorExpected bool
	}{
		{
			name:            "not requested",
			index:           1,
			isErrorExpected: true,
		},
		{
			name:            "requested",
			index:           9,
			isErrorExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const count = 4
			kzgCommitmentsInclusionProof := make([][]byte, 0, count)
			for range count {
				kzgCommitmentsInclusionProof = append(kzgCommitmentsInclusionProof, make([]byte, 32))
			}

			sidecarPb := &ethpb.DataColumnSidecar{
				SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
					Header: &ethpb.BeaconBlockHeader{
						Slot:       0,
						ParentRoot: make([]byte, fieldparams.RootLength),
						StateRoot:  make([]byte, fieldparams.RootLength),
						BodyRoot:   make([]byte, fieldparams.RootLength),
					},
					Signature: make([]byte, fieldparams.BLSSignatureLength),
				},
				KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
				Index:                        tc.index,
			}

			sidecar, err := blocks.NewRODataColumn(sidecarPb)
			require.NoError(t, err)

			err = validator(sidecar)
			if tc.isErrorExpected {
				require.NotNil(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestSendDataColumnSidecarsByRootRequest(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	beaconConfig := params.BeaconConfig()
	beaconConfig.FuluForkEpoch = 0
	params.OverrideBeaconConfig(beaconConfig)
	params.BeaconConfig().InitializeForkSchedule()
	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)
	nilTestCases := []struct {
		name    string
		request p2ptypes.DataColumnsByRootIdentifiers
	}{
		{
			name:    "nil request",
			request: nil,
		},
		{
			name:    "count is 0",
			request: p2ptypes.DataColumnsByRootIdentifiers{{}, {}},
		},
	}

	for _, tc := range nilTestCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := SendDataColumnSidecarsByRootRequest(DataColumnSidecarsParams{Ctx: t.Context()}, "", tc.request)
			require.NoError(t, err)
			require.IsNil(t, actual)
		})
	}

	t.Run("too many columns in request", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		beaconConfig := params.BeaconConfig()
		beaconConfig.MaxRequestDataColumnSidecars = 4
		params.OverrideBeaconConfig(beaconConfig)

		request := p2ptypes.DataColumnsByRootIdentifiers{
			{Columns: []uint64{1, 2, 3}},
			{Columns: []uint64{4, 5, 6}},
		}

		_, err := SendDataColumnSidecarsByRootRequest(DataColumnSidecarsParams{Ctx: t.Context()}, "", request)
		require.ErrorContains(t, errMaxRequestDataColumnSidecarsExceeded.Error(), err)
	})

	type slotIndex struct {
		Slot  primitives.Slot
		Index uint64
	}

	createSidecar := func(rootIndex slotIndex) blocks.RODataColumn {
		const count = 4
		kzgCommitmentsInclusionProof := make([][]byte, 0, count)
		for range count {
			kzgCommitmentsInclusionProof = append(kzgCommitmentsInclusionProof, make([]byte, 32))
		}

		sidecarPb := &ethpb.DataColumnSidecar{
			Index: rootIndex.Index,
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					ParentRoot: make([]byte, fieldparams.RootLength),
					StateRoot:  make([]byte, fieldparams.RootLength),
					BodyRoot:   make([]byte, fieldparams.RootLength),
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
			KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
		}

		roSidecar, err := blocks.NewRODataColumn(sidecarPb)
		require.NoError(t, err)

		return roSidecar
	}

	testCases := []struct {
		name          string
		slotIndices   []slotIndex
		expectedError error
	}{
		{
			name: "too many responses",
			slotIndices: []slotIndex{
				{Slot: 1, Index: 1},
				{Slot: 1, Index: 2},
				{Slot: 1, Index: 3},
				{Slot: 2, Index: 1},
				{Slot: 2, Index: 2},
				{Slot: 2, Index: 3},
				{Slot: 1, Index: 3}, // Duplicate
			},
			expectedError: errMaxResponseDataColumnSidecarsExceeded,
		},
		{
			name: "perfect match",
			slotIndices: []slotIndex{
				{Slot: 1, Index: 1},
				{Slot: 1, Index: 2},
				{Slot: 1, Index: 3},
				{Slot: 2, Index: 1},
				{Slot: 2, Index: 2},
				{Slot: 2, Index: 3},
			},
		},
		{
			name: "few responses than maximum possible",
			slotIndices: []slotIndex{
				{Slot: 1, Index: 1},
				{Slot: 1, Index: 2},
				{Slot: 1, Index: 3},
				{Slot: 2, Index: 1},
				{Slot: 2, Index: 2},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			protocol := fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRootTopicV1)
			clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})

			p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
			p1.Connect(p2)

			expected := make([]blocks.RODataColumn, 0, len(tc.slotIndices))
			for _, slotIndex := range tc.slotIndices {
				roSidecar := createSidecar(slotIndex)
				expected = append(expected, roSidecar)
			}

			blockRoot1, blockRoot2 := expected[0].BlockRoot(), expected[3].BlockRoot()

			sentRequest := p2ptypes.DataColumnsByRootIdentifiers{
				{BlockRoot: blockRoot1[:], Columns: []uint64{1, 2, 3}},
				{BlockRoot: blockRoot2[:], Columns: []uint64{1, 2, 3}},
			}

			var wg sync.WaitGroup
			wg.Add(1)

			p2.SetStreamHandler(protocol, func(stream network.Stream) {
				wg.Done()

				requestReceived := new(p2ptypes.DataColumnsByRootIdentifiers)
				err := p2.Encoding().DecodeWithMaxLength(stream, requestReceived)
				assert.NoError(t, err)

				require.Equal(t, len(sentRequest), len(*requestReceived))
				for i := range sentRequest {
					require.DeepSSZEqual(t, (sentRequest)[i], (*requestReceived)[i])
				}

				for _, sidecar := range expected {
					err := WriteDataColumnSidecarChunk(stream, clock, p2.Encoding(), sidecar.DataColumnSidecar)
					assert.NoError(t, err)
				}

				err = stream.CloseWrite()
				assert.NoError(t, err)
			})

			parameters := DataColumnSidecarsParams{
				Ctx:    t.Context(),
				Tor:    clock,
				P2P:    p1,
				CtxMap: ctxMap,
			}
			actual, err := SendDataColumnSidecarsByRootRequest(parameters, p2.PeerID(), sentRequest)
			if tc.expectedError != nil {
				require.ErrorContains(t, tc.expectedError.Error(), err)
				if util.WaitTimeout(&wg, time.Second) {
					t.Fatal("Did not receive stream within 1 sec")
				}

				return
			}

			require.Equal(t, len(expected), len(actual))
			for i := range expected {
				require.DeepSSZEqual(t, expected[i], actual[i])
			}
		})
	}
}

func TestIsSidecarIndexRootRequested(t *testing.T) {
	testCases := []struct {
		name            string
		root            [fieldparams.RootLength]byte
		index           uint64
		isErrorExpected bool
	}{
		{
			name:            "non requested root",
			root:            [fieldparams.RootLength]byte{2},
			isErrorExpected: true,
		},
		{
			name:            "non requested index",
			root:            [fieldparams.RootLength]byte{1},
			index:           3,
			isErrorExpected: true,
		},
		{
			name:            "nominal",
			root:            [fieldparams.RootLength]byte{1},
			index:           2,
			isErrorExpected: false,
		},
	}

	request := types.DataColumnsByRootIdentifiers{
		{BlockRoot: []byte{1}, Columns: []uint64{1, 2}},
	}

	validator := isSidecarIndexRootRequested(request)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const count = 4
			kzgCommitmentsInclusionProof := make([][]byte, 0, count)
			for range count {
				kzgCommitmentsInclusionProof = append(kzgCommitmentsInclusionProof, make([]byte, 32))
			}

			sidecarPb := &ethpb.DataColumnSidecar{
				SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
					Header: &ethpb.BeaconBlockHeader{
						ParentRoot: make([]byte, fieldparams.RootLength),
						StateRoot:  make([]byte, fieldparams.RootLength),
						BodyRoot:   make([]byte, fieldparams.RootLength),
					},
					Signature: make([]byte, fieldparams.BLSSignatureLength),
				},
				KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
				Index:                        tc.index,
			}

			// There is a discrepancy between `tc.root` and the real root,
			// but we don't care about it here.
			sidecar, err := blocks.NewRODataColumnWithRoot(sidecarPb, tc.root)
			require.NoError(t, err)

			err = validator(sidecar)
			if tc.isErrorExpected {
				require.NotNil(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestReadChunkedDataColumnSidecar(t *testing.T) {
	t.Run("non nil status code", func(t *testing.T) {
		const reason = "a dummy reason"

		p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)
		p2.SetStreamHandler(p2p.RPCDataColumnSidecarsByRootTopicV1, func(stream network.Stream) {
			defer wg.Done()

			_, err := readChunkedDataColumnSidecar(stream, p2, nil)
			require.ErrorContains(t, reason, err)
		})

		p1.Connect(p2)

		stream, err := p1.BHost.NewStream(t.Context(), p2.PeerID(), p2p.RPCDataColumnSidecarsByRootTopicV1)
		require.NoError(t, err)

		writeErrorResponseToStream(responseCodeInvalidRequest, reason, stream, p1)

		if util.WaitTimeout(&wg, time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("unrecognized fork digest", func(t *testing.T) {
		p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)
		p2.SetStreamHandler(p2p.RPCDataColumnSidecarsByRootTopicV1, func(stream network.Stream) {
			defer wg.Done()

			_, err := readChunkedDataColumnSidecar(stream, p2, ContextByteVersions{})
			require.ErrorContains(t, "unrecognized fork digest", err)
		})

		p1.Connect(p2)

		stream, err := p1.BHost.NewStream(t.Context(), p2.PeerID(), p2p.RPCDataColumnSidecarsByRootTopicV1)
		require.NoError(t, err)

		_, err = stream.Write([]byte{responseCodeSuccess})
		require.NoError(t, err)

		err = writeContextToStream([]byte{42, 42, 42, 42}, stream)
		require.NoError(t, err)

		if util.WaitTimeout(&wg, time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("before fulu", func(t *testing.T) {
		p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)
		p2.SetStreamHandler(p2p.RPCDataColumnSidecarsByRootTopicV1, func(stream network.Stream) {
			defer wg.Done()

			_, err := readChunkedDataColumnSidecar(stream, p2, ContextByteVersions{[4]byte{1, 2, 3, 4}: version.Phase0})
			require.ErrorContains(t, "unexpected context bytes", err)
		})

		p1.Connect(p2)

		stream, err := p1.BHost.NewStream(t.Context(), p2.PeerID(), p2p.RPCDataColumnSidecarsByRootTopicV1)
		require.NoError(t, err)

		_, err = stream.Write([]byte{responseCodeSuccess})
		require.NoError(t, err)

		err = writeContextToStream([]byte{1, 2, 3, 4}, stream)
		require.NoError(t, err)

		if util.WaitTimeout(&wg, time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("one validation failed", func(t *testing.T) {
		const reason = "a dummy reason"

		p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)
		p2.SetStreamHandler(p2p.RPCDataColumnSidecarsByRootTopicV1, func(stream network.Stream) {
			defer wg.Done()

			validationOne := func(column blocks.RODataColumn) error {
				return nil
			}

			validationTwo := func(column blocks.RODataColumn) error {
				return errors.New(reason)
			}

			_, err := readChunkedDataColumnSidecar(
				stream,
				p2,
				ContextByteVersions{[4]byte{1, 2, 3, 4}: version.Fulu},
				validationOne, // OK
				validationTwo, // Fail
			)

			require.ErrorContains(t, reason, err)
		})

		p1.Connect(p2)

		stream, err := p1.BHost.NewStream(t.Context(), p2.PeerID(), p2p.RPCDataColumnSidecarsByRootTopicV1)
		require.NoError(t, err)

		const count = 4
		kzgCommitmentsInclusionProof := make([][]byte, 0, count)
		for range count {
			kzgCommitmentsInclusionProof = append(kzgCommitmentsInclusionProof, make([]byte, 32))
		}

		// Success response code.
		_, err = stream.Write([]byte{responseCodeSuccess})
		require.NoError(t, err)

		// Fork digest.
		err = writeContextToStream([]byte{1, 2, 3, 4}, stream)
		require.NoError(t, err)

		// Sidecar.
		_, err = p1.Encoding().EncodeWithMaxLength(stream, &ethpb.DataColumnSidecar{
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					ParentRoot: make([]byte, fieldparams.RootLength),
					StateRoot:  make([]byte, fieldparams.RootLength),
					BodyRoot:   make([]byte, fieldparams.RootLength),
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
			KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
		})
		require.NoError(t, err)

		if util.WaitTimeout(&wg, time.Minute) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("nominal", func(t *testing.T) {
		p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)

		const count = 4
		kzgCommitmentsInclusionProof := make([][]byte, 0, count)
		for range count {
			kzgCommitmentsInclusionProof = append(kzgCommitmentsInclusionProof, make([]byte, 32))
		}

		expected := &ethpb.DataColumnSidecar{
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					ParentRoot: make([]byte, fieldparams.RootLength),
					StateRoot:  make([]byte, fieldparams.RootLength),
					BodyRoot:   make([]byte, fieldparams.RootLength),
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
			KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
		}

		var wg sync.WaitGroup
		wg.Add(1)
		p2.SetStreamHandler(p2p.RPCDataColumnSidecarsByRootTopicV1, func(stream network.Stream) {
			defer wg.Done()

			actual, err := readChunkedDataColumnSidecar(stream, p2, ContextByteVersions{[4]byte{1, 2, 3, 4}: version.Fulu})
			require.NoError(t, err)
			require.DeepSSZEqual(t, expected, actual.DataColumnSidecar)
		})

		p1.Connect(p2)

		stream, err := p1.BHost.NewStream(t.Context(), p2.PeerID(), p2p.RPCDataColumnSidecarsByRootTopicV1)
		require.NoError(t, err)

		// Success response code.
		_, err = stream.Write([]byte{responseCodeSuccess})
		require.NoError(t, err)

		// Fork digest.
		err = writeContextToStream([]byte{1, 2, 3, 4}, stream)
		require.NoError(t, err)

		// Sidecar.
		_, err = p1.Encoding().EncodeWithMaxLength(stream, expected)
		require.NoError(t, err)

		if util.WaitTimeout(&wg, time.Minute) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})
}
