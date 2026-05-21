package hdiff

import (
	"encoding/binary"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/helpers"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

const (
	builderLength                      = 93 // fixed SSZ size: pubkey(48) + version(1) + execution_address(20) + balance(8) + deposit_epoch(8) + withdrawable_epoch(8)
	builderPendingPaymentLength        = 44 // fixed SSZ size: weight(8) + fee_recipient(20) + amount(8) + builder_index(8)
	builderPendingPaymentsCount        = 2 * fieldparams.SlotsPerEpoch
	builderPendingPaymentsTotalLength  = builderPendingPaymentsCount * builderPendingPaymentLength
	builderPendingWithdrawalLength     = 36 // fixed SSZ size: fee_recipient(20) + amount(8) + builder_index(8)
	withdrawalLength                   = 44 // fixed SSZ size: index(8) + validator_index(8) + address(20) + amount(8)
	executionPayloadAvailabilityLength = fieldparams.BlockRootsLength / 8
)

// builderDiff represents a change to a single builder in the registry.
type builderDiff struct {
	index   uint32
	builder *ethpb.Builder
}

// diffGloasFields populates the Gloas-specific fields of the stateDiff.
func diffGloasFields(diff *stateDiff, source, target state.ReadOnlyBeaconState) error {
	// latestExecutionPayloadBid (override, always non-nil for valid Gloas states).
	bid, err := target.LatestExecutionPayloadBid()
	if err != nil {
		return errors.Wrap(err, "failed to get latest execution payload bid")
	}
	if bid == nil || bid.IsNil() {
		return errors.New("target Gloas state has nil execution payload bid")
	}
	parentBlockHash := bid.ParentBlockHash()
	parentBlockRoot := bid.ParentBlockRoot()
	blockHash := bid.BlockHash()
	prevRandao := bid.PrevRandao()
	feeRecipient := bid.FeeRecipient()
	executionRequestsRoot := bid.ExecutionRequestsRoot()
	diff.latestExecutionPayloadBid = &ethpb.ExecutionPayloadBid{
		ParentBlockHash:       parentBlockHash[:],
		ParentBlockRoot:       parentBlockRoot[:],
		BlockHash:             blockHash[:],
		PrevRandao:            prevRandao[:],
		GasLimit:              bid.GasLimit(),
		BuilderIndex:          bid.BuilderIndex(),
		Slot:                  bid.Slot(),
		Value:                 bid.Value(),
		ExecutionPayment:      bid.ExecutionPayment(),
		BlobKzgCommitments:    bid.BlobKzgCommitments(),
		FeeRecipient:          feeRecipient[:],
		ExecutionRequestsRoot: executionRequestsRoot[:],
	}

	// builders (sparse diff: only changed entries).
	if err := diffBuilders(diff, source, target); err != nil {
		return err
	}

	// nextWithdrawalBuilderIndex (override).
	nwbi, err := target.NextWithdrawalBuilderIndex()
	if err != nil {
		return errors.Wrap(err, "failed to get next withdrawal builder index")
	}
	diff.nextWithdrawalBuilderIndex = uint64(nwbi)

	// executionPayloadAvailability (override).
	diff.executionPayloadAvailability, err = target.ExecutionPayloadAvailabilityVector()
	if err != nil {
		return errors.Wrap(err, "failed to get execution payload availability")
	}

	// builderPendingPayments (override).
	diff.builderPendingPayments, err = target.BuilderPendingPayments()
	if err != nil {
		return errors.Wrap(err, "failed to get builder pending payments")
	}

	// builderPendingWithdrawals (prefix-drop + append via KMP).
	if err := diffBuilderPendingWithdrawals(diff, source, target); err != nil {
		return err
	}

	// latestBlockHash (override).
	diff.latestBlockHash, err = target.LatestBlockHash()
	if err != nil {
		return errors.Wrap(err, "failed to get latest block hash")
	}

	// payloadExpectedWithdrawals (override).
	diff.payloadExpectedWithdrawals, err = target.PayloadExpectedWithdrawals()
	if err != nil {
		return errors.Wrap(err, "failed to get payload expected withdrawals")
	}

	// ptcWindow (override).
	diff.ptcWindow, err = target.PTCWindow()
	if err != nil {
		return errors.Wrap(err, "failed to get ptc window")
	}

	return nil
}

func diffBuilders(diff *stateDiff, source, target state.ReadOnlyBeaconState) error {
	tBuilders, err := target.Builders()
	if err != nil {
		return errors.Wrap(err, "failed to get target builders")
	}
	var sBuilders []*ethpb.Builder
	if source.Version() >= version.Gloas {
		sBuilders, err = source.Builders()
		if err != nil {
			return errors.Wrap(err, "failed to get source builders")
		}
	}

	diff.builderDiffs = nil
	for i, tb := range tBuilders {
		if i < len(sBuilders) {
			if proto.Equal(sBuilders[i], tb) {
				continue
			}
		}
		diff.builderDiffs = append(diff.builderDiffs, builderDiff{
			index:   uint32(i),
			builder: tb,
		})
	}
	return nil
}

func diffBuilderPendingWithdrawals(diff *stateDiff, source, target state.ReadOnlyBeaconState) error {
	tBpw, err := target.BuilderPendingWithdrawals()
	if err != nil {
		return errors.Wrap(err, "failed to get target builder pending withdrawals")
	}
	tlen := len(tBpw)
	tBpw = append(tBpw, nil)
	var sBpw []*ethpb.BuilderPendingWithdrawal
	if source.Version() >= version.Gloas {
		sBpw, err = source.BuilderPendingWithdrawals()
		if err != nil {
			return errors.Wrap(err, "failed to get source builder pending withdrawals")
		}
	}
	tBpw = append(tBpw, sBpw...)
	index := kmpIndex(len(sBpw), tBpw, helpers.BuilderPendingWithdrawalsEqual)
	diff.builderPendingWithdrawalsIndex = uint64(index)
	diff.builderPendingWithdrawalsDiff = make([]*ethpb.BuilderPendingWithdrawal, tlen+index-len(sBpw))
	for i, d := range tBpw[len(sBpw)-index : tlen] {
		diff.builderPendingWithdrawalsDiff[i] = &ethpb.BuilderPendingWithdrawal{
			FeeRecipient: d.FeeRecipient,
			Amount:       d.Amount,
			BuilderIndex: d.BuilderIndex,
		}
	}
	return nil
}

// serializeGloasFields appends the Gloas-specific fields to the serialized stateDiff.
func serializeGloasFields(ret []byte, s *stateDiff) []byte {
	// latestExecutionPayloadBid (override, always non-nil).
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.latestExecutionPayloadBid.SizeSSZ()))
	sszBytes, err := s.latestExecutionPayloadBid.MarshalSSZ()
	if err != nil {
		logrus.WithError(err).Error("Failed to marshal latestExecutionPayloadBid")
		return nil
	}
	ret = append(ret, sszBytes...)

	// builderDiffs (sparse: count + per-entry index + SSZ builder).
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.builderDiffs)))
	for _, bd := range s.builderDiffs {
		ret = binary.LittleEndian.AppendUint32(ret, bd.index)
		sszBytes, err := bd.builder.MarshalSSZ()
		if err != nil {
			logrus.WithError(err).Error("Failed to marshal builder diff")
			return nil
		}
		ret = append(ret, sszBytes...)
	}

	// nextWithdrawalBuilderIndex.
	ret = binary.LittleEndian.AppendUint64(ret, s.nextWithdrawalBuilderIndex)

	// executionPayloadAvailability (fixed size: SlotsPerHistoricalRoot / 8).
	ret = append(ret, s.executionPayloadAvailability...)

	// builderPendingPayments (fixed size: 2 * SlotsPerEpoch entries).
	for _, p := range s.builderPendingPayments {
		sszBytes, err := p.MarshalSSZ()
		if err != nil {
			logrus.WithError(err).Error("Failed to marshal builder pending payment")
			return nil
		}
		ret = append(ret, sszBytes...)
	}

	// builderPendingWithdrawals (prefix-drop index + length-prefixed diff).
	ret = binary.LittleEndian.AppendUint64(ret, s.builderPendingWithdrawalsIndex)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.builderPendingWithdrawalsDiff)))
	for _, w := range s.builderPendingWithdrawalsDiff {
		sszBytes, err := w.MarshalSSZ()
		if err != nil {
			logrus.WithError(err).Error("Failed to marshal builder pending withdrawal")
			return nil
		}
		ret = append(ret, sszBytes...)
	}

	// latestBlockHash (32 bytes).
	ret = append(ret, s.latestBlockHash[:]...)

	// payloadExpectedWithdrawals (length-prefixed, each entry fixed SSZ).
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.payloadExpectedWithdrawals)))
	for _, w := range s.payloadExpectedWithdrawals {
		sszBytes, err := w.MarshalSSZ()
		if err != nil {
			logrus.WithError(err).Error("Failed to marshal payload expected withdrawal")
			return nil
		}
		ret = append(ret, sszBytes...)
	}

	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.ptcWindow)))
	for _, p := range s.ptcWindow {
		sszBytes, err := p.MarshalSSZ()
		if err != nil {
			logrus.WithError(err).Error("Failed to marshal ptc window slot")
			return nil
		}
		ret = append(ret, sszBytes...)
	}

	return ret
}

