package decoupled

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// Spec: get_available_head_reward_per_seat. Selection is already balance weighted, so the seat reward is flat.
func availableHeadRewardPerSeat(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot) (uint64, error) {
	cfg := params.BeaconConfig()
	perIncrement, activeBalance, err := baseRewardPerIncrementAtEpoch(ctx, st, slots.ToEpoch(slot))
	if err != nil {
		return 0, err
	}
	totalBaseRewards := activeBalance / cfg.EffectiveBalanceIncrement * perIncrement
	return totalBaseRewards * cfg.TimelyHeadWeight / (cfg.WeightDenominator * uint64(cfg.SlotsPerEpoch) * fieldparams.AvailableCommitteeSize), nil
}

// Spec: get_available_attesting_positions. A signed vote counts for every seat its validator holds.
func availableAttestingPositions(committee []primitives.ValidatorIndex, bits bitfield.Bitvector512) []uint64 {
	attesting := make(map[primitives.ValidatorIndex]struct{})
	for i, v := range committee {
		if bits.BitAt(uint64(i)) {
			attesting[v] = struct{}{}
		}
	}
	positions := make([]uint64, 0, len(committee))
	for i, v := range committee {
		if _, ok := attesting[v]; ok {
			positions = append(positions, uint64(i))
		}
	}
	return positions
}

// Spec: the epoch-structured payment index in process_available_attestation
func paymentIndexForSlot(st state.ReadOnlyBeaconState, slot primitives.Slot) uint64 {
	cfg := params.BeaconConfig()
	if slots.ToEpoch(slot) == time.CurrentEpoch(st) {
		return uint64(cfg.SlotsPerEpoch + slot%cfg.SlotsPerEpoch)
	}
	return uint64(slot % cfg.SlotsPerEpoch)
}

// Spec: process_available_attestation over body.available_attestations
func ProcessAvailableAttestations(ctx context.Context, st state.BeaconState, atts []*ethpb.AvailableAttestation) error {
	for _, att := range atts {
		if err := ProcessAvailableAttestation(ctx, st, att); err != nil {
			return err
		}
	}
	return nil
}

