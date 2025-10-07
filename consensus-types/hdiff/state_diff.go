package hdiff

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/capella"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/deneb"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/execution"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/fulu"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/helpers"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// HdiffBytes represents the serialized difference between two beacon states.
type HdiffBytes struct {
	StateDiff      []byte
	ValidatorDiffs []byte
	BalancesDiff   []byte
}

// Diff computes the difference between two beacon states and returns it as a serialized HdiffBytes object.
func Diff(source, target state.ReadOnlyBeaconState) (HdiffBytes, error) {
	h, err := diffInternal(source, target)
	if err != nil {
		return HdiffBytes{}, err
	}
	return h.serialize(), nil
}

// ApplyDiff appplies the given serialized diff to the source beacon state and returns the resulting state.
func ApplyDiff(ctx context.Context, source state.BeaconState, diff HdiffBytes) (state.BeaconState, error) {
	hdiff, err := newHdiff(diff)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Hdiff")
	}
	if source, err = applyStateDiff(ctx, source, hdiff.stateDiff); err != nil {
		return nil, errors.Wrap(err, "failed to apply state diff")
	}
	if source, err = applyBalancesDiff(source, hdiff.balancesDiff); err != nil {
		return nil, errors.Wrap(err, "failed to apply balances diff")
	}
	if source, err = applyValidatorDiff(source, hdiff.validatorDiffs); err != nil {
		return nil, errors.Wrap(err, "failed to apply validator diff")
	}
	return source, nil
}

// stateDiff is a type that represents a difference between two different beacon states. Except from the validator registry and the balances.
// Fields marked as "override" are either zeroed out or nil when there is no diff or the full new value when there is a diff.
// Except when zero may be a valid value, in which case override means the new value (eg. justificationBits).
// Fields marked as "append only" consist of a list of items that are appended to the existing list.
type stateDiff struct {
	// genesis_time does not change.
	// genesis_validators_root does not change.
	targetVersion               int
	eth1VotesAppend             bool                                                        // Positioned here because of alignement.
	justificationBits           byte                                                        // override.
	slot                        primitives.Slot                                             // override.
	fork                        *ethpb.Fork                                                 // override.
	latestBlockHeader           *ethpb.BeaconBlockHeader                                    // override.
	blockRoots                  [fieldparams.BlockRootsLength][fieldparams.RootLength]byte  // zero or override.
	stateRoots                  [fieldparams.StateRootsLength][fieldparams.RootLength]byte  // zero or override.
	historicalRoots             [][fieldparams.RootLength]byte                              // append only.
	eth1Data                    *ethpb.Eth1Data                                             // override.
	eth1DataVotes               []*ethpb.Eth1Data                                           // append only or override.
	eth1DepositIndex            uint64                                                      // override.
	randaoMixes                 [fieldparams.RandaoMixesLength][fieldparams.RootLength]byte // zero or override.
	slashings                   [fieldparams.SlashingsLength]int64                          // algebraic diff.
	previousEpochAttestations   []*ethpb.PendingAttestation                                 // override.
	currentEpochAttestations    []*ethpb.PendingAttestation                                 // override.
	previousJustifiedCheckpoint *ethpb.Checkpoint                                           // override.
	currentJustifiedCheckpoint  *ethpb.Checkpoint                                           // override.
	finalizedCheckpoint         *ethpb.Checkpoint                                           // override.
	// Altair Fields
	previousEpochParticipation []byte               // override.
	currentEpochParticipation  []byte               // override.
	inactivityScores           []uint64             // override.
	currentSyncCommittee       *ethpb.SyncCommittee // override.
	nextSyncCommittee          *ethpb.SyncCommittee // override.
	// Bellatrix
	executionPayloadHeader interfaces.ExecutionData // override.
	// Capella
	nextWithdrawalIndex          uint64                     // override.
	nextWithdrawalValidatorIndex primitives.ValidatorIndex  // override.
	historicalSummaries          []*ethpb.HistoricalSummary // append only.
	// Electra
	depositRequestsStartIndex     uint64           // override.
	depositBalanceToConsume       primitives.Gwei  // override.
	exitBalanceToConsume          primitives.Gwei  // override.
	earliestExitEpoch             primitives.Epoch // override.
	consolidationBalanceToConsume primitives.Gwei  // override.
	earliestConsolidationEpoch    primitives.Epoch // override.

	pendingDepositIndex            uint64                            // override.
	pendingPartialWithdrawalsIndex uint64                            // override.
	pendingConsolidationsIndex     uint64                            // override.
	pendingDepositDiff             []*ethpb.PendingDeposit           // override.
	pendingPartialWithdrawalsDiff  []*ethpb.PendingPartialWithdrawal // override.
	pendingConsolidationsDiffs     []*ethpb.PendingConsolidation     // override.
	// Fulu
	proposerLookahead []uint64 // override
}

type hdiff struct {
	stateDiff      *stateDiff
	validatorDiffs []validatorDiff
	balancesDiff   []int64
}

// validatorDiff is a type that represents a difference between two validators.
type validatorDiff struct {
	Slashed                    bool             // new value (here because of alignement)
	index                      uint32           // override.
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
	notNilMarker                   = byte(1)
	forkLength                     = 2*fieldparams.VersionLength + 8  // previous_version + current_version + epoch
	blockHeaderLength              = 8 + 8 + 3*fieldparams.RootLength // slot + proposer_index + parent_root + state_root + body_root
	blockRootsLength               = fieldparams.BlockRootsLength * fieldparams.RootLength
	stateRootsLength               = fieldparams.StateRootsLength * fieldparams.RootLength
	eth1DataLength                 = 8 + 2*fieldparams.RootLength // deposit_count + deposit_root + block_hash
	randaoMixesLength              = fieldparams.RandaoMixesLength * fieldparams.RootLength
	checkpointLength               = 8 + fieldparams.RootLength // epoch + root
	syncCommitteeLength            = (fieldparams.SyncCommitteeLength + 1) * fieldparams.BLSPubkeyLength
	pendingDepositLength           = fieldparams.BLSPubkeyLength + fieldparams.RootLength + 8 + fieldparams.BLSSignatureLength + 8 // pubkey + withdrawal_credentials + amount + signature + index
	pendingPartialWithdrawalLength = 8 + 8 + 8                                                                                     // validator_index + amount + withdrawable_epoch
	pendingConsolidationLength     = 8 + 8                                                                                         // souce and target index
	proposerLookaheadLength        = 8 * 2 * fieldparams.SlotsPerEpoch
)