// readGloasFields deserializes the Gloas-specific fields from the serialized stateDiff.
func (ret *stateDiff) readGloasFields(data *[]byte) error {
	// latestExecutionPayloadBid (override, always non-nil).
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "latestExecutionPayloadBid size")
	}
	bidLength := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if bidLength < 0 {
		return errors.Wrap(errDataSmall, "latestExecutionPayloadBid: negative length")
	}
	*data = (*data)[8:]
	if len(*data) < bidLength {
		return errors.Wrap(errDataSmall, "latestExecutionPayloadBid data")
	}
	ret.latestExecutionPayloadBid = &ethpb.ExecutionPayloadBid{}
	if err := ret.latestExecutionPayloadBid.UnmarshalSSZ((*data)[:bidLength]); err != nil {
		return errors.Wrap(err, "failed to unmarshal latestExecutionPayloadBid")
	}
	*data = (*data)[bidLength:]

	// builderDiffs (count + per-entry: uint32 index + fixed-size SSZ builder).
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "builderDiffs count")
	}
	builderDiffsCount := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if builderDiffsCount < 0 {
		return errors.Wrap(errDataSmall, "builderDiffs: negative count")
	}
	*data = (*data)[8:]
	entrySize := 4 + builderLength // uint32 index + fixed SSZ builder
	if len(*data) < builderDiffsCount*entrySize {
		return errors.Wrap(errDataSmall, "builderDiffs data")
	}
	ret.builderDiffs = make([]builderDiff, builderDiffsCount)
	for i := range builderDiffsCount {
		ret.builderDiffs[i].index = binary.LittleEndian.Uint32((*data)[:4])
		*data = (*data)[4:]
		ret.builderDiffs[i].builder = &ethpb.Builder{}
		if err := ret.builderDiffs[i].builder.UnmarshalSSZ((*data)[:builderLength]); err != nil {
			return errors.Wrap(err, "failed to unmarshal builder diff")
		}
		*data = (*data)[builderLength:]
	}

	// nextWithdrawalBuilderIndex.
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "nextWithdrawalBuilderIndex")
	}
	ret.nextWithdrawalBuilderIndex = binary.LittleEndian.Uint64((*data)[:8])
	*data = (*data)[8:]

	// executionPayloadAvailability (fixed size: SlotsPerHistoricalRoot / 8).
	if len(*data) < executionPayloadAvailabilityLength {
		return errors.Wrap(errDataSmall, "executionPayloadAvailability")
	}
	ret.executionPayloadAvailability = make([]byte, executionPayloadAvailabilityLength)
	copy(ret.executionPayloadAvailability, (*data)[:executionPayloadAvailabilityLength])
	*data = (*data)[executionPayloadAvailabilityLength:]

	// builderPendingPayments (fixed size: 2 * SlotsPerEpoch entries).
	if len(*data) < builderPendingPaymentsTotalLength {
		return errors.Wrap(errDataSmall, "builderPendingPayments")
	}
	ret.builderPendingPayments = make([]*ethpb.BuilderPendingPayment, builderPendingPaymentsCount)
	for i := range builderPendingPaymentsCount {
		ret.builderPendingPayments[i] = &ethpb.BuilderPendingPayment{}
		if err := ret.builderPendingPayments[i].UnmarshalSSZ((*data)[:builderPendingPaymentLength]); err != nil {
			return errors.Wrap(err, "failed to unmarshal builder pending payment")
		}
		*data = (*data)[builderPendingPaymentLength:]
	}

	// builderPendingWithdrawals (prefix-drop index + length-prefixed diff).
	if len(*data) < 16 {
		return errors.Wrap(errDataSmall, "builderPendingWithdrawals")
	}
	ret.builderPendingWithdrawalsIndex = binary.LittleEndian.Uint64((*data)[:8])
	bpwCount := int(binary.LittleEndian.Uint64((*data)[8:16])) // lint:ignore uintcast
	if bpwCount < 0 {
		return errors.Wrap(errDataSmall, "builderPendingWithdrawals: negative count")
	}
	*data = (*data)[16:]
	if len(*data) < bpwCount*builderPendingWithdrawalLength {
		return errors.Wrap(errDataSmall, "builderPendingWithdrawals data")
	}
	ret.builderPendingWithdrawalsDiff = make([]*ethpb.BuilderPendingWithdrawal, bpwCount)
	for i := range bpwCount {
		ret.builderPendingWithdrawalsDiff[i] = &ethpb.BuilderPendingWithdrawal{}
		if err := ret.builderPendingWithdrawalsDiff[i].UnmarshalSSZ((*data)[:builderPendingWithdrawalLength]); err != nil {
			return errors.Wrap(err, "failed to unmarshal builder pending withdrawal")
		}
		*data = (*data)[builderPendingWithdrawalLength:]
	}

	// latestBlockHash (32 bytes).
	if len(*data) < fieldparams.RootLength {
		return errors.Wrap(errDataSmall, "latestBlockHash")
	}
	copy(ret.latestBlockHash[:], (*data)[:fieldparams.RootLength])
	*data = (*data)[fieldparams.RootLength:]

	// payloadExpectedWithdrawals (length-prefixed, fixed SSZ entries).
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "payloadExpectedWithdrawals length")
	}
	pewCount := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if pewCount < 0 {
		return errors.Wrap(errDataSmall, "payloadExpectedWithdrawals: negative count")
	}
	*data = (*data)[8:]
	if len(*data) < pewCount*withdrawalLength {
		return errors.Wrap(errDataSmall, "payloadExpectedWithdrawals data")
	}
	ret.payloadExpectedWithdrawals = make([]*enginev1.Withdrawal, pewCount)
	for i := range pewCount {
		ret.payloadExpectedWithdrawals[i] = &enginev1.Withdrawal{}
		if err := ret.payloadExpectedWithdrawals[i].UnmarshalSSZ((*data)[:withdrawalLength]); err != nil {
			return errors.Wrap(err, "failed to unmarshal payload expected withdrawal")
		}
		*data = (*data)[withdrawalLength:]
	}

	// ptcWindow (length-prefixed, each entry fixed SSZ).
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "ptcWindow length")
	}
	ptcCount := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if ptcCount < 0 {
		return errors.Wrap(errDataSmall, "ptcWindow: negative count")
	}
	*data = (*data)[8:]
	ret.ptcWindow = make([]*ethpb.PTCs, ptcCount)
	for i := range ptcCount {
		ret.ptcWindow[i] = &ethpb.PTCs{}
		ptcSize := ret.ptcWindow[i].SizeSSZ()
		if len(*data) < ptcSize {
			return errors.Wrap(errDataSmall, "ptcWindow data")
		}
		if err := ret.ptcWindow[i].UnmarshalSSZ((*data)[:ptcSize]); err != nil {
			return errors.Wrap(err, "failed to unmarshal ptc window slot")
		}
		*data = (*data)[ptcSize:]
	}

	return nil
}

