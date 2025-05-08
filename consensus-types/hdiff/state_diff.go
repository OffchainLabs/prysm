package hdiff

import (
	"bytes"
	"encoding/binary"
	"slices"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/helpers"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// stateDiff is a type that represents a difference between two different beacon states. Except from the validator registry and the balances.
// Fields marked as "override" are either zeroed out or nil when there is no diff or the full new value when there is a diff.
// Except when zero may be a valid value, in which case override means the new value (eg. justificationBits).
// Fields marked as "append only" consist of a list of items that are appended to the existing list.
type stateDiff struct {
	// genesis_time does not change.
	// genesis_validators_root does not change.
	targetVersion               int
	eth1VotesAppend             bool                                  // is eth1DataVotes an append only diff?. Positioned here because of alignement.
	justificationBits           byte                                  // override.
	slot                        primitives.Slot                       // override.
	fork                        *ethpb.Fork                           // override.
	latestBlockHeader           *ethpb.BeaconBlockHeader              // override.
	blockRoots                  [fieldparams.BlockRootsLength][]byte  // zero or override.
	stateRoots                  [fieldparams.StateRootsLength][]byte  // zero or override.
	historicalRoots             [][]byte                              // append only.
	eth1Data                    *ethpb.Eth1Data                       // override.
	eth1DataVotes               []*ethpb.Eth1Data                     // append only or override.
	eth1DepositIndex            uint64                                // override.
	randaoMixes                 [fieldparams.RandaoMixesLength][]byte // zero or override.
	slashings                   [fieldparams.SlashingsLength]int64    // algebraic diff.
	previousEpochParticipation  []byte                                // override.
	currentEpochParticipation   []byte                                // override.
	previousJustifiedCheckpoint *ethpb.Checkpoint                     // override.
	currentJustifiedCheckpoint  *ethpb.Checkpoint                     // override.
	finalizedCheckpoint         *ethpb.Checkpoint                     // override.
	// Altair Fields
	inactivityScores     []uint64             // override.
	currentSyncCommittee *ethpb.SyncCommittee // override.
	nextSyncCommittee    *ethpb.SyncCommittee // override.
	// Bellatrix
	executionPayloadHeader interfaces.ExecutionData // override.
	// Capella
	nextWithdrawalIndex          uint64
	nextWithdrawalValidatorIndex primitives.ValidatorIndex
	historicalSummaries          []*ethpb.HistoricalSummary // append only.
	// Electra
	depositRequestsStartIndex     uint64
	depositBalanceToConsume       primitives.Gwei
	exitBalanceToConsume          primitives.Gwei
	earliestExitEpoch             primitives.Epoch
	consolidationBalanceToConsume primitives.Gwei
	earliestConsolidationEpoch    primitives.Epoch

	pendingDepositIndex            uint64
	pendingPartialWithdrawalsIndex uint64
	pendingConsolidationsIndex     uint64
	pendingDepositDiff             []*ethpb.PendingDeposit
	pendingPartialWithdrawalsDiff  []*ethpb.PendingPartialWithdrawal
	pendingConsolidationsDiffs     []*ethpb.PendingConsolidation
}

type Hdiff struct {
	stateDiff      *stateDiff
	validatorDiffs []validatorDiff
	balancesDiff   []int64
}

type HdiffSerialized struct {
	stateDiff      []byte
	validatorDiffs []byte
	balancesDiff   []byte
}

// validatorDiff is a type that represents a difference between two validators.
type validatorDiff struct {
	Slashed                    bool // new Value (here because of alignement)
	index                      uint32
	PublicKey                  []byte           // override.
	WithdrawalCredentials      []byte           // override.
	EffectiveBalance           uint64           // override.
	ActivationEligibilityEpoch primitives.Epoch // override
	ActivationEpoch            primitives.Epoch // override
	ExitEpoch                  primitives.Epoch // override
	WithdrawableEpoch          primitives.Epoch // override
}

var (
	errDataSmall = errors.New("data is too small")
)

const (
	nilMarker                      = byte(0)
	forkLength                     = 2*fieldparams.VersionLength + 8
	blockHeaderLength              = 8 + 8 + 3*fieldparams.RootLength
	blockRootsLength               = fieldparams.BlockRootsLength * fieldparams.RootLength
	stateRootsLength               = fieldparams.StateRootsLength * fieldparams.RootLength
	eth1DataLength                 = 8 + 2*fieldparams.RootLength
	randaoMixesLength              = fieldparams.RandaoMixesLength * fieldparams.RootLength
	checkpointLength               = 8 + fieldparams.RootLength
	syncCommitteeLength            = (fieldparams.SyncCommitteeLength + 1) * fieldparams.BLSPubkeyLength
	pendingDepositLength           = fieldparams.BLSPubkeyLength + fieldparams.RootLength + 8 + fieldparams.BLSSignatureLength + 8
	pendingPartialWithdrawalLength = 8 + 8 + 8
	pendingConsolidationLength     = 8 + 8
)

// NewHdiff desrializes a new Hdiff object from the given seialized data.
func NewHdiff(data HdiffSerialized) (*Hdiff, error) {
	stateDiff, err := newStateDiff(data.stateDiff)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create state diff")
	}

	validatorDiffs, err := newValidatorDiffs(data.validatorDiffs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create validator diffs")
	}

	balancesDiff, err := newBalancesDiff(data.balancesDiff)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create balances diff")
	}

	return &Hdiff{
		stateDiff:      stateDiff,
		validatorDiffs: validatorDiffs,
		balancesDiff:   balancesDiff,
	}, nil
}

func (ret *stateDiff) readTargetVersion(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "targetVersion")
	}
	ret.targetVersion = int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	*data = (*data)[8:]
	return nil
}

func (ret *stateDiff) readSlot(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "slot")
	}
	ret.slot = primitives.Slot(binary.LittleEndian.Uint64((*data)[:8]))
	*data = (*data)[8:]
	return nil
}

func (ret *stateDiff) readFork(data *[]byte) error {
	if len(*data) < 1 {
		return errors.Wrap(errDataSmall, "fork")
	}
	if (*data)[0] == nilMarker {
		*data = (*data)[1:]
		return nil
	}
	*data = (*data)[1:]
	if len(*data) < forkLength {
		return errors.Wrap(errDataSmall, "fork")
	}
	ret.fork = &ethpb.Fork{
		PreviousVersion: slices.Clone((*data)[:fieldparams.VersionLength]),
		CurrentVersion:  slices.Clone((*data)[fieldparams.VersionLength : fieldparams.VersionLength*2]),
		Epoch:           primitives.Epoch(binary.LittleEndian.Uint64((*data)[2*fieldparams.VersionLength : 2*fieldparams.VersionLength+8])),
	}
	*data = (*data)[forkLength:]
	return nil
}

