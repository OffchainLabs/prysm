package validator

import (
	"bytes"
	"cmp"
	"context"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// getPayloadAttestations returns payload attestations for inclusion in a Gloas block.
// PTC members broadcast PayloadAttestationMessages via P2P gossip during slot N.
// All nodes collect these in a pool. The slot N+1 proposer retrieves and aggregates
// them into PayloadAttestations for block inclusion.
func (vs *Server) getPayloadAttestations(ctx context.Context, head state.BeaconState, blockParentRoot [32]byte) []*ethpb.PayloadAttestation {
	_, span := trace.StartSpan(ctx, "ProposerServer.getPayloadAttestations")
	defer span.End()

	if slots.ToEpoch(head.Slot()) < params.BeaconConfig().GloasForkEpoch {
		return nil
	}

	atts := make([]*ethpb.PayloadAttestation, 0)
	if vs.PayloadAttestationPool == nil || head.Slot() == 0 {
		return atts
	}

	parentSlot := head.Slot() - 1
	pending := vs.PayloadAttestationPool.PendingPayloadAttestations(parentSlot)
	if len(pending) == 0 {
		return atts
	}

	for _, att := range pending {
		if att == nil || att.Data == nil {
			continue
		}
		if att.Data.Slot != parentSlot {
			continue
		}
		if !bytes.Equal(att.Data.BeaconBlockRoot, blockParentRoot[:]) {
			continue
		}
		atts = append(atts, att)
	}

	slices.SortFunc(atts, func(a, b *ethpb.PayloadAttestation) int {
		if c := cmp.Compare(a.Data.Slot, b.Data.Slot); c != 0 {
			return c
		}
		if c := bytes.Compare(a.Data.BeaconBlockRoot, b.Data.BeaconBlockRoot); c != 0 {
			return c
		}
		if a.Data.PayloadPresent != b.Data.PayloadPresent {
			if !a.Data.PayloadPresent {
				return -1
			}
			return 1
		}
		if a.Data.BlobDataAvailable != b.Data.BlobDataAvailable {
			if !a.Data.BlobDataAvailable {
				return -1
			}
			return 1
		}
		return bytes.Compare(a.Signature, b.Signature)
	})

	return atts
}
