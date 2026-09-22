package eth_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	v1alpha1 "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBeaconState_FieldParity(t *testing.T) {
	assertStateFields(t, reflect.TypeFor[v1alpha1.BeaconState](), []string{
		"GenesisTime uint64",
		"GenesisValidatorsRoot []uint8 ssz-size",
		"Slot primitives.Slot",
		"Fork *eth.Fork",
		"LatestBlockHeader *eth.BeaconBlockHeader",
		"BlockRoots [][]uint8 ssz-size",
		"StateRoots [][]uint8 ssz-size",
		"HistoricalRoots [][]uint8 ssz-size ssz-max",
		"Eth1Data *eth.Eth1Data",
		"Eth1DataVotes []*eth.Eth1Data ssz-max",
		"Eth1DepositIndex uint64",
		"Validators []*eth.Validator ssz-max",
		"Balances []uint64 ssz-max",
		"RandaoMixes [][]uint8 ssz-size",
		"Slashings []uint64 ssz-size",
		"PreviousEpochAttestations []*eth.PendingAttestation ssz-max",
		"CurrentEpochAttestations []*eth.PendingAttestation ssz-max",
		"JustificationBits bitfield.Bitvector4 ssz-size",
		"PreviousJustifiedCheckpoint *eth.Checkpoint",
		"CurrentJustifiedCheckpoint *eth.Checkpoint",
		"FinalizedCheckpoint *eth.Checkpoint",
	})
}

func TestBeaconState_Copy(t *testing.T) {
	orig := &v1alpha1.BeaconState{PreviousEpochAttestations: []*v1alpha1.PendingAttestation{{AggregationBits: []byte{1}, Data: &v1alpha1.AttestationData{BeaconBlockRoot: []byte{2}}}}, CurrentEpochAttestations: []*v1alpha1.PendingAttestation{{ProposerIndex: 3}}}
	cp := orig.Copy()
	require.DeepSSZEqual(t, orig, cp)
	cp.PreviousEpochAttestations[0].AggregationBits[0] = 4
	cp.PreviousEpochAttestations[0].Data.BeaconBlockRoot[0] = 5
	cp.CurrentEpochAttestations[0].ProposerIndex = 6
	require.Equal(t, byte(1), orig.PreviousEpochAttestations[0].AggregationBits[0])
	require.Equal(t, byte(2), orig.PreviousEpochAttestations[0].Data.BeaconBlockRoot[0])
	require.Equal(t, primitives.ValidatorIndex(3), orig.CurrentEpochAttestations[0].ProposerIndex)
	require.Equal(t, (*v1alpha1.BeaconState)(nil), (*v1alpha1.BeaconState)(nil).Copy())
}

// Both presets must match the same field schema.
func TestBeaconStateAltair_FieldParity(t *testing.T) {
	want := []string{
		"GenesisTime uint64",
		"GenesisValidatorsRoot []uint8 ssz-size",
		"Slot primitives.Slot",
		"Fork *eth.Fork",
		"LatestBlockHeader *eth.BeaconBlockHeader",
		"BlockRoots [][]uint8 ssz-size",
		"StateRoots [][]uint8 ssz-size",
		"HistoricalRoots [][]uint8 ssz-size ssz-max",
		"Eth1Data *eth.Eth1Data",
		"Eth1DataVotes []*eth.Eth1Data ssz-max",
		"Eth1DepositIndex uint64",
		"Validators []*eth.Validator ssz-max",
		"Balances []uint64 ssz-max",
		"RandaoMixes [][]uint8 ssz-size",
		"Slashings []uint64 ssz-size",
		"PreviousEpochParticipation []uint8 ssz-max",
		"CurrentEpochParticipation []uint8 ssz-max",
		"JustificationBits bitfield.Bitvector4 ssz-size",
		"PreviousJustifiedCheckpoint *eth.Checkpoint",
		"CurrentJustifiedCheckpoint *eth.Checkpoint",
		"FinalizedCheckpoint *eth.Checkpoint",
		"InactivityScores []uint64 ssz-max",
		"CurrentSyncCommittee *eth.SyncCommittee",
		"NextSyncCommittee *eth.SyncCommittee",
	}
	assertStateFields(t, reflect.TypeFor[v1alpha1.BeaconStateAltair](), want)
}

