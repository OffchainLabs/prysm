package sync

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/encoder"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	goPeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var errBlobChunkedReadFailure = errors.New("failed to read stream of chunk-encoded blobs")
var errBlobUnmarshal = errors.New("Could not unmarshal chunk-encoded blob")

// Any error from the following declaration block should result in peer downscoring.
var (
	// ErrInvalidFetchedData is used to signal that an error occurred which should result in peer downscoring.
	ErrInvalidFetchedData                    = errors.New("invalid data returned from peer")
	errBlobIndexOutOfBounds                  = errors.Wrap(verification.ErrBlobInvalid, "blob index out of range")
	errMaxRequestBlobSidecarsExceeded        = errors.Wrap(verification.ErrBlobInvalid, "peer exceeded req blob chunk tx limit")
	errChunkResponseSlotNotAsc               = errors.Wrap(verification.ErrBlobInvalid, "blob slot not higher than previous block root")
	errChunkResponseIndexNotAsc              = errors.Wrap(verification.ErrBlobInvalid, "blob indices for a block must start at 0 and increase by 1")
	errUnrequested                           = errors.Wrap(verification.ErrBlobInvalid, "received BlobSidecar in response that was not requested")
	errBlobResponseOutOfBounds               = errors.Wrap(verification.ErrBlobInvalid, "received BlobSidecar with slot outside BlobSidecarsByRangeRequest bounds")
	errChunkResponseBlockMismatch            = errors.Wrap(verification.ErrBlobInvalid, "blob block details do not match")
	errChunkResponseParentMismatch           = errors.Wrap(verification.ErrBlobInvalid, "parent root for response element doesn't match previous element root")
	errDataColumnChunkedReadFailure          = errors.New("failed to read stream of chunk-encoded data columns")
	errMaxRequestDataColumnSidecarsExceeded  = errors.New("count of requested data column sidecars exceeds MAX_REQUEST_DATA_COLUMN_SIDECARS")
	errMaxResponseDataColumnSidecarsExceeded = errors.New("peer returned more data column sidecars than requested")
)

// ------
// Blocks
// ------

// BeaconBlockProcessor defines a block processing function, which allows to start utilizing
// blocks even before all blocks are ready.
type BeaconBlockProcessor func(block interfaces.ReadOnlySignedBeaconBlock) error

