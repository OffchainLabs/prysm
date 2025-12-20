package eth

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

func TestExecutionPayloadBid_Copy(t *testing.T) {
	tests := []struct {
		name string
		bid  *ExecutionPayloadBid
	}{
		{
			name: "nil bid",
			bid:  nil,
		},
		{
			name: "empty bid",
			bid:  &ExecutionPayloadBid{},
		},
		{
			name: "fully populated bid",
			bid: &ExecutionPayloadBid{
				ParentBlockHash:        []byte("parent_block_hash_32_bytes_long!"),
				ParentBlockRoot:        []byte("parent_block_root_32_bytes_long!"),
				BlockHash:              []byte("block_hash_32_bytes_are_long!!"),
				FeeRecipient:           []byte("fee_recipient_20_byt"),
				GasLimit:               15000000,
				BuilderIndex:           primitives.ValidatorIndex(42),
				Slot:                   primitives.Slot(12345),
				Value:                  1000000000000000000,
				BlobKzgCommitmentsRoot: []byte("blob_kzg_commitments_32_bytes!!"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.bid.Copy()
			if tt.bid == nil {
				if copied != nil {
					t.Errorf("Copy() of nil should return nil, got %v", copied)
				}
				return
			}

			if !reflect.DeepEqual(tt.bid, copied) {
				t.Errorf("Copy() = %v, want %v", copied, tt.bid)
			}

			if len(tt.bid.ParentBlockHash) > 0 {
				tt.bid.ParentBlockHash[0] = 0xFF
				if copied.ParentBlockHash[0] == 0xFF {
					t.Error("Copy() did not create deep copy of ParentBlockHash")
				}
			}
		})
	}
}

func TestBuilderPendingWithdrawal_Copy(t *testing.T) {
	tests := []struct {
		name       string
		withdrawal *BuilderPendingWithdrawal
	}{
		{
			name:       "nil withdrawal",
			withdrawal: nil,
		},
		{
			name:       "empty withdrawal",
			withdrawal: &BuilderPendingWithdrawal{},
		},
		{
			name: "fully populated withdrawal",
			withdrawal: &BuilderPendingWithdrawal{
				FeeRecipient:      []byte("fee_recipient_20_byt"),
				Amount:            primitives.Gwei(5000000000),
				BuilderIndex:      primitives.ValidatorIndex(123),
				WithdrawableEpoch: primitives.Epoch(456),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.withdrawal.Copy()
			if tt.withdrawal == nil {
				if copied != nil {
					t.Errorf("Copy() of nil should return nil, got %v", copied)
				}
				return
			}

			if !reflect.DeepEqual(tt.withdrawal, copied) {
				t.Errorf("Copy() = %v, want %v", copied, tt.withdrawal)
			}

			// Verify deep copy by modifying original
			if len(tt.withdrawal.FeeRecipient) > 0 {
				tt.withdrawal.FeeRecipient[0] = 0xFF
				if copied.FeeRecipient[0] == 0xFF {
					t.Error("Copy() did not create deep copy of FeeRecipient")
				}
			}
		})
	}
}

func TestBuilderPendingPayment_Copy(t *testing.T) {
	tests := []struct {
		name    string
		payment *BuilderPendingPayment
	}{
		{
			name:    "nil payment",
			payment: nil,
		},
		{
			name:    "empty payment",
			payment: &BuilderPendingPayment{},
		},
		{
			name: "payment with nil withdrawal",
			payment: &BuilderPendingPayment{
				Weight:     primitives.Gwei(1000),
				Withdrawal: nil,
			},
		},
		{
			name: "fully populated payment",
			payment: &BuilderPendingPayment{
				Weight: primitives.Gwei(2500),
				Withdrawal: &BuilderPendingWithdrawal{
					FeeRecipient:      []byte("test_recipient_20byt"),
					Amount:            primitives.Gwei(10000),
					BuilderIndex:      primitives.ValidatorIndex(789),
					WithdrawableEpoch: primitives.Epoch(999),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.payment.Copy()
			if tt.payment == nil {
				if copied != nil {
					t.Errorf("Copy() of nil should return nil, got %v", copied)
				}
				return
			}

			if !reflect.DeepEqual(tt.payment, copied) {
				t.Errorf("Copy() = %v, want %v", copied, tt.payment)
			}

			if tt.payment.Withdrawal != nil && len(tt.payment.Withdrawal.FeeRecipient) > 0 {
				tt.payment.Withdrawal.FeeRecipient[0] = 0xFF
				if copied.Withdrawal != nil && len(copied.Withdrawal.FeeRecipient) > 0 && copied.Withdrawal.FeeRecipient[0] == 0xFF {
					t.Error("Copy() did not create deep copy of nested Withdrawal.FeeRecipient")
				}
			}
		})
	}
}