func assertStateFields(t *testing.T, rt reflect.Type, want []string) {
	t.Helper()
	got := make([]string, 0, rt.NumField())
	for i := range rt.NumField() {
		f := rt.Field(i)
		parts := []string{f.Name, f.Type.String()}
		for _, key := range []string{"ssz-size", "ssz-max"} {
			if _, ok := f.Tag.Lookup(key); ok {
				parts = append(parts, key)
			}
		}
		got = append(got, strings.Join(parts, " "))
	}
	require.DeepEqual(t, want, got)
}

func TestBeaconStateAltair_Copy(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		require.Equal(t, (*v1alpha1.BeaconStateAltair)(nil), (*v1alpha1.BeaconStateAltair)(nil).Copy())
	})
	t.Run("nil sub-messages stay nil", func(t *testing.T) {
		cp := (&v1alpha1.BeaconStateAltair{}).Copy()
		require.DeepSSZEqual(t, &v1alpha1.BeaconStateAltair{}, cp)
		require.Equal(t, true, cp.Fork == nil)
	})
	t.Run("deep copy does not alias", func(t *testing.T) {
		orig := &v1alpha1.BeaconStateAltair{
			Slot:              7,
			Fork:              &v1alpha1.Fork{Epoch: 1, PreviousVersion: []byte{1}, CurrentVersion: []byte{2}},
			BlockRoots:        [][]byte{{1}, {2}},
			Eth1DataVotes:     []*v1alpha1.Eth1Data{{DepositCount: 3}},
			Validators:        []*v1alpha1.Validator{{EffectiveBalance: 32}},
			Balances:          []uint64{32},
			JustificationBits: bitfield.Bitvector4{0b1010},
		}
		cp := orig.Copy()
		require.DeepSSZEqual(t, orig, cp)

		cp.Fork.Epoch, cp.BlockRoots[0][0], cp.Eth1DataVotes[0].DepositCount = 9, 9, 9
		cp.Validators[0].EffectiveBalance, cp.Balances[0], cp.JustificationBits[0] = 9, 9, 9
		require.Equal(t, primitives.Epoch(1), orig.Fork.Epoch)
		require.Equal(t, byte(1), orig.BlockRoots[0][0])
		require.Equal(t, uint64(3), orig.Eth1DataVotes[0].DepositCount)
		require.Equal(t, uint64(32), orig.Validators[0].EffectiveBalance)
		require.Equal(t, uint64(32), orig.Balances[0])
		require.Equal(t, byte(0b1010), orig.JustificationBits[0])
	})
}

func TestBeaconStateBellatrix_Copy(t *testing.T) {
	orig := &v1alpha1.BeaconStateBellatrix{
		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeader{ParentHash: []byte{1}},
	}
	cp := orig.Copy()
	require.DeepSSZEqual(t, orig, cp)
	cp.LatestExecutionPayloadHeader.ParentHash[0] = 2
	require.Equal(t, byte(1), orig.LatestExecutionPayloadHeader.ParentHash[0])
	require.Equal(t, (*v1alpha1.BeaconStateBellatrix)(nil), (*v1alpha1.BeaconStateBellatrix)(nil).Copy())
}

func TestBeaconStateBellatrix_FieldParity(t *testing.T) {
	assertStateFields(t, reflect.TypeFor[v1alpha1.BeaconStateBellatrix](), []string{
		"GenesisTime uint64",
		"GenesisValidatorsRoot []uint8 ssz-size",
		"Slot primitives.Slot",
		"Fork *eth.Fork",
		"LatestBlockHeader *eth.BeaconBlockHeader",
		"BlockRoots [][]uint8 ssz-size",
		"StateRoots [][]uint8 ssz-size",
		"HistoricalRoots [][]uint8 ssz-size ssz-max",
		"Eth1Data *eth.Eth1Data",
		"Eth1DataVotes []*eth.Eth1Data ssz-max",
		"Eth1DepositIndex uint64",
		"Validators []*eth.Validator ssz-max",
		"Balances []uint64 ssz-max",
		"RandaoMixes [][]uint8 ssz-size",
		"Slashings []uint64 ssz-size",
		"PreviousEpochParticipation []uint8 ssz-max",
		"CurrentEpochParticipation []uint8 ssz-max",
		"JustificationBits bitfield.Bitvector4 ssz-size",
		"PreviousJustifiedCheckpoint *eth.Checkpoint",
		"CurrentJustifiedCheckpoint *eth.Checkpoint",
		"FinalizedCheckpoint *eth.Checkpoint",
		"InactivityScores []uint64 ssz-max",
		"CurrentSyncCommittee *eth.SyncCommittee",
		"NextSyncCommittee *eth.SyncCommittee",
		"LatestExecutionPayloadHeader *enginev1.ExecutionPayloadHeader",
	})
}