// SendBeaconBlocksByRangeRequest sends BeaconBlocksByRange and returns fetched blocks, if any.
func SendBeaconBlocksByRangeRequest(
	ctx context.Context, tor blockchain.TemporalOracle, p2pProvider p2p.SenderEncoder, pid peer.ID,
	req *ethpb.BeaconBlocksByRangeRequest, blockProcessor BeaconBlockProcessor,
) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	topic, err := p2p.TopicFromMessage(p2p.BeaconBlocksByRangeMessageName, slots.ToEpoch(tor.CurrentSlot()))
	if err != nil {
		return nil, err
	}
	stream, err := p2pProvider.Send(ctx, req, topic, pid)
	if err != nil {
		return nil, err
	}
	defer closeStream(stream, log)

	// Augment block processing function, if non-nil block processor is provided.
	blocks := make([]interfaces.ReadOnlySignedBeaconBlock, 0, req.Count)
	process := func(blk interfaces.ReadOnlySignedBeaconBlock) error {
		blocks = append(blocks, blk)
		if blockProcessor != nil {
			return blockProcessor(blk)
		}
		return nil
	}
	var prevSlot primitives.Slot
	for i := uint64(0); ; i++ {
		isFirstChunk := i == 0
		blk, err := ReadChunkedBlock(stream, tor, p2pProvider, isFirstChunk)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		// The response MUST contain no more than `count` blocks, and no more than
		// MAX_REQUEST_BLOCKS blocks.
		currentEpoch := slots.ToEpoch(tor.CurrentSlot())
		maxBlocks := params.MaxRequestBlock(currentEpoch)
		if i >= req.Count {
			log.WithFields(logrus.Fields{
				"blockIndex":     i,
				"requestedCount": req.Count,
				"blockSlot":      blk.Block().Slot(),
				"peer":           pid,
				"reason":         "exceeded requested count",
			}).Debug("Peer returned invalid data: too many blocks")
			return nil, ErrInvalidFetchedData
		}
		if i >= maxBlocks {
			log.WithFields(logrus.Fields{
				"blockIndex":   i,
				"maxBlocks":    maxBlocks,
				"currentEpoch": currentEpoch,
				"blockSlot":    blk.Block().Slot(),
				"peer":         pid,
				"reason":       "exceeded MAX_REQUEST_BLOCKS",
			}).Debug("Peer returned invalid data: exceeded protocol limit")
			return nil, ErrInvalidFetchedData
		}
		// Returned blocks MUST be in the slot range [start_slot, start_slot + count * step).
		endSlot := req.StartSlot.Add(req.Count * req.Step)
		if blk.Block().Slot() < req.StartSlot {
			log.WithFields(logrus.Fields{
				"blockSlot":      blk.Block().Slot(),
				"requestedStart": req.StartSlot,
				"peer":           pid,
				"reason":         "block slot before requested start",
			}).Debug("Peer returned invalid data: block too early")
			return nil, ErrInvalidFetchedData
		}
		if blk.Block().Slot() >= endSlot {
			log.WithFields(logrus.Fields{
				"blockSlot":      blk.Block().Slot(),
				"requestedStart": req.StartSlot,
				"requestedEnd":   endSlot,
				"requestedCount": req.Count,
				"requestedStep":  req.Step,
				"peer":           pid,
				"reason":         "block slot >= start + count*step",
			}).Debug("Peer returned invalid data: block beyond range")
			return nil, ErrInvalidFetchedData
		}
		// Returned blocks, where they exist, MUST be sent in a consecutive order.
		// Consecutive blocks MUST have values in `step` increments (slots may be skipped in between).
		isSlotOutOfOrder := false
		outOfOrderReason := ""
		if prevSlot >= blk.Block().Slot() {
			isSlotOutOfOrder = true
			outOfOrderReason = "slot not increasing"
		} else if req.Step != 0 && blk.Block().Slot().SubSlot(prevSlot).Mod(req.Step) != 0 {
			isSlotOutOfOrder = true
			slotDiff := blk.Block().Slot().SubSlot(prevSlot)
			outOfOrderReason = fmt.Sprintf("slot diff %d not multiple of step %d", slotDiff, req.Step)
		}
		if !isFirstChunk && isSlotOutOfOrder {
			log.WithFields(logrus.Fields{
				"blockSlot":     blk.Block().Slot(),
				"prevSlot":      prevSlot,
				"requestedStep": req.Step,
				"blockIndex":    i,
				"peer":          pid,
				"reason":        outOfOrderReason,
			}).Debug("Peer returned invalid data: blocks out of order")
			return nil, ErrInvalidFetchedData
		}
		prevSlot = blk.Block().Slot()
		if err := process(blk); err != nil {
			return nil, err
		}
	}
	return blocks, nil
}

// SendBeaconBlocksByRootRequest sends BeaconBlocksByRoot and returns fetched blocks, if any.
func SendBeaconBlocksByRootRequest(
	ctx context.Context, clock blockchain.TemporalOracle, p2pProvider p2p.P2P, pid peer.ID,
	req *p2ptypes.BeaconBlockByRootsReq, blockProcessor BeaconBlockProcessor,
) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	topic, err := p2p.TopicFromMessage(p2p.BeaconBlocksByRootsMessageName, slots.ToEpoch(clock.CurrentSlot()))
	if err != nil {
		return nil, err
	}
	stream, err := p2pProvider.Send(ctx, req, topic, pid)
	if err != nil {
		return nil, err
	}
	defer closeStream(stream, log)

	// Augment block processing function, if non-nil block processor is provided.
	blocks := make([]interfaces.ReadOnlySignedBeaconBlock, 0, len(*req))
	process := func(block interfaces.ReadOnlySignedBeaconBlock) error {
		blocks = append(blocks, block)
		if blockProcessor != nil {
			return blockProcessor(block)
		}
		return nil
	}
	currentEpoch := slots.ToEpoch(clock.CurrentSlot())
	for i := 0; i < len(*req); i++ {
		// Exit if peer sends more than max request blocks.
		if uint64(i) >= params.MaxRequestBlock(currentEpoch) {
			break
		}
		isFirstChunk := i == 0
		blk, err := ReadChunkedBlock(stream, clock, p2pProvider, isFirstChunk)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		if err := process(blk); err != nil {
			return nil, err
		}
	}
	return blocks, nil
}

