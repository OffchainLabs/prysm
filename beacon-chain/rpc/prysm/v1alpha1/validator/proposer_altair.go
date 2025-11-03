package validator

import (
	"bytes"
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	synccontribution "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/attestation/aggregation/sync_contribution"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
)

func (vs *Server) setSyncAggregate(ctx context.Context, blk interfaces.SignedBeaconBlock, headState state.BeaconState) {
	if blk.Version() < version.Altair {
		return
	}

	syncAggregate, err := vs.getSyncAggregate(ctx, slots.PrevSlot(blk.Block().Slot()), blk.Block().ParentRoot(), headState)
	if err != nil {
		log.WithError(err).Error("Could not get sync aggregate")
		emptySig := [96]byte{0xC0}
		emptyAggregate := &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, params.BeaconConfig().SyncCommitteeSize/8),
			SyncCommitteeSignature: emptySig[:],
		}
		if err := blk.SetSyncAggregate(emptyAggregate); err != nil {
			log.WithError(err).Error("Could not set sync aggregate")
		}
		return
	}

	// Can not error. We already filter block versioning at the top. Phase 0 is impossible.
	if err := blk.SetSyncAggregate(syncAggregate); err != nil {
		log.WithError(err).Error("Could not set sync aggregate")
	}
}

// getSyncAggregate retrieves the sync contributions from the pool to construct the sync aggregate object.
// The contributions are filtered based on matching of the input root and slot then profitability.
func (vs *Server) getSyncAggregate(ctx context.Context, slot primitives.Slot, root [32]byte, headState state.BeaconState) (*ethpb.SyncAggregate, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.getSyncAggregate")
	defer span.End()

	if vs.SyncCommitteePool == nil {
		return nil, errors.New("sync committee pool is nil")
	}

	poolContributions, err := vs.SyncCommitteePool.SyncCommitteeContributions(slot)
	if err != nil {
		return nil, err
	}
	// Contributions have to match the input root
	proposerContributions := proposerSyncContributions(poolContributions).filterByBlockRoot(root)

	aggregatedContributions, err := vs.aggregatedSyncCommitteeMessages(ctx, slot, root, poolContributions, headState)
	if err != nil {
		return nil, errors.Wrap(err, "could not get aggregated sync committee messages")
	}
	proposerContributions = append(proposerContributions, aggregatedContributions...)

	subcommitteeCount := params.BeaconConfig().SyncCommitteeSubnetCount
	var bitsHolder [][]byte
	for i := uint64(0); i < subcommitteeCount; i++ {
		bitsHolder = append(bitsHolder, ethpb.NewSyncCommitteeAggregationBits())
	}
	sigsHolder := make([]bls.Signature, 0, params.BeaconConfig().SyncCommitteeSize/subcommitteeCount)

	for i := uint64(0); i < subcommitteeCount; i++ {
		cs := proposerContributions.filterBySubIndex(i)
		aggregates, err := synccontribution.Aggregate(cs)
		if err != nil {
			return nil, err
		}

		// Retrieve the most profitable contribution
		deduped, err := proposerSyncContributions(aggregates).dedup()
		if err != nil {
			return nil, err
		}
		c := deduped.mostProfitable()
		if c == nil {
			continue
		}
		bitsHolder[i] = c.AggregationBits
		sig, err := bls.SignatureFromBytes(c.Signature)
		if err != nil {
			return nil, err
		}
		sigsHolder = append(sigsHolder, sig)
	}

	// Aggregate all the contribution bits and signatures.
	var syncBits []byte
	for _, b := range bitsHolder {
		syncBits = append(syncBits, b...)
	}
	syncSig := bls.AggregateSignatures(sigsHolder)
	var syncSigBytes [96]byte
	if syncSig == nil {
		syncSigBytes = common.InfiniteSignature // Infinity signature if itself is nil.
	} else {
		syncSigBytes = bytesutil.ToBytes96(syncSig.Marshal())
	}

	return &ethpb.SyncAggregate{
		SyncCommitteeBits:      syncBits,
		SyncCommitteeSignature: syncSigBytes[:],
	}, nil
}

