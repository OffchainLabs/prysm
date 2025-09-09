package util

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

// DeterministicGenesisStateFulu returns a genesis state in Fulu format made using the deterministic deposits.
func DeterministicGenesisStateFulu(t testing.TB, numValidators uint64) (state.BeaconState, []bls.SecretKey) {
	deposits, privKeys, err := DeterministicDepositsAndKeys(numValidators)
	if err != nil {
		t.Fatal(errors.Wrapf(err, "failed to get %d deposits", numValidators))
	}
	eth1Data, err := DeterministicEth1Data(len(deposits))
	if err != nil {
		t.Fatal(errors.Wrapf(err, "failed to get eth1data for %d deposits", numValidators))
	}
	beaconState, err := genesisBeaconStateFulu(t.Context(), deposits, uint64(0), eth1Data)
	if err != nil {
		t.Fatal(errors.Wrapf(err, "failed to get genesis beacon state of %d validators", numValidators))
	}
	if err := setKeysToActiveFulu(beaconState); err != nil {
		t.Fatal(errors.Wrapf(err, "failed to set keys to active"))
	}
	resetCache()
	return beaconState, privKeys
}

// setKeysToActiveFulu is a function to set the validators to active post fulu, fulu no longer processes deposits based on eth1data
func setKeysToActiveFulu(beaconState state.BeaconState) error {
	vals := make([]*ethpb.Validator, len(beaconState.Validators()))
	for i, val := range beaconState.Validators() {
		val.ActivationEpoch = 0
		val.EffectiveBalance = params.BeaconConfig().MinActivationBalance
		vals[i] = val
	}
	return beaconState.SetValidators(vals)
}

// genesisBeaconStateFulu returns the genesis beacon state.
func genesisBeaconStateFulu(ctx context.Context, deposits []*ethpb.Deposit, genesisTime uint64, eth1Data *ethpb.Eth1Data) (state.BeaconState, error) {
	st, err := emptyGenesisStateFulu()
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

	return buildGenesisBeaconStateFulu(genesisTime, st, st.Eth1Data())
}

// emptyGenesisStateFulu returns an empty genesis state in Fulu format.
func emptyGenesisStateFulu() (state.BeaconState, error) {
	st := &ethpb.BeaconStateFulu{
		// Misc fields.
		Slot: 0,
		Fork: &ethpb.Fork{
			PreviousVersion: params.BeaconConfig().ElectraForkVersion,
			CurrentVersion:  params.BeaconConfig().FuluForkVersion,
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

		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderDeneb{},

		// Electra fields
		DepositBalanceToConsume:       primitives.Gwei(0),
		ExitBalanceToConsume:          primitives.Gwei(0),
		ConsolidationBalanceToConsume: primitives.Gwei(0),

		// Fulu specific field
		ProposerLookahead: []uint64{},
	}
	return state_native.InitializeFromProtoFulu(st)
}

func buildGenesisBeaconStateFulu(genesisTime uint64, preState state.BeaconState, eth1Data *ethpb.Eth1Data) (state.BeaconState, error) {
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

	st := &ethpb.BeaconStateFulu{
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
		PendingDeposits:               make([]*ethpb.PendingDeposit, 0),
		PendingPartialWithdrawals:     make([]*ethpb.PendingPartialWithdrawal, 0),
		PendingConsolidations:         make([]*ethpb.PendingConsolidation, 0),
	}

	var scBits [fieldparams.SyncAggregateSyncCommitteeBytesLength]byte
	bodyRoot, err := (&ethpb.BeaconBlockBodyElectra{
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
		ExecutionPayload: &enginev1.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			ExtraData:     make([]byte, 0),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
			Transactions:  make([][]byte, 0),
		},
		ExecutionRequests: &enginev1.ExecutionRequests{
			Deposits:       make([]*enginev1.DepositRequest, 0),
			Withdrawals:    make([]*enginev1.WithdrawalRequest, 0),
			Consolidations: make([]*enginev1.ConsolidationRequest, 0),
		},
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

	st.LatestExecutionPayloadHeader = &enginev1.ExecutionPayloadHeaderDeneb{
		ParentHash:       make([]byte, 32),
		FeeRecipient:     make([]byte, 20),
		StateRoot:        make([]byte, 32),
		ReceiptsRoot:     make([]byte, 32),
		LogsBloom:        make([]byte, 256),
		PrevRandao:       make([]byte, 32),
		ExtraData:        make([]byte, 0),
		BaseFeePerGas:    make([]byte, 32),
		BlockHash:        make([]byte, 32),
		TransactionsRoot: make([]byte, 32),
		WithdrawalsRoot:  make([]byte, 32),
	}

	// Calculate proposer lookahead for genesis
	preFuluSt, err := state_native.InitializeFromProtoFulu(st)
	if err != nil {
		return nil, err
	}
	proposerLookahead, err := helpers.InitializeProposerLookahead(context.Background(), preFuluSt, slots.ToEpoch(preState.Slot()))
	if err != nil {
		return nil, errors.Wrap(err, "could not calculate proposer lookahead")
	}

	// Fulu specific field
	st.ProposerLookahead = proposerLookahead
	return state_native.InitializeFromProtoFulu(st)
}
