package structs

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestSignedBLSToExecutionChange_ToConsensus(t *testing.T) {
	s := &SignedBLSToExecutionChange{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSignedValidatorRegistration_ToConsensus(t *testing.T) {
	s := &SignedValidatorRegistration{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSignedContributionAndProof_ToConsensus(t *testing.T) {
	s := &SignedContributionAndProof{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestContributionAndProof_ToConsensus(t *testing.T) {
	c := &ContributionAndProof{
		Contribution:    nil,
		AggregatorIndex: "invalid",
		SelectionProof:  "",
	}
	_, err := c.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSignedAggregateAttestationAndProof_ToConsensus(t *testing.T) {
	s := &SignedAggregateAttestationAndProof{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestAggregateAttestationAndProof_ToConsensus(t *testing.T) {
	a := &AggregateAttestationAndProof{
		AggregatorIndex: "1",
		Aggregate:       nil,
		SelectionProof:  "",
	}
	_, err := a.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestAttestation_ToConsensus(t *testing.T) {
	a := &Attestation{
		AggregationBits: "0x10",
		Data:            nil,
		Signature:       "",
	}
	_, err := a.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSingleAttestation_ToConsensus(t *testing.T) {
	s := &SingleAttestation{
		CommitteeIndex: "1",
		AttesterIndex:  "1",
		Data:           nil,
		Signature:      "",
	}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestSignedVoluntaryExit_ToConsensus(t *testing.T) {
	s := &SignedVoluntaryExit{Message: nil, Signature: ""}
	_, err := s.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestProposerSlashing_ToConsensus(t *testing.T) {
	p := &ProposerSlashing{SignedHeader1: nil, SignedHeader2: nil}
	_, err := p.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestProposerSlashing_FromConsensus(t *testing.T) {
	input := []*eth.ProposerSlashing{
		{
			Header_1: &eth.SignedBeaconBlockHeader{
				Header: &eth.BeaconBlockHeader{
					Slot:          1,
					ProposerIndex: 2,
					ParentRoot:    []byte{3},
					StateRoot:     []byte{4},
					BodyRoot:      []byte{5},
				},
				Signature: []byte{6},
			},
			Header_2: &eth.SignedBeaconBlockHeader{
				Header: &eth.BeaconBlockHeader{
					Slot:          7,
					ProposerIndex: 8,
					ParentRoot:    []byte{9},
					StateRoot:     []byte{10},
					BodyRoot:      []byte{11},
				},
				Signature: []byte{12},
			},
		},
		{
			Header_1: &eth.SignedBeaconBlockHeader{
				Header: &eth.BeaconBlockHeader{
					Slot:          13,
					ProposerIndex: 14,
					ParentRoot:    []byte{15},
					StateRoot:     []byte{16},
					BodyRoot:      []byte{17},
				},
				Signature: []byte{18},
			},
			Header_2: &eth.SignedBeaconBlockHeader{
				Header: &eth.BeaconBlockHeader{
					Slot:          19,
					ProposerIndex: 20,
					ParentRoot:    []byte{21},
					StateRoot:     []byte{22},
					BodyRoot:      []byte{23},
				},
				Signature: []byte{24},
			},
		},
	}

	expectedResult := []*ProposerSlashing{
		{
			SignedHeader1: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          "1",
					ProposerIndex: "2",
					ParentRoot:    hexutil.Encode([]byte{3}),
					StateRoot:     hexutil.Encode([]byte{4}),
					BodyRoot:      hexutil.Encode([]byte{5}),
				},
				Signature: hexutil.Encode([]byte{6}),
			},
			SignedHeader2: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          "7",
					ProposerIndex: "8",
					ParentRoot:    hexutil.Encode([]byte{9}),
					StateRoot:     hexutil.Encode([]byte{10}),
					BodyRoot:      hexutil.Encode([]byte{11}),
				},
				Signature: hexutil.Encode([]byte{12}),
			},
		},
		{
			SignedHeader1: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          "13",
					ProposerIndex: "14",
					ParentRoot:    hexutil.Encode([]byte{15}),
					StateRoot:     hexutil.Encode([]byte{16}),
					BodyRoot:      hexutil.Encode([]byte{17}),
				},
				Signature: hexutil.Encode([]byte{18}),
			},
			SignedHeader2: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          "19",
					ProposerIndex: "20",
					ParentRoot:    hexutil.Encode([]byte{21}),
					StateRoot:     hexutil.Encode([]byte{22}),
					BodyRoot:      hexutil.Encode([]byte{23}),
				},
				Signature: hexutil.Encode([]byte{24}),
			},
		},
	}

	result := ProposerSlashingsFromConsensus(input)
	assert.DeepEqual(t, expectedResult, result)
}

func TestAttesterSlashing_ToConsensus(t *testing.T) {
	a := &AttesterSlashing{Attestation1: nil, Attestation2: nil}
	_, err := a.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestAttesterSlashing_FromConsensus(t *testing.T) {
	input := []*eth.AttesterSlashing{
		{
			Attestation_1: &eth.IndexedAttestation{
				AttestingIndices: []uint64{1, 2},
				Data: &eth.AttestationData{
					Slot:            3,
					Index:           4,
					BeaconBlockRoot: []byte{5},
					Source: &eth.Checkpoint{
						Epoch: 6,
						Root:  []byte{7},
					},
					Target: &eth.Checkpoint{
						Epoch: 8,
						Root:  []byte{9},
					},
				},
				Signature: []byte{10},
			},
			Attestation_2: &eth.IndexedAttestation{
				AttestingIndices: []uint64{11, 12},
				Data: &eth.AttestationData{
					Slot:            13,
					Index:           14,
					BeaconBlockRoot: []byte{15},
					Source: &eth.Checkpoint{
						Epoch: 16,
						Root:  []byte{17},
					},
					Target: &eth.Checkpoint{
						Epoch: 18,
						Root:  []byte{19},
					},
				},
				Signature: []byte{20},
			},
		},
		{
			Attestation_1: &eth.IndexedAttestation{
				AttestingIndices: []uint64{21, 22},
				Data: &eth.AttestationData{
					Slot:            23,
					Index:           24,
					BeaconBlockRoot: []byte{25},
					Source: &eth.Checkpoint{
						Epoch: 26,
						Root:  []byte{27},
					},
					Target: &eth.Checkpoint{
						Epoch: 28,
						Root:  []byte{29},
					},
				},
				Signature: []byte{30},
			},
			Attestation_2: &eth.IndexedAttestation{
				AttestingIndices: []uint64{31, 32},
				Data: &eth.AttestationData{
					Slot:            33,
					Index:           34,
					BeaconBlockRoot: []byte{35},
					Source: &eth.Checkpoint{
						Epoch: 36,
						Root:  []byte{37},
					},
					Target: &eth.Checkpoint{
						Epoch: 38,
						Root:  []byte{39},
					},
				},
				Signature: []byte{40},
			},
		},
	}

	expectedResult := []*AttesterSlashing{
		{
			Attestation1: &IndexedAttestation{
				AttestingIndices: []string{"1", "2"},
				Data: &AttestationData{
					Slot:            "3",
					Index:           "4",
					BeaconBlockRoot: hexutil.Encode([]byte{5}),
					Source: &Checkpoint{
						Epoch: "6",
						Root:  hexutil.Encode([]byte{7}),
					},
					Target: &Checkpoint{
						Epoch: "8",
						Root:  hexutil.Encode([]byte{9}),
					},
				},
				Signature: hexutil.Encode([]byte{10}),
			},
			Attestation2: &IndexedAttestation{
				AttestingIndices: []string{"11", "12"},
				Data: &AttestationData{
					Slot:            "13",
					Index:           "14",
					BeaconBlockRoot: hexutil.Encode([]byte{15}),
					Source: &Checkpoint{
						Epoch: "16",
						Root:  hexutil.Encode([]byte{17}),
					},
					Target: &Checkpoint{
						Epoch: "18",
						Root:  hexutil.Encode([]byte{19}),
					},
				},
				Signature: hexutil.Encode([]byte{20}),
			},
		},
		{
			Attestation1: &IndexedAttestation{
				AttestingIndices: []string{"21", "22"},
				Data: &AttestationData{
					Slot:            "23",
					Index:           "24",
					BeaconBlockRoot: hexutil.Encode([]byte{25}),
					Source: &Checkpoint{
						Epoch: "26",
						Root:  hexutil.Encode([]byte{27}),
					},
					Target: &Checkpoint{
						Epoch: "28",
						Root:  hexutil.Encode([]byte{29}),
					},
				},
				Signature: hexutil.Encode([]byte{30}),
			},
			Attestation2: &IndexedAttestation{
				AttestingIndices: []string{"31", "32"},
				Data: &AttestationData{
					Slot:            "33",
					Index:           "34",
					BeaconBlockRoot: hexutil.Encode([]byte{35}),
					Source: &Checkpoint{
						Epoch: "36",
						Root:  hexutil.Encode([]byte{37}),
					},
					Target: &Checkpoint{
						Epoch: "38",
						Root:  hexutil.Encode([]byte{39}),
					},
				},
				Signature: hexutil.Encode([]byte{40}),
			},
		},
	}

	result := AttesterSlashingsFromConsensus(input)
	assert.DeepEqual(t, expectedResult, result)
}

func TestIndexedAttestation_ToConsensus(t *testing.T) {
	a := &IndexedAttestation{
		AttestingIndices: []string{"1"},
		Data:             nil,
		Signature:        "invalid",
	}
	_, err := a.ToConsensus()
	require.ErrorContains(t, errNilValue.Error(), err)
}

func TestROExecutionPayloadBidFromConsensus(t *testing.T) {
	t.Run("empty blobkzg commitments", func(t *testing.T) {
		bid := &eth.ExecutionPayloadBid{
			ParentBlockHash:    bytes.Repeat([]byte{0x01}, 32),
			ParentBlockRoot:    bytes.Repeat([]byte{0x02}, 32),
			BlockHash:          bytes.Repeat([]byte{0x03}, 32),
			PrevRandao:         bytes.Repeat([]byte{0x04}, 32),
			FeeRecipient:       bytes.Repeat([]byte{0x05}, 20),
			GasLimit:           100,
			BuilderIndex:       7,
			Slot:               9,
			Value:              11,
			ExecutionPayment:   22,
			BlobKzgCommitments: [][]byte{},
		}
		roBid, err := blocks.WrappedROExecutionPayloadBid(bid)
		require.NoError(t, err)

		got := ROExecutionPayloadBidFromConsensus(roBid)
		want := &ExecutionPayloadBid{
			ParentBlockHash:    hexutil.Encode(bid.ParentBlockHash),
			ParentBlockRoot:    hexutil.Encode(bid.ParentBlockRoot),
			BlockHash:          hexutil.Encode(bid.BlockHash),
			PrevRandao:         hexutil.Encode(bid.PrevRandao),
			FeeRecipient:       hexutil.Encode(bid.FeeRecipient),
			GasLimit:           "100",
			BuilderIndex:       "7",
			Slot:               "9",
			Value:              "11",
			ExecutionPayment:   "22",
			BlobKzgCommitments: []string{},
		}
		assert.DeepEqual(t, want, got)
	})

	t.Run("default", func(t *testing.T) {
		bid := &eth.ExecutionPayloadBid{
			ParentBlockHash:    bytes.Repeat([]byte{0x01}, 32),
			ParentBlockRoot:    bytes.Repeat([]byte{0x02}, 32),
			BlockHash:          bytes.Repeat([]byte{0x03}, 32),
			PrevRandao:         bytes.Repeat([]byte{0x04}, 32),
			FeeRecipient:       bytes.Repeat([]byte{0x05}, 20),
			GasLimit:           100,
			BuilderIndex:       7,
			Slot:               9,
			Value:              11,
			ExecutionPayment:   22,
			BlobKzgCommitments: [][]byte{bytes.Repeat([]byte{0x06}, 48)},
		}
		roBid, err := blocks.WrappedROExecutionPayloadBid(bid)
		require.NoError(t, err)

		var bkcs []string
		for _, commitment := range roBid.BlobKzgCommitments() {
			bkcs = append(bkcs, hexutil.Encode(commitment))
		}

		got := ROExecutionPayloadBidFromConsensus(roBid)
		want := &ExecutionPayloadBid{
			ParentBlockHash:    hexutil.Encode(bid.ParentBlockHash),
			ParentBlockRoot:    hexutil.Encode(bid.ParentBlockRoot),
			BlockHash:          hexutil.Encode(bid.BlockHash),
			PrevRandao:         hexutil.Encode(bid.PrevRandao),
			FeeRecipient:       hexutil.Encode(bid.FeeRecipient),
			GasLimit:           "100",
			BuilderIndex:       "7",
			Slot:               "9",
			Value:              "11",
			ExecutionPayment:   "22",
			BlobKzgCommitments: bkcs,
		}
		assert.DeepEqual(t, want, got)
	})
}

func TestBuilderConversionsFromConsensus(t *testing.T) {
	builder := &eth.Builder{
		Pubkey:            bytes.Repeat([]byte{0xAA}, 48),
		Version:           bytes.Repeat([]byte{0x01}, 4),
		ExecutionAddress:  bytes.Repeat([]byte{0xBB}, 20),
		Balance:           42,
		DepositEpoch:      3,
		WithdrawableEpoch: 4,
	}
	wantBuilder := &Builder{
		Pubkey:            hexutil.Encode(builder.Pubkey),
		Version:           hexutil.Encode(builder.Version),
		ExecutionAddress:  hexutil.Encode(builder.ExecutionAddress),
		Balance:           "42",
		DepositEpoch:      "3",
		WithdrawableEpoch: "4",
	}

	assert.DeepEqual(t, wantBuilder, BuilderFromConsensus(builder))
	assert.DeepEqual(t, []*Builder{wantBuilder}, BuildersFromConsensus([]*eth.Builder{builder}))
}

func TestBuilderPendingPaymentConversionsFromConsensus(t *testing.T) {
	withdrawal := &eth.BuilderPendingWithdrawal{
		FeeRecipient: bytes.Repeat([]byte{0x10}, 20),
		Amount:       15,
		BuilderIndex: 2,
	}
	payment := &eth.BuilderPendingPayment{
		Weight:     5,
		Withdrawal: withdrawal,
	}
	wantWithdrawal := &BuilderPendingWithdrawal{
		FeeRecipient: hexutil.Encode(withdrawal.FeeRecipient),
		Amount:       "15",
		BuilderIndex: "2",
	}
	wantPayment := &BuilderPendingPayment{
		Weight:     "5",
		Withdrawal: wantWithdrawal,
	}

	assert.DeepEqual(t, wantPayment, BuilderPendingPaymentFromConsensus(payment))
	assert.DeepEqual(t, []*BuilderPendingPayment{wantPayment}, BuilderPendingPaymentsFromConsensus([]*eth.BuilderPendingPayment{payment}))
	assert.DeepEqual(t, wantWithdrawal, BuilderPendingWithdrawalFromConsensus(withdrawal))
	assert.DeepEqual(t, []*BuilderPendingWithdrawal{wantWithdrawal}, BuilderPendingWithdrawalsFromConsensus([]*eth.BuilderPendingWithdrawal{withdrawal}))
}

func TestBeaconStateGloasFromConsensus(t *testing.T) {
	st, err := util.NewBeaconStateGloas(func(state *eth.BeaconStateGloas) error {
		state.GenesisTime = 123
		state.GenesisValidatorsRoot = bytes.Repeat([]byte{0x10}, 32)
		state.Slot = 5
		state.ProposerLookahead = []uint64{1, 2}
		state.LatestExecutionPayloadBid = &eth.ExecutionPayloadBid{
			ParentBlockHash:    bytes.Repeat([]byte{0x11}, 32),
			ParentBlockRoot:    bytes.Repeat([]byte{0x12}, 32),
			BlockHash:          bytes.Repeat([]byte{0x13}, 32),
			PrevRandao:         bytes.Repeat([]byte{0x14}, 32),
			FeeRecipient:       bytes.Repeat([]byte{0x15}, 20),
			GasLimit:           64,
			BuilderIndex:       3,
			Slot:               5,
			Value:              99,
			ExecutionPayment:   7,
			BlobKzgCommitments: [][]byte{bytes.Repeat([]byte{0x16}, 48)},
		}
		state.Builders = []*eth.Builder{
			{
				Pubkey:            bytes.Repeat([]byte{0x20}, 48),
				Version:           bytes.Repeat([]byte{0x21}, 4),
				ExecutionAddress:  bytes.Repeat([]byte{0x22}, 20),
				Balance:           88,
				DepositEpoch:      1,
				WithdrawableEpoch: 2,
			},
		}
		state.NextWithdrawalBuilderIndex = 9
		state.ExecutionPayloadAvailability = []byte{0x01, 0x02}
		state.BuilderPendingPayments = []*eth.BuilderPendingPayment{
			{
				Weight: 3,
				Withdrawal: &eth.BuilderPendingWithdrawal{
					FeeRecipient: bytes.Repeat([]byte{0x23}, 20),
					Amount:       4,
					BuilderIndex: 5,
				},
			},
		}
		state.BuilderPendingWithdrawals = []*eth.BuilderPendingWithdrawal{
			{
				FeeRecipient: bytes.Repeat([]byte{0x24}, 20),
				Amount:       6,
				BuilderIndex: 7,
			},
		}
		state.LatestBlockHash = bytes.Repeat([]byte{0x25}, 32)
		state.PayloadExpectedWithdrawals = []*enginev1.Withdrawal{
			{Index: 1, ValidatorIndex: 2, Address: bytes.Repeat([]byte{0x26}, 20), Amount: 10},
		}
		return nil
	})
	require.NoError(t, err)

	got, err := BeaconStateGloasFromConsensus(st)
	require.NoError(t, err)

	require.Equal(t, "123", got.GenesisTime)
	require.Equal(t, hexutil.Encode(bytes.Repeat([]byte{0x10}, 32)), got.GenesisValidatorsRoot)
	require.Equal(t, "5", got.Slot)
	require.DeepEqual(t, []string{"1", "2"}, got.ProposerLookahead)
	require.Equal(t, "9", got.NextWithdrawalBuilderIndex)
	require.Equal(t, hexutil.Encode([]byte{0x01, 0x02}), got.ExecutionPayloadAvailability)
	require.Equal(t, hexutil.Encode(bytes.Repeat([]byte{0x25}, 32)), got.LatestBlockHash)

	require.NotNil(t, got.LatestExecutionPayloadBid)
	require.Equal(t, "64", got.LatestExecutionPayloadBid.GasLimit)
	require.Equal(t, hexutil.Encode(bytes.Repeat([]byte{0x11}, 32)), got.LatestExecutionPayloadBid.ParentBlockHash)

	require.NotNil(t, got.Builders)
	require.Equal(t, hexutil.Encode(bytes.Repeat([]byte{0x20}, 48)), got.Builders[0].Pubkey)
	require.Equal(t, "88", got.Builders[0].Balance)

	require.Equal(t, "3", got.BuilderPendingPayments[0].Weight)
	require.Equal(t, "4", got.BuilderPendingPayments[0].Withdrawal.Amount)
	require.Equal(t, hexutil.Encode(bytes.Repeat([]byte{0x23}, 20)), got.BuilderPendingPayments[0].Withdrawal.FeeRecipient)

	require.Equal(t, "6", got.BuilderPendingWithdrawals[0].Amount)
	require.Equal(t, hexutil.Encode(bytes.Repeat([]byte{0x24}, 20)), got.BuilderPendingWithdrawals[0].FeeRecipient)

	require.Equal(t, "1", got.PayloadExpectedWithdrawals[0].WithdrawalIndex)
	require.Equal(t, "2", got.PayloadExpectedWithdrawals[0].ValidatorIndex)
	require.Equal(t, hexutil.Encode(bytes.Repeat([]byte{0x26}, 20)), got.PayloadExpectedWithdrawals[0].ExecutionAddress)
	require.Equal(t, "10", got.PayloadExpectedWithdrawals[0].Amount)
}