func (ret *stateDiff) readLatestBlockHeader(data *[]byte) error {
	// Read latestBlockHeader.
	if len((*data)) < 1 {
		return errors.Wrap(errDataSmall, "latestBlockHeader")
	}
	if (*data)[0] == nilMarker {
		*data = (*data)[1:]
		return nil
	}
	*data = (*data)[1:]
	if len(*data) < blockHeaderLength {
		return errors.Wrap(errDataSmall, "latestBlockHeader")
	}
	ret.latestBlockHeader = &ethpb.BeaconBlockHeader{
		Slot:          primitives.Slot(binary.LittleEndian.Uint64((*data)[:8])),
		ProposerIndex: primitives.ValidatorIndex(binary.LittleEndian.Uint64((*data)[8:16])),
		ParentRoot:    slices.Clone((*data)[16 : 16+fieldparams.RootLength]),
		StateRoot:     slices.Clone((*data)[16+fieldparams.RootLength : 16+2*fieldparams.RootLength]),
		BodyRoot:      slices.Clone((*data)[16+2*fieldparams.RootLength : 16+3*fieldparams.RootLength]),
	}
	*data = (*data)[blockHeaderLength:]
	return nil
}

func (ret *stateDiff) readBlockRoots(data *[]byte) error {
	if len(*data) < blockRootsLength {
		return errors.Wrap(errDataSmall, "blockRoots")
	}
	for i := range fieldparams.BlockRootsLength {
		copy(ret.blockRoots[i], (*data)[i*fieldparams.RootLength:(i+1)*fieldparams.RootLength])
	}
	*data = (*data)[blockRootsLength:]
	return nil
}

func (ret *stateDiff) readStateRoots(data *[]byte) error {
	if len(*data) < stateRootsLength {
		return errors.Wrap(errDataSmall, "stateRoots")
	}
	for i := range fieldparams.StateRootsLength {
		copy(ret.stateRoots[i], (*data)[i*fieldparams.RootLength:(i+1)*fieldparams.RootLength])
	}
	*data = (*data)[stateRootsLength:]
	return nil
}

func (ret *stateDiff) readHistoricalRoots(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "historicalRoots")
	}
	historicalRootsLength := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	(*data) = (*data)[8:]
	if len(*data) < historicalRootsLength*fieldparams.RootLength {
		return errors.Wrap(errDataSmall, "historicalRoots")
	}
	ret.historicalRoots = make([][]byte, historicalRootsLength)
	for i := range historicalRootsLength {
		ret.historicalRoots[i] = slices.Clone((*data)[i*fieldparams.RootLength : (i+1)*fieldparams.RootLength])
	}
	*data = (*data)[historicalRootsLength*fieldparams.RootLength:]
	return nil
}

func (ret *stateDiff) readEth1Data(data *[]byte) error {
	if len(*data) < 1 {
		return errors.Wrap(errDataSmall, "eth1Data")
	}
	if (*data)[0] == nilMarker {
		*data = (*data)[1:]
		return nil
	}
	if len(*data) < eth1DataLength+1 {
		return errors.Wrap(errDataSmall, "eth1Data")
	}
	ret.eth1Data = &ethpb.Eth1Data{
		DepositRoot:  slices.Clone((*data)[1 : 1+fieldparams.RootLength]),
		DepositCount: binary.LittleEndian.Uint64((*data)[1+fieldparams.RootLength : 1+fieldparams.RootLength+8]),
		BlockHash:    slices.Clone((*data)[1+fieldparams.RootLength+8 : 1+2*fieldparams.RootLength+8]),
	}
	*data = (*data)[1+eth1DataLength:]
	return nil
}

func (ret *stateDiff) readEth1DataVotes(data *[]byte) error {
	// Read eth1DataVotes.
	if len(*data) < 9 {
		return errors.Wrap(errDataSmall, "eth1DataVotes")
	}
	if (*data)[0] == nilMarker {
		ret.eth1VotesAppend = true
	} else {
		ret.eth1VotesAppend = false
	}
	eth1DataVotesLength := int(binary.LittleEndian.Uint64((*data)[1 : 1+8])) // lint:ignore uintcast
	if len(*data) < 1+8+eth1DataVotesLength*eth1DataLength {
		return errors.Wrap(errDataSmall, "eth1DataVotes")
	}
	ret.eth1DataVotes = make([]*ethpb.Eth1Data, eth1DataVotesLength)
	cursor := 9
	for i := range eth1DataVotesLength {
		ret.eth1DataVotes[i] = &ethpb.Eth1Data{
			DepositRoot:  slices.Clone((*data)[cursor : cursor+fieldparams.RootLength]),
			DepositCount: binary.LittleEndian.Uint64((*data)[cursor+fieldparams.RootLength : cursor+fieldparams.RootLength+8]),
			BlockHash:    slices.Clone((*data)[cursor+fieldparams.RootLength+8 : cursor+2*fieldparams.RootLength+8]),
		}
		cursor += eth1DataLength
	}
	*data = (*data)[1+8+eth1DataVotesLength*eth1DataLength:]
	return nil
}

func (ret *stateDiff) readEth1DepositIndex(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "eth1DepositIndex")
	}
	ret.eth1DepositIndex = binary.LittleEndian.Uint64((*data)[:8])
	*data = (*data)[8:]
	return nil
}

func (ret *stateDiff) readRandaoMixes(data *[]byte) error {
	if len(*data) < randaoMixesLength {
		return errors.Wrap(errDataSmall, "randaoMixes")
	}
	cursor := 0
	for i := range fieldparams.RandaoMixesLength {
		copy(ret.randaoMixes[i], (*data)[cursor:cursor+fieldparams.RootLength])
		cursor += fieldparams.RootLength
	}
	*data = (*data)[randaoMixesLength:]
	return nil
}

func (ret *stateDiff) readSlashings(data *[]byte) error {
	if len(*data) < fieldparams.SlashingsLength*8 {
		return errors.Wrap(errDataSmall, "slashings")
	}
	cursor := 0
	for i := range fieldparams.SlashingsLength {
		ret.slashings[i] = int64(binary.LittleEndian.Uint64((*data)[cursor : cursor+8])) // lint:ignore uintcast
		cursor += 8
	}
	*data = (*data)[fieldparams.SlashingsLength*8:]
	return nil
}

func (ret *stateDiff) readPreviousEpochParticipation(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "previousEpochParticipation")
	}
	previousEpochParticipationLength := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if len(*data) < 8+previousEpochParticipationLength {
		return errors.Wrap(errDataSmall, "previousEpochParticipation")
	}
	ret.previousEpochParticipation = make([]byte, previousEpochParticipationLength)
	copy(ret.previousEpochParticipation, (*data)[8:8+previousEpochParticipationLength])
	*data = (*data)[8+previousEpochParticipationLength:]
	return nil
}
func (ret *stateDiff) readCurrentEpochParticipation(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "currentEpochParticipation")
	}
	currentEpochParticipationLength := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if len(*data) < 8+currentEpochParticipationLength {
		return errors.Wrap(errDataSmall, "currentEpochParticipation")
	}
	ret.currentEpochParticipation = make([]byte, currentEpochParticipationLength)
	copy(ret.currentEpochParticipation, (*data)[8:8+currentEpochParticipationLength])
	*data = (*data)[8+currentEpochParticipationLength:]
	return nil
}

func (ret *stateDiff) readJustificationBits(data *[]byte) error {
	if len(*data) < 1 {
		return errors.Wrap(errDataSmall, "justificationBits")
	}
	ret.justificationBits = (*data)[0]
	*data = (*data)[1:]
	return nil
}