// Spec: process_available_attestation
func ProcessAvailableAttestation(ctx context.Context, st state.BeaconState, att *ethpb.AvailableAttestation) error {
	cfg := params.BeaconConfig()
	data := att.Data
	if data == nil {
		return errors.New("nil available attestation data")
	}
	fork := st.Fork()
	isDecoupledCurrent := bytes.Equal(fork.CurrentVersion, cfg.DecoupledForkVersion)
	activationSlot, err := slots.EpochStart(fork.Epoch)
	if err != nil {
		return err
	}
	if isDecoupledCurrent && data.Slot < activationSlot {
		return errors.New("available attestation before the Decoupled activation slot")
	}
	isTransitionReceipt := isDecoupledCurrent && bytes.Equal(fork.PreviousVersion, cfg.GloasForkVersion) &&
		time.CurrentEpoch(st) == fork.Epoch && data.Slot == activationSlot
	attestationRound := slots.ToRound(data.Slot)
	inRoundWindow := attestationRound == CurrentRound(st) || attestationRound == PreviousRound(st)
	if !isTransitionReceipt && !inRoundWindow {
		return fmt.Errorf("available attestation round %d outside the inclusion window", attestationRound)
	}
	if data.Slot+cfg.MinAttestationInclusionDelay > st.Slot() {
		return errors.New("available attestation too recent")
	}
	committee, err := AvailableCommittee(st, data.Slot)
	if err != nil {
		return err
	}
	if att.AggregationBits.Len() != fieldparams.AvailableCommitteeSize || len(committee) != fieldparams.AvailableCommitteeSize {
		return errors.New("available attestation bits do not match the committee size")
	}
	if att.AggregationBits.Count() == 0 {
		return errors.New("available attestation has no bits set")
	}

	blockRoot, err := helpers.BlockRootAtSlot(st, data.Slot)
	if err != nil {
		return err
	}
	isMatchingHead := bytes.Equal(data.BeaconBlockRoot, blockRoot)
	isSameSlotBlock := isMatchingHead
	if isSameSlotBlock && data.Slot > 0 {
		parentRoot, err := helpers.BlockRootAtSlot(st, data.Slot-1)
		if err != nil {
			return err
		}
		isSameSlotBlock = !bytes.Equal(data.BeaconBlockRoot, parentRoot)
	}
	if isSameSlotBlock && data.PayloadPresent {
		return errors.New("same slot block cannot claim a present payload")
	}

	positions := availableAttestingPositions(committee, att.AggregationBits)
	seen := make(map[primitives.ValidatorIndex]struct{})
	attestingIndices := make([]primitives.ValidatorIndex, 0, len(positions))
	for _, p := range positions {
		if _, ok := seen[committee[p]]; !ok {
			seen[committee[p]] = struct{}{}
			attestingIndices = append(attestingIndices, committee[p])
		}
	}
	slices.Sort(attestingIndices)
	if err := verifyAvailableAttestationSignature(st, att, attestingIndices); err != nil {
		return err
	}

	payments, err := st.BuilderPendingPaymentsDecoupled()
	if err != nil {
		return err
	}
	paymentIndex := paymentIndexForSlot(st, data.Slot)
	payment := payments[paymentIndex]
	if isSameSlotBlock && payment.Withdrawal != nil && payment.Withdrawal.Amount > 0 {
		for _, p := range positions {
			payment.AvailableParticipation.SetBitAt(p, true)
		}
	}

	var proposerRewardBasis uint64
	if isMatchingHead && st.Slot()-data.Slot == cfg.MinAttestationInclusionDelay {
		if !inRoundWindow {
			return errors.New("timely head attestation outside the round window")
		}
		seatReward, err := availableHeadRewardPerSeat(ctx, st, data.Slot)
		if err != nil {
			return err
		}
		leak, err := IsInInactivityLeak(st)
		if err != nil {
			return err
		}
		for _, p := range positions {
			if payment.TimelyHeadParticipation.BitAt(p) {
				continue
			}
			payment.TimelyHeadParticipation.SetBitAt(p, true)
			index := committee[p]
			proposerRewardBasis += seatReward
			val, err := st.ValidatorAtIndexReadOnly(index)
			if err != nil {
				return err
			}
			if !val.Slashed() && !leak {
				if err := helpers.IncreaseBalance(st, index, seatReward); err != nil {
					return err
				}
			}
		}
		// The round flag is kept for observability only, seat rewards above never read it.
		modify := st.ModifyCurrentParticipationBits
		if attestationRound == PreviousRound(st) {
			modify = st.ModifyPreviousParticipationBits
		}
		if err := modify(func(participation []byte) ([]byte, error) {
			for _, index := range attestingIndices {
				if _, err := setFlag(participation, index, cfg.TimelyHeadFlagIndex); err != nil {
					return nil, err
				}
			}
			return participation, nil
		}); err != nil {
			return err
		}
	}
	denominator := (cfg.WeightDenominator - cfg.ProposerWeight) / cfg.ProposerWeight
	if reward := proposerRewardBasis / denominator; reward > 0 {
		proposer, err := helpers.BeaconProposerIndex(ctx, st)
		if err != nil {
			return err
		}
		if err := helpers.IncreaseBalance(st, proposer, reward); err != nil {
			return err
		}
	}
	payments[paymentIndex] = payment

	// Spec: the transition receipt, an activation-slot head vote credits every residual Gloas payment.
	if isTransitionReceipt && isMatchingHead {
		for i := range uint64(cfg.SlotsPerEpoch) {
			legacy := payments[i]
			if legacy.Withdrawal == nil || legacy.Withdrawal.Amount == 0 {
				continue
			}
			for _, p := range positions {
				legacy.AvailableParticipation.SetBitAt(p, true)
			}
			payments[i] = legacy
		}
	}
	return st.SetBuilderPendingPaymentsDecoupled(payments)
}

// Spec: the signature check in process_available_attestation, DOMAIN_AVAILABLE_ATTESTER at the attestation's epoch.
func verifyAvailableAttestationSignature(st state.ReadOnlyBeaconState, att *ethpb.AvailableAttestation, indices []primitives.ValidatorIndex) error {
	domain, err := signing.Domain(st.Fork(), slots.ToEpoch(att.Data.Slot), params.BeaconConfig().DomainAvailableAttester, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	root, err := signing.ComputeSigningRoot(att.Data, domain)
	if err != nil {
		return err
	}
	pubkeys := make([]bls.PublicKey, len(indices))
	for i, idx := range indices {
		pk := st.PubkeyAtIndex(idx)
		pubkeys[i], err = bls.PublicKeyFromBytes(pk[:])
		if err != nil {
			return err
		}
	}
	sig, err := bls.SignatureFromBytes(att.Signature)
	if err != nil {
		return err
	}
	if !sig.FastAggregateVerify(pubkeys, root) {
		return errors.New("available attestation signature did not verify")
	}
	return nil
}