func (vs *Server) aggregatedSyncCommitteeMessages(
	ctx context.Context,
	slot primitives.Slot,
	root [32]byte,
	poolContributions []*ethpb.SyncCommitteeContribution,
	st state.BeaconState,
) ([]*ethpb.SyncCommitteeContribution, error) {
	subcommitteeCount := params.BeaconConfig().SyncCommitteeSubnetCount
	subcommitteeSize := params.BeaconConfig().SyncCommitteeSize / subcommitteeCount
	sigsPerSubcommittee := make([][][]byte, subcommitteeCount)
	bitsPerSubcommittee := make([]bitfield.Bitfield, subcommitteeCount)
	for i := uint64(0); i < subcommitteeCount; i++ {
		sigsPerSubcommittee[i] = make([][]byte, 0, subcommitteeSize)
		bitsPerSubcommittee[i] = ethpb.NewSyncCommitteeAggregationBits()
	}

	// Get committee position(s) for each message's validator index.
	scMessages, err := vs.SyncCommitteePool.SyncCommitteeMessages(slot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get sync committee messages")
	}
	messageIndices := make([]primitives.ValidatorIndex, 0, len(scMessages))
	messageSigs := make([][]byte, 0, len(scMessages))
	for _, msg := range scMessages {
		if bytes.Equal(root[:], msg.BlockRoot) {
			messageIndices = append(messageIndices, msg.ValidatorIndex)
			messageSigs = append(messageSigs, msg.Signature)
		}
	}

	positions, err := helpers.CurrentPeriodPositions(st, messageIndices)
	if err != nil {
		return nil, errors.Wrap(err, "could not get sync committee positions")
	}

	// Based on committee position(s), set the appropriate subcommittee bit and signature.
	for i, ci := range positions {
		for _, index := range ci {
			k := uint64(index)
			subnetIndex := k / subcommitteeSize
			indexMod := k % subcommitteeSize

			// Existing aggregated contributions from the pool intersecting with aggregates
			// created from single sync committee messages can result in bit intersections
			// that fail to produce the best possible final aggregate. Ignoring bits that are
			// already set in pool contributions makes intersections impossible.
			intersects := false
			for _, poolContrib := range poolContributions {
				if poolContrib.SubcommitteeIndex == subnetIndex && poolContrib.AggregationBits.BitAt(indexMod) {
					intersects = true
				}
			}
			if !intersects && !bitsPerSubcommittee[subnetIndex].BitAt(indexMod) {
				bitsPerSubcommittee[subnetIndex].SetBitAt(indexMod, true)
				sigsPerSubcommittee[subnetIndex] = append(sigsPerSubcommittee[subnetIndex], messageSigs[i])
			}
		}
	}

	// Aggregate.
	result := make([]*ethpb.SyncCommitteeContribution, 0, subcommitteeCount)
	for i := uint64(0); i < subcommitteeCount; i++ {
		aggregatedSig := make([]byte, 96)
		aggregatedSig[0] = 0xC0
		if len(sigsPerSubcommittee[i]) != 0 {
			contrib, err := aggregateSyncSubcommitteeMessages(slot, root, i, bitsPerSubcommittee[i], sigsPerSubcommittee[i])
			if err != nil {
				// Skip aggregating this subcommittee
				log.WithError(err).Errorf("Could not aggregate sync messages for subcommittee %d", i)
				continue
			}
			result = append(result, contrib)
		}
	}

	return result, nil
}

func aggregateSyncSubcommitteeMessages(
	slot primitives.Slot,
	root [32]byte,
	subcommitteeIndex uint64,
	bits bitfield.Bitfield,
	sigs [][]byte,
) (*ethpb.SyncCommitteeContribution, error) {
	var err error
	uncompressedSigs := make([]bls.Signature, len(sigs))
	for i, sig := range sigs {
		uncompressedSigs[i], err = bls.SignatureFromBytesNoValidation(sig)
		if err != nil {
			return nil, errors.Wrap(err, "could not create signature from bytes")
		}
	}
	return &ethpb.SyncCommitteeContribution{
		Slot:              slot,
		BlockRoot:         root[:],
		SubcommitteeIndex: subcommitteeIndex,
		AggregationBits:   bits.Bytes(),
		Signature:         bls.AggregateSignatures(uncompressedSigs).Marshal(),
	}, nil
}
