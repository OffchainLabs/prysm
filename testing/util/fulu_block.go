package util

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	v1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
)

// GenerateFullBlockFulu generates a fully valid Fulu block with the requested parameters.
// Use BlockGenConfig to declare the conditions you would like the block generated under.
// This function modifies the passed state as follows:
func GenerateFullBlockFulu(bState state.BeaconState, privs []bls.SecretKey, conf *BlockGenConfig, slot primitives.Slot) (*ethpb.SignedBeaconBlockFulu, error) {
	ctx := context.Background()
	currentSlot := bState.Slot()
	if currentSlot > slot {
		return nil, fmt.Errorf("current slot in state is larger than given slot. %d > %d", currentSlot, slot)
	}
	bState = bState.Copy()

	if conf == nil {
		conf = &BlockGenConfig{}
	}

	var err error
	var pSlashings []*ethpb.ProposerSlashing
	numToGen := conf.NumProposerSlashings
	if numToGen > 0 {
		pSlashings, err = generateProposerSlashings(bState, privs, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d proposer slashings:", numToGen)
		}
	}

	numToGen = conf.NumAttesterSlashings
	var aSlashings []*ethpb.AttesterSlashingElectra
	if numToGen > 0 {
		generated, err := generateAttesterSlashings(bState, privs, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d attester slashings:", numToGen)
		}
		aSlashings = make([]*ethpb.AttesterSlashingElectra, len(generated))
		var ok bool
		for i, s := range generated {
			aSlashings[i], ok = s.(*ethpb.AttesterSlashingElectra)
			if !ok {
				return nil, fmt.Errorf("attester slashing has the wrong type (expected %T, got %T)", &ethpb.AttesterSlashingElectra{}, s)
			}
		}
	}

	numToGen = conf.NumAttestations
	var atts []*ethpb.AttestationElectra
	if numToGen > 0 {
		generatedAtts, err := GenerateAttestations(bState, privs, numToGen, slot, false)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d attestations:", numToGen)
		}
		atts = make([]*ethpb.AttestationElectra, len(generatedAtts))
		var ok bool
		for i, a := range generatedAtts {
			atts[i], ok = a.(*ethpb.AttestationElectra)
			if !ok {
				return nil, fmt.Errorf("attestation has the wrong type (expected %T, got %T)", &ethpb.AttestationElectra{}, a)
			}
		}
	}

	numToGen = conf.NumDeposits
	var newDeposits []*ethpb.Deposit
	eth1Data := bState.Eth1Data()
	if numToGen > 0 {
		newDeposits, eth1Data, err = generateDepositsAndEth1Data(bState, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d deposits:", numToGen)
		}
	}

	numToGen = conf.NumVoluntaryExits
	var exits []*ethpb.SignedVoluntaryExit
	if numToGen > 0 {
		exits, err = generateVoluntaryExits(bState, privs, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d attester slashings:", numToGen)
		}
	}

	numToGen = conf.NumTransactions
	newTransactions := make([][]byte, numToGen)
	for i := uint64(0); i < numToGen; i++ {
		newTransactions[i] = bytesutil.Uint64ToBytesLittleEndian(i)
	}

	random, err := helpers.RandaoMix(bState, time.CurrentEpoch(bState))
	if err != nil {
		return nil, errors.Wrap(err, "could not process randao mix")
	}

	timestamp, err := slots.StartTime(bState.GenesisTime(), slot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get current timestamp")
	}

	stCopy := bState.Copy()
	stCopy, err = transition.ProcessSlots(context.Background(), stCopy, slot)
	if err != nil {
		return nil, err
	}

	newWithdrawals := make([]*v1.Withdrawal, 0)
	if conf.NumWithdrawals > 0 {
		newWithdrawals, err = generateWithdrawals(bState, privs, numToGen)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d withdrawals:", numToGen)
		}
	}

	depositRequests := make([]*v1.DepositRequest, 0)
	if conf.NumDepositRequests > 0 {
		depositRequests, err = generateDepositRequests(bState, privs, conf.NumDepositRequests)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d deposit requests:", conf.NumDepositRequests)
		}
	}

	withdrawalRequests := make([]*v1.WithdrawalRequest, 0)
	if conf.NumWithdrawalRequests > 0 {
		withdrawalRequests, err = generateWithdrawalRequests(bState, privs, conf.NumWithdrawalRequests)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d withdrawal requests:", conf.NumWithdrawalRequests)
		}
	}

	consolidationRequests := make([]*v1.ConsolidationRequest, 0)
	if conf.NumConsolidationRequests > 0 {
		consolidationRequests, err = generateConsolidationRequests(bState, privs, conf.NumConsolidationRequests)
		if err != nil {
			return nil, errors.Wrapf(err, "failed generating %d consolidation requests:", conf.NumConsolidationRequests)
		}
	}

	executionRequests := &v1.ExecutionRequests{
		Withdrawals:    withdrawalRequests,
		Deposits:       depositRequests,
		Consolidations: consolidationRequests,
	}

	parentExecution, err := stCopy.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, err
	}
	blockHash := indexToHash(uint64(slot))
	newExecutionPayloadElectra := &v1.ExecutionPayloadDeneb{
		ParentHash:    parentExecution.BlockHash(),
		FeeRecipient:  make([]byte, 20),
		StateRoot:     params.BeaconConfig().ZeroHash[:],
		ReceiptsRoot:  params.BeaconConfig().ZeroHash[:],
		LogsBloom:     make([]byte, 256),
		PrevRandao:    random,
		BlockNumber:   uint64(slot),
		ExtraData:     params.BeaconConfig().ZeroHash[:],
		BaseFeePerGas: params.BeaconConfig().ZeroHash[:],
		BlockHash:     blockHash[:],
		Timestamp:     uint64(timestamp.Unix()),
		Transactions:  newTransactions,
		Withdrawals:   newWithdrawals,
	}
	var syncCommitteeBits []byte
	currSize := new(ethpb.SyncAggregate).SyncCommitteeBits.Len()
	switch currSize {
	case 512:
		syncCommitteeBits = bitfield.NewBitvector512()
	case 32:
		syncCommitteeBits = bitfield.NewBitvector32()
	default:
		return nil, errors.New("invalid bit vector size")
	}
	newSyncAggregate := &ethpb.SyncAggregate{
		SyncCommitteeBits:      syncCommitteeBits,
		SyncCommitteeSignature: append([]byte{0xC0}, make([]byte, 95)...),
	}

	newHeader := bState.LatestBlockHeader()
	prevStateRoot, err := bState.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not hash state")
	}
	newHeader.StateRoot = prevStateRoot[:]
	parentRoot, err := newHeader.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not hash the new header")
	}

	if slot == currentSlot {
		slot = currentSlot + 1
	}

	reveal, err := RandaoReveal(stCopy, time.CurrentEpoch(stCopy), privs)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute randao reveal")
	}

	idx, err := helpers.BeaconProposerIndex(ctx, stCopy)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute beacon proposer index")
	}

	changes := make([]*ethpb.SignedBLSToExecutionChange, conf.NumBLSChanges)
	for i := uint64(0); i < conf.NumBLSChanges; i++ {
		changes[i], err = GenerateBLSToExecutionChange(bState, privs[i+1], primitives.ValidatorIndex(i))
		if err != nil {
			return nil, err
		}
	}

	blobKzgCommitments := make([][]byte, 0, conf.NumBlobKzgCommitments)
	for range conf.NumBlobKzgCommitments {
		blobKzgCommitments = append(blobKzgCommitments, make([]byte, 48))
	}

	block := &ethpb.BeaconBlockElectra{
		Slot:          slot,
		ParentRoot:    parentRoot[:],
		ProposerIndex: idx,
		Body: &ethpb.BeaconBlockBodyElectra{
			Eth1Data:              eth1Data,
			RandaoReveal:          reveal,
			ProposerSlashings:     pSlashings,
			AttesterSlashings:     aSlashings,
			Attestations:          atts,
			VoluntaryExits:        exits,
			Deposits:              newDeposits,
			Graffiti:              make([]byte, fieldparams.RootLength),
			SyncAggregate:         newSyncAggregate,
			ExecutionPayload:      newExecutionPayloadElectra,
			BlsToExecutionChanges: changes,
			ExecutionRequests:     executionRequests,
			BlobKzgCommitments:    blobKzgCommitments,
		},
	}

	// The fork can change after processing the state
	signature, err := BlockSignature(bState, block, privs)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block signature")
	}

	return &ethpb.SignedBeaconBlockFulu{Block: block, Signature: signature.Marshal()}, nil
}
