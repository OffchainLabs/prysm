package transition_test

import (
	"context"
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestProcessOperationsWithNilRequests(t *testing.T) {
	tests := []struct {
		name      string
		modifyBlk func(blockElectra *ethpb.SignedBeaconBlockElectra)
		errMsg    string
	}{
		{
			name: "Nil deposit request",
			modifyBlk: func(blk *ethpb.SignedBeaconBlockElectra) {
				blk.Block.Body.ExecutionRequests.Deposits = []*enginev1.DepositRequest{nil}
			},
			errMsg: "nil deposit request",
		},
		{
			name: "Nil withdrawal request",
			modifyBlk: func(blk *ethpb.SignedBeaconBlockElectra) {
				blk.Block.Body.ExecutionRequests.Withdrawals = []*enginev1.WithdrawalRequest{nil}
			},
			errMsg: "nil withdrawal request",
		},
		{
			name: "Nil consolidation request",
			modifyBlk: func(blk *ethpb.SignedBeaconBlockElectra) {
				blk.Block.Body.ExecutionRequests.Consolidations = []*enginev1.ConsolidationRequest{nil}
			},
			errMsg: "nil consolidation request",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, ks := util.DeterministicGenesisStateElectra(t, 128)
			blk, err := util.GenerateFullBlockElectra(st, ks, util.DefaultBlockGenConfig(), 1)
			require.NoError(t, err)

			tc.modifyBlk(blk)

			b, err := blocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)

			require.NoError(t, st.SetSlot(1))

			_, err = transition.ElectraOperations(t.Context(), st, b.Block())
			require.ErrorContains(t, tc.errMsg, err)
		})
	}
}

func TestElectraOperations_ProcessingErrors(t *testing.T) {
	tests := []struct {
		name      string
		modifyBlk func(blk *ethpb.SignedBeaconBlockElectra)
		errCheck  func(t *testing.T, err error)
	}{
		{
			name: "ErrProcessProposerSlashingsFailed",
			modifyBlk: func(blk *ethpb.SignedBeaconBlockElectra) {
				// Create invalid proposer slashing with out-of-bounds proposer index
				blk.Block.Body.ProposerSlashings = []*ethpb.ProposerSlashing{
					{
						Header_1: &ethpb.SignedBeaconBlockHeader{
							Header: &ethpb.BeaconBlockHeader{
								Slot:          1,
								ProposerIndex: 999999, // Invalid index (out of bounds)
								ParentRoot:    make([]byte, 32),
								StateRoot:     make([]byte, 32),
								BodyRoot:      make([]byte, 32),
							},
							Signature: make([]byte, 96),
						},
						Header_2: &ethpb.SignedBeaconBlockHeader{
							Header: &ethpb.BeaconBlockHeader{
								Slot:          1,
								ProposerIndex: 999999,
								ParentRoot:    make([]byte, 32),
								StateRoot:     make([]byte, 32),
								BodyRoot:      make([]byte, 32),
							},
							Signature: make([]byte, 96),
						},
					},
				}
			},
			errCheck: func(t *testing.T, err error) {
				require.ErrorContains(t, "process proposer slashings failed", err)
				require.Equal(t, true, errors.Is(err, transition.ErrProcessProposerSlashingsFailed))
			},
		},
		{
			name: "ErrProcessAttestationsFailed",
			modifyBlk: func(blk *ethpb.SignedBeaconBlockElectra) {
				// Create attestation with invalid committee index
				blk.Block.Body.Attestations = []*ethpb.AttestationElectra{
					{
						AggregationBits: []byte{0b00000001},
						Data: &ethpb.AttestationData{
							Slot:            1,
							CommitteeIndex:  999999, // Invalid committee index
							BeaconBlockRoot: make([]byte, 32),
							Source: &ethpb.Checkpoint{
								Epoch: 0,
								Root:  make([]byte, 32),
							},
							Target: &ethpb.Checkpoint{
								Epoch: 0,
								Root:  make([]byte, 32),
							},
						},
						CommitteeBits: []byte{0b00000001},
						Signature:     make([]byte, 96),
					},
				}
			},
			errCheck: func(t *testing.T, err error) {
				require.ErrorContains(t, "process attestations failed", err)
				require.Equal(t, true, errors.Is(err, transition.ErrProcessAttestationsFailed))
			},
		},
		{
			name: "ErrProcessDepositsFailed",
			modifyBlk: func(blk *ethpb.SignedBeaconBlockElectra) {
				// Create deposit with invalid proof length
				blk.Block.Body.Deposits = []*ethpb.Deposit{
					{
						Proof: [][]byte{}, // Invalid: empty proof
						Data: &ethpb.Deposit_Data{
							PublicKey:             make([]byte, 48),
							WithdrawalCredentials: make([]byte, 32),
							Amount:                32000000000, // 32 ETH in Gwei
							Signature:             make([]byte, 96),
						},
					},
				}
			},
			errCheck: func(t *testing.T, err error) {
				require.ErrorContains(t, "process deposits failed", err)
				require.Equal(t, true, errors.Is(err, transition.ErrProcessDepositsFailed))
			},
		},
		{
			name: "ErrProcessVoluntaryExitsFailed",
			modifyBlk: func(blk *ethpb.SignedBeaconBlockElectra) {
				// Create voluntary exit with invalid validator index
				blk.Block.Body.VoluntaryExits = []*ethpb.SignedVoluntaryExit{
					{
						Exit: &ethpb.VoluntaryExit{
							Epoch:          0,
							ValidatorIndex: 999999, // Invalid index (out of bounds)
						},
						Signature: make([]byte, 96),
					},
				}
			},
			errCheck: func(t *testing.T, err error) {
				require.ErrorContains(t, "process voluntary exits failed", err)
				require.Equal(t, true, errors.Is(err, transition.ErrProcessVoluntaryExitsFailed))
			},
		},
		{
			name: "ErrProcessBLSChangesFailed",
			modifyBlk: func(blk *ethpb.SignedBeaconBlockElectra) {
				// Create BLS to execution change with invalid validator index
				blk.Block.Body.BlsToExecutionChanges = []*ethpb.SignedBLSToExecutionChange{
					{
						Message: &ethpb.BLSToExecutionChange{
							ValidatorIndex:     999999, // Invalid index (out of bounds)
							FromBlsPubkey:      make([]byte, 48),
							ToExecutionAddress: make([]byte, 20),
						},
						Signature: make([]byte, 96),
					},
				}
			},
			errCheck: func(t *testing.T, err error) {
				require.ErrorContains(t, "process BLS to execution changes failed", err)
				require.Equal(t, true, errors.Is(err, transition.ErrProcessBLSChangesFailed))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st, ks := util.DeterministicGenesisStateElectra(t, 128)
			blk, err := util.GenerateFullBlockElectra(st, ks, util.DefaultBlockGenConfig(), 1)
			require.NoError(t, err)

			tc.modifyBlk(blk)

			b, err := blocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)

			require.NoError(t, st.SetSlot(primitives.Slot(1)))

			_, err = transition.ElectraOperations(ctx, st, b.Block())
			require.NotNil(t, err, "Expected an error but got nil")
			tc.errCheck(t, err)
		})
	}
}
