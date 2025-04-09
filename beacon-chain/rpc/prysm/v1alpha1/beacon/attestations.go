package beacon

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/prysmaticlabs/prysm/v5/api/pagination"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filters"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/operations/attestations"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stategen"
	"github.com/prysmaticlabs/prysm/v5/cmd"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// sortableAttestations implements the Sort interface to sort attestations
// by slot as the canonical sorting attribute.
type sortableAttestations []ethpb.Att

// Len is the number of elements in the collection.
func (s sortableAttestations) Len() int { return len(s) }

// Swap swaps the elements with indexes i and j.
func (s sortableAttestations) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// Less reports whether the element with index i must sort before the element with index j.
func (s sortableAttestations) Less(i, j int) bool {
	return s[i].GetData().Slot < s[j].GetData().Slot
}

func mapAttestationsByTargetRoot(atts []ethpb.Att) map[[32]byte][]ethpb.Att {
	attsMap := make(map[[32]byte][]ethpb.Att, len(atts))
	if len(atts) == 0 {
		return attsMap
	}
	for _, att := range atts {
		attsMap[bytesutil.ToBytes32(att.GetData().Target.Root)] = append(attsMap[bytesutil.ToBytes32(att.GetData().Target.Root)], att)
	}
	return attsMap
}

// ListAttestations retrieves attestations by block root, slot, or epoch.
// Attestations are sorted by data slot by default.
//
// The server may return an empty list when no attestations match the given
// filter criteria. This RPC should not return NOT_FOUND. Only one filter
// criteria should be used.
func (bs *Server) ListAttestations(
	ctx context.Context, req *ethpb.ListAttestationsRequest,
) (*ethpb.ListAttestationsResponse, error) {
	if int(req.PageSize) > cmd.Get().MaxRPCPageSize {
		return nil, status.Errorf(codes.InvalidArgument, "Requested page size %d can not be greater than max size %d",
			req.PageSize, cmd.Get().MaxRPCPageSize)
	}
	var blocks []interfaces.ReadOnlySignedBeaconBlock
	var err error
	switch q := req.QueryFilter.(type) {
	case *ethpb.ListAttestationsRequest_GenesisEpoch:
		blocks, _, err = bs.BeaconDB.Blocks(ctx, filters.NewFilter().SetStartEpoch(0).SetEndEpoch(0))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not fetch attestations: %v", err)
		}
	case *ethpb.ListAttestationsRequest_Epoch:
		if q.Epoch >= params.BeaconConfig().ElectraForkEpoch {
			return &ethpb.ListAttestationsResponse{
				Attestations:  make([]*ethpb.Attestation, 0),
				TotalSize:     int32(0),
				NextPageToken: strconv.Itoa(0),
			}, nil
		}
		blocks, _, err = bs.BeaconDB.Blocks(ctx, filters.NewFilter().SetStartEpoch(q.Epoch).SetEndEpoch(q.Epoch))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not fetch attestations: %v", err)
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "Must specify a filter criteria for fetching attestations")
	}

	atts, err := blockAttestations[*ethpb.Attestation](blocks)
	if err != nil {
		return nil, err
	}

	// If there are no attestations, we simply return a response specifying this.
	// Otherwise, attempting to paginate 0 attestations below would result in an error.
	if len(atts) == 0 {
		return &ethpb.ListAttestationsResponse{
			Attestations:  make([]*ethpb.Attestation, 0),
			TotalSize:     int32(0),
			NextPageToken: strconv.Itoa(0),
		}, nil
	}

	start, end, nextPageToken, err := pagination.StartAndEndPage(req.PageToken, int(req.PageSize), len(atts))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not paginate attestations: %v", err)
	}

	return &ethpb.ListAttestationsResponse{
		Attestations:  atts[start:end],
		TotalSize:     int32(len(atts)),
		NextPageToken: nextPageToken,
	}, nil
}

