package eth

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// HTR(blinded) must equal HTR(full) so the validator signature stays valid against either form.
func TestWireBlindedHTRMatchesFull(t *testing.T) {
	full := &ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    bytes.Repeat([]byte{0x01}, 32),
			FeeRecipient:  bytes.Repeat([]byte{0x02}, 20),
			StateRoot:     bytes.Repeat([]byte{0x03}, 32),
			ReceiptsRoot:  bytes.Repeat([]byte{0x04}, 32),
			LogsBloom:     bytes.Repeat([]byte{0x05}, 256),
			PrevRandao:    bytes.Repeat([]byte{0x06}, 32),
			BaseFeePerGas: bytes.Repeat([]byte{0x07}, 32),
			BlockHash:     bytes.Repeat([]byte{0x08}, 32),
			Transactions:  [][]byte{[]byte("tx1"), []byte("tx2")},
			Withdrawals:   []*enginev1.Withdrawal{},
			SlotNumber:    primitives.Slot(100),
		},
		ExecutionRequests:     &enginev1.ExecutionRequests{},
		BuilderIndex:          primitives.BuilderIndex(42),
		BeaconBlockRoot:       bytes.Repeat([]byte{0x09}, 32),
		ParentBeaconBlockRoot: bytes.Repeat([]byte{0x0a}, 32),
	}

	blinded, err := WireBlindedFromFull(full)
	require.NoError(t, err)
	fullHTR, err := full.HashTreeRoot()
	require.NoError(t, err)
	blindedHTR, err := blinded.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, fullHTR, blindedHTR)

	// SSZ roundtrip.
	enc, err := blinded.MarshalSSZ()
	require.NoError(t, err)
	decoded := &WireBlindedExecutionPayloadEnvelope{}
	require.NoError(t, decoded.UnmarshalSSZ(enc))
	rtHTR, err := decoded.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, fullHTR, rtHTR)

	// Signed wrapper SSZ roundtrip.
	signedBlinded, err := SignedWireBlindedFromFull(&SignedExecutionPayloadEnvelope{
		Message:   full,
		Signature: bytes.Repeat([]byte{0x0b}, 96),
	})
	require.NoError(t, err)
	signedEnc, err := signedBlinded.MarshalSSZ()
	require.NoError(t, err)
	decodedSigned := &SignedWireBlindedExecutionPayloadEnvelope{}
	require.NoError(t, decodedSigned.UnmarshalSSZ(signedEnc))
	rtBlindedHTR, err := decodedSigned.Message.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, fullHTR, rtBlindedHTR)
}

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
				ParentBlockHash:    []byte("parent_block_hash_32_bytes_long!"),
				ParentBlockRoot:    []byte("parent_block_root_32_bytes_long!"),
				BlockHash:          []byte("block_hash_32_bytes_are_long!!"),
				PrevRandao:         []byte("prev_randao_32_bytes_long!!!"),
				FeeRecipient:       []byte("fee_recipient_20_byt"),
				GasLimit:           15000000,
				BuilderIndex:       primitives.BuilderIndex(42),
				Slot:               primitives.Slot(12345),
				Value:              1000000000000000000,
				ExecutionPayment:   5645654,
				BlobKzgCommitments: [][]byte{[]byte("blob_kzg_commitments_48_bytes_longer_than_needed")},
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
				FeeRecipient: []byte("fee_recipient_20_byt"),
				Amount:       primitives.Gwei(5000000000),
				BuilderIndex: primitives.BuilderIndex(123),
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
					FeeRecipient: []byte("test_recipient_20byt"),
					Amount:       primitives.Gwei(10000),
					BuilderIndex: primitives.BuilderIndex(789),
				},
				ProposerIndex: primitives.ValidatorIndex(456),
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

			if copied.ProposerIndex != tt.payment.ProposerIndex {
				t.Errorf("Copy() ProposerIndex = %d, want %d", copied.ProposerIndex, tt.payment.ProposerIndex)
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

func TestCopyBuilder(t *testing.T) {
	tests := []struct {
		name    string
		builder *Builder
	}{
		{
			name:    "nil builder",
			builder: nil,
		},
		{
			name:    "empty builder",
			builder: &Builder{},
		},
		{
			name: "fully populated builder",
			builder: &Builder{
				Pubkey:            []byte("pubkey_48_bytes_long_pubkey_48_bytes_long_pubkey_48!"),
				Version:           []byte{'a'},
				ExecutionAddress:  []byte("execution_address_20"),
				Balance:           primitives.Gwei(12345),
				DepositEpoch:      primitives.Epoch(10),
				WithdrawableEpoch: primitives.Epoch(20),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := CopyBuilder(tt.builder)
			if tt.builder == nil {
				if copied != nil {
					t.Errorf("CopyBuilder() of nil should return nil, got %v", copied)
				}
				return
			}

			if !reflect.DeepEqual(tt.builder, copied) {
				t.Errorf("CopyBuilder() = %v, want %v", copied, tt.builder)
			}

			if len(tt.builder.Pubkey) > 0 {
				tt.builder.Pubkey[0] = 0xFF
				if copied.Pubkey[0] == 0xFF {
					t.Error("CopyBuilder() did not create deep copy of Pubkey")
				}
			}

			if len(tt.builder.ExecutionAddress) > 0 {
				tt.builder.ExecutionAddress[0] = 0xFF
				if copied.ExecutionAddress[0] == 0xFF {
					t.Error("CopyBuilder() did not create deep copy of ExecutionAddress")
				}
			}
		})
	}
}