func TestBeaconStateCapella_FieldParity(t *testing.T) {
	assertStateFields(t, reflect.TypeFor[v1alpha1.BeaconStateCapella](), []string{
		"GenesisTime uint64",
		"GenesisValidatorsRoot []uint8 ssz-size",
		"Slot primitives.Slot",
		"Fork *eth.Fork",
		"LatestBlockHeader *eth.BeaconBlockHeader",
		"BlockRoots [][]uint8 ssz-size",
		"StateRoots [][]uint8 ssz-size",
		"HistoricalRoots [][]uint8 ssz-size ssz-max",
		"Eth1Data *eth.Eth1Data",
		"Eth1DataVotes []*eth.Eth1Data ssz-max",
		"Eth1DepositIndex uint64",
		"Validators []*eth.Validator ssz-max",
		"Balances []uint64 ssz-max",
		"RandaoMixes [][]uint8 ssz-size",
		"Slashings []uint64 ssz-size",
		"PreviousEpochParticipation []uint8 ssz-max",
		"CurrentEpochParticipation []uint8 ssz-max",
		"JustificationBits bitfield.Bitvector4 ssz-size",
		"PreviousJustifiedCheckpoint *eth.Checkpoint",
		"CurrentJustifiedCheckpoint *eth.Checkpoint",
		"FinalizedCheckpoint *eth.Checkpoint",
		"InactivityScores []uint64 ssz-max",
		"CurrentSyncCommittee *eth.SyncCommittee",
		"NextSyncCommittee *eth.SyncCommittee",
		"LatestExecutionPayloadHeader *enginev1.ExecutionPayloadHeaderCapella",
		"NextWithdrawalIndex uint64",
		"NextWithdrawalValidatorIndex primitives.ValidatorIndex",
		"HistoricalSummaries []*eth.HistoricalSummary ssz-max",
	})
}

func TestBeaconStateCapella_Copy(t *testing.T) {
	orig := &v1alpha1.BeaconStateCapella{LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderCapella{WithdrawalsRoot: []byte{1}}, HistoricalSummaries: []*v1alpha1.HistoricalSummary{{BlockSummaryRoot: []byte{2}}}}
	cp := orig.Copy()
	require.DeepSSZEqual(t, orig, cp)
	cp.LatestExecutionPayloadHeader.WithdrawalsRoot[0] = 3
	cp.HistoricalSummaries[0].BlockSummaryRoot[0] = 4
	require.Equal(t, byte(1), orig.LatestExecutionPayloadHeader.WithdrawalsRoot[0])
	require.Equal(t, byte(2), orig.HistoricalSummaries[0].BlockSummaryRoot[0])
	require.Equal(t, (*v1alpha1.BeaconStateCapella)(nil), (*v1alpha1.BeaconStateCapella)(nil).Copy())
}

func TestBeaconStateDeneb_FieldParity(t *testing.T) {
	assertStateFields(t, reflect.TypeFor[v1alpha1.BeaconStateDeneb](), []string{
		"GenesisTime uint64",
		"GenesisValidatorsRoot []uint8 ssz-size",
		"Slot primitives.Slot",
		"Fork *eth.Fork",
		"LatestBlockHeader *eth.BeaconBlockHeader",
		"BlockRoots [][]uint8 ssz-size",
		"StateRoots [][]uint8 ssz-size",
		"HistoricalRoots [][]uint8 ssz-size ssz-max",
		"Eth1Data *eth.Eth1Data",
		"Eth1DataVotes []*eth.Eth1Data ssz-max",
		"Eth1DepositIndex uint64",
		"Validators []*eth.Validator ssz-max",
		"Balances []uint64 ssz-max",
		"RandaoMixes [][]uint8 ssz-size",
		"Slashings []uint64 ssz-size",
		"PreviousEpochParticipation []uint8 ssz-max",
		"CurrentEpochParticipation []uint8 ssz-max",
		"JustificationBits bitfield.Bitvector4 ssz-size",
		"PreviousJustifiedCheckpoint *eth.Checkpoint",
		"CurrentJustifiedCheckpoint *eth.Checkpoint",
		"FinalizedCheckpoint *eth.Checkpoint",
		"InactivityScores []uint64 ssz-max",
		"CurrentSyncCommittee *eth.SyncCommittee",
		"NextSyncCommittee *eth.SyncCommittee",
		"LatestExecutionPayloadHeader *enginev1.ExecutionPayloadHeaderDeneb",
		"NextWithdrawalIndex uint64",
		"NextWithdrawalValidatorIndex primitives.ValidatorIndex",
		"HistoricalSummaries []*eth.HistoricalSummary ssz-max",
	})
}