// -------------
// Blob sidecars
// -------------

// BlobResponseValidation represents a function that can validate aspects of a single unmarshaled blob sidecar
// that was received from a peer in response to an rpc request.
type BlobResponseValidation func(blocks.ROBlob) error

func SendBlobsByRangeRequest(ctx context.Context, tor blockchain.TemporalOracle, p2pApi p2p.SenderEncoder, pid peer.ID, ctxMap ContextByteVersions, req *ethpb.BlobSidecarsByRangeRequest, bvs ...BlobResponseValidation) ([]blocks.ROBlob, error) {
	topic, err := p2p.TopicFromMessage(p2p.BlobSidecarsByRangeName, slots.ToEpoch(tor.CurrentSlot()))
	if err != nil {
		return nil, err
	}
	log.WithFields(logrus.Fields{
		"topic":     topic,
		"startSlot": req.StartSlot,
		"count":     req.Count,
	}).Debug("Sending blob by range request")
	stream, err := p2pApi.Send(ctx, req, topic, pid)
	if err != nil {
		return nil, err
	}
	defer closeStream(stream, log)

	maxBlobsPerBlock := uint64(params.BeaconConfig().MaxBlobsPerBlock(req.StartSlot + primitives.Slot(req.Count)))
	max := params.BeaconConfig().MaxRequestBlobSidecars
	if slots.ToEpoch(req.StartSlot) >= params.BeaconConfig().ElectraForkEpoch {
		max = params.BeaconConfig().MaxRequestBlobSidecarsElectra
	}
	if max > req.Count*maxBlobsPerBlock {
		max = req.Count * maxBlobsPerBlock
	}
	vfuncs := []BlobResponseValidation{blobValidatorFromRangeReq(req), newSequentialBlobValidator()}
	if len(bvs) > 0 {
		vfuncs = append(vfuncs, bvs...)
	}
	return readChunkEncodedBlobs(stream, p2pApi.Encoding(), ctxMap, composeBlobValidations(vfuncs...), max)
}

func SendBlobSidecarByRoot(
	ctx context.Context, tor blockchain.TemporalOracle, p2pApi p2p.P2P, pid peer.ID,
	ctxMap ContextByteVersions, req *p2ptypes.BlobSidecarsByRootReq, slot primitives.Slot,
) ([]blocks.ROBlob, error) {
	if uint64(len(*req)) > params.BeaconConfig().MaxRequestBlobSidecars {
		return nil, errors.Wrapf(p2ptypes.ErrMaxBlobReqExceeded, "length=%d", len(*req))
	}

	topic, err := p2p.TopicFromMessage(p2p.BlobSidecarsByRootName, slots.ToEpoch(tor.CurrentSlot()))
	if err != nil {
		return nil, err
	}
	log.WithField("topic", topic).Debug("Sending blob sidecar request")
	stream, err := p2pApi.Send(ctx, req, topic, pid)
	if err != nil {
		return nil, err
	}
	defer closeStream(stream, log)

	max := params.BeaconConfig().MaxRequestBlobSidecars
	if slots.ToEpoch(slot) >= params.BeaconConfig().ElectraForkEpoch {
		max = params.BeaconConfig().MaxRequestBlobSidecarsElectra
	}
	maxBlobCount := params.BeaconConfig().MaxBlobsPerBlock(slot)
	if max > uint64(len(*req)*maxBlobCount) {
		max = uint64(len(*req) * maxBlobCount)
	}
	return readChunkEncodedBlobs(stream, p2pApi.Encoding(), ctxMap, blobValidatorFromRootReq(req), max)
}