// applyGloasFields applies the Gloas-specific fields from the diff to the source state.
func applyGloasFields(source state.BeaconState, diff *stateDiff) error {
	// latestExecutionPayloadBid (override, always non-nil).
	bid, err := blocks.WrappedROExecutionPayloadBid(diff.latestExecutionPayloadBid)
	if err != nil {
		return errors.Wrap(err, "failed to wrap execution payload bid")
	}
	if err := source.SetExecutionPayloadBid(bid); err != nil {
		return errors.Wrap(err, "failed to set execution payload bid")
	}

	// builders (sparse diff: patch changed indices).
	if len(diff.builderDiffs) > 0 {
		builders, err := source.Builders()
		if err != nil {
			return errors.Wrap(err, "failed to get builders")
		}
		for _, bd := range diff.builderDiffs {
			idx := int(bd.index)
			for len(builders) <= idx {
				builders = append(builders, nil)
			}
			builders[idx] = bd.builder
		}
		if err := source.SetBuilders(builders); err != nil {
			return errors.Wrap(err, "failed to set builders")
		}
	}

	// nextWithdrawalBuilderIndex.
	if err := source.SetNextWithdrawalBuilderIndex(primitives.BuilderIndex(diff.nextWithdrawalBuilderIndex)); err != nil {
		return errors.Wrap(err, "failed to set next withdrawal builder index")
	}

	// executionPayloadAvailability.
	if err := source.SetExecutionPayloadAvailabilityVector(diff.executionPayloadAvailability); err != nil {
		return errors.Wrap(err, "failed to set execution payload availability")
	}

	// builderPendingPayments.
	if err := source.SetBuilderPendingPayments(diff.builderPendingPayments); err != nil {
		return errors.Wrap(err, "failed to set builder pending payments")
	}

	// builderPendingWithdrawals (prefix-drop + append).
	if err := applyBuilderPendingWithdrawalsDiff(source, diff); err != nil {
		return errors.Wrap(err, "failed to apply builder pending withdrawals diff")
	}

	// latestBlockHash.
	if err := source.SetLatestBlockHash(diff.latestBlockHash); err != nil {
		return errors.Wrap(err, "failed to set latest block hash")
	}

	// payloadExpectedWithdrawals.
	if err := source.SetPayloadExpectedWithdrawals(diff.payloadExpectedWithdrawals); err != nil {
		return errors.Wrap(err, "failed to set payload expected withdrawals")
	}

	// ptcWindow.
	if err := source.SetPTCWindow(diff.ptcWindow); err != nil {
		return errors.Wrap(err, "failed to set ptc window")
	}

	return nil
}

func applyBuilderPendingWithdrawalsDiff(source state.BeaconState, diff *stateDiff) error {
	sBpw, err := source.BuilderPendingWithdrawals()
	if err != nil {
		return errors.Wrap(err, "failed to get builder pending withdrawals")
	}
	sBpw = sBpw[int(diff.builderPendingWithdrawalsIndex):]
	for _, d := range diff.builderPendingWithdrawalsDiff {
		sBpw = append(sBpw, &ethpb.BuilderPendingWithdrawal{
			FeeRecipient: d.FeeRecipient,
			Amount:       d.Amount,
			BuilderIndex: d.BuilderIndex,
		})
	}
	return source.SetBuilderPendingWithdrawals(sBpw)
}