// ListAttestationsElectra retrieves attestations by block root, slot, or epoch.
// Attestations are sorted by data slot by default.
//
// The server may return an empty list when no attestations match the given
// filter criteria. This RPC should not return NOT_FOUND. Only one filter
// criteria should be used.
func (bs *Server) ListAttestationsElectra(ctx context.Context, req *ethpb.ListAttestationsRequest) (*ethpb.ListAttestationsElectraResponse, error) {
	if int(req.PageSize) > cmd.Get().MaxRPCPageSize {
		return nil, status.Errorf(codes.InvalidArgument, "Requested page size %d can not be greater than max size %d",
			req.PageSize, cmd.Get().MaxRPCPageSize)
	}
	var blocks []interfaces.ReadOnlySignedBeaconBlock
	var err error
	switch q := req.QueryFilter.(type) {
	case *ethpb.ListAttestationsRequest_GenesisEpoch:
		return &ethpb.ListAttestationsElectraResponse{
			Attestations:  make([]*ethpb.AttestationElectra, 0),
			TotalSize:     int32(0),
			NextPageToken: strconv.Itoa(0),
		}, nil
	case *ethpb.ListAttestationsRequest_Epoch:
		if q.Epoch < params.BeaconConfig().ElectraForkEpoch {
			return &ethpb.ListAttestationsElectraResponse{
				Attestations:  make([]*ethpb.AttestationElectra, 0),
				TotalSize:     int32(0),
				NextPageToken: strconv.Itoa(0),
			}, nil
		}
		blocks, _, err = bs.BeaconDB.Blocks(ctx, filters.NewFilter().SetStartEpoch(q.Epoch).SetEndEpoch(q.Epoch))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not fetch attestations: %v", err)
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "Must specify a filter criteria for fetching attestations")
	}

	atts, err := blockAttestations[*ethpb.AttestationElectra](blocks)
	if err != nil {
		return nil, err
	}

	// If there are no attestations, we simply return a response specifying this.
	// Otherwise, attempting to paginate 0 attestations below would result in an error.
	if len(atts) == 0 {
		return &ethpb.ListAttestationsElectraResponse{
			Attestations:  make([]*ethpb.AttestationElectra, 0),
			TotalSize:     int32(0),
			NextPageToken: strconv.Itoa(0),
		}, nil
	}

	start, end, nextPageToken, err := pagination.StartAndEndPage(req.PageToken, int(req.PageSize), len(atts))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not paginate attestations: %v", err)
	}

	return &ethpb.ListAttestationsElectraResponse{
		Attestations:  atts[start:end],
		TotalSize:     int32(len(atts)),
		NextPageToken: nextPageToken,
	}, nil
}