// newHdiff deserializes a new Hdiff object from the given serialized data.
func newHdiff(data HdiffBytes) (*hdiff, error) {
	stateDiff, err := newStateDiff(data.StateDiff)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create state diff")
	}

	validatorDiffs, err := newValidatorDiffs(data.ValidatorDiffs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create validator diffs")
	}

	balancesDiff, err := newBalancesDiff(data.BalancesDiff)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create balances diff")
	}

	return &hdiff{
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
		copy(ret.blockRoots[i][:], (*data)[i*fieldparams.RootLength:(i+1)*fieldparams.RootLength])
	}
	*data = (*data)[blockRootsLength:]
	return nil
}

func (ret *stateDiff) readStateRoots(data *[]byte) error {
	if len(*data) < stateRootsLength {
		return errors.Wrap(errDataSmall, "stateRoots")
	}
	for i := range fieldparams.StateRootsLength {
		copy(ret.stateRoots[i][:], (*data)[i*fieldparams.RootLength:(i+1)*fieldparams.RootLength])
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
	ret.historicalRoots = make([][fieldparams.RootLength]byte, historicalRootsLength)
	for i := range historicalRootsLength {
		copy(ret.historicalRoots[i][:], (*data)[i*fieldparams.RootLength:(i+1)*fieldparams.RootLength])
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
	*data = (*data)[1:]
	if len(*data) < eth1DataLength {
		return errors.Wrap(errDataSmall, "eth1Data")
	}
	ret.eth1Data = &ethpb.Eth1Data{
		DepositRoot:  slices.Clone((*data)[:fieldparams.RootLength]),
		DepositCount: binary.LittleEndian.Uint64((*data)[fieldparams.RootLength : fieldparams.RootLength+8]),
		BlockHash:    slices.Clone((*data)[fieldparams.RootLength+8 : 2*fieldparams.RootLength+8]),
	}
	*data = (*data)[eth1DataLength:]
	return nil
}

func (ret *stateDiff) readEth1DataVotes(data *[]byte) error {
	// Read eth1DataVotes.
	if len(*data) < 9 {
		return errors.Wrap(errDataSmall, "eth1DataVotes")
	}
	ret.eth1VotesAppend = ((*data)[0] == nilMarker)
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
		copy(ret.randaoMixes[i][:], (*data)[cursor:cursor+fieldparams.RootLength])
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

func readPendingAttestation(data *[]byte) (*ethpb.PendingAttestation, error) {
	if len(*data) < 8 {
		return nil, errors.Wrap(errDataSmall, "pendingAttestation")
	}
	bitsLength := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if bitsLength < 0 {
		return nil, errors.Wrap(errDataSmall, "pendingAttestation: negative bitsLength")
	}
	// Check for integer overflow: 8 + bitsLength + 144
	const fixedSize = 152 // 8 (length field) + 144 (fixed fields)
	if bitsLength > len(*data)-fixedSize {
		return nil, errors.Wrap(errDataSmall, "pendingAttestation")
	}
	pending := &ethpb.PendingAttestation{}
	pending.AggregationBits = bitfield.Bitlist(slices.Clone((*data)[8 : 8+bitsLength]))
	*data = (*data)[8+bitsLength:]
	pending.Data = &ethpb.AttestationData{}
	if err := pending.Data.UnmarshalSSZ((*data)[:128]); err != nil { // pending.Data is 128 bytes
		return nil, errors.Wrap(err, "failed to unmarshal pendingAttestation")
	}
	pending.InclusionDelay = primitives.Slot(binary.LittleEndian.Uint64((*data)[128:136]))
	pending.ProposerIndex = primitives.ValidatorIndex(binary.LittleEndian.Uint64((*data)[136:144]))
	*data = (*data)[144:]
	return pending, nil
}

func (ret *stateDiff) readPreviousEpochAttestations(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "previousEpochAttestations")
	}
	previousEpochAttestationsLength := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if previousEpochAttestationsLength < 0 {
		return errors.Wrap(errDataSmall, "previousEpochAttestations: negative length")
	}
	ret.previousEpochAttestations = make([]*ethpb.PendingAttestation, previousEpochAttestationsLength)
	(*data) = (*data)[8:]
	var err error
	for i := range previousEpochAttestationsLength {
		ret.previousEpochAttestations[i], err = readPendingAttestation(data)
		if err != nil {
			return errors.Wrap(err, "failed to read previousEpochAttestation")
		}
	}
	return nil
}

func (ret *stateDiff) readCurrentEpochAttestations(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "currentEpochAttestations")
	}
	currentEpochAttestationsLength := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if currentEpochAttestationsLength < 0 {
		return errors.Wrap(errDataSmall, "currentEpochAttestations: negative length")
	}
	ret.currentEpochAttestations = make([]*ethpb.PendingAttestation, currentEpochAttestationsLength)
	(*data) = (*data)[8:]
	var err error
	for i := range currentEpochAttestationsLength {
		ret.currentEpochAttestations[i], err = readPendingAttestation(data)
		if err != nil {
			return errors.Wrap(err, "failed to read currentEpochAttestation")
		}
	}
	return nil
}

