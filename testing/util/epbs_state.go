package util

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stateutil"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/ssz"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// emptyGenesisStateEpbs returns an empty genesis state in ePBS format.
func emptyGenesisStateEpbs() (state.BeaconState, error) {
	st := &ethpb.BeaconStateEPBS{
		// Misc fields.
		Slot: 0,
		Fork: &ethpb.Fork{
			PreviousVersion: params.BeaconConfig().BellatrixForkVersion,
			CurrentVersion:  params.BeaconConfig().DenebForkVersion,
			Epoch:           0,
		},
		// Validator registry fields.
		Validators:       []*ethpb.Validator{},
		Balances:         []uint64{},
		InactivityScores: []uint64{},

		JustificationBits:          []byte{0},
		HistoricalRoots:            [][]byte{},
		CurrentEpochParticipation:  []byte{},
		PreviousEpochParticipation: []byte{},

		// Eth1 data.
		Eth1Data:         &ethpb.Eth1Data{},
		Eth1DataVotes:    []*ethpb.Eth1Data{},
		Eth1DepositIndex: 0,

		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderEPBS{},

		DepositBalanceToConsume:       primitives.Gwei(0),
		ExitBalanceToConsume:          primitives.Gwei(0),
		ConsolidationBalanceToConsume: primitives.Gwei(0),
	}
	return state_native.InitializeFromProtoEpbs(st)
}

// genesisBeaconStateEpbs returns the genesis beacon state.
func genesisBeaconStateEpbs(ctx context.Context, deposits []*ethpb.Deposit, genesisTime uint64, eth1Data *ethpb.Eth1Data) (state.BeaconState, error) {
	st, err := emptyGenesisStateEpbs()
	if err != nil {
		return nil, err
	}

	// Process initial deposits.
	st, err = helpers.UpdateGenesisEth1Data(st, deposits, eth1Data)
	if err != nil {
		return nil, err
	}

	st, err = processPreGenesisDeposits(ctx, st, deposits)
	if err != nil {
		return nil, errors.Wrap(err, "could not process validator deposits")
	}

	return buildGenesisBeaconStateEpbs(genesisTime, st, st.Eth1Data())
}

