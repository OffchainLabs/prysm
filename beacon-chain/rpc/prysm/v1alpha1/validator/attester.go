package validator

import (
	"context"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	mvslice "github.com/OffchainLabs/prysm/v7/container/multi-value-slice"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// GetAttestationData requests that the beacon node produce an attestation data object,
// which the validator acting as an attester will then sign.
func (vs *Server) GetAttestationData(ctx context.Context, req *ethpb.AttestationDataRequest) (*ethpb.AttestationData, error) {
	ctx, span := trace.StartSpan(ctx, "AttesterServer.RequestAttestation")
	defer span.End()
	span.SetAttributes(
		trace.Int64Attribute("slot", int64(req.Slot)),
		trace.Int64Attribute("committeeIndex", int64(req.CommitteeIndex)),
	)

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}
	res, err := vs.CoreService.GetAttestationData(ctx, req)
	if err != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(err.Reason), "Could not get attestation data: %v", err.Err)
	}
	return res, nil
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// ProposeAttestation is a function called by an attester to vote
// on a block via an attestation object as defined in the Ethereum specification.
func (vs *Server) ProposeAttestation(ctx context.Context, att *ethpb.Attestation) (*ethpb.AttestResponse, error) {
	ctx, span := trace.StartSpan(ctx, "AttesterServer.ProposeAttestation")
	defer span.End()

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	resp, err := vs.proposeAtt(ctx, att, att.GetData().CommitteeIndex)
	if err != nil {
		return nil, err
	}

	go func() {
		if features.Get().EnableExperimentalAttestationPool {
			if err := vs.AttestationCache.Add(att); err != nil {
				log.WithError(err).Error("Could not save attestation")
			}
		} else {
			attCopy := att.Copy()
			if err := vs.AttPool.SaveUnaggregatedAttestation(attCopy); err != nil {
				log.WithError(err).Error("Could not save unaggregated attestation")
			}
		}
	}()

	return resp, nil
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// ProposeAttestationElectra is a function called by an attester to vote
// on a block via an attestation object as defined in the Ethereum specification.
// Used for Post Electra
func (vs *Server) ProposeAttestationElectra(ctx context.Context, singleAtt *ethpb.SingleAttestation) (*ethpb.AttestResponse, error) {
	ctx, span := trace.StartSpan(ctx, "AttesterServer.ProposeAttestationElectra")
	defer span.End()

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	resp, err := vs.proposeAtt(ctx, singleAtt, singleAtt.GetCommitteeIndex())
	if err != nil {
		return nil, err
	}

	targetState, err := vs.AttestationStateFetcher.AttestationTargetState(ctx, singleAtt.Data.Target)
	if err != nil {
		return nil, status.Error(codes.Internal, "Could not get target state")
	}
	committee, err := helpers.BeaconCommitteeFromState(ctx, targetState, singleAtt.Data.Slot, singleAtt.GetCommitteeIndex())
	if err != nil {
		return nil, status.Error(codes.Internal, "Could not get committee")
	}

	singleAttCopy := singleAtt.Copy()
	att := singleAttCopy.ToAttestationElectra(committee)
	go func() {
		if features.Get().EnableExperimentalAttestationPool {
			if err := vs.AttestationCache.Add(att); err != nil {
				log.WithError(err).Error("Could not save attestation")
			}
		} else {
			if err := vs.AttPool.SaveUnaggregatedAttestation(att); err != nil {
				log.WithError(err).Error("Could not save unaggregated attestation")
			}
		}
	}()

	return resp, nil
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// SubscribeCommitteeSubnets subscribes to the committee ID subnet given subscribe request.
func (vs *Server) SubscribeCommitteeSubnets(ctx context.Context, req *ethpb.CommitteeSubnetsSubscribeRequest) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "AttesterServer.SubscribeCommitteeSubnets")
	defer span.End()

	if len(req.Slots) != len(req.CommitteeIds) || len(req.CommitteeIds) != len(req.IsAggregator) {
		return nil, status.Error(codes.InvalidArgument, "request fields are not the same length")
	}
	if len(req.Slots) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no attester slots provided")
	}
	// validator_indices and committees_at_slot are 1-to-1 with slots when provided;
	// older VCs may omit them. An empty validator_indices is "no attached-set update".
	if len(req.ValidatorIndices) > 0 && len(req.ValidatorIndices) != len(req.Slots) {
		return nil, status.Error(codes.InvalidArgument, "validator_indices length must match slots length when provided")
	}
	if len(req.CommitteesAtSlot) > 0 && len(req.CommitteesAtSlot) != len(req.Slots) {
		return nil, status.Error(codes.InvalidArgument, "committees_at_slot length must match slots length when provided")
	}
	if len(req.ValidatorIndices) > 0 {
		st, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
		}
		for _, idx := range req.ValidatorIndices {
			if _, err := st.ValidatorAtIndexReadOnly(idx); err != nil {
				if errors.Is(err, mvslice.ErrOutOfBounds) {
					return nil, status.Errorf(codes.InvalidArgument, "Could not get validator: %v", err)
				}
				return nil, status.Errorf(codes.Internal, "Could not get validator: %v", err)
			}
			vs.SubscribedValidatorsCache.Add(idx)
		}
	}

	subs := make([]core.SubnetSubscription, len(req.Slots))
	for i := range req.Slots {
		subs[i] = core.SubnetSubscription{
			Slot:           req.Slots[i],
			CommitteeIndex: req.CommitteeIds[i],
			IsAggregator:   req.IsAggregator[i],
		}
		if len(req.CommitteesAtSlot) == len(req.Slots) {
			subs[i].CommitteesAtSlot = req.CommitteesAtSlot[i]
		}
	}
	if err := core.ComputeAndCacheCommitteeSubnets(ctx, vs.HeadFetcher, subs); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not retrieve head validator length: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (vs *Server) proposeAtt(
	ctx context.Context,
	att ethpb.Att,
	committeeIndex primitives.CommitteeIndex,
) (*ethpb.AttestResponse, error) {
	if _, err := bls.SignatureFromBytes(att.GetSignature()); err != nil {
		return nil, status.Error(codes.InvalidArgument, "Incorrect attestation signature")
	}

	root, err := att.GetData().HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get attestation root: %v", err)
	}

	currentEpoch := slots.ToEpoch(vs.TimeFetcher.CurrentSlot())
	if att.Version() < version.Electra && currentEpoch >= params.BeaconConfig().ElectraForkEpoch {
		return nil, status.Error(codes.InvalidArgument, "old attestation format, ProposeAttestationElectra should be called post Electra")
	}
	if att.Version() >= version.Electra {
		if currentEpoch < params.BeaconConfig().ElectraForkEpoch {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("ProposeAttestationElectra not supported yet. The current epoch is %d supported starting epoch is %d", currentEpoch, params.BeaconConfig().ElectraForkEpoch))
		}
		data := att.GetData()
		attEpoch := slots.ToEpoch(data.Slot)
		if attEpoch >= params.BeaconConfig().ElectraForkEpoch && attEpoch < params.BeaconConfig().GloasForkEpoch {
			if data.CommitteeIndex != 0 {
				return nil, status.Error(codes.InvalidArgument, "Committee index must be 0 in Electra and Fulu")
			}
		} else if attEpoch >= params.BeaconConfig().GloasForkEpoch {
			if data.CommitteeIndex >= 2 {
				return nil, status.Error(codes.InvalidArgument, "index must be < 2 post-Gloas")
			}
			if data.CommitteeIndex != 0 {
				blockSlot, err := vs.ForkchoiceFetcher.RecentBlockSlot(bytesutil.ToBytes32(data.BeaconBlockRoot))
				if err != nil {
					return nil, status.Error(codes.Internal, "could not determine block slot")
				}
				if blockSlot == data.Slot {
					return nil, status.Error(codes.InvalidArgument, "same slot attestations must use index 0 post-Gloas")
				}
			}
		}
	}

	// Broadcast the unaggregated attestation on a feed to notify other services in the beacon node
	// of a received unaggregated attestation.
	if att.IsSingle() {
		vs.OperationNotifier.OperationFeed().Send(&feed.Event{
			Type: operation.SingleAttReceived,
			Data: &operation.SingleAttReceivedData{
				Attestation: att,
			},
		})
	} else {
		vs.OperationNotifier.OperationFeed().Send(&feed.Event{
			Type: operation.UnaggregatedAttReceived,
			Data: &operation.UnAggregatedAttReceivedData{
				Attestation: att,
			},
		})
	}

	// Determine subnet to broadcast attestation to
	wantedEpoch := slots.ToEpoch(att.GetData().Slot)
	vals, err := vs.HeadFetcher.HeadValidatorsIndices(ctx, wantedEpoch)
	if err != nil {
		return nil, err
	}
	subnet := helpers.ComputeSubnetFromCommitteeAndSlot(uint64(len(vals)), committeeIndex, att.GetData().Slot)

	// Broadcast the new attestation to the network.
	if err := vs.P2P.BroadcastAttestation(ctx, subnet, att); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast attestation: %v", err)
	}

	return &ethpb.AttestResponse{
		AttestationDataRoot: root[:],
	}, nil
}