func (ret *stateDiff) readPreviousJustifiedCheckpoint(data *[]byte) error {
	if len(*data) < checkpointLength {
		return errors.Wrap(errDataSmall, "previousJustifiedCheckpoint")
	}
	ret.previousJustifiedCheckpoint = &ethpb.Checkpoint{
		Epoch: primitives.Epoch(binary.LittleEndian.Uint64((*data)[:8])),
		Root:  slices.Clone((*data)[8 : 8+fieldparams.RootLength]),
	}
	*data = (*data)[checkpointLength:]
	return nil
}

func (ret *stateDiff) readCurrentJustifiedCheckpoint(data *[]byte) error {
	if len(*data) < checkpointLength {
		return errors.Wrap(errDataSmall, "currentJustifiedCheckpoint")
	}
	ret.currentJustifiedCheckpoint = &ethpb.Checkpoint{
		Epoch: primitives.Epoch(binary.LittleEndian.Uint64((*data)[:8])),
		Root:  slices.Clone((*data)[8 : 8+fieldparams.RootLength]),
	}
	*data = (*data)[checkpointLength:]
	return nil
}

func (ret *stateDiff) readFinalizedCheckpoint(data *[]byte) error {
	if len(*data) < checkpointLength {
		return errors.Wrap(errDataSmall, "finalizedCheckpoint")
	}
	ret.finalizedCheckpoint = &ethpb.Checkpoint{
		Epoch: primitives.Epoch(binary.LittleEndian.Uint64((*data)[:8])),
		Root:  slices.Clone((*data)[8 : 8+fieldparams.RootLength]),
	}
	*data = (*data)[checkpointLength:]
	return nil
}

func (ret *stateDiff) readInactivityScores(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "inactivityScores")
	}
	inactivityScoresLength := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if len(*data) < 8+inactivityScoresLength*8 {
		return errors.Wrap(errDataSmall, "inactivityScores")
	}
	ret.inactivityScores = make([]uint64, inactivityScoresLength)
	cursor := 8
	for i := range inactivityScoresLength {
		ret.inactivityScores[i] = binary.LittleEndian.Uint64((*data)[cursor : cursor+8])
		cursor += 8
	}
	*data = (*data)[cursor:]
	return nil
}

func (ret *stateDiff) readCurrentSyncCommittee(data *[]byte) error {
	if len(*data) < 1 {
		return errors.Wrap(errDataSmall, "currentSyncCommittee")
	}
	if (*data)[0] == nilMarker {
		*data = (*data)[1:]
		return nil
	}
	*data = (*data)[1:]
	if len(*data) < syncCommitteeLength {
		return errors.Wrap(errDataSmall, "currentSyncCommittee")
	}
	ret.currentSyncCommittee = &ethpb.SyncCommittee{}
	if err := ret.currentSyncCommittee.UnmarshalSSZ((*data)[:syncCommitteeLength]); err != nil {
		return errors.Wrap(err, "failed to unmarshal currentSyncCommittee")
	}
	*data = (*data)[syncCommitteeLength:]
	return nil
}

func (ret *stateDiff) readNextSyncCommittee(data *[]byte) error {
	if len(*data) < 1 {
		return errors.Wrap(errDataSmall, "nextSyncCommittee")
	}
	if (*data)[0] == nilMarker {
		*data = (*data)[1:]
		return nil
	}
	*data = (*data)[1:]
	if len(*data) < syncCommitteeLength {
		return errors.Wrap(errDataSmall, "nextSyncCommittee")
	}
	ret.nextSyncCommittee = &ethpb.SyncCommittee{}
	if err := ret.nextSyncCommittee.UnmarshalSSZ((*data)[:syncCommitteeLength]); err != nil {
		return errors.Wrap(err, "failed to unmarshal nextSyncCommittee")
	}
	*data = (*data)[syncCommitteeLength:]
	return nil
}

func (ret *stateDiff) readExecutionPayloadHeader(data *[]byte) error {
	if len(*data) < 1 {
		return errors.Wrap(errDataSmall, "executionPayloadHeader")
	}
	if (*data)[0] == nilMarker {
		*data = (*data)[1:]
		return nil
	}
	if len(*data) < 9 {
		return errors.Wrap(errDataSmall, "executionPayloadHeader")
	}
	headerLength := int(binary.LittleEndian.Uint64((*data)[1:9])) // lint:ignore uintcast
	*data = (*data)[9:]
	type sszSizeUnmarshaler interface {
		ssz.Unmarshaler
		ssz.Marshaler
		proto.Message
	}
	var header sszSizeUnmarshaler
	switch ret.targetVersion {
	case version.Bellatrix:
		header = &enginev1.ExecutionPayloadHeader{}
	case version.Capella:
		header = &enginev1.ExecutionPayloadHeaderCapella{}
	case version.Deneb, version.Electra:
		header = &enginev1.ExecutionPayloadHeaderDeneb{}
	default:
		return errors.Errorf("unknown target version %d", ret.targetVersion)
	}
	if len(*data) < headerLength {
		return errors.Wrap(errDataSmall, "executionPayloadHeader")
	}
	if err := header.UnmarshalSSZ((*data)[:headerLength]); err != nil {
		return errors.Wrap(err, "failed to unmarshal executionPayloadHeader")
	}
	var err error
	ret.executionPayloadHeader, err = blocks.NewWrappedExecutionData(header)
	if err != nil {
		return err
	}
	*data = (*data)[headerLength:]
	return nil
}

func (ret *stateDiff) readWithdrawalIndices(data *[]byte) error {
	if len(*data) < 16 {
		return errors.Wrap(errDataSmall, "withdrawalIndices")
	}
	ret.nextWithdrawalIndex = binary.LittleEndian.Uint64((*data)[:8])
	ret.nextWithdrawalValidatorIndex = primitives.ValidatorIndex(binary.LittleEndian.Uint64((*data)[8:16]))
	*data = (*data)[16:]
	return nil
}

func (ret *stateDiff) readHistoricalSummaries(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "historicalSummaries")
	}
	historicalSummariesLength := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if len(*data) < 8+historicalSummariesLength*fieldparams.RootLength*2 {
		return errors.Wrap(errDataSmall, "historicalSummaries")
	}
	ret.historicalSummaries = make([]*ethpb.HistoricalSummary, historicalSummariesLength)
	cursor := 8
	for i := range historicalSummariesLength {
		ret.historicalSummaries[i] = &ethpb.HistoricalSummary{
			BlockSummaryRoot: slices.Clone((*data)[cursor : cursor+fieldparams.RootLength]),
			StateSummaryRoot: slices.Clone((*data)[cursor+fieldparams.RootLength : cursor+2*fieldparams.RootLength]),
		}
		cursor += 2 * fieldparams.RootLength
	}
	*data = (*data)[cursor:]
	return nil
}

func (ret *stateDiff) readElectraPendingIndices(data *[]byte) error {
	// Read depositRequestsStartIndex.
	if len(*data) < 8*6 {
		return errors.Wrap(errDataSmall, "electraPendingIndices")
	}
	ret.depositRequestsStartIndex = binary.LittleEndian.Uint64((*data)[:8])
	ret.depositBalanceToConsume = primitives.Gwei(binary.LittleEndian.Uint64((*data)[8:16]))
	ret.exitBalanceToConsume = primitives.Gwei(binary.LittleEndian.Uint64((*data)[16:24]))
	ret.earliestExitEpoch = primitives.Epoch(binary.LittleEndian.Uint64((*data)[24:32]))
	ret.consolidationBalanceToConsume = primitives.Gwei(binary.LittleEndian.Uint64((*data)[32:40]))
	ret.earliestConsolidationEpoch = primitives.Epoch(binary.LittleEndian.Uint64((*data)[40:48]))
	*data = (*data)[48:]
	return nil
}