func composeBlobValidations(vf ...BlobResponseValidation) BlobResponseValidation {
	return func(blob blocks.ROBlob) error {
		for i := range vf {
			if err := vf[i](blob); err != nil {
				return err
			}
		}
		return nil
	}
}

type seqBlobValid struct {
	prev *blocks.ROBlob
}

func (sbv *seqBlobValid) nextValid(blob blocks.ROBlob) error {
	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(blob.Slot())
	if blob.Index >= uint64(maxBlobsPerBlock) {
		return errBlobIndexOutOfBounds
	}
	if sbv.prev == nil {
		// The first blob we see for a block must have index 0.
		if blob.Index != 0 {
			return errChunkResponseIndexNotAsc
		}
		sbv.prev = &blob
		return nil
	}
	if sbv.prev.Slot() == blob.Slot() {
		if sbv.prev.BlockRoot() != blob.BlockRoot() {
			return errors.Wrap(errChunkResponseBlockMismatch, "block roots do not match")
		}
		if sbv.prev.ParentRoot() != blob.ParentRoot() {
			return errors.Wrap(errChunkResponseBlockMismatch, "block parent roots do not match")
		}
		// Blob indices in responses should be strictly monotonically incrementing.
		if blob.Index != sbv.prev.Index+1 {
			return errChunkResponseIndexNotAsc
		}
	} else {
		// If the slot is adjacent we know there are no intervening blocks with missing blobs, so we can
		// check that the new blob descends from the last seen.
		if blob.Slot() == sbv.prev.Slot()+1 && blob.ParentRoot() != sbv.prev.BlockRoot() {
			return errChunkResponseParentMismatch
		}
		// The first blob we see for a block must have index 0.
		if blob.Index != 0 {
			return errChunkResponseIndexNotAsc
		}
		// Blocks must be in ascending slot order.
		if sbv.prev.Slot() >= blob.Slot() {
			return errChunkResponseSlotNotAsc
		}
	}
	sbv.prev = &blob
	return nil
}

func newSequentialBlobValidator() BlobResponseValidation {
	sbv := &seqBlobValid{}
	return func(blob blocks.ROBlob) error {
		return sbv.nextValid(blob)
	}
}

func blobValidatorFromRootReq(req *p2ptypes.BlobSidecarsByRootReq) BlobResponseValidation {
	blobIds := make(map[[32]byte]map[uint64]bool)
	for _, sc := range *req {
		blockRoot := bytesutil.ToBytes32(sc.BlockRoot)
		if blobIds[blockRoot] == nil {
			blobIds[blockRoot] = make(map[uint64]bool)
		}
		blobIds[blockRoot][sc.Index] = true
	}
	return func(sc blocks.ROBlob) error {
		blobIndices := blobIds[sc.BlockRoot()]
		if blobIndices == nil {
			return errors.Wrapf(errUnrequested, "root=%#x", sc.BlockRoot())
		}
		requested := blobIndices[sc.Index]
		if !requested {
			return errors.Wrapf(errUnrequested, "root=%#x index=%d", sc.BlockRoot(), sc.Index)
		}
		return nil
	}
}

func blobValidatorFromRangeReq(req *ethpb.BlobSidecarsByRangeRequest) BlobResponseValidation {
	end := req.StartSlot + primitives.Slot(req.Count)
	return func(sc blocks.ROBlob) error {
		if sc.Slot() < req.StartSlot || sc.Slot() >= end {
			return errors.Wrapf(errBlobResponseOutOfBounds, "req start,end:%d,%d, resp:%d", req.StartSlot, end, sc.Slot())
		}
		return nil
	}
}

