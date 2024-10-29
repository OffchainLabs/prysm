package execution

import (
	"context"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	pb "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"google.golang.org/protobuf/proto"
)

var errNilPayloadBody = errors.New("nil payload body for block")

type blockWithHeader struct {
	block  interfaces.ReadOnlySignedBeaconBlock
	header interfaces.ExecutionData
}

// reconstructionBatch is a map of block hashes to block numbers.
type reconstructionBatch map[[32]byte]uint64

type blindedBlockReconstructor struct {
	orderedBlocks []*blockWithHeader
	bodies        map[[32]byte]*pb.ExecutionPayloadBody
	batches       map[string]reconstructionBatch
}

func reconstructBlindedBlockBatch(ctx context.Context, client RPCClient, sbb []interfaces.ReadOnlySignedBeaconBlock) ([]interfaces.SignedBeaconBlock, error) {
	r, err := newBlindedBlockReconstructor(sbb)
	if err != nil {
		return nil, err
	}
	if err := r.requestBodies(ctx, client); err != nil {
		return nil, err
	}
	return r.unblinded()
}

func newBlindedBlockReconstructor(sbb []interfaces.ReadOnlySignedBeaconBlock) (*blindedBlockReconstructor, error) {
	r := &blindedBlockReconstructor{
		orderedBlocks: make([]*blockWithHeader, 0, len(sbb)),
		bodies:        make(map[[32]byte]*pb.ExecutionPayloadBody),
	}
	for i := range sbb {
		if err := r.addToBatch(sbb[i]); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *blindedBlockReconstructor) addToBatch(b interfaces.ReadOnlySignedBeaconBlock) error {
	if err := blocks.BeaconBlockIsNil(b); err != nil {
		return errors.Wrap(err, "cannot reconstruct bellatrix block from nil data")
	}
	if !b.Block().IsBlinded() {
		return errors.New("can only reconstruct block from blinded block format")
	}
	header, err := b.Block().Body().Execution()
	if err != nil {
		return err
	}
	if header == nil || header.IsNil() {
		return errors.New("execution payload header in blinded block was nil")
	}
	r.orderedBlocks = append(r.orderedBlocks, &blockWithHeader{block: b, header: header})
	blockHash := bytesutil.ToBytes32(header.BlockHash())
	if blockHash == params.BeaconConfig().ZeroHash {
		return nil
	}

	method := payloadBodyMethodForBlock(b)
	if r.batches == nil {
		r.batches = make(map[string]reconstructionBatch)
	}
	if _, ok := r.batches[method]; !ok {
		r.batches[method] = make(reconstructionBatch)
	}
	r.batches[method][bytesutil.ToBytes32(header.BlockHash())] = header.BlockNumber()
	return nil
}

func payloadBodyMethodForBlock(_ interface{ Version() int }) string {
	return GetPayloadBodiesByHashV1
}

func (r *blindedBlockReconstructor) requestBodies(ctx context.Context, client RPCClient) error {
	for method := range r.batches {
		nilResults, err := r.requestBodiesByHash(ctx, client, method)
		if err != nil {
			return err
		}
		if err := r.handleNilResults(ctx, client, method, nilResults); err != nil {
			return err
		}
	}
	return nil
}

type hashBlockNumber struct {
	h [32]byte
	n uint64
}

func (r *blindedBlockReconstructor) handleNilResults(ctx context.Context, client RPCClient, method string, nilResults [][32]byte) error {
	if len(nilResults) == 0 {
		return nil
	}
	hbns := make([]hashBlockNumber, len(nilResults))
	for i := range nilResults {
		h := nilResults[i]
		hbns[i] = hashBlockNumber{h: h, n: r.batches[method][h]}
	}
	reqs := computeRanges(hbns)
	for i := range reqs {
		if err := r.requestBodiesByRange(ctx, client, rangeMethodForHashMethod(method), reqs[i]); err != nil {
			return err
		}
	}
	return nil
}

type byRangeReq struct {
	start uint64
	count uint64
	hbns  []hashBlockNumber
}

func computeRanges(hbns []hashBlockNumber) []byRangeReq {
	if len(hbns) == 0 {
		return nil
	}
	sort.Slice(hbns, func(i, j int) bool {
		return hbns[i].n < hbns[j].n
	})
	ranges := make([]byRangeReq, 0)
	start := hbns[0].n
	count := uint64(0)
	for i := 0; i < len(hbns); i++ {
		if hbns[i].n == start+count {
			count++
			continue
		}
		ranges = append(ranges, byRangeReq{start: start, count: count, hbns: hbns[uint64(i)-count : i]})
		start = hbns[i].n
		count = 1
	}
	ranges = append(ranges, byRangeReq{start: start, count: count, hbns: hbns[uint64(len(hbns))-count:]})
	return ranges
}

func (r *blindedBlockReconstructor) requestBodiesByRange(ctx context.Context, client RPCClient, method string, req byRangeReq) error {
	result := make([]*pb.ExecutionPayloadBody, 0)
	if err := client.CallContext(ctx, &result, method, hexutil.EncodeUint64(req.start), hexutil.EncodeUint64(req.count)); err != nil {
		return err
	}
	if uint64(len(result)) != req.count {
		return errors.Wrapf(errInvalidPayloadBodyResponse, "received %d payload bodies from %s with count=%d (start=%d)", len(result), method, req.count, req.start)
	}
	for i := range result {
		if result[i] == nil {
			return errors.Wrapf(errNilPayloadBody, "from %s, hash=%#x", method, req.hbns[i].h)
		}
		r.bodies[req.hbns[i].h] = result[i]
	}
	return nil
}

func (r *blindedBlockReconstructor) requestBodiesByHash(ctx context.Context, client RPCClient, method string) ([][32]byte, error) {
	batch := r.batches[method]
	if len(batch) == 0 {
		return nil, nil
	}
	hashes := make([]common.Hash, 0, len(batch))
	for h := range batch {
		if h == params.BeaconConfig().ZeroHash {
			continue
		}
		hashes = append(hashes, h)
	}
	result := make([]*pb.ExecutionPayloadBody, 0)
	if err := client.CallContext(ctx, &result, method, hashes); err != nil {
		return nil, err
	}
	if len(hashes) != len(result) {
		return nil, errors.Wrapf(errInvalidPayloadBodyResponse, "received %d payload bodies for %d requested hashes", len(result), len(hashes))
	}
	nilBodies := make([][32]byte, 0)
	for i := range result {
		if result[i] == nil {
			nilBodies = append(nilBodies, hashes[i])
			continue
		}
		r.bodies[hashes[i]] = result[i]
	}
	return nilBodies, nil
}

func (r *blindedBlockReconstructor) payloadForHeader(header interfaces.ExecutionData, v int) (proto.Message, error) {
	bodyKey := bytesutil.ToBytes32(header.BlockHash())
	if bodyKey == params.BeaconConfig().ZeroHash {
		payload, err := buildEmptyExecutionPayload(v)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to reconstruct payload for body hash %#x", bodyKey)
		}
		return payload, nil
	}
	body, ok := r.bodies[bodyKey]
	if !ok {
		return nil, errors.Wrapf(errNilPayloadBody, "hash %#x", bodyKey)
	}
	ed, err := fullPayloadFromPayloadBody(header, body, v)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to reconstruct payload for body hash %#x", bodyKey)
	}
	return ed.Proto(), nil
}

func (r *blindedBlockReconstructor) unblinded() ([]interfaces.SignedBeaconBlock, error) {
	unblinded := make([]interfaces.SignedBeaconBlock, len(r.orderedBlocks))
	for i := range r.orderedBlocks {
		blk, header := r.orderedBlocks[i].block, r.orderedBlocks[i].header
		payload, err := r.payloadForHeader(header, blk.Version())
		if err != nil {
			return nil, err
		}
		full, err := blocks.BuildSignedBeaconBlockFromExecutionPayload(blk, payload)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to build full block from execution payload for block hash %#x", header.BlockHash())
		}
		unblinded[i] = full
	}
	return unblinded, nil
}

func rangeMethodForHashMethod(_ string) string {
	return GetPayloadBodiesByRangeV1
}