func TestBeaconStateDeneb_Copy(t *testing.T) {
	orig := &v1alpha1.BeaconStateDeneb{LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderDeneb{ParentHash: []byte{1}, BlobGasUsed: 2}}
	cp := orig.Copy()
	require.DeepSSZEqual(t, orig, cp)
	cp.LatestExecutionPayloadHeader.ParentHash[0] = 3
	cp.LatestExecutionPayloadHeader.BlobGasUsed = 4
	require.Equal(t, byte(1), orig.LatestExecutionPayloadHeader.ParentHash[0])
	require.Equal(t, uint64(2), orig.LatestExecutionPayloadHeader.BlobGasUsed)
	require.Equal(t, (*v1alpha1.BeaconStateDeneb)(nil), (*v1alpha1.BeaconStateDeneb)(nil).Copy())
}

func TestBeaconStateElectra_FieldParity(t *testing.T) {
	assertStateFields(t, reflect.TypeFor[v1alpha1.BeaconStateElectra](), []string{
		"GenesisTime uint64",
		"GenesisValidatorsRoot []uint8 ssz-size",
		"Slot primitives.Slot",
		"Fork *eth.Fork",
		"LatestBlockHeader *eth.BeaconBlockHeader",
		"BlockRoots [][]uint8 ssz-size",
		"StateRoots [][]uint8 ssz-size",
		"HistoricalRoots [][]uint8 ssz-size ssz-max",
		"Eth1Data *eth.Eth1Data",
		"Eth1DataVotes []*eth.Eth1Data ssz-max",
		"Eth1DepositIndex uint64",
		"Validators []*eth.Validator ssz-max",
		"Balances []uint64 ssz-max",
		"RandaoMixes [][]uint8 ssz-size",
		"Slashings []uint64 ssz-size",
		"PreviousEpochParticipation []uint8 ssz-max",
		"CurrentEpochParticipation []uint8 ssz-max",
		"JustificationBits bitfield.Bitvector4 ssz-size",
		"PreviousJustifiedCheckpoint *eth.Checkpoint",
		"CurrentJustifiedCheckpoint *eth.Checkpoint",
		"FinalizedCheckpoint *eth.Checkpoint",
		"InactivityScores []uint64 ssz-max",
		"CurrentSyncCommittee *eth.SyncCommittee",
		"NextSyncCommittee *eth.SyncCommittee",
		"LatestExecutionPayloadHeader *enginev1.ExecutionPayloadHeaderDeneb",
		"NextWithdrawalIndex uint64",
		"NextWithdrawalValidatorIndex primitives.ValidatorIndex",
		"HistoricalSummaries []*eth.HistoricalSummary ssz-max",
		"DepositRequestsStartIndex uint64",
		"DepositBalanceToConsume primitives.Gwei",
		"ExitBalanceToConsume primitives.Gwei",
		"EarliestExitEpoch primitives.Epoch",
		"ConsolidationBalanceToConsume primitives.Gwei",
		"EarliestConsolidationEpoch primitives.Epoch",
		"PendingDeposits []*eth.PendingDeposit ssz-max",
		"PendingPartialWithdrawals []*eth.PendingPartialWithdrawal ssz-max",
		"PendingConsolidations []*eth.PendingConsolidation ssz-max",
	})
}