func (ret *stateDiff) readPendingDeposits(data *[]byte) error {
	if len(*data) < 16 {
		return errors.Wrap(errDataSmall, "pendingDeposits")
	}
	ret.pendingDepositIndex = binary.LittleEndian.Uint64((*data)[:8])
	pendingDepositDiffLength := int(binary.LittleEndian.Uint64((*data)[8:16])) // lint:ignore uintcast
	if len(*data) < 16+pendingDepositDiffLength*pendingDepositLength {
		return errors.Wrap(errDataSmall, "pendingDepositDiff")
	}
	ret.pendingDepositDiff = make([]*ethpb.PendingDeposit, pendingDepositDiffLength)
	cursor := 16
	for i := range pendingDepositDiffLength {
		ret.pendingDepositDiff[i] = &ethpb.PendingDeposit{
			PublicKey:             slices.Clone((*data)[cursor : cursor+fieldparams.BLSPubkeyLength]),
			WithdrawalCredentials: slices.Clone((*data)[cursor+fieldparams.BLSPubkeyLength : cursor+fieldparams.BLSPubkeyLength+fieldparams.RootLength]),
			Amount:                binary.LittleEndian.Uint64((*data)[cursor+fieldparams.BLSPubkeyLength+fieldparams.RootLength : cursor+fieldparams.BLSPubkeyLength+fieldparams.RootLength+8]),
			Signature:             slices.Clone((*data)[cursor+fieldparams.BLSPubkeyLength+fieldparams.RootLength+8 : cursor+fieldparams.BLSPubkeyLength+fieldparams.RootLength+8+fieldparams.BLSSignatureLength]),
			Slot:                  primitives.Slot(binary.LittleEndian.Uint64((*data)[cursor+fieldparams.BLSPubkeyLength+fieldparams.RootLength+8+fieldparams.BLSSignatureLength : cursor+fieldparams.BLSPubkeyLength+fieldparams.RootLength+8+fieldparams.BLSSignatureLength+8])),
		}
		cursor += pendingDepositLength
	}
	*data = (*data)[cursor:]
	return nil
}

func (ret *stateDiff) readPendingPartialWithdrawals(data *[]byte) error {
	if len(*data) < 16 {
		return errors.Wrap(errDataSmall, "pendingPartialWithdrawals")
	}
	ret.pendingPartialWithdrawalsIndex = binary.LittleEndian.Uint64((*data)[:8])
	pendingPartialWithdrawalsDiffLength := int(binary.LittleEndian.Uint64((*data)[8:16])) // lint:ignore uintcast
	if len(*data) < 16+pendingPartialWithdrawalsDiffLength*pendingPartialWithdrawalLength {
		return errors.Wrap(errDataSmall, "pendingPartialWithdrawalsDiff")
	}
	ret.pendingPartialWithdrawalsDiff = make([]*ethpb.PendingPartialWithdrawal, pendingPartialWithdrawalsDiffLength)
	cursor := 16
	for i := range pendingPartialWithdrawalsDiffLength {
		ret.pendingPartialWithdrawalsDiff[i] = &ethpb.PendingPartialWithdrawal{
			Index:             primitives.ValidatorIndex(binary.LittleEndian.Uint64((*data)[cursor : cursor+8])),
			Amount:            binary.LittleEndian.Uint64((*data)[cursor+8 : cursor+16]),
			WithdrawableEpoch: primitives.Epoch(binary.LittleEndian.Uint64((*data)[cursor+16 : cursor+24])),
		}
		cursor += pendingPartialWithdrawalLength
	}
	*data = (*data)[cursor:]
	return nil
}

func (ret *stateDiff) readPendingConsolidations(data *[]byte) error {
	if len(*data) < 16 {
		return errors.Wrap(errDataSmall, "pendingConsolidations")
	}
	ret.pendingConsolidationsIndex = binary.LittleEndian.Uint64((*data)[:8])
	pendingConsolidationsDiffsLength := int(binary.LittleEndian.Uint64((*data)[8:16])) // lint:ignore uintcast
	if len(*data) < 16+pendingConsolidationsDiffsLength*pendingConsolidationLength {
		return errors.Wrap(errDataSmall, "pendingConsolidationsDiffs")
	}
	ret.pendingConsolidationsDiffs = make([]*ethpb.PendingConsolidation, pendingConsolidationsDiffsLength)
	cursor := 16
	for i := range pendingConsolidationsDiffsLength {
		ret.pendingConsolidationsDiffs[i] = &ethpb.PendingConsolidation{
			SourceIndex: primitives.ValidatorIndex(binary.LittleEndian.Uint64((*data)[cursor : cursor+8])),
			TargetIndex: primitives.ValidatorIndex(binary.LittleEndian.Uint64((*data)[cursor+8 : cursor+16])),
		}
		cursor += pendingConsolidationLength
	}
	*data = (*data)[cursor:]
	return nil
}

// newStateDiff deserializes a new StateDiff object from the given data.
func newStateDiff(data []byte) (*stateDiff, error) {
	ret := &stateDiff{}
	if err := ret.readTargetVersion(&data); err != nil {
		return nil, err
	}
	if err := ret.readSlot(&data); err != nil {
		return nil, err
	}
	if err := ret.readFork(&data); err != nil {
		return nil, err
	}
	if err := ret.readLatestBlockHeader(&data); err != nil {
		return nil, err
	}
	if err := ret.readBlockRoots(&data); err != nil {
		return nil, err
	}
	if err := ret.readStateRoots(&data); err != nil {
		return nil, err
	}
	if err := ret.readHistoricalRoots(&data); err != nil {
		return nil, err
	}
	if err := ret.readEth1Data(&data); err != nil {
		return nil, err
	}
	if err := ret.readEth1DataVotes(&data); err != nil {
		return nil, err
	}
	if err := ret.readEth1DepositIndex(&data); err != nil {
		return nil, err
	}
	if err := ret.readRandaoMixes(&data); err != nil {
		return nil, err
	}
	if err := ret.readSlashings(&data); err != nil {
		return nil, err
	}
	if err := ret.readPreviousEpochParticipation(&data); err != nil {
		return nil, err
	}
	if err := ret.readCurrentEpochParticipation(&data); err != nil {
		return nil, err
	}
	if err := ret.readJustificationBits(&data); err != nil {
		return nil, err
	}
	if err := ret.readPreviousJustifiedCheckpoint(&data); err != nil {
		return nil, err
	}
	if err := ret.readCurrentJustifiedCheckpoint(&data); err != nil {
		return nil, err
	}
	if err := ret.readFinalizedCheckpoint(&data); err != nil {
		return nil, err
	}
	if err := ret.readInactivityScores(&data); err != nil {
		return nil, err
	}
	if err := ret.readCurrentSyncCommittee(&data); err != nil {
		return nil, err
	}
	if err := ret.readNextSyncCommittee(&data); err != nil {
		return nil, err
	}
	if err := ret.readExecutionPayloadHeader(&data); err != nil {
		return nil, err
	}
	if err := ret.readWithdrawalIndices(&data); err != nil {
		return nil, err
	}
	if err := ret.readHistoricalSummaries(&data); err != nil {
		return nil, err
	}
	if err := ret.readElectraPendingIndices(&data); err != nil {
		return nil, err
	}
	if err := ret.readPendingDeposits(&data); err != nil {
		return nil, err
	}
	if err := ret.readPendingPartialWithdrawals(&data); err != nil {
		return nil, err
	}
	if err := ret.readPendingConsolidations(&data); err != nil {
		return nil, err
	}

	if len(data) > 0 {
		return nil, errors.Errorf("data is too large, exceeded by %d bytes", len(data))
	}
	return ret, nil
}