func (ret *stateDiff) readPreviousEpochParticipation(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "previousEpochParticipation")
	}
	previousEpochParticipationLength := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	if previousEpochParticipationLength < 0 {
		return errors.Wrap(errDataSmall, "previousEpochParticipation: negative length")
	}
	if len(*data)-8 < previousEpochParticipationLength {
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
	if currentEpochParticipationLength < 0 {
		return errors.Wrap(errDataSmall, "currentEpochParticipation: negative length")
	}
	if len(*data)-8 < currentEpochParticipationLength {
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
	if inactivityScoresLength < 0 {
		return errors.Wrap(errDataSmall, "inactivityScores: negative length")
	}
	if len(*data)-8 < inactivityScoresLength*8 {
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
	if headerLength < 0 {
		return errors.Wrap(errDataSmall, "executionPayloadHeader: negative length")
	}
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
	case version.Deneb, version.Electra, version.Fulu:
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
	if historicalSummariesLength < 0 {
		return errors.Wrap(errDataSmall, "historicalSummaries: negative length")
	}
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
	if pendingDepositDiffLength < 0 {
		return errors.Wrap(errDataSmall, "pendingDeposits: negative length")
	}
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
	if pendingPartialWithdrawalsDiffLength < 0 {
		return errors.Wrap(errDataSmall, "pendingPartialWithdrawals: negative length")
	}
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
	if pendingConsolidationsDiffsLength < 0 {
		return errors.Wrap(errDataSmall, "pendingConsolidations: negative length")
	}
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

func (ret *stateDiff) readProposerLookahead(data *[]byte) error {
	if len(*data) < proposerLookaheadLength {
		return errors.Wrap(errDataSmall, "proposerLookahead data")
	}
	// Read the proposer lookahead (2 * SlotsPerEpoch uint64 values)
	numProposers := 2 * fieldparams.SlotsPerEpoch
	ret.proposerLookahead = make([]uint64, numProposers)
	for i := 0; i < numProposers; i++ {
		ret.proposerLookahead[i] = binary.LittleEndian.Uint64((*data)[i*8 : (i+1)*8])
	}
	*data = (*data)[proposerLookaheadLength:]
	return nil
}

// newStateDiff deserializes a new stateDiff object from the given data.
func newStateDiff(input []byte) (*stateDiff, error) {
	data, err := snappy.Decode(nil, input)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode snappy")
	}
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
	if ret.targetVersion == version.Phase0 {
		if err := ret.readPreviousEpochAttestations(&data); err != nil {
			return nil, err
		}
		if err := ret.readCurrentEpochAttestations(&data); err != nil {
			return nil, err
		}
	} else {
		if err := ret.readPreviousEpochParticipation(&data); err != nil {
			return nil, err
		}
		if err := ret.readCurrentEpochParticipation(&data); err != nil {
			return nil, err
		}
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
	if ret.targetVersion >= version.Fulu {
		// Proposer lookahead has fixed size and it is not added for forks previous to Fulu.
		if err := ret.readProposerLookahead(&data); err != nil {
			return nil, err
		}
	}
	if len(data) > 0 {
		return nil, errors.Errorf("data is too large, exceeded by %d bytes", len(data))
	}
	return ret, nil
}

// newValidatorDiffs deserializes a new validator diffs from the given data.
func newValidatorDiffs(input []byte) ([]validatorDiff, error) {
	data, err := snappy.Decode(nil, input)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode snappy")
	}
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
func newBalancesDiff(input []byte) ([]int64, error) {
	data, err := snappy.Decode(nil, input)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode snappy")
	}
	if len(data) < 8 {
		return nil, errors.Wrap(errDataSmall, "balancesDiff")
	}
	balancesLength := int(binary.LittleEndian.Uint64(data[:8])) // lint:ignore uintcast
	if balancesLength < 0 {
		return nil, errors.Wrap(errDataSmall, "balancesDiff: negative length")
	}
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
	ret := make([]byte, 0)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.targetVersion))
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.slot))
	if s.fork == nil {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, notNilMarker)
		ret = append(ret, s.fork.PreviousVersion...)
		ret = append(ret, s.fork.CurrentVersion...)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(s.fork.Epoch))
	}

	if s.latestBlockHeader == nil {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, notNilMarker)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(s.latestBlockHeader.Slot))
		ret = binary.LittleEndian.AppendUint64(ret, uint64(s.latestBlockHeader.ProposerIndex))
		ret = append(ret, s.latestBlockHeader.ParentRoot...)
		ret = append(ret, s.latestBlockHeader.StateRoot...)
		ret = append(ret, s.latestBlockHeader.BodyRoot...)
	}

	for _, r := range s.blockRoots {
		ret = append(ret, r[:]...)
	}

	for _, r := range s.stateRoots {
		ret = append(ret, r[:]...)
	}

	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.historicalRoots)))
	for _, r := range s.historicalRoots {
		ret = append(ret, r[:]...)
	}

	if s.eth1Data == nil {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, notNilMarker)
		ret = append(ret, s.eth1Data.DepositRoot...)
		ret = binary.LittleEndian.AppendUint64(ret, s.eth1Data.DepositCount)
		ret = append(ret, s.eth1Data.BlockHash...)
	}

	if s.eth1VotesAppend {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, notNilMarker)
	}
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.eth1DataVotes)))
	for _, v := range s.eth1DataVotes {
		ret = append(ret, v.DepositRoot...)
		ret = binary.LittleEndian.AppendUint64(ret, v.DepositCount)
		ret = append(ret, v.BlockHash...)
	}
	ret = binary.LittleEndian.AppendUint64(ret, s.eth1DepositIndex)

	for _, r := range s.randaoMixes {
		ret = append(ret, r[:]...)
	}

	for _, s := range s.slashings {
		ret = binary.LittleEndian.AppendUint64(ret, uint64(s))
	}

	if s.targetVersion == version.Phase0 {
		ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.previousEpochAttestations)))
		for _, a := range s.previousEpochAttestations {
			ret = binary.LittleEndian.AppendUint64(ret, uint64(len(a.AggregationBits)))
			ret = append(ret, a.AggregationBits...)
			var err error
			ret, err = a.Data.MarshalSSZTo(ret)
			if err != nil {
				// this is impossible to happen.
				logrus.WithError(err).Error("Failed to marshal previousEpochAttestation")
				return nil
			}
			ret = binary.LittleEndian.AppendUint64(ret, uint64(a.InclusionDelay))
			ret = binary.LittleEndian.AppendUint64(ret, uint64(a.ProposerIndex))
		}
		ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.currentEpochAttestations)))
		for _, a := range s.currentEpochAttestations {
			ret = binary.LittleEndian.AppendUint64(ret, uint64(len(a.AggregationBits)))
			ret = append(ret, a.AggregationBits...)
			var err error
			ret, err = a.Data.MarshalSSZTo(ret)
			if err != nil {
				// this is impossible to happen.
				logrus.WithError(err).Error("Failed to marshal currentEpochAttestation")
				return nil
			}
			ret = binary.LittleEndian.AppendUint64(ret, uint64(a.InclusionDelay))
			ret = binary.LittleEndian.AppendUint64(ret, uint64(a.ProposerIndex))
		}
	} else {
		ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.previousEpochParticipation)))
		ret = append(ret, s.previousEpochParticipation...)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.currentEpochParticipation)))
		ret = append(ret, s.currentEpochParticipation...)
	}

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
		ret = append(ret, notNilMarker)
		for _, pubkey := range s.currentSyncCommittee.Pubkeys {
			ret = append(ret, pubkey...)
		}
		ret = append(ret, s.currentSyncCommittee.AggregatePubkey...)
	}

	if s.nextSyncCommittee == nil {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, notNilMarker)
		for _, pubkey := range s.nextSyncCommittee.Pubkeys {
			ret = append(ret, pubkey...)
		}
		ret = append(ret, s.nextSyncCommittee.AggregatePubkey...)
	}

	if s.executionPayloadHeader == nil {
		ret = append(ret, nilMarker)
	} else {
		ret = append(ret, notNilMarker)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(s.executionPayloadHeader.SizeSSZ()))
		var err error
		ret, err = s.executionPayloadHeader.MarshalSSZTo(ret)
		if err != nil {
			// this is impossible to happen.
			logrus.WithError(err).Error("Failed to marshal executionPayloadHeader")
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
	for _, d := range s.pendingDepositDiff {
		ret = append(ret, d.PublicKey...)
		ret = append(ret, d.WithdrawalCredentials...)
		ret = binary.LittleEndian.AppendUint64(ret, d.Amount)
		ret = append(ret, d.Signature...)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.Slot))
	}
	ret = binary.LittleEndian.AppendUint64(ret, s.pendingPartialWithdrawalsIndex)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.pendingPartialWithdrawalsDiff)))
	for _, d := range s.pendingPartialWithdrawalsDiff {
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.Index))
		ret = binary.LittleEndian.AppendUint64(ret, d.Amount)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.WithdrawableEpoch))
	}
	ret = binary.LittleEndian.AppendUint64(ret, s.pendingConsolidationsIndex)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.pendingConsolidationsDiffs)))
	for _, d := range s.pendingConsolidationsDiffs {
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.SourceIndex))
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.TargetIndex))
	}
	// Fulu: Proposer lookahead (override strategy - always fixed size)
	if s.targetVersion >= version.Fulu {
		for _, proposer := range s.proposerLookahead {
			ret = binary.LittleEndian.AppendUint64(ret, proposer)
		}
	}
	return ret
}

