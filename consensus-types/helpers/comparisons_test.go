package helpers

import (
	"testing"

	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

func TestForksEqual(t *testing.T) {
	tests := []struct {
		name string
		s    *ethpb.Fork
		t    *ethpb.Fork
		want bool
	}{
		{
			name: "both nil",
			s:    nil,
			t:    nil,
			want: true,
		},
		{
			name: "first nil",
			s:    nil,
			t:    &ethpb.Fork{Epoch: 1},
			want: false,
		},
		{
			name: "second nil",
			s:    &ethpb.Fork{Epoch: 1},
			t:    nil,
			want: false,
		},
		{
			name: "equal forks",
			s: &ethpb.Fork{
				Epoch:           100,
				PreviousVersion: []byte{1, 2, 3, 4},
				CurrentVersion:  []byte{5, 6, 7, 8},
			},
			t: &ethpb.Fork{
				Epoch:           100,
				PreviousVersion: []byte{1, 2, 3, 4},
				CurrentVersion:  []byte{5, 6, 7, 8},
			},
			want: true,
		},
		{
			name: "different epoch",
			s: &ethpb.Fork{
				Epoch:           100,
				PreviousVersion: []byte{1, 2, 3, 4},
				CurrentVersion:  []byte{5, 6, 7, 8},
			},
			t: &ethpb.Fork{
				Epoch:           200,
				PreviousVersion: []byte{1, 2, 3, 4},
				CurrentVersion:  []byte{5, 6, 7, 8},
			},
			want: false,
		},
		{
			name: "different previous version",
			s: &ethpb.Fork{
				Epoch:           100,
				PreviousVersion: []byte{1, 2, 3, 4},
				CurrentVersion:  []byte{5, 6, 7, 8},
			},
			t: &ethpb.Fork{
				Epoch:           100,
				PreviousVersion: []byte{9, 10, 11, 12},
				CurrentVersion:  []byte{5, 6, 7, 8},
			},
			want: false,
		},
		{
			name: "different current version",
			s: &ethpb.Fork{
				Epoch:           100,
				PreviousVersion: []byte{1, 2, 3, 4},
				CurrentVersion:  []byte{5, 6, 7, 8},
			},
			t: &ethpb.Fork{
				Epoch:           100,
				PreviousVersion: []byte{1, 2, 3, 4},
				CurrentVersion:  []byte{9, 10, 11, 12},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForksEqual(tt.s, tt.t); got != tt.want {
				t.Errorf("ForksEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlockHeadersEqual(t *testing.T) {
	tests := []struct {
		name string
		s    *ethpb.BeaconBlockHeader
		t    *ethpb.BeaconBlockHeader
		want bool
	}{
		{
			name: "both nil",
			s:    nil,
			t:    nil,
			want: true,
		},
		{
			name: "first nil",
			s:    nil,
			t:    &ethpb.BeaconBlockHeader{Slot: 1},
			want: false,
		},
		{
			name: "second nil",
			s:    &ethpb.BeaconBlockHeader{Slot: 1},
			t:    nil,
			want: false,
		},
		{
			name: "equal headers",
			s: &ethpb.BeaconBlockHeader{
				Slot:          100,
				ProposerIndex: 50,
				ParentRoot:    []byte{1, 2, 3, 4},
				StateRoot:     []byte{5, 6, 7, 8},
				BodyRoot:      []byte{9, 10, 11, 12},
			},
			t: &ethpb.BeaconBlockHeader{
				Slot:          100,
				ProposerIndex: 50,
				ParentRoot:    []byte{1, 2, 3, 4},
				StateRoot:     []byte{5, 6, 7, 8},
				BodyRoot:      []byte{9, 10, 11, 12},
			},
			want: true,
		},
		{
			name: "different slot",
			s: &ethpb.BeaconBlockHeader{
				Slot:          100,
				ProposerIndex: 50,
				ParentRoot:    []byte{1, 2, 3, 4},
				StateRoot:     []byte{5, 6, 7, 8},
				BodyRoot:      []byte{9, 10, 11, 12},
			},
			t: &ethpb.BeaconBlockHeader{
				Slot:          200,
				ProposerIndex: 50,
				ParentRoot:    []byte{1, 2, 3, 4},
				StateRoot:     []byte{5, 6, 7, 8},
				BodyRoot:      []byte{9, 10, 11, 12},
			},
			want: false,
		},
		{
			name: "different proposer index",
			s: &ethpb.BeaconBlockHeader{
				Slot:          100,
				ProposerIndex: 50,
				ParentRoot:    []byte{1, 2, 3, 4},
				StateRoot:     []byte{5, 6, 7, 8},
				BodyRoot:      []byte{9, 10, 11, 12},
			},
			t: &ethpb.BeaconBlockHeader{
				Slot:          100,
				ProposerIndex: 75,
				ParentRoot:    []byte{1, 2, 3, 4},
				StateRoot:     []byte{5, 6, 7, 8},
				BodyRoot:      []byte{9, 10, 11, 12},
			},
			want: false,
		},
		{
			name: "different parent root",
			s: &ethpb.BeaconBlockHeader{
				Slot:          100,
				ProposerIndex: 50,
				ParentRoot:    []byte{1, 2, 3, 4},
				StateRoot:     []byte{5, 6, 7, 8},
				BodyRoot:      []byte{9, 10, 11, 12},
			},
			t: &ethpb.BeaconBlockHeader{
				Slot:          100,
				ProposerIndex: 50,
				ParentRoot:    []byte{13, 14, 15, 16},
				StateRoot:     []byte{5, 6, 7, 8},
				BodyRoot:      []byte{9, 10, 11, 12},
			},
			want: false,
		},
		{
			name: "different state root",
			s: &ethpb.BeaconBlockHeader{
				Slot:          100,
				ProposerIndex: 50,
				ParentRoot:    []byte{1, 2, 3, 4},
				StateRoot:     []byte{5, 6, 7, 8},
				BodyRoot:      []byte{9, 10, 11, 12},
			},
			t: &ethpb.BeaconBlockHeader{
				Slot:          100,
				ProposerIndex: 50,
				ParentRoot:    []byte{1, 2, 3, 4},
				StateRoot:     []byte{13, 14, 15, 16},
				BodyRoot:      []byte{9, 10, 11, 12},
			},
			want: false,
		},
		{
			name: "different body root",
			s: &ethpb.BeaconBlockHeader{
				Slot:          100,
				ProposerIndex: 50,
				ParentRoot:    []byte{1, 2, 3, 4},
				StateRoot:     []byte{5, 6, 7, 8},
				BodyRoot:      []byte{9, 10, 11, 12},
			},
			t: &ethpb.BeaconBlockHeader{
				Slot:          100,
				ProposerIndex: 50,
				ParentRoot:    []byte{1, 2, 3, 4},
				StateRoot:     []byte{5, 6, 7, 8},
				BodyRoot:      []byte{13, 14, 15, 16},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BlockHeadersEqual(tt.s, tt.t); got != tt.want {
				t.Errorf("BlockHeadersEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEth1DataEqual(t *testing.T) {
	tests := []struct {
		name string
		s    *ethpb.Eth1Data
		t    *ethpb.Eth1Data
		want bool
	}{
		{
			name: "both nil",
			s:    nil,
			t:    nil,
			want: true,
		},
		{
			name: "first nil",
			s:    nil,
			t:    &ethpb.Eth1Data{DepositCount: 1},
			want: false,
		},
		{
			name: "second nil",
			s:    &ethpb.Eth1Data{DepositCount: 1},
			t:    nil,
			want: false,
		},
		{
			name: "equal eth1 data",
			s: &ethpb.Eth1Data{
				DepositRoot:  []byte{1, 2, 3, 4},
				DepositCount: 100,
				BlockHash:    []byte{5, 6, 7, 8},
			},
			t: &ethpb.Eth1Data{
				DepositRoot:  []byte{1, 2, 3, 4},
				DepositCount: 100,
				BlockHash:    []byte{5, 6, 7, 8},
			},
			want: true,
		},
		{
			name: "different deposit root",
			s: &ethpb.Eth1Data{
				DepositRoot:  []byte{1, 2, 3, 4},
				DepositCount: 100,
				BlockHash:    []byte{5, 6, 7, 8},
			},
			t: &ethpb.Eth1Data{
				DepositRoot:  []byte{9, 10, 11, 12},
				DepositCount: 100,
				BlockHash:    []byte{5, 6, 7, 8},
			},
			want: false,
		},
		{
			name: "different deposit count",
			s: &ethpb.Eth1Data{
				DepositRoot:  []byte{1, 2, 3, 4},
				DepositCount: 100,
				BlockHash:    []byte{5, 6, 7, 8},
			},
			t: &ethpb.Eth1Data{
				DepositRoot:  []byte{1, 2, 3, 4},
				DepositCount: 200,
				BlockHash:    []byte{5, 6, 7, 8},
			},
			want: false,
		},
		{
			name: "different block hash",
			s: &ethpb.Eth1Data{
				DepositRoot:  []byte{1, 2, 3, 4},
				DepositCount: 100,
				BlockHash:    []byte{5, 6, 7, 8},
			},
			t: &ethpb.Eth1Data{
				DepositRoot:  []byte{1, 2, 3, 4},
				DepositCount: 100,
				BlockHash:    []byte{9, 10, 11, 12},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Eth1DataEqual(tt.s, tt.t); got != tt.want {
				t.Errorf("Eth1DataEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPendingDepositsEqual(t *testing.T) {
	tests := []struct {
		name string
		s    *ethpb.PendingDeposit
		t    *ethpb.PendingDeposit
		want bool
	}{
		{
			name: "both nil",
			s:    nil,
			t:    nil,
			want: true,
		},
		{
			name: "first nil",
			s:    nil,
			t:    &ethpb.PendingDeposit{Amount: 1},
			want: false,
		},
		{
			name: "second nil",
			s:    &ethpb.PendingDeposit{Amount: 1},
			t:    nil,
			want: false,
		},
		{
			name: "equal pending deposits",
			s: &ethpb.PendingDeposit{
				PublicKey:             []byte{1, 2, 3, 4},
				WithdrawalCredentials: []byte{5, 6, 7, 8},
				Amount:                32000000000,
				Signature:             []byte{9, 10, 11, 12},
				Slot:                  100,
			},
			t: &ethpb.PendingDeposit{
				PublicKey:             []byte{1, 2, 3, 4},
				WithdrawalCredentials: []byte{5, 6, 7, 8},
				Amount:                32000000000,
				Signature:             []byte{9, 10, 11, 12},
				Slot:                  100,
			},
			want: true,
		},
		{
			name: "different public key",
			s: &ethpb.PendingDeposit{
				PublicKey:             []byte{1, 2, 3, 4},
				WithdrawalCredentials: []byte{5, 6, 7, 8},
				Amount:                32000000000,
				Signature:             []byte{9, 10, 11, 12},
				Slot:                  100,
			},
			t: &ethpb.PendingDeposit{
				PublicKey:             []byte{13, 14, 15, 16},
				WithdrawalCredentials: []byte{5, 6, 7, 8},
				Amount:                32000000000,
				Signature:             []byte{9, 10, 11, 12},
				Slot:                  100,
			},
			want: false,
		},
		{
			name: "different withdrawal credentials",
			s: &ethpb.PendingDeposit{
				PublicKey:             []byte{1, 2, 3, 4},
				WithdrawalCredentials: []byte{5, 6, 7, 8},
				Amount:                32000000000,
				Signature:             []byte{9, 10, 11, 12},
				Slot:                  100,
			},
			t: &ethpb.PendingDeposit{
				PublicKey:             []byte{1, 2, 3, 4},
				WithdrawalCredentials: []byte{13, 14, 15, 16},
				Amount:                32000000000,
				Signature:             []byte{9, 10, 11, 12},
				Slot:                  100,
			},
			want: false,
		},
		{
			name: "different amount",
			s: &ethpb.PendingDeposit{
				PublicKey:             []byte{1, 2, 3, 4},
				WithdrawalCredentials: []byte{5, 6, 7, 8},
				Amount:                32000000000,
				Signature:             []byte{9, 10, 11, 12},
				Slot:                  100,
			},
			t: &ethpb.PendingDeposit{
				PublicKey:             []byte{1, 2, 3, 4},
				WithdrawalCredentials: []byte{5, 6, 7, 8},
				Amount:                16000000000,
				Signature:             []byte{9, 10, 11, 12},
				Slot:                  100,
			},
			want: false,
		},
		{
			name: "different signature",
			s: &ethpb.PendingDeposit{
				PublicKey:             []byte{1, 2, 3, 4},
				WithdrawalCredentials: []byte{5, 6, 7, 8},
				Amount:                32000000000,
				Signature:             []byte{9, 10, 11, 12},
				Slot:                  100,
			},
			t: &ethpb.PendingDeposit{
				PublicKey:             []byte{1, 2, 3, 4},
				WithdrawalCredentials: []byte{5, 6, 7, 8},
				Amount:                32000000000,
				Signature:             []byte{13, 14, 15, 16},
				Slot:                  100,
			},
			want: false,
		},
		{
			name: "different slot",
			s: &ethpb.PendingDeposit{
				PublicKey:             []byte{1, 2, 3, 4},
				WithdrawalCredentials: []byte{5, 6, 7, 8},
				Amount:                32000000000,
				Signature:             []byte{9, 10, 11, 12},
				Slot:                  100,
			},
			t: &ethpb.PendingDeposit{
				PublicKey:             []byte{1, 2, 3, 4},
				WithdrawalCredentials: []byte{5, 6, 7, 8},
				Amount:                32000000000,
				Signature:             []byte{9, 10, 11, 12},
				Slot:                  200,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PendingDepositsEqual(tt.s, tt.t); got != tt.want {
				t.Errorf("PendingDepositsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPendingPartialWithdrawalsEqual(t *testing.T) {
	tests := []struct {
		name string
		s    *ethpb.PendingPartialWithdrawal
		t    *ethpb.PendingPartialWithdrawal
		want bool
	}{
		{
			name: "both nil",
			s:    nil,
			t:    nil,
			want: true,
		},
		{
			name: "first nil",
			s:    nil,
			t:    &ethpb.PendingPartialWithdrawal{Index: 1},
			want: false,
		},
		{
			name: "second nil",
			s:    &ethpb.PendingPartialWithdrawal{Index: 1},
			t:    nil,
			want: false,
		},
		{
			name: "equal pending partial withdrawals",
			s: &ethpb.PendingPartialWithdrawal{
				Index:             50,
				Amount:            1000000000,
				WithdrawableEpoch: 200,
			},
			t: &ethpb.PendingPartialWithdrawal{
				Index:             50,
				Amount:            1000000000,
				WithdrawableEpoch: 200,
			},
			want: true,
		},
		{
			name: "different index",
			s: &ethpb.PendingPartialWithdrawal{
				Index:             50,
				Amount:            1000000000,
				WithdrawableEpoch: 200,
			},
			t: &ethpb.PendingPartialWithdrawal{
				Index:             75,
				Amount:            1000000000,
				WithdrawableEpoch: 200,
			},
			want: false,
		},
		{
			name: "different amount",
			s: &ethpb.PendingPartialWithdrawal{
				Index:             50,
				Amount:            1000000000,
				WithdrawableEpoch: 200,
			},
			t: &ethpb.PendingPartialWithdrawal{
				Index:             50,
				Amount:            2000000000,
				WithdrawableEpoch: 200,
			},
			want: false,
		},
		{
			name: "different withdrawable epoch",
			s: &ethpb.PendingPartialWithdrawal{
				Index:             50,
				Amount:            1000000000,
				WithdrawableEpoch: 200,
			},
			t: &ethpb.PendingPartialWithdrawal{
				Index:             50,
				Amount:            1000000000,
				WithdrawableEpoch: 300,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PendingPartialWithdrawalsEqual(tt.s, tt.t); got != tt.want {
				t.Errorf("PendingPartialWithdrawalsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPendingConsolidationsEqual(t *testing.T) {
	tests := []struct {
		name string
		s    *ethpb.PendingConsolidation
		t    *ethpb.PendingConsolidation
		want bool
	}{
		{
			name: "both nil",
			s:    nil,
			t:    nil,
			want: true,
		},
		{
			name: "first nil",
			s:    nil,
			t:    &ethpb.PendingConsolidation{SourceIndex: 1},
			want: false,
		},
		{
			name: "second nil",
			s:    &ethpb.PendingConsolidation{SourceIndex: 1},
			t:    nil,
			want: false,
		},
		{
			name: "equal pending consolidations",
			s: &ethpb.PendingConsolidation{
				SourceIndex: 10,
				TargetIndex: 20,
			},
			t: &ethpb.PendingConsolidation{
				SourceIndex: 10,
				TargetIndex: 20,
			},
			want: true,
		},
		{
			name: "different source index",
			s: &ethpb.PendingConsolidation{
				SourceIndex: 10,
				TargetIndex: 20,
			},
			t: &ethpb.PendingConsolidation{
				SourceIndex: 15,
				TargetIndex: 20,
			},
			want: false,
		},
		{
			name: "different target index",
			s: &ethpb.PendingConsolidation{
				SourceIndex: 10,
				TargetIndex: 20,
			},
			t: &ethpb.PendingConsolidation{
				SourceIndex: 10,
				TargetIndex: 25,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PendingConsolidationsEqual(tt.s, tt.t); got != tt.want {
				t.Errorf("PendingConsolidationsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
