package beacon

import (
	"context"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/pagination"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/cmd"
	"github.com/OffchainLabs/prysm/v7/config/features"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// AttestationPool retrieves pending attestations.
//
// The server returns a list of attestations that have been seen but not
// yet processed. Pool attestations eventually expire as the slot
// advances, so an attestation missing from this request does not imply
// that it was included in a block. The attestation may have expired.
// Refer to the ethereum consensus specification for more details on how
// attestations are processed and when they are no longer valid.
// https://github.com/ethereum/consensus-specs/blob/master/specs/phase0/beacon-chain.md#attestations
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

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
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