func TestBeaconStateElectra_Copy(t *testing.T) {
	orig := &v1alpha1.BeaconStateElectra{PendingDeposits: []*v1alpha1.PendingDeposit{{PublicKey: []byte{1}}}, PendingPartialWithdrawals: []*v1alpha1.PendingPartialWithdrawal{{Amount: 2}}, PendingConsolidations: []*v1alpha1.PendingConsolidation{{SourceIndex: 3}}}
	cp := orig.Copy()
	require.DeepSSZEqual(t, orig, cp)
	cp.PendingDeposits[0].PublicKey[0] = 4
	cp.PendingPartialWithdrawals[0].Amount = 5
	cp.PendingConsolidations[0].SourceIndex = 6
	require.Equal(t, byte(1), orig.PendingDeposits[0].PublicKey[0])
	require.Equal(t, uint64(2), orig.PendingPartialWithdrawals[0].Amount)
	require.Equal(t, primitives.ValidatorIndex(3), orig.PendingConsolidations[0].SourceIndex)
	require.Equal(t, (*v1alpha1.BeaconStateElectra)(nil), (*v1alpha1.BeaconStateElectra)(nil).Copy())
}

func TestBeaconStateFulu_FieldParity(t *testing.T) {
	assertStateFields(t, reflect.TypeFor[v1alpha1.BeaconStateFulu](), []string{
		"GenesisTime uint64",
		"GenesisValidatorsRoot []uint8 ssz-size",
		"Slot primitives.Slot",
		"Fork *eth.Fork",
		"LatestBlockHeader *eth.BeaconBlockHeader",
		"BlockRoots [][]uint8 ssz-size",
		"StateRoots [][]uint8 ssz-size",
		"HistoricalRoots [][]uint8 ssz-size ssz-max",
		"Eth1Data *eth.Eth1Data",
		"Eth1DataVotes []*eth.Eth1Data ssz-max",
		"Eth1DepositIndex uint64",
		"Validators []*eth.Validator ssz-max",
		"Balances []uint64 ssz-max",
		"RandaoMixes [][]uint8 ssz-size",
		"Slashings []uint64 ssz-size",
		"PreviousEpochParticipation []uint8 ssz-max",
		"CurrentEpochParticipation []uint8 ssz-max",
		"JustificationBits bitfield.Bitvector4 ssz-size",
		"PreviousJustifiedCheckpoint *eth.Checkpoint",
		"CurrentJustifiedCheckpoint *eth.Checkpoint",
		"FinalizedCheckpoint *eth.Checkpoint",
		"InactivityScores []uint64 ssz-max",
		"CurrentSyncCommittee *eth.SyncCommittee",
		"NextSyncCommittee *eth.SyncCommittee",
		"LatestExecutionPayloadHeader *enginev1.ExecutionPayloadHeaderDeneb",
		"NextWithdrawalIndex uint64",
		"NextWithdrawalValidatorIndex primitives.ValidatorIndex",
		"HistoricalSummaries []*eth.HistoricalSummary ssz-max",
		"DepositRequestsStartIndex uint64",
		"DepositBalanceToConsume primitives.Gwei",
		"ExitBalanceToConsume primitives.Gwei",
		"EarliestExitEpoch primitives.Epoch",
		"ConsolidationBalanceToConsume primitives.Gwei",
		"EarliestConsolidationEpoch primitives.Epoch",
		"PendingDeposits []*eth.PendingDeposit ssz-max",
		"PendingPartialWithdrawals []*eth.PendingPartialWithdrawal ssz-max",
		"PendingConsolidations []*eth.PendingConsolidation ssz-max",
		"ProposerLookahead []primitives.ValidatorIndex ssz-size",
	})
}

func TestBeaconStateFulu_Copy(t *testing.T) {
	orig := &v1alpha1.BeaconStateFulu{ProposerLookahead: []primitives.ValidatorIndex{1}}
	cp := orig.Copy()
	require.DeepSSZEqual(t, orig, cp)
	cp.ProposerLookahead[0] = 2
	require.Equal(t, primitives.ValidatorIndex(1), orig.ProposerLookahead[0])
	require.Equal(t, (*v1alpha1.BeaconStateFulu)(nil), (*v1alpha1.BeaconStateFulu)(nil).Copy())
}