// newValidatorDiffs deserializes a new validator diffs from the given data.
func newValidatorDiffs(data []byte) ([]validatorDiff, error) {
	cursor := 0
	if len(data[cursor:]) < 8 {
		return nil, errors.Wrap(errDataSmall, "validatorDiffs")
	}
	validatorDiffsLength := binary.LittleEndian.Uint64(data[cursor : cursor+8])
	cursor += 8
	validatorDiffs := make([]validatorDiff, validatorDiffsLength)
	for i := range validatorDiffsLength {
		if len(data[cursor:]) < 4 {
			return nil, errors.Wrap(errDataSmall, "validatorDiffs: index")
		}
		validatorDiffs[i].index = binary.LittleEndian.Uint32(data[cursor : cursor+4])
		cursor += 4
		if len(data[cursor:]) < 1 {
			return nil, errors.Wrap(errDataSmall, "validatorDiffs: PublicKey")
		}
		cursor++
		if data[cursor-1] != nilMarker {
			if len(data[cursor:]) < fieldparams.BLSPubkeyLength {
				return nil, errors.Wrap(errDataSmall, "validatorDiffs: PublicKey")
			}
			validatorDiffs[i].PublicKey = data[cursor : cursor+fieldparams.BLSPubkeyLength]
			cursor += fieldparams.BLSPubkeyLength
		}
		if len(data[cursor:]) < 1 {
			return nil, errors.Wrap(errDataSmall, "validatorDiffs: WithdrawalCredentials")
		}
		cursor++
		if data[cursor-1] != nilMarker {
			if len(data[cursor:]) < fieldparams.RootLength {
				return nil, errors.Wrap(errDataSmall, "validatorDiffs: WithdrawalCredentials")
			}
			validatorDiffs[i].WithdrawalCredentials = data[cursor : cursor+fieldparams.RootLength]
			cursor += fieldparams.RootLength
		}
		if len(data[cursor:]) < 8 {
			return nil, errors.Wrap(errDataSmall, "validatorDiffs: EffectiveBalance")
		}
		validatorDiffs[i].EffectiveBalance = binary.LittleEndian.Uint64(data[cursor : cursor+8])
		cursor += 8
		if len(data[cursor:]) < 1 {
			return nil, errors.Wrap(errDataSmall, "validatorDiffs: Slashed")
		}
		validatorDiffs[i].Slashed = data[cursor] != nilMarker
		cursor++
		if len(data[cursor:]) < 8 {
			return nil, errors.Wrap(errDataSmall, "validatorDiffs: ActivationEligibilityEpoch")
		}
		validatorDiffs[i].ActivationEligibilityEpoch = primitives.Epoch(binary.LittleEndian.Uint64(data[cursor : cursor+8]))
		cursor += 8
		if len(data[cursor:]) < 8 {
			return nil, errors.Wrap(errDataSmall, "validatorDiffs: ActivationEpoch")
		}
		validatorDiffs[i].ActivationEpoch = primitives.Epoch(binary.LittleEndian.Uint64(data[cursor : cursor+8]))
		cursor += 8
		if len(data[cursor:]) < 8 {
			return nil, errors.Wrap(errDataSmall, "validatorDiffs: ExitEpoch")
		}
		validatorDiffs[i].ExitEpoch = primitives.Epoch(binary.LittleEndian.Uint64(data[cursor : cursor+8]))
		cursor += 8
		if len(data[cursor:]) < 8 {
			return nil, errors.Wrap(errDataSmall, "validatorDiffs: WithdrawableEpoch")
		}
		validatorDiffs[i].WithdrawableEpoch = primitives.Epoch(binary.LittleEndian.Uint64(data[cursor : cursor+8]))
		cursor += 8
	}
	if cursor != len(data) {
		return nil, errors.Errorf("data is too large, expected %d bytes, got %d", len(data), cursor)
	}
	return validatorDiffs, nil
}

// newBalancesDiff deserializes a new balances diff from the given data.
func newBalancesDiff(data []byte) ([]int64, error) {
	if len(data) < 8 {
		return nil, errors.Wrap(errDataSmall, "balancesDiff")
	}
	balancesLength := int(binary.LittleEndian.Uint64(data[:8])) // lint:ignore uintcast
	if len(data) != 8+balancesLength*8 {
		return nil, errors.Errorf("incorrect length of balancesDiff, expected %d, got %d", 8+balancesLength*8, len(data))
	}
	balances := make([]int64, balancesLength)
	for i := range balancesLength {
		balances[i] = int64(binary.LittleEndian.Uint64(data[8*(i+1) : 8*(i+2)])) // lint:ignore uintcast
	}
	return balances, nil
}

