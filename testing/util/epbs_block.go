package util

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/encoding/ssz"
	v1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// GenerateFullBlockEpbs generates a fully valid Epbs block with the requested parameters.
// Use BlockGenConfig to declare the conditions you would like the block generated under.
// This function modifies the passed state as follows:
func GenerateFullBlockEpbs(
	bState state.BeaconState,
	privs []bls.SecretKey,
	conf *BlockGenConfig,
	slot primitives.Slot,
) (*ethpb.SignedBeaconBlockEpbs, error) {
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
	stCopy := bState.Copy()
	stCopy, err = transition.ProcessSlots(context.Background(), stCopy, slot)
	if err != nil {
		return nil, err
	}

	blockHash := indexToHash(uint64(slot))
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
	parentExecution, err := stCopy.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, err
	}
	parentHash, err := parentExecution.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	kzgRoot, err := generateKzgCommitmentsRoot(conf.NumKzgCommitmens)
	if err != nil {
		return nil, err
	}
	newExecutionPayloadHeader := &v1.ExecutionPayloadHeaderEPBS{
		ParentBlockHash:        parentHash[:],
		ParentBlockRoot:        parentRoot[:],
		BlockHash:              blockHash[:],
		BuilderIndex:           conf.BuilderIndex,
		Slot:                   slot,
		Value:                  conf.PayloadValue,
		BlobKzgCommitmentsRoot: kzgRoot[:],
	}
	newSignedExecutionPayloadHeader, err := SignExecutionPayloadHeader(bState, privs[conf.BuilderIndex], newExecutionPayloadHeader)
	if err != nil {
		return nil, err
	}

	block := &ethpb.BeaconBlockEpbs{
		Slot:          slot,
		ParentRoot:    parentRoot[:],
		ProposerIndex: idx,
		Body: &ethpb.BeaconBlockBodyEpbs{
			Eth1Data:                     eth1Data,
			RandaoReveal:                 reveal,
			ProposerSlashings:            pSlashings,
			AttesterSlashings:            aSlashings,
			Attestations:                 atts,
			VoluntaryExits:               exits,
			Deposits:                     newDeposits,
			Graffiti:                     make([]byte, fieldparams.RootLength),
			SyncAggregate:                newSyncAggregate,
			SignedExecutionPayloadHeader: newSignedExecutionPayloadHeader,
			BlsToExecutionChanges:        changes,
		},
	}

	// The fork can change after processing the state
	signature, err := BlockSignature(bState, block, privs)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block signature")
	}

	return &ethpb.SignedBeaconBlockEpbs{Block: block, Signature: signature.Marshal()}, nil
}

// SignExecutionPayloadHeader generates a valid SignedExecutionPayloadHeader
func SignExecutionPayloadHeader(
	st state.BeaconState,
	priv bls.SecretKey,
	message *v1.ExecutionPayloadHeaderEPBS,
) (*v1.SignedExecutionPayloadHeader, error) {
	c := params.BeaconConfig()
	domain, err := signing.ComputeDomain(c.DomainBeaconBuilder, c.GenesisForkVersion, st.GenesisValidatorsRoot())
	if err != nil {
		return nil, err
	}
	sr, err := signing.ComputeSigningRoot(message, domain)
	if err != nil {
		return nil, err
	}
	signature := priv.Sign(sr[:]).Marshal()
	return &v1.SignedExecutionPayloadHeader{
		Message:   message,
		Signature: signature,
	}, nil
}
func generateKzgCommitments(n uint64) ([][]byte, error) {
	kzgs := make([][]byte, n)
	for i := range kzgs {
		kzgs[i] = make([]byte, 48)
		_, err := rand.Read(kzgs[i])
		if err != nil {
			return nil, err
		}
	}
	return kzgs, nil
}

func generateKzgCommitmentsRoot(n uint64) ([32]byte, error) {
	kzgs, err := generateKzgCommitments(n)
	if err != nil {
		return [32]byte{}, err
	}
	return ssz.KzgCommitmentsRoot(kzgs)
}