func (h *hdiff) serialize() HdiffBytes {
	vals := make([]byte, 0)
	vals = binary.LittleEndian.AppendUint64(vals, uint64(len(h.validatorDiffs)))
	for _, v := range h.validatorDiffs {
		vals = binary.LittleEndian.AppendUint32(vals, v.index)
		if v.PublicKey == nil {
			vals = append(vals, nilMarker)
		} else {
			vals = append(vals, notNilMarker)
			vals = append(vals, v.PublicKey...)
		}
		if v.WithdrawalCredentials == nil {
			vals = append(vals, nilMarker)
		} else {
			vals = append(vals, notNilMarker)
			vals = append(vals, v.WithdrawalCredentials...)
		}
		vals = binary.LittleEndian.AppendUint64(vals, v.EffectiveBalance)
		if v.Slashed {
			vals = append(vals, notNilMarker)
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
	return HdiffBytes{
		StateDiff:      snappy.Encode(nil, h.stateDiff.serialize()),
		ValidatorDiffs: snappy.Encode(nil, vals),
		BalancesDiff:   snappy.Encode(nil, bals),
	}
}

// diffToVals computes the difference between two BeaconStates and returns a slice of validatorDiffs.
func diffToVals(source, target state.ReadOnlyBeaconState) ([]validatorDiff, error) {
	sVals := source.ValidatorsReadOnly()
	tVals := target.ValidatorsReadOnly()
	if len(tVals) < len(sVals) {
		return nil, errors.Errorf("target validators length %d is less than source %d", len(tVals), len(sVals))
	}
	diffs := make([]validatorDiff, 0)
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
func diffToBalances(source, target state.ReadOnlyBeaconState) ([]int64, error) {
	sBalances := source.Balances()
	tBalances := target.Balances()
	if len(tBalances) < len(sBalances) {
		return nil, errors.Errorf("target balances length %d is less than source %d", len(tBalances), len(sBalances))
	}
	diffs := make([]int64, len(tBalances))
	for i, s := range sBalances {
		if tBalances[i] >= s {
			diffs[i] = int64(tBalances[i] - s)
		} else {
			diffs[i] = -int64(s - tBalances[i])
		}
	}
	for i, t := range tBalances[len(sBalances):] {
		diffs[i+len(sBalances)] = int64(t) // lint:ignore uintcast
	}
	return diffs, nil
}

func diffInternal(source, target state.ReadOnlyBeaconState) (*hdiff, error) {
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
	return &hdiff{
		stateDiff:      stateDiff,
		validatorDiffs: validatorDiffs,
		balancesDiff:   balancesDiffs,
	}, nil
}

// diffToState computes the difference between two BeaconStates and returns a stateDiff object.
func diffToState(source, target state.ReadOnlyBeaconState) (*stateDiff, error) {
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
	ret.eth1DepositIndex = target.Eth1DepositIndex()
	diffRandaoMixes(ret, source, target)
	diffSlashings(ret, source, target)
	if target.Version() < version.Altair {
		ret.previousEpochAttestations, err = target.PreviousEpochAttestations()
		if err != nil {
			return nil, err
		}
		ret.currentEpochAttestations, err = target.CurrentEpochAttestations()
		if err != nil {
			return nil, err
		}
	} else {
		ret.previousEpochParticipation, err = target.PreviousEpochParticipation()
		if err != nil {
			return nil, err
		}
		ret.currentEpochParticipation, err = target.CurrentEpochParticipation()
		if err != nil {
			return nil, err
		}
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
	if target.Version() < version.Fulu {
		return ret, nil
	}

	// Fulu: Proposer lookahead (override strategy - always use target's lookahead)
	proposerLookahead, err := target.ProposerLookahead()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get proposer lookahead from Fulu target state")
	}
	// Convert []primitives.ValidatorIndex to []uint64
	ret.proposerLookahead = make([]uint64, len(proposerLookahead))
	for i, idx := range proposerLookahead {
		ret.proposerLookahead[i] = uint64(idx)
	}

	return ret, nil
}

func diffJustificationBits(target state.ReadOnlyBeaconState) byte {
	j := target.JustificationBits().Bytes()
	if len(j) != 0 {
		return j[0]
	}
	return 0
}

// diffBlockRoots computes the difference between two BeaconStates' block roots.
func diffBlockRoots(diff *stateDiff, source, target state.ReadOnlyBeaconState) {
	sRoots := source.BlockRoots()
	tRoots := target.BlockRoots()
	if len(sRoots) != len(tRoots) {
		logrus.Errorf("Block roots length mismatch: source %d, target %d", len(sRoots), len(tRoots))
		return
	}
	if len(sRoots) != fieldparams.BlockRootsLength {
		logrus.Errorf("Block roots length mismatch: expected: %d, source %d", fieldparams.BlockRootsLength, len(sRoots))
		return
	}
	for i := range fieldparams.BlockRootsLength {
		if !bytes.Equal(sRoots[i], tRoots[i]) {
			// This copy can be avoided if we use [][]byte instead of [][32]byte.
			copy(diff.blockRoots[i][:], tRoots[i])
		}
	}
}

// diffStateRoots computes the difference between two BeaconStates' state roots.
func diffStateRoots(diff *stateDiff, source, target state.ReadOnlyBeaconState) {
	sRoots := source.StateRoots()
	tRoots := target.StateRoots()
	if len(sRoots) != len(tRoots) {
		logrus.Errorf("State roots length mismatch: source %d, target %d", len(sRoots), len(tRoots))
		return
	}
	if len(sRoots) != fieldparams.StateRootsLength {
		logrus.Errorf("State roots length mismatch: expected %d, source %d", fieldparams.StateRootsLength, len(sRoots))
		return
	}
	for i := range fieldparams.StateRootsLength {
		if !bytes.Equal(sRoots[i], tRoots[i]) {
			// This copy can be avoided if we use [][]byte instead of [][32]byte.
			copy(diff.stateRoots[i][:], tRoots[i])
		}
	}
}

func diffHistoricalRoots(source, target state.ReadOnlyBeaconState) ([][fieldparams.RootLength]byte, error) {
	sRoots := source.HistoricalRoots()
	tRoots := target.HistoricalRoots()
	if len(tRoots) < len(sRoots) {
		return nil, errors.New("target historical roots length is less than source")
	}
	ret := make([][fieldparams.RootLength]byte, len(tRoots)-len(sRoots))
	// We assume the states are consistent.
	for i, root := range tRoots[len(sRoots):] {
		// This copy can be avoided if we use [][]byte instead of [][32]byte.
		copy(ret[i][:], root)
	}
	return ret, nil
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

func diffEth1DataVotes(diff *stateDiff, source, target state.ReadOnlyBeaconState) {
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

func diffRandaoMixes(diff *stateDiff, source, target state.ReadOnlyBeaconState) {
	sMixes := source.RandaoMixes()
	tMixes := target.RandaoMixes()
	if len(sMixes) != len(tMixes) {
		logrus.Errorf("Randao mixes length mismatch: source %d, target %d", len(sMixes), len(tMixes))
		return
	}
	if len(sMixes) != fieldparams.RandaoMixesLength {
		logrus.Errorf("Randao mixes length mismatch: expected %d, source %d", fieldparams.RandaoMixesLength, len(sMixes))
		return
	}
	for i := range fieldparams.RandaoMixesLength {
		if !bytes.Equal(sMixes[i], tMixes[i]) {
			// This copy can be avoided if we use [][]byte instead of [][32]byte.
			copy(diff.randaoMixes[i][:], tMixes[i])
		}
	}
}

func diffSlashings(diff *stateDiff, source, target state.ReadOnlyBeaconState) {
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

func diffHistoricalSummaries(diff *stateDiff, source, target state.ReadOnlyBeaconState) error {
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
	diff.historicalSummaries = make([]*ethpb.HistoricalSummary, len(tSummaries)-start)
	for i, summary := range tSummaries[start:] {
		diff.historicalSummaries[i] = &ethpb.HistoricalSummary{
			BlockSummaryRoot: slices.Clone(summary.BlockSummaryRoot),
			StateSummaryRoot: slices.Clone(summary.StateSummaryRoot),
		}
	}
	return nil
}

func diffElectraFields(diff *stateDiff, source, target state.ReadOnlyBeaconState) (err error) {
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
	if err := diffPendingDeposits(diff, source, target); err != nil {
		return err
	}
	if err := diffPendingPartialWithdrawals(diff, source, target); err != nil {
		return err
	}
	return diffPendingConsolidations(diff, source, target)
}

// kmpIndex returns the index of the first occurrence of the pattern in the slice using the Knuth-Morris-Pratt algorithm.
func kmpIndex[T any](lens int, t []*T, equals func(a, b *T) bool) int {
	if lens == 0 || len(t) <= 1 {
		return lens
	}

	lps := computeLPS(t, equals)
	result := lens - lps[len(lps)-1]
	// Clamp result to valid range [0, lens] to handle cases where
	// the LPS value exceeds lens due to repetitive patterns
	if result < 0 {
		return 0
	}
	return result
}

// computeLPS computes the longest prefix-suffix (LPS) array for the given pattern.
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

func diffPendingDeposits(diff *stateDiff, source, target state.ReadOnlyBeaconState) error {
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
	diff.pendingDepositDiff = make([]*ethpb.PendingDeposit, tlen+index-len(sPendingDeposits))
	for i, d := range tPendingDeposits[len(sPendingDeposits)-index : tlen] {
		diff.pendingDepositDiff[i] = &ethpb.PendingDeposit{
			PublicKey:             slices.Clone(d.PublicKey),
			WithdrawalCredentials: slices.Clone(d.WithdrawalCredentials),
			Amount:                d.Amount,
			Signature:             slices.Clone(d.Signature),
			Slot:                  d.Slot,
		}
	}
	return nil
}

func diffPendingPartialWithdrawals(diff *stateDiff, source, target state.ReadOnlyBeaconState) error {
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
	diff.pendingPartialWithdrawalsDiff = make([]*ethpb.PendingPartialWithdrawal, tlen+index-len(sPendingPartialWithdrawals))
	for i, d := range tPendingPartialWithdrawals[len(sPendingPartialWithdrawals)-index : tlen] {
		diff.pendingPartialWithdrawalsDiff[i] = &ethpb.PendingPartialWithdrawal{
			Index:             d.Index,
			Amount:            d.Amount,
			WithdrawableEpoch: d.WithdrawableEpoch,
		}
	}
	return nil
}

func diffPendingConsolidations(diff *stateDiff, source, target state.ReadOnlyBeaconState) error {
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
	diff.pendingConsolidationsDiffs = make([]*ethpb.PendingConsolidation, tlen+index-len(sPendingConsolidations))
	for i, d := range tPendingConsolidations[len(sPendingConsolidations)-index : tlen] {
		diff.pendingConsolidationsDiffs[i] = &ethpb.PendingConsolidation{
			SourceIndex: d.SourceIndex,
			TargetIndex: d.TargetIndex,
		}
	}
	return nil
}

// applyValidatorDiff applies the validator diff to the source state in place.
func applyValidatorDiff(source state.BeaconState, diff []validatorDiff) (state.BeaconState, error) {
	sVals := source.Validators()
	if len(sVals) < len(diff) {
		return nil, errors.Errorf("target validators length %d is less than source %d", len(diff), len(sVals))
	}
	for _, d := range diff {
		if d.index > uint32(len(sVals)) {
			return nil, errors.Errorf("validator index %d is greater than length %d", d.index, len(sVals))
		}
		if d.index == uint32(len(sVals)) {
			// A valid diff should never have an index greater than the length of the source validators.
			sVals = append(sVals, &ethpb.Validator{})
		}
		if d.PublicKey != nil {
			sVals[d.index].PublicKey = slices.Clone(d.PublicKey)
		}
		if d.WithdrawalCredentials != nil {
			sVals[d.index].WithdrawalCredentials = slices.Clone(d.WithdrawalCredentials)
		}
		sVals[d.index].EffectiveBalance = d.EffectiveBalance
		sVals[d.index].Slashed = d.Slashed
		sVals[d.index].ActivationEligibilityEpoch = d.ActivationEligibilityEpoch
		sVals[d.index].ActivationEpoch = d.ActivationEpoch
		sVals[d.index].ExitEpoch = d.ExitEpoch
		sVals[d.index].WithdrawableEpoch = d.WithdrawableEpoch
	}
	if err := source.SetValidators(sVals); err != nil {
		return nil, errors.Wrap(err, "failed to set validators")
	}
	return source, nil
}

// applyBalancesDiff applies the balances diff to the source state in place.
func applyBalancesDiff(source state.BeaconState, diff []int64) (state.BeaconState, error) {
	sBalances := source.Balances()
	if len(diff) < len(sBalances) {
		return nil, errors.Errorf("target balances length %d is less than source %d", len(diff), len(sBalances))
	}
	sBalances = append(sBalances, make([]uint64, len(diff)-len(sBalances))...)
	for i, t := range diff {
		if t >= 0 {
			sBalances[i] += uint64(t)
		} else {
			sBalances[i] -= uint64(-t)
		}
	}
	if err := source.SetBalances(sBalances); err != nil {
		return nil, errors.Wrap(err, "failed to set balances")
	}
	return source, nil
}

// applyStateDiff applies the given diff to the source state in place.
func applyStateDiff(ctx context.Context, source state.BeaconState, diff *stateDiff) (state.BeaconState, error) {
	var err error
	if source, err = updateToVersion(ctx, source, diff.targetVersion); err != nil {
		return nil, errors.Wrap(err, "failed to update state to target version")
	}
	if err := source.SetSlot(diff.slot); err != nil {
		return nil, errors.Wrap(err, "failed to set slot")
	}
	if diff.fork != nil {
		if err := source.SetFork(diff.fork); err != nil {
			return nil, errors.Wrap(err, "failed to set fork")
		}
	}
	if diff.latestBlockHeader != nil {
		if err := source.SetLatestBlockHeader(diff.latestBlockHeader); err != nil {
			return nil, errors.Wrap(err, "failed to set latest block header")
		}
	}
	if err := applyBlockRootsDiff(source, diff); err != nil {
		return nil, errors.Wrap(err, "failed to apply block roots diff")
	}
	if err := applyStateRootsDiff(source, diff); err != nil {
		return nil, errors.Wrap(err, "failed to apply state roots diff")
	}
	if err := applyHistoricalRootsDiff(source, diff); err != nil {
		return nil, errors.Wrap(err, "failed to apply historical roots diff")
	}
	if diff.eth1Data != nil {
		if err := source.SetEth1Data(diff.eth1Data); err != nil {
			return nil, errors.Wrap(err, "failed to set eth1 data")
		}
	}
	if err := applyEth1DataVotesDiff(source, diff); err != nil {
		return nil, errors.Wrap(err, "failed to apply eth1 data votes diff")
	}
	if err := source.SetEth1DepositIndex(diff.eth1DepositIndex); err != nil {
		return nil, errors.Wrap(err, "failed to set eth1 deposit index")
	}
	if err := applyRandaoMixesDiff(source, diff); err != nil {
		return nil, errors.Wrap(err, "failed to apply randao mixes diff")
	}
	if err := applySlashingsDiff(source, diff); err != nil {
		return nil, errors.Wrap(err, "failed to apply slashings diff")
	}
	if diff.targetVersion == version.Phase0 {
		if err := source.SetPreviousEpochAttestations(diff.previousEpochAttestations); err != nil {
			return nil, errors.Wrap(err, "failed to set previous epoch attestations")
		}
		if err := source.SetCurrentEpochAttestations(diff.currentEpochAttestations); err != nil {
			return nil, errors.Wrap(err, "failed to set current epoch attestations")
		}
	} else {
		if err := source.SetPreviousParticipationBits(diff.previousEpochParticipation); err != nil {
			return nil, errors.Wrap(err, "failed to set previous epoch participation")
		}
		if err := source.SetCurrentParticipationBits(diff.currentEpochParticipation); err != nil {
			return nil, errors.Wrap(err, "failed to set current epoch participation")
		}
	}
	if err := source.SetJustificationBits([]byte{diff.justificationBits}); err != nil {
		return nil, errors.Wrap(err, "failed to set justification bits")
	}
	if diff.previousJustifiedCheckpoint != nil {
		if err := source.SetPreviousJustifiedCheckpoint(diff.previousJustifiedCheckpoint); err != nil {
			return nil, errors.Wrap(err, "failed to set previous justified checkpoint")
		}
	}
	if diff.currentJustifiedCheckpoint != nil {
		if err := source.SetCurrentJustifiedCheckpoint(diff.currentJustifiedCheckpoint); err != nil {
			return nil, errors.Wrap(err, "failed to set current justified checkpoint")
		}
	}
	if diff.finalizedCheckpoint != nil {
		if err := source.SetFinalizedCheckpoint(diff.finalizedCheckpoint); err != nil {
			return nil, errors.Wrap(err, "failed to set finalized checkpoint")
		}
	}
	if diff.targetVersion < version.Altair {
		return source, nil
	}
	if err := source.SetInactivityScores(diff.inactivityScores); err != nil {
		return nil, errors.Wrap(err, "failed to set inactivity scores")
	}
	if diff.currentSyncCommittee != nil {
		if err := source.SetCurrentSyncCommittee(diff.currentSyncCommittee); err != nil {
			return nil, errors.Wrap(err, "failed to set current sync committee")
		}
	}
	if diff.nextSyncCommittee != nil {
		if err := source.SetNextSyncCommittee(diff.nextSyncCommittee); err != nil {
			return nil, errors.Wrap(err, "failed to set next sync committee")
		}
	}
	if diff.targetVersion < version.Bellatrix {
		return source, nil
	}
	if diff.executionPayloadHeader != nil {
		if err := source.SetLatestExecutionPayloadHeader(diff.executionPayloadHeader); err != nil {
			return nil, errors.Wrap(err, "failed to set latest execution payload header")
		}
	}
	if diff.targetVersion < version.Capella {
		return source, nil
	}
	if err := source.SetNextWithdrawalIndex(diff.nextWithdrawalIndex); err != nil {
		return nil, errors.Wrap(err, "failed to set next withdrawal index")
	}
	if err := source.SetNextWithdrawalValidatorIndex(diff.nextWithdrawalValidatorIndex); err != nil {
		return nil, errors.Wrap(err, "failed to set next withdrawal validator index")
	}
	if err := applyHistoricalSummariesDiff(source, diff); err != nil {
		return nil, errors.Wrap(err, "failed to apply historical summaries diff")
	}
	if diff.targetVersion < version.Electra {
		return source, nil
	}
	if err := source.SetDepositRequestsStartIndex(diff.depositRequestsStartIndex); err != nil {
		return nil, errors.Wrap(err, "failed to set deposit requests start index")
	}
	if err := source.SetDepositBalanceToConsume(diff.depositBalanceToConsume); err != nil {
		return nil, errors.Wrap(err, "failed to set deposit balance to consume")
	}
	if err := source.SetExitBalanceToConsume(diff.exitBalanceToConsume); err != nil {
		return nil, errors.Wrap(err, "failed to set exit balance to consume")
	}
	if err := source.SetEarliestExitEpoch(diff.earliestExitEpoch); err != nil {
		return nil, errors.Wrap(err, "failed to set earliest exit epoch")
	}
	if err := source.SetConsolidationBalanceToConsume(diff.consolidationBalanceToConsume); err != nil {
		return nil, errors.Wrap(err, "failed to set consolidation balance to consume")
	}
	if err := source.SetEarliestConsolidationEpoch(diff.earliestConsolidationEpoch); err != nil {
		return nil, errors.Wrap(err, "failed to set earliest consolidation epoch")
	}
	if err := applyPendingDepositsDiff(source, diff); err != nil {
		return nil, errors.Wrap(err, "failed to apply pending deposits diff")
	}
	if err := applyPendingPartialWithdrawalsDiff(source, diff); err != nil {
		return nil, errors.Wrap(err, "failed to apply pending partial withdrawals diff")
	}
	if err := applyPendingConsolidationsDiff(source, diff); err != nil {
		return nil, errors.Wrap(err, "failed to apply pending consolidations diff")
	}
	if diff.targetVersion < version.Fulu {
		return source, nil
	}
	if err := applyProposerLookaheadDiff(source, diff); err != nil {
		return nil, errors.Wrap(err, "failed to apply proposer lookahead diff")
	}
	return source, nil
}

// applyPendingDepositsDiff applies the pending deposits diff to the source state in place.
func applyPendingDepositsDiff(source state.BeaconState, diff *stateDiff) error {
	sPendingDeposits, err := source.PendingDeposits()
	if err != nil {
		return errors.Wrap(err, "failed to get pending deposits")
	}
	sPendingDeposits = sPendingDeposits[int(diff.pendingDepositIndex):]
	for _, t := range diff.pendingDepositDiff {
		sPendingDeposits = append(sPendingDeposits, &ethpb.PendingDeposit{
			PublicKey:             slices.Clone(t.PublicKey),
			WithdrawalCredentials: slices.Clone(t.WithdrawalCredentials),
			Amount:                t.Amount,
			Signature:             slices.Clone(t.Signature),
			Slot:                  t.Slot,
		})
	}
	return source.SetPendingDeposits(sPendingDeposits)
}

// applyPendingPartialWithdrawalsDiff applies the pending partial withdrawals diff to the source state in place.
func applyPendingPartialWithdrawalsDiff(source state.BeaconState, diff *stateDiff) error {
	sPendingPartialWithdrawals, err := source.PendingPartialWithdrawals()
	if err != nil {
		return errors.Wrap(err, "failed to get pending partial withdrawals")
	}
	sPendingPartialWithdrawals = sPendingPartialWithdrawals[int(diff.pendingPartialWithdrawalsIndex):]
	for _, t := range diff.pendingPartialWithdrawalsDiff {
		sPendingPartialWithdrawals = append(sPendingPartialWithdrawals, &ethpb.PendingPartialWithdrawal{
			Index:             t.Index,
			Amount:            t.Amount,
			WithdrawableEpoch: t.WithdrawableEpoch,
		})
	}
	return source.SetPendingPartialWithdrawals(sPendingPartialWithdrawals)
}

// applyPendingConsolidationsDiff applies the pending consolidations diff to the source state in place.
func applyPendingConsolidationsDiff(source state.BeaconState, diff *stateDiff) error {
	sPendingConsolidations, err := source.PendingConsolidations()
	if err != nil {
		return errors.Wrap(err, "failed to get pending consolidations")
	}
	sPendingConsolidations = sPendingConsolidations[int(diff.pendingConsolidationsIndex):]
	for _, t := range diff.pendingConsolidationsDiffs {
		sPendingConsolidations = append(sPendingConsolidations, &ethpb.PendingConsolidation{
			SourceIndex: t.SourceIndex,
			TargetIndex: t.TargetIndex,
		})
	}
	return source.SetPendingConsolidations(sPendingConsolidations)
}

// applyHistoricalSummariesDiff applies the historical summaries diff to the source state in place.
func applyHistoricalSummariesDiff(source state.BeaconState, diff *stateDiff) error {
	tSummaries := diff.historicalSummaries
	for _, t := range tSummaries {
		if err := source.AppendHistoricalSummaries(&ethpb.HistoricalSummary{
			BlockSummaryRoot: slices.Clone(t.BlockSummaryRoot),
			StateSummaryRoot: slices.Clone(t.StateSummaryRoot),
		}); err != nil {
			return errors.Wrap(err, "failed to append historical summary")
		}
	}
	return nil
}

// applySlashingsDiff applies the slashings diff to the source state in place.
func applySlashingsDiff(source state.BeaconState, diff *stateDiff) error {
	sSlashings := source.Slashings()
	tSlashings := diff.slashings
	if len(sSlashings) != len(tSlashings) {
		return errors.Errorf("slashings length mismatch source %d, target %d", len(sSlashings), len(tSlashings))
	}
	if len(sSlashings) != fieldparams.SlashingsLength {
		return errors.Errorf("slashings length mismatch expected %d, source %d", fieldparams.SlashingsLength, len(sSlashings))
	}
	for i, t := range tSlashings {
		if t > 0 {
			sSlashings[i] += uint64(t)
		} else {
			sSlashings[i] -= uint64(-t)
		}
	}
	return source.SetSlashings(sSlashings)
}

// applyRandaoMixesDiff applies the randao mixes diff to the source state in place.
func applyRandaoMixesDiff(source state.BeaconState, diff *stateDiff) error {
	sMixes := source.RandaoMixes()
	tMixes := diff.randaoMixes
	if len(sMixes) != len(tMixes) {
		return errors.Errorf("randao mixes length mismatch, source %d, target %d", len(sMixes), len(tMixes))
	}
	if len(sMixes) != fieldparams.RandaoMixesLength {
		return errors.Errorf("randao mixes length mismatch, expected %d, source %d", fieldparams.RandaoMixesLength, len(sMixes))
	}
	for i := range fieldparams.RandaoMixesLength {
		if tMixes[i] != [fieldparams.RootLength]byte{} {
			sMixes[i] = slices.Clone(tMixes[i][:])
		}
	}
	return source.SetRandaoMixes(sMixes)
}

// applyEth1DataVotesDiff applies the eth1 data votes diff to the source state in place.
func applyEth1DataVotesDiff(source state.BeaconState, diff *stateDiff) error {
	sVotes := source.Eth1DataVotes()
	tVotes := diff.eth1DataVotes
	if diff.eth1VotesAppend {
		sVotes = append(sVotes, tVotes...)
		return source.SetEth1DataVotes(sVotes)
	}
	return source.SetEth1DataVotes(tVotes)
}

// applyHistoricalRootsDiff applies the historical roots diff to the source state in place.
func applyHistoricalRootsDiff(source state.BeaconState, diff *stateDiff) error {
	sRoots := source.HistoricalRoots()
	tRoots := diff.historicalRoots
	for _, t := range tRoots {
		sRoots = append(sRoots, t[:])
	}
	return source.SetHistoricalRoots(sRoots)
}

// applyStateRootsDiff applies the state roots diff to the source state in place.
func applyStateRootsDiff(source state.BeaconState, diff *stateDiff) error {
	sRoots := source.StateRoots()
	tRoots := diff.stateRoots
	if len(sRoots) != len(tRoots) {
		return errors.Errorf("state roots length mismatch, source %d, target %d", len(sRoots), len(tRoots))
	}
	if len(sRoots) != fieldparams.StateRootsLength {
		return errors.Errorf("state roots length mismatch, expected %d, source %d", fieldparams.StateRootsLength, len(sRoots))
	}
	for i := range fieldparams.StateRootsLength {
		if tRoots[i] != [fieldparams.RootLength]byte{} {
			sRoots[i] = slices.Clone(tRoots[i][:])
		}
	}
	return source.SetStateRoots(sRoots)
}

// applyBlockRootsDiff applies the block roots diff to the source state in place.
func applyBlockRootsDiff(source state.BeaconState, diff *stateDiff) error {
	sRoots := source.BlockRoots()
	tRoots := diff.blockRoots
	if len(sRoots) != len(tRoots) {
		return errors.Errorf("block roots length mismatch, source %d, target %d", len(sRoots), len(tRoots))
	}
	if len(sRoots) != fieldparams.BlockRootsLength {
		return errors.Errorf("block roots length mismatch, expected %d, source %d", fieldparams.BlockRootsLength, len(sRoots))
	}
	for i := range fieldparams.BlockRootsLength {
		if tRoots[i] != [fieldparams.RootLength]byte{} {
			sRoots[i] = slices.Clone(tRoots[i][:])
		}
	}
	return source.SetBlockRoots(sRoots)
}

// applyProposerLookaheadDiff applies the proposer lookahead diff to the source state in place.
func applyProposerLookaheadDiff(source state.BeaconState, diff *stateDiff) error {
	// Fulu: Proposer lookahead (override strategy - always use target's lookahead)
	proposerIndices := make([]primitives.ValidatorIndex, len(diff.proposerLookahead))
	for i, idx := range diff.proposerLookahead {
		proposerIndices[i] = primitives.ValidatorIndex(idx)
	}
	return source.SetProposerLookahead(proposerIndices)
}

// updateToVersion updates the state to the given version in place.
func updateToVersion(ctx context.Context, source state.BeaconState, target int) (ret state.BeaconState, err error) {
	if source.Version() == target {
		return source, nil
	}
	if source.Version() > target {
		return nil, errors.Errorf("cannot downgrade state from %s to %s", version.String(source.Version()), version.String(target))
	}
	switch source.Version() {
	case version.Phase0:
		ret, err = altair.ConvertToAltair(source)
	case version.Altair:
		ret, err = execution.UpgradeToBellatrix(source)
	case version.Bellatrix:
		ret, err = capella.UpgradeToCapella(source)
	case version.Capella:
		ret, err = deneb.UpgradeToDeneb(source)
	case version.Deneb:
		ret, err = electra.ConvertToElectra(source)
	case version.Electra:
		ret, err = fulu.ConvertToFulu(source)
	default:
		return nil, errors.Errorf("unsupported version %s", version.String(source.Version()))
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to upgrade state")
	}
	return updateToVersion(ctx, ret, target)
}