func (s *stateDiff) serialize() []byte {
	ret := make([]byte, 0) // TODO: compute a sensible default capacity.
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.targetVersion))
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.slot))
	if s.fork == nil {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, 0x1)
		ret = append(ret, s.fork.PreviousVersion...)
		ret = append(ret, s.fork.CurrentVersion...)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(s.fork.Epoch))
	}

	if s.latestBlockHeader == nil {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, 0x1)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(s.latestBlockHeader.Slot))
		ret = binary.LittleEndian.AppendUint64(ret, uint64(s.latestBlockHeader.ProposerIndex))
		ret = append(ret, s.latestBlockHeader.ParentRoot...)
		ret = append(ret, s.latestBlockHeader.StateRoot...)
		ret = append(ret, s.latestBlockHeader.BodyRoot...)
	}

	for _, r := range s.blockRoots {
		ret = append(ret, r...)
	}

	for _, r := range s.stateRoots {
		ret = append(ret, r...)
	}

	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.historicalRoots)))
	for _, r := range s.historicalRoots {
		ret = append(ret, r...)
	}

	if s.eth1Data == nil {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, 0x1)
		ret = append(ret, s.eth1Data.DepositRoot...)
		ret = binary.LittleEndian.AppendUint64(ret, s.eth1Data.DepositCount)
		ret = append(ret, s.eth1Data.BlockHash...)
	}

	if s.eth1VotesAppend {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, 0x1)
	}
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.eth1DataVotes)))
	for _, v := range s.eth1DataVotes {
		ret = append(ret, v.DepositRoot...)
		ret = binary.LittleEndian.AppendUint64(ret, v.DepositCount)
		ret = append(ret, v.BlockHash...)
	}
	ret = binary.LittleEndian.AppendUint64(ret, s.eth1DepositIndex)

	for _, r := range s.randaoMixes {
		ret = append(ret, r...)
	}

	for _, s := range s.slashings {
		ret = binary.LittleEndian.AppendUint64(ret, uint64(s))
	}

	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.previousEpochParticipation)))
	ret = append(ret, s.previousEpochParticipation...)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.currentEpochParticipation)))
	ret = append(ret, s.currentEpochParticipation...)

	ret = append(ret, s.justificationBits)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.previousJustifiedCheckpoint.Epoch))
	ret = append(ret, s.previousJustifiedCheckpoint.Root...)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.currentJustifiedCheckpoint.Epoch))
	ret = append(ret, s.currentJustifiedCheckpoint.Root...)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.finalizedCheckpoint.Epoch))
	ret = append(ret, s.finalizedCheckpoint.Root...)

	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.inactivityScores)))
	for _, s := range s.inactivityScores {
		ret = binary.LittleEndian.AppendUint64(ret, s)
	}

	if s.currentSyncCommittee == nil {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, 0x1)
		for _, pubkey := range s.currentSyncCommittee.Pubkeys {
			ret = append(ret, pubkey...)
		}
		ret = append(ret, s.currentSyncCommittee.AggregatePubkey...)
	}

	if s.nextSyncCommittee == nil {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, 0x1)
		for _, pubkey := range s.nextSyncCommittee.Pubkeys {
			ret = append(ret, pubkey...)
		}
		ret = append(ret, s.nextSyncCommittee.AggregatePubkey...)
	}

	if s.executionPayloadHeader == nil {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, 0x1)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(s.executionPayloadHeader.SizeSSZ()))
		cursor := len(ret)
		ret = append(ret, make([]byte, s.executionPayloadHeader.SizeSSZ())...)
		var err error
		_, err = s.executionPayloadHeader.MarshalSSZTo(ret[cursor:])
		if err != nil {
			// this is impossible to happen.
			logrus.WithError(err).Error("failed to marshal executionPayloadHeader")
			return nil
		}
	}

	ret = binary.LittleEndian.AppendUint64(ret, s.nextWithdrawalIndex)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.nextWithdrawalValidatorIndex))

	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.historicalSummaries)))
	for i := range s.historicalSummaries {
		ret = append(ret, s.historicalSummaries[i].BlockSummaryRoot...)
		ret = append(ret, s.historicalSummaries[i].StateSummaryRoot...)
	}

	ret = binary.LittleEndian.AppendUint64(ret, s.depositRequestsStartIndex)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.depositBalanceToConsume))
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.exitBalanceToConsume))
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.earliestExitEpoch))
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.consolidationBalanceToConsume))
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.earliestConsolidationEpoch))

	ret = binary.LittleEndian.AppendUint64(ret, s.pendingDepositIndex)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.pendingDepositDiff)))
	for i := range s.pendingDepositDiff {
		d := s.pendingDepositDiff[i]
		ret = append(ret, d.PublicKey...)
		ret = append(ret, d.WithdrawalCredentials...)
		ret = binary.LittleEndian.AppendUint64(ret, d.Amount)
		ret = append(ret, d.Signature...)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.Slot))
	}
	ret = binary.LittleEndian.AppendUint64(ret, s.pendingPartialWithdrawalsIndex)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.pendingPartialWithdrawalsDiff)))
	for i := range s.pendingPartialWithdrawalsDiff {
		d := s.pendingPartialWithdrawalsDiff[i]
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.Index))
		ret = binary.LittleEndian.AppendUint64(ret, d.Amount)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.WithdrawableEpoch))
	}
	ret = binary.LittleEndian.AppendUint64(ret, s.pendingConsolidationsIndex)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.pendingConsolidationsDiffs)))
	for i := range s.pendingConsolidationsDiffs {
		d := s.pendingConsolidationsDiffs[i]
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.SourceIndex))
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.TargetIndex))
	}
	return ret
}

func (h Hdiff) Serialize() HdiffSerialized {
	vals := make([]byte, 0) // TODO: compute a sensible default capacity.
	vals = binary.LittleEndian.AppendUint64(vals, uint64(len(h.validatorDiffs)))
	for _, v := range h.validatorDiffs {
		vals = binary.LittleEndian.AppendUint32(vals, v.index)
		if v.PublicKey == nil {
			vals = append(vals, nilMarker)
		} else {
			vals = append(vals, 0x1)
			vals = append(vals, v.PublicKey...)
		}
		if v.WithdrawalCredentials == nil {
			vals = append(vals, nilMarker)
		} else {
			vals = append(vals, 0x1)
			vals = append(vals, v.WithdrawalCredentials...)
		}
		vals = binary.LittleEndian.AppendUint64(vals, v.EffectiveBalance)
		if v.Slashed {
			vals = append(vals, 0x1)
		} else {
			vals = append(vals, nilMarker)
		}
		vals = binary.LittleEndian.AppendUint64(vals, uint64(v.ActivationEligibilityEpoch))
		vals = binary.LittleEndian.AppendUint64(vals, uint64(v.ActivationEpoch))
		vals = binary.LittleEndian.AppendUint64(vals, uint64(v.ExitEpoch))
		vals = binary.LittleEndian.AppendUint64(vals, uint64(v.WithdrawableEpoch))
	}

	bals := make([]byte, 0, 8+len(h.balancesDiff)*8)
	bals = binary.LittleEndian.AppendUint64(bals, uint64(len(h.balancesDiff)))
	for _, b := range h.balancesDiff {
		bals = binary.LittleEndian.AppendUint64(bals, uint64(b))
	}
	return HdiffSerialized{
		stateDiff:      h.stateDiff.serialize(),
		validatorDiffs: vals,
		balancesDiff:   bals,
	}
}

// diffToVals computes the difference between two BeaconStates and returns a slice of validatorDiffs.
func diffToVals(source, target state.BeaconState) ([]validatorDiff, error) {
	sVals := source.ValidatorsReadOnly()
	tVals := target.ValidatorsReadOnly()
	if len(tVals) < len(sVals) {
		return nil, errors.Errorf("target validators length %d is less than source %d", len(tVals), len(sVals))
	}
	diffs := make([]validatorDiff, 0) // TODO: compute a sensible default capacity.
	for i, s := range sVals {
		ti := tVals[i]
		if validatorsEqual(s, ti) {
			continue
		}
		d := validatorDiff{
			Slashed:                    ti.Slashed(),
			index:                      uint32(i),
			EffectiveBalance:           ti.EffectiveBalance(),
			ActivationEligibilityEpoch: ti.ActivationEligibilityEpoch(),
			ActivationEpoch:            ti.ActivationEpoch(),
			ExitEpoch:                  ti.ExitEpoch(),
			WithdrawableEpoch:          ti.WithdrawableEpoch(),
		}
		if !bytes.Equal(s.GetWithdrawalCredentials(), tVals[i].GetWithdrawalCredentials()) {
			d.WithdrawalCredentials = slices.Clone(tVals[i].GetWithdrawalCredentials())
		}
		diffs = append(diffs, d)
	}
	for i, ti := range tVals[len(sVals):] {
		pubkey := ti.PublicKey()
		diffs = append(diffs, validatorDiff{
			Slashed:                    ti.Slashed(),
			index:                      uint32(i + len(sVals)),
			PublicKey:                  pubkey[:],
			WithdrawalCredentials:      slices.Clone(ti.GetWithdrawalCredentials()),
			EffectiveBalance:           ti.EffectiveBalance(),
			ActivationEligibilityEpoch: ti.ActivationEligibilityEpoch(),
			ActivationEpoch:            ti.ActivationEpoch(),
			ExitEpoch:                  ti.ExitEpoch(),
			WithdrawableEpoch:          ti.WithdrawableEpoch(),
		})
	}
	return diffs, nil
}