// ListIndexedAttestations retrieves indexed attestations by block root.
// IndexedAttestationsForEpoch are sorted by data slot by default. Start-end epoch
// filter is used to retrieve blocks with.
//
// The server may return an empty list when no attestations match the given
// filter criteria. This RPC should not return NOT_FOUND.
func (bs *Server) ListIndexedAttestations(
	ctx context.Context, req *ethpb.ListIndexedAttestationsRequest,
) (*ethpb.ListIndexedAttestationsResponse, error) {
	var blocks []interfaces.ReadOnlySignedBeaconBlock
	var err error
	switch q := req.QueryFilter.(type) {
	case *ethpb.ListIndexedAttestationsRequest_GenesisEpoch:
		blocks, _, err = bs.BeaconDB.Blocks(ctx, filters.NewFilter().SetStartEpoch(0).SetEndEpoch(0))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not fetch attestations: %v", err)
		}
	case *ethpb.ListIndexedAttestationsRequest_Epoch:
		if q.Epoch >= params.BeaconConfig().ElectraForkEpoch {
			return &ethpb.ListIndexedAttestationsResponse{
				IndexedAttestations: make([]*ethpb.IndexedAttestation, 0),
				TotalSize:           int32(0),
				NextPageToken:       strconv.Itoa(0),
			}, nil
		}
		blocks, _, err = bs.BeaconDB.Blocks(ctx, filters.NewFilter().SetStartEpoch(q.Epoch).SetEndEpoch(q.Epoch))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not fetch attestations: %v", err)
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "Must specify a filter criteria for fetching attestations")
	}

	indexedAtts, err := blockIndexedAttestations[*ethpb.IndexedAttestation](ctx, blocks, bs.StateGen)
	if err != nil {
		return nil, err
	}

	// If there are no attestations, we simply return a response specifying this.
	// Otherwise, attempting to paginate 0 attestations below would result in an error.
	if len(indexedAtts) == 0 {
		return &ethpb.ListIndexedAttestationsResponse{
			IndexedAttestations: make([]*ethpb.IndexedAttestation, 0),
			TotalSize:           int32(0),
			NextPageToken:       strconv.Itoa(0),
		}, nil
	}

	start, end, nextPageToken, err := pagination.StartAndEndPage(req.PageToken, int(req.PageSize), len(indexedAtts))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not paginate attestations: %v", err)
	}

	return &ethpb.ListIndexedAttestationsResponse{
		IndexedAttestations: indexedAtts[start:end],
		TotalSize:           int32(len(indexedAtts)),
		NextPageToken:       nextPageToken,
	}, nil
}

// ListIndexedAttestationsElectra retrieves indexed attestations by block root.
// IndexedAttestationsForEpoch are sorted by data slot by default. Start-end epoch
// filter is used to retrieve blocks with.
//
// The server may return an empty list when no attestations match the given
// filter criteria. This RPC should not return NOT_FOUND.
func (bs *Server) ListIndexedAttestationsElectra(
	ctx context.Context,
	req *ethpb.ListIndexedAttestationsRequest,
) (*ethpb.ListIndexedAttestationsElectraResponse, error) {
	var blocks []interfaces.ReadOnlySignedBeaconBlock
	var err error
	switch q := req.QueryFilter.(type) {
	case *ethpb.ListIndexedAttestationsRequest_GenesisEpoch:
		return &ethpb.ListIndexedAttestationsElectraResponse{
			IndexedAttestations: make([]*ethpb.IndexedAttestationElectra, 0),
			TotalSize:           int32(0),
			NextPageToken:       strconv.Itoa(0),
		}, nil
	case *ethpb.ListIndexedAttestationsRequest_Epoch:
		if q.Epoch < params.BeaconConfig().ElectraForkEpoch {
			return &ethpb.ListIndexedAttestationsElectraResponse{
				IndexedAttestations: make([]*ethpb.IndexedAttestationElectra, 0),
				TotalSize:           int32(0),
				NextPageToken:       strconv.Itoa(0),
			}, nil
		}
		blocks, _, err = bs.BeaconDB.Blocks(ctx, filters.NewFilter().SetStartEpoch(q.Epoch).SetEndEpoch(q.Epoch))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not fetch attestations: %v", err)
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "Must specify a filter criteria for fetching attestations")
	}

	indexedAtts, err := blockIndexedAttestations[*ethpb.IndexedAttestationElectra](ctx, blocks, bs.StateGen)
	if err != nil {
		return nil, err
	}
	// If there are no attestations, we simply return a response specifying this.
	// Otherwise, attempting to paginate 0 attestations below would result in an error.
	if len(indexedAtts) == 0 {
		return &ethpb.ListIndexedAttestationsElectraResponse{
			IndexedAttestations: make([]*ethpb.IndexedAttestationElectra, 0),
			TotalSize:           int32(0),
			NextPageToken:       strconv.Itoa(0),
		}, nil
	}

	start, end, nextPageToken, err := pagination.StartAndEndPage(req.PageToken, int(req.PageSize), len(indexedAtts))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not paginate attestations: %v", err)
	}

	return &ethpb.ListIndexedAttestationsElectraResponse{
		IndexedAttestations: indexedAtts[start:end],
		TotalSize:           int32(len(indexedAtts)),
		NextPageToken:       nextPageToken,
	}, nil
}