func TestBeaconStateGloas_FieldParity(t *testing.T) {
	assertStateFields(t, reflect.TypeFor[v1alpha1.BeaconStateGloas](), []string{
		"GenesisTime uint64",
		"GenesisValidatorsRoot []uint8 ssz-size",
		"Slot primitives.Slot",
		"Fork *eth.Fork",
		"LatestBlockHeader *eth.BeaconBlockHeader",
		"BlockRoots [][]uint8 ssz-size",
		"StateRoots [][]uint8 ssz-size",
		"HistoricalRoots [][]uint8 ssz-size ssz-max",
		"Eth1Data *eth.Eth1Data",
		"Eth1DataVotes []*eth.Eth1Data ssz-max",
		"Eth1DepositIndex uint64",
		"Validators []*eth.Validator ssz-max",
		"Balances []uint64 ssz-max",
		"RandaoMixes [][]uint8 ssz-size",
		"Slashings []uint64 ssz-size",
		"PreviousEpochParticipation []uint8 ssz-max",
		"CurrentEpochParticipation []uint8 ssz-max",
		"JustificationBits bitfield.Bitvector4 ssz-size",
		"PreviousJustifiedCheckpoint *eth.Checkpoint",
		"CurrentJustifiedCheckpoint *eth.Checkpoint",
		"FinalizedCheckpoint *eth.Checkpoint",
		"InactivityScores []uint64 ssz-max",
		"CurrentSyncCommittee *eth.SyncCommittee",
		"NextSyncCommittee *eth.SyncCommittee",
		"LatestBlockHash []uint8 ssz-size",
		"NextWithdrawalIndex uint64",
		"NextWithdrawalValidatorIndex primitives.ValidatorIndex",
		"HistoricalSummaries []*eth.HistoricalSummary ssz-max",
		"DepositRequestsStartIndex uint64",
		"DepositBalanceToConsume primitives.Gwei",
		"ExitBalanceToConsume primitives.Gwei",
		"EarliestExitEpoch primitives.Epoch",
		"ConsolidationBalanceToConsume primitives.Gwei",
		"EarliestConsolidationEpoch primitives.Epoch",
		"PendingDeposits []*eth.PendingDeposit ssz-max",
		"PendingPartialWithdrawals []*eth.PendingPartialWithdrawal ssz-max",
		"PendingConsolidations []*eth.PendingConsolidation ssz-max",
		"ProposerLookahead []primitives.ValidatorIndex ssz-size",
		"Builders []*eth.Builder ssz-max",
		"NextWithdrawalBuilderIndex primitives.BuilderIndex",
		"ExecutionPayloadAvailability []uint8 ssz-size",
		"BuilderPendingPayments []*eth.BuilderPendingPayment ssz-size",
		"BuilderPendingWithdrawals []*eth.BuilderPendingWithdrawal ssz-max",
		"LatestExecutionPayloadBid *eth.ExecutionPayloadBid",
		"PayloadExpectedWithdrawals []*enginev1.Withdrawal ssz-max",
		"PtcWindow []*eth.PTCs ssz-size",
	})
}

func TestBeaconStateGloas_Copy(t *testing.T) {
	orig := &v1alpha1.BeaconStateGloas{Builders: []*v1alpha1.Builder{{Pubkey: []byte{1}}}, ExecutionPayloadAvailability: []byte{2}, PtcWindow: []*v1alpha1.PTCs{{ValidatorIndices: []primitives.ValidatorIndex{3}}}, LatestExecutionPayloadBid: &v1alpha1.ExecutionPayloadBid{BuilderIndex: 4, ParentBlockHash: []byte{5}}}
	cp := orig.Copy()
	require.DeepSSZEqual(t, orig, cp)
	cp.Builders[0].Pubkey[0] = 6
	cp.ExecutionPayloadAvailability[0] = 7
	cp.PtcWindow[0].ValidatorIndices[0] = 8
	cp.LatestExecutionPayloadBid.ParentBlockHash[0] = 9
	require.Equal(t, byte(1), orig.Builders[0].Pubkey[0])
	require.Equal(t, byte(2), orig.ExecutionPayloadAvailability[0])
	require.Equal(t, primitives.ValidatorIndex(3), orig.PtcWindow[0].ValidatorIndices[0])
	require.Equal(t, byte(5), orig.LatestExecutionPayloadBid.ParentBlockHash[0])
	require.Equal(t, (*v1alpha1.BeaconStateGloas)(nil), (*v1alpha1.BeaconStateGloas)(nil).Copy())
}