// validatorsEqual compares two ReadOnlyValidator objects for equality. This function makes extra assumptions that the validators
// are of the same index and thus does not check for certain fields that cannot change, like the PublicKey.
func validatorsEqual(s, t state.ReadOnlyValidator) bool {
	if s == nil && t == nil {
		return true
	}
	if s == nil || t == nil {
		return false
	}
	if !bytes.Equal(s.GetWithdrawalCredentials(), t.GetWithdrawalCredentials()) {
		return false
	}
	if s.EffectiveBalance() != t.EffectiveBalance() {
		return false
	}
	if s.Slashed() != t.Slashed() {
		return false
	}
	if s.ActivationEligibilityEpoch() != t.ActivationEligibilityEpoch() {
		return false
	}
	if s.ActivationEpoch() != t.ActivationEpoch() {
		return false
	}
	if s.ExitEpoch() != t.ExitEpoch() {
		return false
	}
	return s.WithdrawableEpoch() == t.WithdrawableEpoch()
}

// diffToBalances computes the difference between two BeaconStates' balances.
func diffToBalances(source, target state.BeaconState) ([]int64, error) {
	sBalances := source.Balances()
	tBalances := target.Balances()
	if len(tBalances) < len(sBalances) {
		return nil, errors.Errorf("target balances length %d is less than source %d", len(tBalances), len(sBalances))
	}
	diffs := make([]int64, len(tBalances))
	for i, s := range sBalances {
		if tBalances[i] > s {
			diffs[i] = int64(tBalances[i] - s)
		} else {
			diffs[i] = -int64(s - tBalances[i])
		}
	}
	return diffs, nil
}

func diff(source, target state.BeaconState) (*Hdiff, error) {
	stateDiff, err := diffToState(source, target)
	if err != nil {
		return nil, err
	}
	validatorDiffs, err := diffToVals(source, target)
	if err != nil {
		return nil, err
	}
	balancesDiffs, err := diffToBalances(source, target)
	if err != nil {
		return nil, err
	}
	return &Hdiff{
		stateDiff:      stateDiff,
		validatorDiffs: validatorDiffs,
		balancesDiff:   balancesDiffs,
	}, nil
}

// diffToState computes the difference between two BeaconStates and returns a stateDiff object.
func diffToState(source, target state.BeaconState) (*stateDiff, error) {
	ret := &stateDiff{}
	ret.targetVersion = target.Version()
	ret.slot = target.Slot()
	if !helpers.ForksEqual(source.Fork(), target.Fork()) {
		ret.fork = target.Fork()
	}
	if !helpers.BlockHeadersEqual(source.LatestBlockHeader(), target.LatestBlockHeader()) {
		ret.latestBlockHeader = target.LatestBlockHeader()
	}
	diffBlockRoots(ret, source, target)
	diffStateRoots(ret, source, target)
	var err error
	ret.historicalRoots, err = diffHistoricalRoots(source, target)
	if err != nil {
		return nil, err
	}
	if !helpers.Eth1DataEqual(source.Eth1Data(), target.Eth1Data()) {
		ret.eth1Data = target.Eth1Data()
	}
	diffEth1DataVotes(ret, source, target)
	diffRandaoMixes(ret, source, target)
	diffSlashings(ret, source, target)
	ret.previousEpochParticipation, err = target.PreviousEpochParticipation()
	if err != nil {
		return nil, err
	}
	ret.currentEpochParticipation, err = target.CurrentEpochParticipation()
	if err != nil {
		return nil, err
	}
	ret.justificationBits = diffJustificationBits(target)
	ret.previousJustifiedCheckpoint = target.PreviousJustifiedCheckpoint()
	ret.currentJustifiedCheckpoint = target.CurrentJustifiedCheckpoint()
	ret.finalizedCheckpoint = target.FinalizedCheckpoint()
	if target.Version() < version.Altair {
		return ret, nil
	}
	ret.inactivityScores, err = target.InactivityScores()
	if err != nil {
		return nil, err
	}
	ret.currentSyncCommittee, err = target.CurrentSyncCommittee()
	if err != nil {
		return nil, err
	}
	ret.nextSyncCommittee, err = target.NextSyncCommittee()
	if err != nil {
		return nil, err
	}
	if target.Version() < version.Bellatrix {
		return ret, nil
	}
	ret.executionPayloadHeader, err = target.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, err
	}
	if target.Version() < version.Capella {
		return ret, nil
	}
	ret.nextWithdrawalIndex, err = target.NextWithdrawalIndex()
	if err != nil {
		return nil, err
	}
	ret.nextWithdrawalValidatorIndex, err = target.NextWithdrawalValidatorIndex()
	if err != nil {
		return nil, err
	}
	if err := diffHistoricalSummaries(ret, source, target); err != nil {
		return nil, err
	}
	if target.Version() < version.Electra {
		return ret, nil
	}

	if err := diffElectraFields(ret, source, target); err != nil {
		return nil, err
	}
	return ret, nil
}

func diffJustificationBits(target state.BeaconState) byte {
	j := target.JustificationBits().Bytes()
	if len(j) != 0 {
		return j[0]
	}
	return 0
}

// diffBlockRoots computes the difference between two BeaconStates' block roots.
func diffBlockRoots(diff *stateDiff, source, target state.BeaconState) {
	sRoots := source.BlockRoots()
	tRoots := target.BlockRoots()
	if len(sRoots) != len(tRoots) {
		logrus.Errorf("Block roots length mismatch: source %d, target %d", len(sRoots), len(tRoots))
		return
	}
	if len(sRoots) != fieldparams.BlockRootsLength {
		logrus.Errorf("Block roots length mismatch: source %d", len(sRoots))
		return
	}
	for i := range fieldparams.BlockRootsLength {
		if !bytes.Equal(sRoots[i], tRoots[i]) {
			diff.blockRoots[i] = tRoots[i]
		}
	}
}

// diffStateRoots computes the difference between two BeaconStates' state roots.
func diffStateRoots(diff *stateDiff, source, target state.BeaconState) {
	sRoots := source.StateRoots()
	tRoots := target.StateRoots()
	if len(sRoots) != len(tRoots) {
		logrus.Errorf("State roots length mismatch: source %d, target %d", len(sRoots), len(tRoots))
		return
	}
	if len(sRoots) != fieldparams.StateRootsLength {
		logrus.Errorf("State roots length mismatch: source %d", len(sRoots))
		return
	}
	for i := range fieldparams.StateRootsLength {
		if !bytes.Equal(sRoots[i], tRoots[i]) {
			diff.stateRoots[i] = tRoots[i]
		}
	}
}

func diffHistoricalRoots(source, target state.BeaconState) ([][]byte, error) {
	sRoots := source.HistoricalRoots()
	tRoots := target.HistoricalRoots()
	if len(tRoots) < len(sRoots) {
		return nil, errors.New("target historical roots length is less than source")
	}
	return tRoots[len(sRoots):], nil
}