// AttestationPool retrieves pending attestations.
//
// The server returns a list of attestations that have been seen but not
// yet processed. Pool attestations eventually expire as the slot
// advances, so an attestation missing from this request does not imply
// that it was included in a block. The attestation may have expired.
// Refer to the ethereum consensus specification for more details on how
// attestations are processed and when they are no longer valid.
// https://github.com/ethereum/consensus-specs/blob/dev/specs/phase0/beacon-chain.md#attestations
func (bs *Server) AttestationPool(_ context.Context, req *ethpb.AttestationPoolRequest) (*ethpb.AttestationPoolResponse, error) {
	var atts []*ethpb.Attestation
	var err error

	if features.Get().EnableExperimentalAttestationPool {
		atts, err = attestationsFromCache[*ethpb.Attestation](req.PageSize, bs.AttestationCache)
	} else {
		atts, err = attestationsFromPool[*ethpb.Attestation](req.PageSize, bs.AttestationsPool)
	}
	if err != nil {
		return nil, err
	}
	// If there are no attestations, we simply return a response specifying this.
	// Otherwise, attempting to paginate 0 attestations below would result in an error.
	if len(atts) == 0 {
		return &ethpb.AttestationPoolResponse{
			Attestations:  make([]*ethpb.Attestation, 0),
			TotalSize:     int32(0),
			NextPageToken: strconv.Itoa(0),
		}, nil
	}

	start, end, nextPageToken, err := pagination.StartAndEndPage(req.PageToken, int(req.PageSize), len(atts))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not paginate attestations: %v", err)
	}

	return &ethpb.AttestationPoolResponse{
		Attestations:  atts[start:end],
		TotalSize:     int32(len(atts)),
		NextPageToken: nextPageToken,
	}, nil
}

func (bs *Server) AttestationPoolElectra(_ context.Context, req *ethpb.AttestationPoolRequest) (*ethpb.AttestationPoolElectraResponse, error) {
	var atts []*ethpb.AttestationElectra
	var err error

	if features.Get().EnableExperimentalAttestationPool {
		atts, err = attestationsFromCache[*ethpb.AttestationElectra](req.PageSize, bs.AttestationCache)
	} else {
		atts, err = attestationsFromPool[*ethpb.AttestationElectra](req.PageSize, bs.AttestationsPool)
	}
	if err != nil {
		return nil, err
	}

	// If there are no attestations, we simply return a response specifying this.
	// Otherwise, attempting to paginate 0 attestations below would result in an error.
	if len(atts) == 0 {
		return &ethpb.AttestationPoolElectraResponse{
			Attestations:  make([]*ethpb.AttestationElectra, 0),
			TotalSize:     int32(0),
			NextPageToken: strconv.Itoa(0),
		}, nil
	}

	start, end, nextPageToken, err := pagination.StartAndEndPage(req.PageToken, int(req.PageSize), len(atts))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not paginate attestations: %v", err)
	}

	return &ethpb.AttestationPoolElectraResponse{
		Attestations:  atts[start:end],
		TotalSize:     int32(len(atts)),
		NextPageToken: nextPageToken,
	}, nil
}

func blockAttestations[T ethpb.Att](blocks []interfaces.ReadOnlySignedBeaconBlock) ([]T, error) {
	blockAtts := make([]ethpb.Att, 0, params.BeaconConfig().MaxAttestations*uint64(len(blocks)))
	for _, blk := range blocks {
		blockAtts = append(blockAtts, blk.Block().Body().Attestations()...)
	}
	// We sort attestations according to the Sortable interface.
	sort.Sort(sortableAttestations(blockAtts))
	numAttestations := len(blockAtts)
	if numAttestations == 0 {
		return []T{}, nil
	}

	atts := make([]T, 0, len(blockAtts))
	for _, att := range blockAtts {
		a, ok := att.(T)
		if !ok {
			var expected T
			return nil, status.Errorf(codes.Internal, "Attestation is of the wrong type (expected %T, got %T)", expected, att)
		}
		atts = append(atts, a)
	}

	return atts, nil
}