func buildGenesisBeaconStateEpbs(genesisTime uint64, preState state.BeaconState, eth1Data *ethpb.Eth1Data) (state.BeaconState, error) {
	if eth1Data == nil {
		return nil, errors.New("no eth1data provided for genesis state")
	}

	randaoMixes := make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector)
	for i := 0; i < len(randaoMixes); i++ {
		h := make([]byte, 32)
		copy(h, eth1Data.BlockHash)
		randaoMixes[i] = h
	}

	zeroHash := params.BeaconConfig().ZeroHash[:]

	activeIndexRoots := make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector)
	for i := 0; i < len(activeIndexRoots); i++ {
		activeIndexRoots[i] = zeroHash
	}

	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
	for i := 0; i < len(blockRoots); i++ {
		blockRoots[i] = zeroHash
	}

	stateRoots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
	for i := 0; i < len(stateRoots); i++ {
		stateRoots[i] = zeroHash
	}

	slashings := make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector)

	genesisValidatorsRoot, err := stateutil.ValidatorRegistryRoot(preState.Validators())
	if err != nil {
		return nil, errors.Wrapf(err, "could not hash tree root genesis validators %v", err)
	}

	prevEpochParticipation, err := preState.PreviousEpochParticipation()
	if err != nil {
		return nil, err
	}
	currEpochParticipation, err := preState.CurrentEpochParticipation()
	if err != nil {
		return nil, err
	}
	scores, err := preState.InactivityScores()
	if err != nil {
		return nil, err
	}
	tab, err := helpers.TotalActiveBalance(preState)
	if err != nil {
		return nil, err
	}
	st := &ethpb.BeaconStateEPBS{
		// Misc fields.
		Slot:                  0,
		GenesisTime:           genesisTime,
		GenesisValidatorsRoot: genesisValidatorsRoot[:],

		Fork: &ethpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
			Epoch:           0,
		},

		// Validator registry fields.
		Validators:                 preState.Validators(),
		Balances:                   preState.Balances(),
		PreviousEpochParticipation: prevEpochParticipation,
		CurrentEpochParticipation:  currEpochParticipation,
		InactivityScores:           scores,

		// Randomness and committees.
		RandaoMixes: randaoMixes,

		// Finality.
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		CurrentJustifiedCheckpoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		JustificationBits: []byte{0},
		FinalizedCheckpoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},

		HistoricalRoots: [][]byte{},
		BlockRoots:      blockRoots,
		StateRoots:      stateRoots,
		Slashings:       slashings,

		// Eth1 data.
		Eth1Data:         eth1Data,
		Eth1DataVotes:    []*ethpb.Eth1Data{},
		Eth1DepositIndex: preState.Eth1DepositIndex(),

		// Electra Data
		DepositRequestsStartIndex:     params.BeaconConfig().UnsetDepositRequestsStartIndex,
		ExitBalanceToConsume:          helpers.ActivationExitChurnLimit(primitives.Gwei(tab)),
		EarliestConsolidationEpoch:    helpers.ActivationExitEpoch(slots.ToEpoch(preState.Slot())),
		ConsolidationBalanceToConsume: helpers.ConsolidationChurnLimit(primitives.Gwei(tab)),
		PendingBalanceDeposits:        make([]*ethpb.PendingBalanceDeposit, 0),
		PendingPartialWithdrawals:     make([]*ethpb.PendingPartialWithdrawal, 0),
		PendingConsolidations:         make([]*ethpb.PendingConsolidation, 0),
	}

	var scBits [fieldparams.SyncAggregateSyncCommitteeBytesLength]byte
	kzgs := make([][]byte, 0)
	kzgRoot, err := ssz.KzgCommitmentsRoot(kzgs)
	if err != nil {
		return nil, err
	}
	bodyRoot, err := (&ethpb.BeaconBlockBodyEpbs{
		RandaoReveal: make([]byte, 96),
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: make([]byte, 32),
			BlockHash:   make([]byte, 32),
		},
		Graffiti: make([]byte, 32),
		SyncAggregate: &ethpb.SyncAggregate{
			SyncCommitteeBits:      scBits[:],
			SyncCommitteeSignature: make([]byte, 96),
		},
		SignedExecutionPayloadHeader: &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				ParentBlockHash:        make([]byte, 32),
				ParentBlockRoot:        make([]byte, 32),
				BlockHash:              make([]byte, 32),
				BlobKzgCommitmentsRoot: kzgRoot[:],
			},
			Signature: make([]byte, 96),
		},
		BlsToExecutionChanges: make([]*ethpb.SignedBLSToExecutionChange, 0),
		PayloadAttestations:   make([]*ethpb.PayloadAttestation, 0),
	}).HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not hash tree root empty block body")
	}

	st.LatestBlockHeader = &ethpb.BeaconBlockHeader{
		ParentRoot: zeroHash,
		StateRoot:  zeroHash,
		BodyRoot:   bodyRoot[:],
	}

	var pubKeys [][]byte
	vals := preState.Validators()
	for i := uint64(0); i < params.BeaconConfig().SyncCommitteeSize; i++ {
		j := i % uint64(len(vals))
		pubKeys = append(pubKeys, vals[j].PublicKey)
	}
	aggregated, err := bls.AggregatePublicKeys(pubKeys)
	if err != nil {
		return nil, err
	}
	st.CurrentSyncCommittee = &ethpb.SyncCommittee{
		Pubkeys:         pubKeys,
		AggregatePubkey: aggregated.Marshal(),
	}
	st.NextSyncCommittee = &ethpb.SyncCommittee{
		Pubkeys:         pubKeys,
		AggregatePubkey: aggregated.Marshal(),
	}

	st.LatestExecutionPayloadHeader = &enginev1.ExecutionPayloadHeaderEPBS{
		ParentBlockHash:        make([]byte, 32),
		ParentBlockRoot:        make([]byte, 32),
		BlockHash:              make([]byte, 32),
		BlobKzgCommitmentsRoot: kzgRoot[:],
	}

	return state_native.InitializeFromProtoEpbs(st)
}

// DeterministicGenesisStateEpbs returns a genesis state in ePBS format made using the deterministic deposits.
func DeterministicGenesisStateEpbs(t testing.TB, numValidators uint64) (state.BeaconState, []bls.SecretKey) {
	deposits, privKeys, err := DeterministicDepositsAndKeys(numValidators)
	if err != nil {
		t.Fatal(errors.Wrapf(err, "failed to get %d deposits", numValidators))
	}
	eth1Data, err := DeterministicEth1Data(len(deposits))
	if err != nil {
		t.Fatal(errors.Wrapf(err, "failed to get eth1data for %d deposits", numValidators))
	}
	beaconState, err := genesisBeaconStateEpbs(context.Background(), deposits, uint64(0), eth1Data)
	if err != nil {
		t.Fatal(errors.Wrapf(err, "failed to get genesis beacon state of %d validators", numValidators))
	}
	if err := setKeysToActive(beaconState); err != nil {
		t.Fatal(errors.Wrapf(err, "failed to set keys to active"))
	}
	resetCache()
	return beaconState, privKeys
}
