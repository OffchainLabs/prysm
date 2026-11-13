package transition_test

import (
	"context"
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
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

// TestOperations_SlashingAndExitEpochsPostElectra checks that a slashing and a voluntary
// exit get the exit epochs from the state churn without an exit info.
func TestOperations_SlashingAndExitEpochsPostElectra(t *testing.T) {
	cfg := params.BeaconConfig()
	tests := []struct {
		name    string
		genesis func(testing.TB, uint64) (state.BeaconState, []bls.SecretKey)
		process func(context.Context, state.BeaconState, *ethpb.ProposerSlashing, *ethpb.SignedVoluntaryExit) (state.BeaconState, error)
		// Exit epochs of slashed validator 0 and exited validator 1.
		wantEpochs [2]primitives.Epoch
	}{
		{
			name: "electra",
			genesis: func(t testing.TB, n uint64) (state.BeaconState, []bls.SecretKey) {
				return util.DeterministicGenesisStateElectra(t, n)
			},
			process: func(ctx context.Context, st state.BeaconState, ps *ethpb.ProposerSlashing, exit *ethpb.SignedVoluntaryExit) (state.BeaconState, error) {
				blk := util.NewBeaconBlockElectra()
				blk.Block.Slot = st.Slot()
				blk.Block.Body.ProposerSlashings = []*ethpb.ProposerSlashing{ps}
				blk.Block.Body.VoluntaryExits = []*ethpb.SignedVoluntaryExit{exit}
				b, err := blocks.NewSignedBeaconBlock(blk)
				if err != nil {
					return nil, err
				}
				return transition.ElectraOperations(ctx, st, b.Block())
			},
			// Churn: min(256 ETH, 12,800,000 ETH / 65,536) = 195 ETH.
			// 261 + ceil((100,000 - 195) / 195) = 773, and 35 ETH of churn is left.
			// 773 + ceil((100,000 - 35) / 195) = 1286.
			wantEpochs: [2]primitives.Epoch{773, 1286},
		},
		{
			name:    "gloas",
			genesis: util.DeterministicGenesisStateGloas,
			process: func(ctx context.Context, st state.BeaconState, ps *ethpb.ProposerSlashing, exit *ethpb.SignedVoluntaryExit) (state.BeaconState, error) {
				blk := util.NewBeaconBlockGloas()
				blk.Block.Slot = st.Slot()
				blk.Block.Body.ProposerSlashings = []*ethpb.ProposerSlashing{ps}
				blk.Block.Body.VoluntaryExits = []*ethpb.SignedVoluntaryExit{exit}
				b, err := blocks.NewSignedBeaconBlock(blk)
				if err != nil {
					return nil, err
				}
				return transition.GloasOperations(ctx, st, b.Block(), st.Slot()-1)
			},
			// EIP-8061 churn: 12,800,000 ETH / 32,768 = 390 ETH, with no 256 ETH cap.
			// 261 + ceil((100,000 - 390) / 390) = 517, and 230 ETH of churn is left.
			// 517 + ceil((100,000 - 230) / 390) = 773.
			wantEpochs: [2]primitives.Epoch{517, 773},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helpers.ClearCache()
			t.Cleanup(helpers.ClearCache)

			st, ks := tt.genesis(t, 128)
			require.NoError(t, st.SetSlot(primitives.Slot(cfg.ShardCommitteePeriod)*cfg.SlotsPerEpoch))
			// Make the total active balance 12,800,000 ETH, so the exit churn is above its minimum.
			// Then a wrong total active balance gives different exit epochs.
			vals := st.Validators()
			for _, val := range vals {
				val.EffectiveBalance = 100_000 * cfg.GweiPerEth
			}
			require.NoError(t, st.SetValidators(vals))

			ps, err := util.GenerateProposerSlashingForValidator(st, ks[0], 0)
			require.NoError(t, err)
			exit := &ethpb.SignedVoluntaryExit{Exit: &ethpb.VoluntaryExit{Epoch: time.CurrentEpoch(st), ValidatorIndex: 1}}
			// EIP-7044: exits are signed with the Capella fork version.
			capellaFork := &ethpb.Fork{CurrentVersion: cfg.CapellaForkVersion, PreviousVersion: cfg.CapellaForkVersion}
			domain, err := signing.Domain(capellaFork, exit.Exit.Epoch, cfg.DomainVoluntaryExit, st.GenesisValidatorsRoot())
			require.NoError(t, err)
			sr, err := signing.ComputeSigningRoot(exit.Exit, domain)
			require.NoError(t, err)
			exit.Signature = ks[1].Sign(sr[:]).Marshal()

			st, err = tt.process(t.Context(), st, ps, exit)
			require.NoError(t, err)

			// The earliest exit epoch is 256 + 1 + MAX_SEED_LOOKAHEAD = 261.
			for idx, want := range tt.wantEpochs {
				val, err := st.ValidatorAtIndexReadOnly(primitives.ValidatorIndex(idx))
				require.NoError(t, err)
				require.Equal(t, want, val.ExitEpoch(), "validator %d", idx)
			}
		})
	}
}