func blockIndexedAttestations[T ethpb.IndexedAtt](
	ctx context.Context,
	blocks []interfaces.ReadOnlySignedBeaconBlock,
	stateGen stategen.StateManager,
) ([]T, error) {
	attsArray := make([]ethpb.Att, 0, params.BeaconConfig().MaxAttestations*uint64(len(blocks)))
	for _, b := range blocks {
		attsArray = append(attsArray, b.Block().Body().Attestations()...)
	}

	// We sort attestations according to the Sortable interface.
	sort.Sort(sortableAttestations(attsArray))
	numAttestations := len(attsArray)
	if numAttestations == 0 {
		return []T{}, nil
	}

	// We use the retrieved committees for the b root to convert all attestations
	// into indexed form effectively.
	mappedAttestations := mapAttestationsByTargetRoot(attsArray)
	indexed := make([]T, 0, numAttestations)
	for targetRoot, atts := range mappedAttestations {
		attState, err := stateGen.StateByRoot(ctx, targetRoot)
		if err != nil && strings.Contains(err.Error(), "unknown state summary") {
			// We shouldn't stop the request if we encounter an attestation we don't have the state for.
			log.Debugf("Could not get state for attestation target root %#x", targetRoot)
			continue
		} else if err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"Could not retrieve state for attestation target root %#x: %v",
				targetRoot,
				err,
			)
		}
		for i := 0; i < len(atts); i++ {
			att := atts[i]
			committee, err := helpers.BeaconCommitteeFromState(ctx, attState, att.GetData().Slot, att.GetData().CommitteeIndex)
			if err != nil {
				return nil, status.Errorf(
					codes.Internal,
					"Could not retrieve committee from state %v",
					err,
				)
			}
			idxAtt, err := attestation.ConvertToIndexed(ctx, att, committee)
			if err != nil {
				return nil, err
			}
			a, ok := idxAtt.(T)
			if !ok {
				var expected T
				return nil, status.Errorf(codes.Internal, "Indexed attestation is of the wrong type (expected %T, got %T)", expected, idxAtt)
			}
			indexed = append(indexed, a)
		}
	}

	return indexed, nil
}

func attestationsFromPool[T ethpb.Att](pageSize int32, pool attestations.Pool) ([]T, error) {
	if int(pageSize) > cmd.Get().MaxRPCPageSize {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"Requested page size %d can not be greater than max size %d",
			pageSize,
			cmd.Get().MaxRPCPageSize,
		)
	}
	poolAtts := pool.AggregatedAttestations()
	atts := make([]T, 0, len(poolAtts))
	for _, att := range poolAtts {
		a, ok := att.(T)
		if !ok {
			var expected T
			return nil, status.Errorf(codes.Internal, "Attestation is of the wrong type (expected %T, got %T)", expected, att)
		}
		atts = append(atts, a)
	}
	return atts, nil
}

func attestationsFromCache[T ethpb.Att](pageSize int32, c *cache.AttestationCache) ([]T, error) {
	if int(pageSize) > cmd.Get().MaxRPCPageSize {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"Requested page size %d can not be greater than max size %d",
			pageSize,
			cmd.Get().MaxRPCPageSize,
		)
	}
	cacheAtts := c.GetAll()
	atts := make([]T, 0, len(cacheAtts))
	for _, att := range cacheAtts {
		a, ok := att.(T)
		if !ok {
			var expected T
			return nil, status.Errorf(codes.Internal, "Attestation is of the wrong type (expected %T, got %T)", expected, att)
		}
		atts = append(atts, a)
	}
	return atts, nil
}
