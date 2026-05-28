package transition_test

import (
	"context"
	"errors"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func newGloasBlock(t *testing.T, body *ethpb.BeaconBlockBodyGloas) interfaces.ReadOnlyBeaconBlock {
	t.Helper()
	hydrated := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{Body: body},
	})
	signed, err := blocks.NewSignedBeaconBlock(hydrated)
	require.NoError(t, err)
	return signed.Block()
}

func emptyGloasBody() *ethpb.BeaconBlockBodyGloas {
	return util.HydrateBeaconBlockBodyGloas(nil)
}

func TestGloasOperations_HappyPath(t *testing.T) {
	st, _ := util.DeterministicGenesisStateElectra(t, 16)
	// A plain Electra state is fine here because we exercise zero operations.
	blk := newGloasBlock(t, emptyGloasBody())

	_, err := transition.GloasOperations(context.Background(), st, blk)
	require.NoError(t, err)
}

// TestGloasOperations_ProcessingErrors covers every sentinel error the
// function can return, one sub-test per operation step.
func TestGloasOperations_ProcessingErrors(t *testing.T) {
	tests := []struct {
		name        string
		modifyBlk   func(*ethpb.BeaconBlockBodyGloas)
		errSentinel error
		errSubstr   string
	}{
		{
			name: "ErrProcessProposerSlashingsFailed – out-of-bounds proposer index",
			modifyBlk: func(b *ethpb.BeaconBlockBodyGloas) {
				b.ProposerSlashings = []*ethpb.ProposerSlashing{
					{
						Header_1: &ethpb.SignedBeaconBlockHeader{
							Header: &ethpb.BeaconBlockHeader{
								Slot:          1,
								ProposerIndex: 999999,
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
			errSentinel: transition.ErrProcessProposerSlashingsFailed,
			errSubstr:   "process proposer slashings failed",
		},
		{
			name: "ErrProcessAttesterSlashingsFailed – out-of-bounds attesting index",
			modifyBlk: func(b *ethpb.BeaconBlockBodyGloas) {
				makeIndexed := func(root []byte) *ethpb.IndexedAttestationElectra {
					return &ethpb.IndexedAttestationElectra{
						AttestingIndices: []uint64{999999},
						Data: &ethpb.AttestationData{
							Slot:            1,
							CommitteeIndex:  0,
							BeaconBlockRoot: root,
							Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
							Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
						},
						Signature: make([]byte, 96),
					}
				}
				root1 := make([]byte, 32)
				root2 := make([]byte, 32)
				root2[0] = 0xff // different roots → slashable
				b.AttesterSlashings = []*ethpb.AttesterSlashingElectra{
					{
						Attestation_1: makeIndexed(root1),
						Attestation_2: makeIndexed(root2),
					},
				}
			},
			errSentinel: transition.ErrProcessAttesterSlashingsFailed,
			errSubstr:   "process attester slashings failed",
		},

		{
			name: "ErrProcessAttestationsFailed – invalid committee index",
			modifyBlk: func(b *ethpb.BeaconBlockBodyGloas) {
				b.Attestations = []*ethpb.AttestationElectra{
					{
						AggregationBits: []byte{0b00000001},
						Data: &ethpb.AttestationData{
							Slot:            1,
							CommitteeIndex:  999999, // no such committee
							BeaconBlockRoot: make([]byte, 32),
							Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
							Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
						},
						CommitteeBits: []byte{0b00000001},
						Signature:     make([]byte, 96),
					},
				}
			},
			errSentinel: transition.ErrProcessAttestationsFailed,
			errSubstr:   "process attestations failed",
		},

		{
			// Legacy eth1 deposits are rejected post-Fulu (consensus-specs#4704).
			name: "ErrProcessDepositsFailed – legacy deposits rejected",
			modifyBlk: func(b *ethpb.BeaconBlockBodyGloas) {
				b.Deposits = []*ethpb.Deposit{
					{
						Proof: make([][]byte, 33),
						Data: &ethpb.Deposit_Data{
							PublicKey:             make([]byte, 48),
							WithdrawalCredentials: make([]byte, 32),
							Amount:                32_000_000_000,
							Signature:             make([]byte, 96),
						},
					},
				}
			},
			errSentinel: transition.ErrProcessDepositsFailed,
			errSubstr:   "legacy deposits not allowed post-Fulu",
		},

		{
			name: "ErrProcessVoluntaryExitsFailed – out-of-bounds validator index",
			modifyBlk: func(b *ethpb.BeaconBlockBodyGloas) {
				b.VoluntaryExits = []*ethpb.SignedVoluntaryExit{
					{
						Exit: &ethpb.VoluntaryExit{
							Epoch:          0,
							ValidatorIndex: 999999,
						},
						Signature: make([]byte, 96),
					},
				}
			},
			errSentinel: transition.ErrProcessVoluntaryExitsFailed,
			errSubstr:   "process voluntary exits failed",
		},

		{
			name: "ErrProcessBLSChangesFailed – out-of-bounds validator index",
			modifyBlk: func(b *ethpb.BeaconBlockBodyGloas) {
				b.BlsToExecutionChanges = []*ethpb.SignedBLSToExecutionChange{
					{
						Message: &ethpb.BLSToExecutionChange{
							ValidatorIndex:     999999,
							FromBlsPubkey:      make([]byte, 48),
							ToExecutionAddress: make([]byte, 20),
						},
						Signature: make([]byte, 96),
					},
				}
			},
			errSentinel: transition.ErrProcessBLSChangesFailed,
			errSubstr:   "process BLS to execution changes failed",
		},

		{
			name: "ErrProcessPayloadAttestationsFailed – wrong beacon block root",
			modifyBlk: func(b *ethpb.BeaconBlockBodyGloas) {
				b.PayloadAttestations = []*ethpb.PayloadAttestation{
					{
						AggregationBits: bitfield.NewBitvector512(),
						Data: &ethpb.PayloadAttestationData{
							BeaconBlockRoot: make([]byte, 32), // all-zeros ≠ header.parent_root
							Slot:            0,
						},
						Signature: make([]byte, 96),
					},
				}
			},
			errSentinel: transition.ErrProcessPayloadAttestationsFailed,
			errSubstr:   "process payload attestations failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			st, _ := util.DeterministicGenesisStateElectra(t, 128)

			// For the payload-attestation sub-test we need the state's latest block
			// header to have a non-zero parent root so the all-zeros root in the
			// attestation definitely mismatches.
			if tc.errSentinel == transition.ErrProcessPayloadAttestationsFailed {
				hdr := &ethpb.BeaconBlockHeader{
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
				}
				hdr.ParentRoot[0] = 0xde
				require.NoError(t, st.SetLatestBlockHeader(hdr))
			}

			body := emptyGloasBody()
			tc.modifyBlk(body)

			gloasBlk := newGloasBlock(t, body)

			_, err := transition.GloasOperations(ctx, st, gloasBlk)
			require.NotNil(t, err, "expected an error but got nil")
			require.ErrorContains(t, tc.errSubstr, err)
			require.Equal(t, true, errors.Is(err, tc.errSentinel))
		})
	}
}