func readChunkEncodedBlobs(stream network.Stream, encoding encoder.NetworkEncoding, ctxMap ContextByteVersions, vf BlobResponseValidation, max uint64) ([]blocks.ROBlob, error) {
	sidecars := make([]blocks.ROBlob, 0)
	// Attempt an extra read beyond max to check if the peer is violating the spec by
	// sending more than MAX_REQUEST_BLOB_SIDECARS, or more blobs than requested.
	for i := uint64(0); i < max+1; i++ {
		sc, err := readChunkedBlobSidecar(stream, encoding, ctxMap, vf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if i == max {
			// We have read an extra sidecar beyond what the spec allows. Since this is a spec violation, we return
			// an error that wraps ErrInvalidFetchedData. The part of the state machine that handles rpc peer scoring
			// will downscore the peer if the request ends in an error that wraps that one.
			return nil, errMaxRequestBlobSidecarsExceeded
		}
		sidecars = append(sidecars, sc)
	}

	return sidecars, nil
}

func readChunkedBlobSidecar(stream network.Stream, encoding encoder.NetworkEncoding, ctxMap ContextByteVersions, vf BlobResponseValidation) (blocks.ROBlob, error) {
	var b blocks.ROBlob
	pb := &ethpb.BlobSidecar{}
	decode := encoding.DecodeWithMaxLength
	var (
		code uint8
		msg  string
	)
	code, msg, err := ReadStatusCode(stream, encoding)
	if err != nil {
		return b, err
	}
	if code != 0 {
		return b, errors.Wrap(errBlobChunkedReadFailure, msg)
	}
	ctxb, err := readContextFromStream(stream)
	if err != nil {
		return b, errors.Wrap(err, "error reading chunk context bytes from stream")
	}

	v, found := ctxMap[bytesutil.ToBytes4(ctxb)]
	if !found {
		return b, errors.Wrapf(errBlobUnmarshal, "unrecognized fork digest %#x", ctxb)
	}
	// Only deneb and electra are supported at this time, because we lack a fork-spanning interface/union type for blobs.
	// In electra, there's no changes to blob type.
	if v < version.Deneb {
		return b, fmt.Errorf("unexpected context bytes for BlobSidecar, ctx=%#x, v=%s", ctxb, version.String(v))
	}
	if err := decode(stream, pb); err != nil {
		return b, errors.Wrap(err, "failed to decode the protobuf-encoded BlobSidecar message from RPC chunk stream")
	}

	rob, err := blocks.NewROBlob(pb)
	if err != nil {
		return b, errors.Wrap(err, "unexpected error initializing ROBlob")
	}
	if err := vf(rob); err != nil {
		return b, errors.Wrap(err, "validation failure decoding blob RPC response")
	}

	return rob, nil
}

// --------------------
// Data column sidecars
// --------------------

// SendDataColumnSidecarsByRangeRequest sends a request for data column sidecars by range
// and returns the fetched data column sidecars.
func SendDataColumnSidecarsByRangeRequest(
	p DataColumnSidecarsParams,
	pid peer.ID,
	request *ethpb.DataColumnSidecarsByRangeRequest,
) ([]blocks.RODataColumn, error) {
	// Return early if nothing to request.
	if request == nil || request.Count == 0 || len(request.Columns) == 0 {
		return nil, nil
	}

	beaconConfig := params.BeaconConfig()
	numberOfColumns := beaconConfig.NumberOfColumns
	maxRequestDataColumnSidecars := params.BeaconConfig().MaxRequestDataColumnSidecars

	// Check if we do not request too many sidecars.
	columnsCount := uint64(len(request.Columns))
	totalCount := request.Count * columnsCount
	if totalCount > maxRequestDataColumnSidecars {
		return nil, errors.Wrapf(errMaxRequestDataColumnSidecarsExceeded, "requestedCount=%d, allowedCount=%d", totalCount, maxRequestDataColumnSidecars)
	}

	// Build the topic.
	currentSlot := p.Tor.CurrentSlot()
	currentEpoch := slots.ToEpoch(currentSlot)
	topic, err := p2p.TopicFromMessage(p2p.DataColumnSidecarsByRangeName, currentEpoch)
	if err != nil {
		return nil, errors.Wrap(err, "topic from message")
	}

	// Build the logs.
	var columnsLog interface{} = "all"
	if columnsCount < numberOfColumns {
		columns := request.Columns
		slices.Sort(columns)
		columnsLog = columns
	}

	log := log.WithFields(logrus.Fields{
		"peer":       pid,
		"topic":      topic,
		"startSlot":  request.StartSlot,
		"count":      request.Count,
		"columns":    columnsLog,
		"totalCount": totalCount,
	})

	// Send the request.
	stream, err := p.P2P.Send(p.Ctx, request, topic, pid)
	if err != nil {
		if p.DownscorePeerOnRPCFault {
			downscorePeer(p.P2P, pid, "cannotSendDataColumnSidecarsByRangeRequest")
		}

		return nil, errors.Wrap(err, "p2p send")
	}
	defer closeStream(stream, log)

	// Read the data column sidecars from the stream.
	roDataColumns := make([]blocks.RODataColumn, 0, totalCount)
	for range totalCount {
		// Avoid reading extra chunks if the context is done.
		if err := p.Ctx.Err(); err != nil {
			return nil, err
		}

		validatorSlotWithinBounds, err := isSidecarSlotWithinBounds(request)
		if err != nil {
			if p.DownscorePeerOnRPCFault {
				downscorePeer(p.P2P, pid, "servedSidecarSlotOutOfBounds")
			}

			return nil, errors.Wrap(err, "is sidecar slot within bounds")
		}

		roDataColumn, err := readChunkedDataColumnSidecar(
			stream, p.P2P, p.CtxMap,
			validatorSlotWithinBounds,
			isSidecarIndexRequested(request),
		)
		if errors.Is(err, io.EOF) {
			if p.DownscorePeerOnRPCFault && len(roDataColumns) == 0 {
				downscorePeer(p.P2P, pid, "noReturnedSidecar")
			}

			return roDataColumns, nil
		}
		if err != nil {
			if p.DownscorePeerOnRPCFault {
				downscorePeer(p.P2P, pid, "readChunkedDataColumnSidecarError")
			}

			return nil, errors.Wrap(err, "read chunked data column sidecar")
		}

		if roDataColumn == nil {
			return nil, errors.New("nil data column sidecar, should never happen")
		}

		roDataColumns = append(roDataColumns, *roDataColumn)
	}

	// All requested sidecars were delivered by the peer. Expecting EOF.
	if _, err := readChunkedDataColumnSidecar(stream, p.P2P, p.CtxMap); !errors.Is(err, io.EOF) {
		if p.DownscorePeerOnRPCFault {
			downscorePeer(p.P2P, pid, "tooManyResponseDataColumnSidecars")
		}

		return nil, errors.Wrapf(errMaxResponseDataColumnSidecarsExceeded, "requestedCount=%d", totalCount)
	}

	return roDataColumns, nil
}

// isSidecarSlotWithinBounds verifies that the slot of the data column sidecar is within the bounds of the request.
func isSidecarSlotWithinBounds(request *ethpb.DataColumnSidecarsByRangeRequest) (DataColumnResponseValidation, error) {
	// endSlot is exclusive (while request.StartSlot is inclusive).
	endSlot, err := request.StartSlot.SafeAdd(request.Count)
	if err != nil {
		return nil, errors.Wrap(err, "calculate end slot")
	}

	validator := func(sidecar blocks.RODataColumn) error {
		slot := sidecar.Slot()

		if !(request.StartSlot <= slot && slot < endSlot) {
			return errors.Errorf("data column sidecar slot %d out of range [%d, %d[", slot, request.StartSlot, endSlot)
		}

		return nil
	}

	return validator, nil
}

// isSidecarIndexRequested verifies that the index of the data column sidecar is found in the requested indices.
func isSidecarIndexRequested(request *ethpb.DataColumnSidecarsByRangeRequest) DataColumnResponseValidation {
	requestedIndices := make(map[uint64]bool)
	for _, col := range request.Columns {
		requestedIndices[col] = true
	}

	return func(sidecar blocks.RODataColumn) error {
		columnIndex := sidecar.Index
		if !requestedIndices[columnIndex] {
			requested := helpers.SortedPrettySliceFromMap(requestedIndices)
			return errors.Errorf("data column sidecar index %d returned by the peer but not found in requested indices %v", columnIndex, requested)
		}

		return nil
	}
}

// SendDataColumnSidecarsByRootRequest sends a request for data column sidecars by root
// and returns the fetched data column sidecars.
func SendDataColumnSidecarsByRootRequest(p DataColumnSidecarsParams, peer goPeer.ID, identifiers p2ptypes.DataColumnsByRootIdentifiers) ([]blocks.RODataColumn, error) {
	// Compute how many sidecars are requested.
	count := uint64(0)
	for _, identifier := range identifiers {
		count += uint64(len(identifier.Columns))
	}

	// Return early if nothing to request.
	if count == 0 {
		return nil, nil
	}

	// Verify that the request count is within the maximum allowed.
	maxRequestDataColumnSidecars := params.BeaconConfig().MaxRequestDataColumnSidecars
	if count > maxRequestDataColumnSidecars {
		return nil, errors.Wrapf(errMaxRequestDataColumnSidecarsExceeded, "current: %d, max: %d", count, maxRequestDataColumnSidecars)
	}

	// Get the topic for the request.
	currentSlot := p.Tor.CurrentSlot()
	currentEpoch := slots.ToEpoch(currentSlot)
	topic, err := p2p.TopicFromMessage(p2p.DataColumnSidecarsByRootName, currentEpoch)
	if err != nil {
		return nil, errors.Wrap(err, "topic from message")
	}

	// Send the request to the peer.
	stream, err := p.P2P.Send(p.Ctx, identifiers, topic, peer)
	if err != nil {
		if p.DownscorePeerOnRPCFault {
			downscorePeer(p.P2P, peer, "cannotSendDataColumnSidecarsByRootRequest")
		}

		return nil, errors.Wrap(err, "p2p api send")
	}
	defer closeStream(stream, log)

	// Read the data column sidecars from the stream.
	roDataColumns := make([]blocks.RODataColumn, 0, count)

	// Read the data column sidecars from the stream.
	for range count {
		roDataColumn, err := readChunkedDataColumnSidecar(stream, p.P2P, p.CtxMap, isSidecarIndexRootRequested(identifiers))
		if errors.Is(err, io.EOF) {
			if p.DownscorePeerOnRPCFault && len(roDataColumns) == 0 {
				downscorePeer(p.P2P, peer, "noReturnedSidecar")
			}

			return roDataColumns, nil
		}
		if err != nil {
			if p.DownscorePeerOnRPCFault {
				downscorePeer(p.P2P, peer, "readChunkedDataColumnSidecarError")
			}

			return nil, errors.Wrap(err, "read chunked data column sidecar")
		}

		if roDataColumn == nil {
			return nil, errors.Wrap(err, "nil data column sidecar, should never happen")
		}

		roDataColumns = append(roDataColumns, *roDataColumn)
	}

	// All requested sidecars were delivered by the peer. Expecting EOF.
	if _, err := readChunkedDataColumnSidecar(stream, p.P2P, p.CtxMap); !errors.Is(err, io.EOF) {
		if p.DownscorePeerOnRPCFault {
			downscorePeer(p.P2P, peer, "tooManyResponseDataColumnSidecars")
		}

		return nil, errors.Wrapf(errMaxResponseDataColumnSidecarsExceeded, "requestedCount=%d", count)
	}

	return roDataColumns, nil
}

func isSidecarIndexRootRequested(request p2ptypes.DataColumnsByRootIdentifiers) DataColumnResponseValidation {
	columnsIndexFromRoot := make(map[[fieldparams.RootLength]byte]map[uint64]bool)

	for _, sidecar := range request {
		blockRoot := bytesutil.ToBytes32(sidecar.BlockRoot)
		if columnsIndexFromRoot[blockRoot] == nil {
			columnsIndexFromRoot[blockRoot] = make(map[uint64]bool)
		}

		for _, column := range sidecar.Columns {
			columnsIndexFromRoot[blockRoot][column] = true
		}
	}

	return func(sidecar blocks.RODataColumn) error {
		root, index := sidecar.BlockRoot(), sidecar.Index
		indices, ok := columnsIndexFromRoot[root]

		if !ok {
			return errors.Errorf("root %#x returned by peer but not requested", root)
		}

		if !indices[index] {
			return errors.Errorf("index %d for root %#x returned by peer but not requested", index, root)
		}

		return nil
	}
}

// DataColumnResponseValidation represents a function that can validate aspects of a single unmarshaled data column sidecar
// that was received from a peer in response to an rpc request.
type DataColumnResponseValidation func(column blocks.RODataColumn) error

func readChunkedDataColumnSidecar(
	stream network.Stream,
	p2pApi p2p.P2P,
	ctxMap ContextByteVersions,
	validationFunctions ...DataColumnResponseValidation,
) (*blocks.RODataColumn, error) {
	// Read the status code from the stream.
	statusCode, errMessage, err := ReadStatusCode(stream, p2pApi.Encoding())
	if err != nil {
		return nil, errors.Wrap(err, "read status code")
	}

	if statusCode != 0 {
		return nil, errors.Wrap(errDataColumnChunkedReadFailure, errMessage)
	}

	// Retrieve the fork digest.
	ctxBytes, err := readContextFromStream(stream)
	if err != nil {
		return nil, errors.Wrap(err, "read context from stream")
	}

	// Check if the fork digest is recognized.
	msgVersion, ok := ctxMap[bytesutil.ToBytes4(ctxBytes)]
	if !ok {
		return nil, errors.Errorf("unrecognized fork digest %#x", ctxBytes)
	}

	// Check if we are on Fulu.
	if msgVersion < version.Fulu {
		return nil, errors.Errorf(
			"unexpected context bytes for DataColumnSidecar, ctx=%#x, msgVersion=%v, minimalSupportedVersion=%v",
			ctxBytes, version.String(msgVersion), version.String(version.Fulu),
		)
	}

	// Decode the data column sidecar from the stream.
	dataColumnSidecar := new(ethpb.DataColumnSidecar)
	if err := p2pApi.Encoding().DecodeWithMaxLength(stream, dataColumnSidecar); err != nil {
		return nil, errors.Wrap(err, "failed to decode the protobuf-encoded BlobSidecar message from RPC chunk stream")
	}

	// Create a read-only data column from the data column sidecar.
	roDataColumn, err := blocks.NewRODataColumn(dataColumnSidecar)
	if err != nil {
		return nil, errors.Wrap(err, "new read only data column")
	}

	// Run validation functions.
	for _, validationFunction := range validationFunctions {
		if err := validationFunction(roDataColumn); err != nil {
			return nil, errors.Wrap(err, "validation function")
		}
	}

	return &roDataColumn, nil
}

func downscorePeer(p2p p2p.P2P, peerID peer.ID, reason string, fields ...logrus.Fields) {
	log := log
	for _, field := range fields {
		log = log.WithFields(field)
	}

	newScore := p2p.Peers().Scorers().BadResponsesScorer().Increment(peerID)
	log.WithFields(logrus.Fields{"peerID": peerID, "reason": reason, "newScore": newScore}).Debug("Downscore peer")
}