func shouldAppendEth1DataVotes(sVotes, tVotes []*ethpb.Eth1Data) bool {
	if len(tVotes) < len(sVotes) {
		return false
	}
	for i, v := range sVotes {
		if !helpers.Eth1DataEqual(v, tVotes[i]) {
			return false
		}
	}
	return true
}

func diffEth1DataVotes(diff *stateDiff, source, target state.BeaconState) {
	sVotes := source.Eth1DataVotes()
	tVotes := target.Eth1DataVotes()
	if shouldAppendEth1DataVotes(sVotes, tVotes) {
		diff.eth1VotesAppend = true
		diff.eth1DataVotes = tVotes[len(sVotes):]
		return
	}
	diff.eth1VotesAppend = false
	diff.eth1DataVotes = tVotes
}

func diffRandaoMixes(diff *stateDiff, source, target state.BeaconState) {
	sMixes := source.RandaoMixes()
	tMixes := target.RandaoMixes()
	if len(sMixes) != len(tMixes) {
		logrus.Errorf("Randao mixes length mismatch: source %d, target %d", len(sMixes), len(tMixes))
		return
	}
	if len(sMixes) != fieldparams.RandaoMixesLength {
		logrus.Errorf("Randao mixes length mismatch: source %d", len(sMixes))
		return
	}
	for i := range fieldparams.RandaoMixesLength {
		if !bytes.Equal(sMixes[i], tMixes[i]) {
			diff.randaoMixes[i] = tMixes[i]
		}
	}
}

func diffSlashings(diff *stateDiff, source, target state.BeaconState) {
	sSlashings := source.Slashings()
	tSlashings := target.Slashings()
	for i := range fieldparams.SlashingsLength {
		if tSlashings[i] < sSlashings[i] {
			diff.slashings[i] = -int64(sSlashings[i] - tSlashings[i]) // lint:ignore uintcast
		} else {
			diff.slashings[i] = int64(tSlashings[i] - sSlashings[i]) // lint:ignore uintcast
		}
	}
}

func diffHistoricalSummaries(diff *stateDiff, source, target state.BeaconState) error {
	tSummaries, err := target.HistoricalSummaries()
	if err != nil {
		return err
	}
	start := 0
	if source.Version() >= version.Capella {
		sSummaries, err := source.HistoricalSummaries()
		if err != nil {
			return err
		}
		start = len(sSummaries)
	}
	if len(tSummaries) < start {
		return errors.New("target historical summaries length is less than source")
	}
	diff.historicalSummaries = tSummaries[start:]
	return nil
}

func diffElectraFields(diff *stateDiff, source, target state.BeaconState) (err error) {
	diff.depositRequestsStartIndex, err = target.DepositRequestsStartIndex()
	if err != nil {
		return
	}
	diff.depositBalanceToConsume, err = target.DepositBalanceToConsume()
	if err != nil {
		return
	}
	diff.exitBalanceToConsume, err = target.ExitBalanceToConsume()
	if err != nil {
		return
	}
	diff.earliestExitEpoch, err = target.EarliestExitEpoch()
	if err != nil {
		return
	}
	diff.consolidationBalanceToConsume, err = target.ConsolidationBalanceToConsume()
	if err != nil {
		return
	}
	diff.earliestConsolidationEpoch, err = target.EarliestConsolidationEpoch()
	if err != nil {
		return
	}
	if err := diffPEndingDeposits(diff, source, target); err != nil {
		return err
	}
	if err := diffPendingPartialWithdrawals(diff, source, target); err != nil {
		return err
	}
	return diffPendingConsolidations(diff, source, target)
}

func kmpIndex[T any](lens int, t []*T, equals func(a, b *T) bool) int {
	if lens == 0 || len(t) == 1 {
		return lens
	}

	lps := computeLPS(t, equals)
	return lens - lps[len(lps)-1]
}

func computeLPS[T any](combined []*T, equals func(a, b *T) bool) []int {
	lps := make([]int, len(combined))
	length := 0
	i := 1

	for i < len(combined) {
		if equals(combined[i], combined[length]) {
			length++
			lps[i] = length
			i++
		} else {
			if length != 0 {
				length = lps[length-1]
			} else {
				lps[i] = 0
				i++
			}
		}
	}
	return lps
}

func diffPEndingDeposits(diff *stateDiff, source, target state.BeaconState) error {
	tPendingDeposits, err := target.PendingDeposits()
	if err != nil {
		return err
	}
	tlen := len(tPendingDeposits)
	tPendingDeposits = append(tPendingDeposits, nil)
	var sPendingDeposits []*ethpb.PendingDeposit
	if source.Version() >= version.Electra {
		sPendingDeposits, err = source.PendingDeposits()
		if err != nil {
			return err
		}
	}
	tPendingDeposits = append(tPendingDeposits, sPendingDeposits...)
	index := kmpIndex(len(sPendingDeposits), tPendingDeposits, helpers.PendingDepositsEqual)

	diff.pendingDepositIndex = uint64(index)
	diff.pendingDepositDiff = tPendingDeposits[len(sPendingDeposits)-index : tlen]
	return nil
}

func diffPendingPartialWithdrawals(diff *stateDiff, source, target state.BeaconState) error {
	tPendingPartialWithdrawals, err := target.PendingPartialWithdrawals()
	if err != nil {
		return err
	}
	tlen := len(tPendingPartialWithdrawals)
	tPendingPartialWithdrawals = append(tPendingPartialWithdrawals, nil)
	var sPendingPartialWithdrawals []*ethpb.PendingPartialWithdrawal
	if source.Version() >= version.Electra {
		sPendingPartialWithdrawals, err = source.PendingPartialWithdrawals()
		if err != nil {
			return err
		}
	}
	tPendingPartialWithdrawals = append(tPendingPartialWithdrawals, sPendingPartialWithdrawals...)
	index := kmpIndex(len(sPendingPartialWithdrawals), tPendingPartialWithdrawals, helpers.PendingPartialWithdrawalsEqual)
	diff.pendingPartialWithdrawalsIndex = uint64(index)
	diff.pendingPartialWithdrawalsDiff = tPendingPartialWithdrawals[len(sPendingPartialWithdrawals)-index : tlen]
	return nil
}

func diffPendingConsolidations(diff *stateDiff, source, target state.BeaconState) error {
	tPendingConsolidations, err := target.PendingConsolidations()
	if err != nil {
		return err
	}
	tlen := len(tPendingConsolidations)
	tPendingConsolidations = append(tPendingConsolidations, nil)
	var sPendingConsolidations []*ethpb.PendingConsolidation
	if source.Version() >= version.Electra {
		sPendingConsolidations, err = source.PendingConsolidations()
		if err != nil {
			return err
		}
	}
	tPendingConsolidations = append(tPendingConsolidations, sPendingConsolidations...)
	index := kmpIndex(len(sPendingConsolidations), tPendingConsolidations, helpers.PendingConsolidationsEqual)
	diff.pendingConsolidationsIndex = uint64(index)
	diff.pendingConsolidationsDiffs = tPendingConsolidations[len(sPendingConsolidations)-index : tlen]
	return nil
}
