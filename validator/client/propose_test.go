package client

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	lruwrpr "github.com/OffchainLabs/prysm/v6/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/config/proposer"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	blocktest "github.com/OffchainLabs/prysm/v6/consensus-types/blocks/testing"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	validatormock "github.com/OffchainLabs/prysm/v6/testing/validator-mock"
	testing2 "github.com/OffchainLabs/prysm/v6/validator/db/testing"
	"github.com/OffchainLabs/prysm/v6/validator/graffiti"
	"github.com/ethereum/go-ethereum/common/hexutil"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
)

type mocks struct {
	validatorClient *validatormock.MockValidatorClient
	nodeClient      *validatormock.MockNodeClient
	signfunc        func(context.Context, *validatorpb.SignRequest) (bls.Signature, error)
}

type mockSignature struct{}

func (mockSignature) Verify(bls.PublicKey, []byte) bool {
	return true
}
func (mockSignature) AggregateVerify([]bls.PublicKey, [][32]byte) bool {
	return true
}
func (mockSignature) FastAggregateVerify([]bls.PublicKey, [32]byte) bool {
	return true
}
func (mockSignature) Eth2FastAggregateVerify([]bls.PublicKey, [32]byte) bool {
	return true
}
func (mockSignature) Marshal() []byte {
	return make([]byte, 32)
}
func (m mockSignature) Copy() bls.Signature {
	return m
}

func testKeyFromBytes(t *testing.T, b []byte) keypair {
	pri, err := bls.SecretKeyFromBytes(bytesutil.PadTo(b, 32))
	require.NoError(t, err, "Failed to generate key from bytes")
	return keypair{pub: bytesutil.ToBytes48(pri.PublicKey().Marshal()), pri: pri}
}

// setup sets up a validator instance.
// The `isSlashingProtectionMinimal` flag indicates whether the DB should be instantiated with minimal, filesystem
// slashing protection database.
func setup(t *testing.T, isSlashingProtectionMinimal bool) (*validator, *mocks, bls.SecretKey, func()) {
	validatorKey, err := bls.RandKey()
	require.NoError(t, err)
	return setupWithKey(t, validatorKey, isSlashingProtectionMinimal)
}

// setupWithKey sets up a validator instance with a given key.
// The `isSlashingProtectionMinimal` flag indicates whether the DB should be instantiated with minimal, filesystem
// slashing protection database.
func setupWithKey(t *testing.T, validatorKey bls.SecretKey, isSlashingProtectionMinimal bool) (*validator, *mocks, bls.SecretKey, func()) {
	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	valDB := testing2.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{pubKey}, isSlashingProtectionMinimal)
	ctrl := gomock.NewController(t)
	m := &mocks{
		validatorClient: validatormock.NewMockValidatorClient(ctrl),
		nodeClient:      validatormock.NewMockNodeClient(ctrl),
		signfunc: func(ctx context.Context, req *validatorpb.SignRequest) (bls.Signature, error) {
			return mockSignature{}, nil
		},
	}
	aggregatedSlotCommitteeIDCache := lruwrpr.New(int(params.BeaconConfig().MaxCommitteesPerSlot))

	validator := &validator{
		db:                             valDB,
		km:                             newMockKeymanager(t, keypair{pub: pubKey, pri: validatorKey}),
		validatorClient:                m.validatorClient,
		graffiti:                       []byte{},
		submittedAtts:                  make(map[submittedAttKey]*submittedAtt),
		submittedAggregates:            make(map[submittedAttKey]*submittedAtt),
		aggregatedSlotCommitteeIDCache: aggregatedSlotCommitteeIDCache,
	}

	return validator, m, validatorKey, ctrl.Finish
}

func TestProposeBlock_DoesNotProposeGenesisBlock(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, _, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.ProposeBlock(t.Context(), 0, pubKey)

			require.LogsContain(t, hook, "Assigned to genesis slot, skipping proposal")
		})
	}
}

func TestProposeBlock_DomainDataFailed(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(nil /*response*/, errors.New("uh oh"))

			validator.ProposeBlock(t.Context(), 1, pubKey)
			require.LogsContain(t, hook, "Failed to sign randao reveal")
		})
	}
}

func TestProposeBlock_DomainDataIsNil(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(nil /*response*/, nil)

			validator.ProposeBlock(t.Context(), 1, pubKey)
			require.LogsContain(t, hook, domainDataErr)
		})
	}
}

func TestProposeBlock_RequestBlockFailed(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 2
	cfg.BellatrixForkEpoch = 4
	params.OverrideBeaconConfig(cfg)

	tests := []struct {
		name string
		slot primitives.Slot
	}{
		{
			name: "phase 0",
			slot: 1,
		},
		{
			name: "altair",
			slot: params.BeaconConfig().SlotsPerEpoch.Mul(uint64(cfg.AltairForkEpoch)),
		},
		{
			name: "bellatrix",
			slot: params.BeaconConfig().SlotsPerEpoch.Mul(uint64(cfg.BellatrixForkEpoch)),
		},
	}

	for _, tt := range tests {
		for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
			t.Run(fmt.Sprintf("%s/SlashingProtectionMinimal:%v", tt.name, isSlashingProtectionMinimal), func(t *testing.T) {
				hook := logTest.NewGlobal()
				validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
				defer finish()
				var pubKey [fieldparams.BLSPubkeyLength]byte
				copy(pubKey[:], validatorKey.PublicKey().Marshal())

				m.validatorClient.EXPECT().DomainData(
					gomock.Any(), // ctx
					gomock.Any(), // epoch
				).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

				m.validatorClient.EXPECT().BeaconBlock(
					gomock.Any(), // ctx
					gomock.AssignableToTypeOf(&ethpb.BlockRequest{}),
				).Return(nil /*response*/, errors.New("uh oh"))

				validator.ProposeBlock(t.Context(), tt.slot, pubKey)
				require.LogsContain(t, hook, "Failed to request block from beacon node")
			})
		}
	}
}

func TestProposeBlock_ProposeBlockFailed(t *testing.T) {
	tests := []struct {
		name  string
		block *ethpb.GenericBeaconBlock
	}{
		{
			name: "phase0",
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Phase0{
					Phase0: util.NewBeaconBlock().Block,
				},
			},
		},
		{
			name: "altair",
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Altair{
					Altair: util.NewBeaconBlockAltair().Block,
				},
			},
		},
		{
			name: "bellatrix",
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Bellatrix{
					Bellatrix: util.NewBeaconBlockBellatrix().Block,
				},
			},
		},
	}
	for _, tt := range tests {
		for _, isSlashingProtectionMinimal := range [...]bool{true, true} {
			t.Run(fmt.Sprintf("%s/SlashingProtectionMinimal:%v", tt.name, isSlashingProtectionMinimal), func(t *testing.T) {
				hook := logTest.NewGlobal()
				validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
				defer finish()
				var pubKey [fieldparams.BLSPubkeyLength]byte
				copy(pubKey[:], validatorKey.PublicKey().Marshal())

				m.validatorClient.EXPECT().DomainData(
					gomock.Any(), // ctx
					gomock.Any(), // epoch
				).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

				m.validatorClient.EXPECT().BeaconBlock(
					gomock.Any(), // ctx
					gomock.AssignableToTypeOf(&ethpb.BlockRequest{}),
				).Return(tt.block, nil /*err*/)

				m.validatorClient.EXPECT().DomainData(
					gomock.Any(), // ctx
					gomock.Any(), // epoch
				).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

				m.validatorClient.EXPECT().ProposeBeaconBlock(
					gomock.Any(), // ctx
					gomock.AssignableToTypeOf(&ethpb.GenericSignedBeaconBlock{}),
				).Return(nil /*response*/, errors.New("uh oh"))

				validator.ProposeBlock(t.Context(), 1, pubKey)

				require.LogsContain(t, hook, "Failed to propose block")

			})
		}
	}
}

func TestProposeBlock_BlocksDoubleProposal(t *testing.T) {
	slot := params.BeaconConfig().SlotsPerEpoch.Mul(5).Add(2)
	var blockGraffiti [32]byte
	copy(blockGraffiti[:], "someothergraffiti")

	tests := []struct {
		name   string
		blocks []*ethpb.GenericBeaconBlock
	}{
		{
			name: "phase0",
			blocks: func() []*ethpb.GenericBeaconBlock {
				block0, block1 := util.NewBeaconBlock(), util.NewBeaconBlock()
				block1.Block.Body.Graffiti = blockGraffiti[:]

				var bs []*ethpb.GenericBeaconBlock
				for _, block := range []*ethpb.SignedBeaconBlock{block0, block1} {
					block.Block.Slot = slot
					bs = append(bs, &ethpb.GenericBeaconBlock{
						Block: &ethpb.GenericBeaconBlock_Phase0{
							Phase0: block.Block,
						},
					})
				}
				return bs
			}(),
		},
		{
			name: "altair",
			blocks: func() []*ethpb.GenericBeaconBlock {
				block0, block1 := util.NewBeaconBlockAltair(), util.NewBeaconBlockAltair()
				block1.Block.Body.Graffiti = blockGraffiti[:]

				var bs []*ethpb.GenericBeaconBlock
				for _, block := range []*ethpb.SignedBeaconBlockAltair{block0, block1} {
					block.Block.Slot = slot
					bs = append(bs, &ethpb.GenericBeaconBlock{
						Block: &ethpb.GenericBeaconBlock_Altair{
							Altair: block.Block,
						},
					})
				}
				return bs
			}(),
		},
		{
			name: "bellatrix",
			blocks: func() []*ethpb.GenericBeaconBlock {
				block0, block1 := util.NewBeaconBlockBellatrix(), util.NewBeaconBlockBellatrix()
				block1.Block.Body.Graffiti = blockGraffiti[:]

				var bs []*ethpb.GenericBeaconBlock
				for _, block := range []*ethpb.SignedBeaconBlockBellatrix{block0, block1} {
					block.Block.Slot = slot
					bs = append(bs, &ethpb.GenericBeaconBlock{
						Block: &ethpb.GenericBeaconBlock_Bellatrix{
							Bellatrix: block.Block,
						},
					})
				}
				return bs
			}(),
		},
	}
	for _, tt := range tests {
		for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
			t.Run(fmt.Sprintf("%s/SlashingProtectionMinimal:%v", tt.name, isSlashingProtectionMinimal), func(t *testing.T) {
				hook := logTest.NewGlobal()
				validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
				defer finish()
				var pubKey [fieldparams.BLSPubkeyLength]byte
				copy(pubKey[:], validatorKey.PublicKey().Marshal())

				var dummyRoot [32]byte
				// Save a dummy proposal history at slot 1.
				err := validator.db.SaveProposalHistoryForSlot(t.Context(), pubKey, 1, dummyRoot[:])
				require.NoError(t, err)

				m.validatorClient.EXPECT().DomainData(
					gomock.Any(), // ctx
					gomock.Any(), // epoch
				).Times(1).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

				m.validatorClient.EXPECT().BeaconBlock(
					gomock.Any(), // ctx
					gomock.AssignableToTypeOf(&ethpb.BlockRequest{}),
				).Return(tt.blocks[0], nil /*err*/)

				m.validatorClient.EXPECT().BeaconBlock(
					gomock.Any(), // ctx
					gomock.AssignableToTypeOf(&ethpb.BlockRequest{}),
				).Return(tt.blocks[1], nil /*err*/)

				m.validatorClient.EXPECT().DomainData(
					gomock.Any(), // ctx
					gomock.Any(), // epoch
				).Times(3).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

				m.validatorClient.EXPECT().ProposeBeaconBlock(
					gomock.Any(), // ctx
					gomock.AssignableToTypeOf(&ethpb.GenericSignedBeaconBlock{}),
				).Return(&ethpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil /*error*/)

				validator.ProposeBlock(t.Context(), slot, pubKey)
				require.LogsDoNotContain(t, hook, failedBlockSignLocalErr)

				validator.ProposeBlock(t.Context(), slot, pubKey)
				require.LogsContain(t, hook, failedBlockSignLocalErr)
			})
		}
	}
}

func TestProposeBlock_BlocksDoubleProposal_After54KEpochs(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			var dummyRoot [32]byte
			// Save a dummy proposal history at slot 1.
			err := validator.db.SaveProposalHistoryForSlot(t.Context(), pubKey, 1, dummyRoot[:])
			require.NoError(t, err)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(1).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			testBlock := util.NewBeaconBlock()
			farFuture := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().WeakSubjectivityPeriod + 9))
			testBlock.Block.Slot = farFuture
			m.validatorClient.EXPECT().BeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.BlockRequest{}),
			).Return(&ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Phase0{
					Phase0: testBlock.Block,
				},
			}, nil /*err*/)

			secondTestBlock := util.NewBeaconBlock()
			secondTestBlock.Block.Slot = farFuture
			var blockGraffiti [32]byte
			copy(blockGraffiti[:], "someothergraffiti")
			secondTestBlock.Block.Body.Graffiti = blockGraffiti[:]
			m.validatorClient.EXPECT().BeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.BlockRequest{}),
			).Return(&ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Phase0{
					Phase0: secondTestBlock.Block,
				},
			}, nil /*err*/)
			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(3).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().ProposeBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&ethpb.GenericSignedBeaconBlock{}),
			).Return(&ethpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil /*error*/)

			validator.ProposeBlock(t.Context(), farFuture, pubKey)
			require.LogsDoNotContain(t, hook, failedBlockSignLocalErr)

			validator.ProposeBlock(t.Context(), farFuture, pubKey)
			require.LogsContain(t, hook, failedBlockSignLocalErr)
		})
	}
}

func TestProposeBlock_AllowsOrNotPastProposals(t *testing.T) {
	slot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().WeakSubjectivityPeriod + 9))

	tests := []struct {
		name     string
		pastSlot primitives.Slot
	}{
		{
			name:     "400 slots ago",
			pastSlot: slot.Sub(400),
		},
		{
			name:     "same epoch",
			pastSlot: slot.Sub(4),
		},
	}
	for _, tt := range tests {
		for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
			t.Run(fmt.Sprintf("%s/SlashingProtectionMinimal:%v", tt.name, isSlashingProtectionMinimal), func(t *testing.T) {
				hook := logTest.NewGlobal()
				validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
				defer finish()
				var pubKey [fieldparams.BLSPubkeyLength]byte
				copy(pubKey[:], validatorKey.PublicKey().Marshal())

				// Save a dummy proposal history at slot 0.
				err := validator.db.SaveProposalHistoryForSlot(t.Context(), pubKey, 0, []byte{})
				require.NoError(t, err)

				m.validatorClient.EXPECT().DomainData(
					gomock.Any(), // ctx
					gomock.Any(), // epoch
				).Times(2).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

				blk := util.NewBeaconBlock()
				blk.Block.Slot = slot
				m.validatorClient.EXPECT().BeaconBlock(
					gomock.Any(), // ctx
					gomock.AssignableToTypeOf(&ethpb.BlockRequest{}),
				).Return(&ethpb.GenericBeaconBlock{
					Block: &ethpb.GenericBeaconBlock_Phase0{
						Phase0: blk.Block,
					},
				}, nil /*err*/)

				m.validatorClient.EXPECT().DomainData(
					gomock.Any(), // ctx
					gomock.Any(), // epoch
				).Times(2).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

				proposeBeaconBlockCount := 2
				if isSlashingProtectionMinimal {
					proposeBeaconBlockCount = 1
				}

				m.validatorClient.EXPECT().ProposeBeaconBlock(
					gomock.Any(), // ctx
					gomock.AssignableToTypeOf(&ethpb.GenericSignedBeaconBlock{}),
				).Times(proposeBeaconBlockCount).Return(&ethpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil /*error*/)

				validator.ProposeBlock(t.Context(), slot, pubKey)
				require.LogsDoNotContain(t, hook, failedBlockSignLocalErr)

				blk2 := util.NewBeaconBlock()
				blk2.Block.Slot = tt.pastSlot
				m.validatorClient.EXPECT().BeaconBlock(
					gomock.Any(), // ctx
					gomock.AssignableToTypeOf(&ethpb.BlockRequest{}),
				).Return(&ethpb.GenericBeaconBlock{
					Block: &ethpb.GenericBeaconBlock_Phase0{
						Phase0: blk2.Block,
					},
				}, nil /*err*/)
				validator.ProposeBlock(t.Context(), tt.pastSlot, pubKey)
				if isSlashingProtectionMinimal {
					require.LogsContain(t, hook, failedBlockSignLocalErr)
				} else {
					require.LogsDoNotContain(t, hook, failedBlockSignLocalErr)
				}
			})
		}
	}
}

func TestProposeBlock_BroadcastsBlock(t *testing.T) {
	testProposeBlock(t, make([]byte, 32) /*graffiti*/)
}

func TestProposeBlock_BroadcastsBlock_WithGraffiti(t *testing.T) {
	blockGraffiti := []byte("12345678901234567890123456789012")
	testProposeBlock(t, blockGraffiti)
}

func testProposeBlock(t *testing.T, graffiti []byte) {
	tests := []struct {
		name    string
		block   *ethpb.GenericBeaconBlock
		version int
	}{
		{
			name: "phase0",
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Phase0{
					Phase0: func() *ethpb.BeaconBlock {
						blk := util.NewBeaconBlock()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
		{
			name: "altair",
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Altair{
					Altair: func() *ethpb.BeaconBlockAltair {
						blk := util.NewBeaconBlockAltair()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
		{
			name: "bellatrix",
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Bellatrix{
					Bellatrix: func() *ethpb.BeaconBlockBellatrix {
						blk := util.NewBeaconBlockBellatrix()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
		{
			name: "bellatrix blind block",
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_BlindedBellatrix{
					BlindedBellatrix: func() *ethpb.BlindedBeaconBlockBellatrix {
						blk := util.NewBlindedBeaconBlockBellatrix()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
		{
			name: "capella",
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Capella{
					Capella: func() *ethpb.BeaconBlockCapella {
						blk := util.NewBeaconBlockCapella()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
		{
			name: "capella blind block",
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_BlindedCapella{
					BlindedCapella: func() *ethpb.BlindedBeaconBlockCapella {
						blk := util.NewBlindedBeaconBlockCapella()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
		{
			name:    "deneb block",
			version: version.Deneb,
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Deneb{
					Deneb: func() *ethpb.BeaconBlockContentsDeneb {
						blk := util.NewBeaconBlockContentsDeneb()
						blk.Block.Block.Body.Graffiti = graffiti
						return &ethpb.BeaconBlockContentsDeneb{Block: blk.Block.Block, KzgProofs: blk.KzgProofs, Blobs: blk.Blobs}
					}(),
				},
			},
		},
		{
			name:    "deneb blind block",
			version: version.Deneb,
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_BlindedDeneb{
					BlindedDeneb: func() *ethpb.BlindedBeaconBlockDeneb {
						blk := util.NewBlindedBeaconBlockDeneb()
						blk.Message.Body.Graffiti = graffiti
						return blk.Message
					}(),
				},
			},
		},
		{
			name:    "electra block",
			version: version.Electra,
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Electra{
					Electra: func() *ethpb.BeaconBlockContentsElectra {
						blk := util.NewBeaconBlockContentsElectra()
						blk.Block.Block.Body.Graffiti = graffiti
						return &ethpb.BeaconBlockContentsElectra{Block: blk.Block.Block, KzgProofs: blk.KzgProofs, Blobs: blk.Blobs}
					}(),
				},
			},
		},
		{
			name:    "fulu block",
			version: version.Fulu,
			block: &ethpb.GenericBeaconBlock{
				Block: &ethpb.GenericBeaconBlock_Fulu{
					Fulu: func() *ethpb.BeaconBlockContentsFulu {
						blk := util.NewBeaconBlockContentsFulu()
						blk.Block.Block.Body.Graffiti = graffiti
						return &ethpb.BeaconBlockContentsFulu{Block: blk.Block.Block, KzgProofs: blk.KzgProofs, Blobs: blk.Blobs}
					}(),
				},
			},
		},
	}

	for _, tt := range tests {
		for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
			t.Run(fmt.Sprintf("%s/SlashingProtectionMinimal:%v", tt.name, isSlashingProtectionMinimal), func(t *testing.T) {
				hook := logTest.NewGlobal()
				validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
				defer finish()
				var pubKey [fieldparams.BLSPubkeyLength]byte
				copy(pubKey[:], validatorKey.PublicKey().Marshal())

				validator.graffiti = graffiti

				m.validatorClient.EXPECT().DomainData(
					gomock.Any(), // ctx
					gomock.Any(), // epoch
				).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

				m.validatorClient.EXPECT().BeaconBlock(
					gomock.Any(), // ctx
					gomock.AssignableToTypeOf(&ethpb.BlockRequest{}),
				).DoAndReturn(func(ctx context.Context, req *ethpb.BlockRequest) (*ethpb.GenericBeaconBlock, error) {
					assert.DeepEqual(t, graffiti, req.Graffiti, "Unexpected graffiti in request")

					return tt.block, nil
				})

				m.validatorClient.EXPECT().DomainData(
					gomock.Any(), // ctx
					gomock.Any(), // epoch
				).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

				var sentBlock interfaces.ReadOnlySignedBeaconBlock
				var err error

				m.validatorClient.EXPECT().ProposeBeaconBlock(
					gomock.Any(), // ctx
					gomock.AssignableToTypeOf(&ethpb.GenericSignedBeaconBlock{}),
				).DoAndReturn(func(ctx context.Context, block *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error) {
					sentBlock, err = blocktest.NewSignedBeaconBlockFromGeneric(block)
					require.NoError(t, err)
					return &ethpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil
				})

				validator.ProposeBlock(t.Context(), 1, pubKey)
				g := sentBlock.Block().Body().Graffiti()
				assert.Equal(t, string(validator.graffiti), string(g[:]))
				require.LogsContain(t, hook, "Submitted new block")

			})
		}
	}
}

func TestProposeExit_ValidatorIndexFailed(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			_, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			m.validatorClient.EXPECT().ValidatorIndex(
				gomock.Any(),
				gomock.Any(),
			).Return(nil, errors.New("uh oh"))

			err := ProposeExit(
				t.Context(),
				m.validatorClient,
				m.signfunc,
				validatorKey.PublicKey().Marshal(),
				params.BeaconConfig().GenesisEpoch,
			)
			assert.NotNil(t, err)
			assert.ErrorContains(t, "uh oh", err)
			assert.ErrorContains(t, "gRPC call to get validator index failed", err)
		})
	}
}

func TestProposeExit_DomainDataFailed(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			_, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			m.validatorClient.EXPECT().
				ValidatorIndex(gomock.Any(), gomock.Any()).
				Return(&ethpb.ValidatorIndexResponse{Index: 1}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(nil, errors.New("uh oh"))

			err := ProposeExit(
				t.Context(),
				m.validatorClient,
				m.signfunc,
				validatorKey.PublicKey().Marshal(),
				params.BeaconConfig().GenesisEpoch,
			)
			assert.NotNil(t, err)
			assert.ErrorContains(t, domainDataErr, err)
			assert.ErrorContains(t, "uh oh", err)
			assert.ErrorContains(t, "failed to sign voluntary exit", err)
		})
	}
}

func TestProposeExit_DomainDataIsNil(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			_, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			m.validatorClient.EXPECT().
				ValidatorIndex(gomock.Any(), gomock.Any()).
				Return(&ethpb.ValidatorIndexResponse{Index: 1}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(nil, nil)

			err := ProposeExit(
				t.Context(),
				m.validatorClient,
				m.signfunc,
				validatorKey.PublicKey().Marshal(),
				params.BeaconConfig().GenesisEpoch,
			)
			assert.NotNil(t, err)
			assert.ErrorContains(t, domainDataErr, err)
			assert.ErrorContains(t, "failed to sign voluntary exit", err)
		})
	}
}

func TestProposeBlock_ProposeExitFailed(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			_, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			m.validatorClient.EXPECT().
				ValidatorIndex(gomock.Any(), gomock.Any()).
				Return(&ethpb.ValidatorIndexResponse{Index: 1}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			m.validatorClient.EXPECT().
				ProposeExit(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.SignedVoluntaryExit{})).
				Return(nil, errors.New("uh oh"))

			err := ProposeExit(
				t.Context(),
				m.validatorClient,
				m.signfunc,
				validatorKey.PublicKey().Marshal(),
				params.BeaconConfig().GenesisEpoch,
			)
			assert.NotNil(t, err)
			assert.ErrorContains(t, "uh oh", err)
			assert.ErrorContains(t, "failed to propose voluntary exit", err)
		})
	}
}

func TestProposeExit_BroadcastsBlock(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			_, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			m.validatorClient.EXPECT().
				ValidatorIndex(gomock.Any(), gomock.Any()).
				Return(&ethpb.ValidatorIndexResponse{Index: 1}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			m.validatorClient.EXPECT().
				ProposeExit(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.SignedVoluntaryExit{})).
				Return(&ethpb.ProposeExitResponse{}, nil)

			assert.NoError(t, ProposeExit(
				t.Context(),
				m.validatorClient,
				m.signfunc,
				validatorKey.PublicKey().Marshal(),
				params.BeaconConfig().GenesisEpoch,
			))
		})
	}
}

func TestSignBlock(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, _, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			proposerDomain := make([]byte, 32)
			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: proposerDomain}, nil)
			ctx := t.Context()
			blk := util.NewBeaconBlock()
			blk.Block.Slot = 1
			blk.Block.ProposerIndex = 100

			kp := testKeyFromBytes(t, []byte{1})

			validator.km = newMockKeymanager(t, kp)
			b, err := blocks.NewBeaconBlock(blk.Block)
			require.NoError(t, err)
			sig, blockRoot, err := validator.signBlock(ctx, kp.pub, 0, 0, b)
			require.NoError(t, err, "%x,%v", sig, err)
			require.Equal(t, "a049e1dc723e5a8b5bd14f292973572dffd53785ddb337"+
				"82f20bf762cbe10ee7b9b4f5ae1ad6ff2089d352403750bed402b94b58469c072536"+
				"faa9a09a88beaff697404ca028b1c7052b0de37dbcff985dfa500459783370312bdd"+
				"36d6e0f224", hex.EncodeToString(sig))

			// Verify the returned block root matches the expected root using the proposer signature
			// domain.
			wantedBlockRoot, err := signing.ComputeSigningRoot(b, proposerDomain)
			if err != nil {
				require.NoError(t, err)
			}
			require.DeepEqual(t, wantedBlockRoot, blockRoot)
		})
	}
}

func TestSignAltairBlock(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, _, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			kp := testKeyFromBytes(t, []byte{1})
			proposerDomain := make([]byte, 32)
			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: proposerDomain}, nil)
			ctx := t.Context()
			blk := util.NewBeaconBlockAltair()
			blk.Block.Slot = 1
			blk.Block.ProposerIndex = 100
			validator.km = newMockKeymanager(t, kp)
			wb, err := blocks.NewBeaconBlock(blk.Block)
			require.NoError(t, err)
			sig, blockRoot, err := validator.signBlock(ctx, kp.pub, 0, 0, wb)
			require.NoError(t, err, "%x,%v", sig, err)
			// Verify the returned block root matches the expected root using the proposer signature
			// domain.
			wantedBlockRoot, err := signing.ComputeSigningRoot(wb, proposerDomain)
			if err != nil {
				require.NoError(t, err)
			}
			require.DeepEqual(t, wantedBlockRoot, blockRoot)
		})
	}
}

func TestSignBellatrixBlock(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			validator, m, _, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			proposerDomain := make([]byte, 32)
			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: proposerDomain}, nil)

			ctx := t.Context()
			blk := util.NewBeaconBlockBellatrix()
			blk.Block.Slot = 1
			blk.Block.ProposerIndex = 100

			kp := randKeypair(t)
			validator.km = newMockKeymanager(t, kp)
			wb, err := blocks.NewBeaconBlock(blk.Block)
			require.NoError(t, err)
			sig, blockRoot, err := validator.signBlock(ctx, kp.pub, 0, 0, wb)
			require.NoError(t, err, "%x,%v", sig, err)
			// Verify the returned block root matches the expected root using the proposer signature
			// domain.
			wantedBlockRoot, err := signing.ComputeSigningRoot(wb, proposerDomain)
			if err != nil {
				require.NoError(t, err)
			}
			require.DeepEqual(t, wantedBlockRoot, blockRoot)
		})
	}
}

func TestGetGraffiti_Ok(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := &mocks{
		validatorClient: validatormock.NewMockValidatorClient(ctrl),
	}
	pubKey := [fieldparams.BLSPubkeyLength]byte{'a'}
	config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
	config[pubKey] = &proposer.Option{
		GraffitiConfig: &proposer.GraffitiConfig{
			Graffiti: "specific graffiti",
		},
	}

	tests := []struct {
		name string
		v    *validator
		want []byte
	}{
		{name: "use default cli graffiti",
			v: &validator{
				graffiti: []byte{'b'},
				graffitiStruct: &graffiti.Graffiti{
					Default: "c",
					Random:  []string{"d", "e"},
					Specific: map[primitives.ValidatorIndex]string{
						1: "f",
						2: "g",
					},
				},
			},
			want: bytesutil.PadTo([]byte{'b'}, 32),
		},
		{name: "use default file graffiti",
			v: &validator{
				validatorClient: m.validatorClient,
				graffitiStruct: &graffiti.Graffiti{
					Default: "c",
				},
			},
			want: bytesutil.PadTo([]byte{'c'}, 32),
		},
		{name: "use random file graffiti",
			v: &validator{
				validatorClient: m.validatorClient,
				graffitiStruct: &graffiti.Graffiti{
					Random:  []string{"d"},
					Default: "c",
				},
			},
			want: bytesutil.PadTo([]byte{'d'}, 32),
		},
		{name: "use validator file graffiti, has validator",
			v: &validator{
				validatorClient: m.validatorClient,
				graffitiStruct: &graffiti.Graffiti{
					Random:  []string{"d"},
					Default: "c",
					Specific: map[primitives.ValidatorIndex]string{
						1: "f",
						2: "g",
					},
				},
			},
			want: bytesutil.PadTo([]byte{'g'}, 32),
		},
		{name: "use validator file graffiti, none specified",
			v: &validator{
				validatorClient: m.validatorClient,
				graffitiStruct:  &graffiti.Graffiti{},
			},
			want: []byte{},
		},
		{name: "graffiti from proposer settings for specific pubkey",
			v: &validator{
				validatorClient: m.validatorClient,
				proposerSettings: &proposer.Settings{
					ProposeConfig: config,
				},
			},
			want: []byte("specific graffiti"),
		},
		{name: "graffiti from proposer settings default config",
			v: &validator{
				validatorClient: m.validatorClient,
				proposerSettings: &proposer.Settings{
					DefaultConfig: &proposer.Option{
						GraffitiConfig: &proposer.GraffitiConfig{
							Graffiti: "default graffiti",
						},
					},
				},
			},
			want: []byte("default graffiti"),
		},
		{name: "graffiti from proposer settings , specific pubkey overrides default config",
			v: &validator{
				validatorClient: m.validatorClient,
				proposerSettings: &proposer.Settings{
					ProposeConfig: config,
					DefaultConfig: &proposer.Option{
						GraffitiConfig: &proposer.GraffitiConfig{
							Graffiti: "default graffiti",
						},
					},
				},
			},
			want: []byte("specific graffiti"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.name, "use default cli graffiti") && tt.v.proposerSettings == nil {
				m.validatorClient.EXPECT().
					ValidatorIndex(gomock.Any(), &ethpb.ValidatorIndexRequest{PublicKey: pubKey[:]}).
					Return(&ethpb.ValidatorIndexResponse{Index: 2}, nil)
			}
			got, err := tt.v.Graffiti(t.Context(), pubKey)
			require.NoError(t, err)
			require.DeepEqual(t, tt.want, got)
		})
	}
}

func TestGetGraffitiOrdered_Ok(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			pubKey := [fieldparams.BLSPubkeyLength]byte{'a'}
			valDB := testing2.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{pubKey}, isSlashingProtectionMinimal)
			ctrl := gomock.NewController(t)
			m := &mocks{
				validatorClient: validatormock.NewMockValidatorClient(ctrl),
			}
			m.validatorClient.EXPECT().
				ValidatorIndex(gomock.Any(), &ethpb.ValidatorIndexRequest{PublicKey: pubKey[:]}).
				Times(5).
				Return(&ethpb.ValidatorIndexResponse{Index: 2}, nil)

			v := &validator{
				db:              valDB,
				validatorClient: m.validatorClient,
				graffitiStruct: &graffiti.Graffiti{
					Ordered: []string{"a", "b", "c"},
					Default: "d",
				},
			}
			for _, want := range [][]byte{bytesutil.PadTo([]byte{'a'}, 32), bytesutil.PadTo([]byte{'b'}, 32), bytesutil.PadTo([]byte{'c'}, 32), bytesutil.PadTo([]byte{'d'}, 32), bytesutil.PadTo([]byte{'d'}, 32)} {
				got, err := v.Graffiti(t.Context(), pubKey)
				require.NoError(t, err)
				require.DeepEqual(t, want, got)
			}
		})
	}
}

func Test_validator_DeleteGraffiti(t *testing.T) {
	pubKey := [fieldparams.BLSPubkeyLength]byte{'a'}
	tests := []struct {
		name             string
		proposerSettings *proposer.Settings
		wantErr          string
	}{
		{
			name: "delete existing graffiti ok",
			proposerSettings: &proposer.Settings{
				ProposeConfig: func() map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option {
					config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
					config[pubKey] = &proposer.Option{
						GraffitiConfig: &proposer.GraffitiConfig{
							Graffiti: "specific graffiti",
						},
					}
					return config
				}(),
			},
		},
		{
			name: "delete with proposer settings but only default configs",
			proposerSettings: &proposer.Settings{
				DefaultConfig: &proposer.Option{
					GraffitiConfig: &proposer.GraffitiConfig{
						Graffiti: "default graffiti",
					},
				},
			},
			wantErr: "attempted to delete graffiti without proposer settings, graffiti will default to flag options",
		},
		{
			name: "delete with proposer settings but without the specific public key setting",
			proposerSettings: &proposer.Settings{
				ProposeConfig: func() map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option {
					config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
					pk := make([]byte, fieldparams.BLSPubkeyLength)
					config[bytesutil.ToBytes48(pk)] = &proposer.Option{
						GraffitiConfig: &proposer.GraffitiConfig{
							Graffiti: "specific graffiti",
						},
					}
					return config
				}(),
			},
			wantErr: fmt.Sprintf("graffiti not found in proposer settings for pubkey:%s", hexutil.Encode(pubKey[:])),
		},
		{
			name:    "delete without proposer settings",
			wantErr: "attempted to delete graffiti without proposer settings, graffiti will default to flag options",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &validator{
				db:               testing2.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{pubKey}, false),
				proposerSettings: tt.proposerSettings,
			}
			err := v.DeleteGraffiti(t.Context(), pubKey)
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, v.proposerSettings.ProposeConfig[pubKey].GraffitiConfig == nil, true)
			}
		})
	}
}

func Test_validator_SetGraffiti(t *testing.T) {
	pubKey := [fieldparams.BLSPubkeyLength]byte{'a'}
	tests := []struct {
		name                 string
		graffiti             string
		proposerSettings     *proposer.Settings
		wantProposerSettings *proposer.Settings
		wantErr              string
	}{
		{
			name:     "setting existing graffiti ok",
			graffiti: "new graffiti",
			proposerSettings: &proposer.Settings{
				ProposeConfig: func() map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option {
					config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
					config[pubKey] = &proposer.Option{
						GraffitiConfig: &proposer.GraffitiConfig{
							Graffiti: "specific graffiti",
						},
					}
					return config
				}(),
			},
		},
		{
			name: "set with proposer settings but only default configs",
			proposerSettings: &proposer.Settings{
				DefaultConfig: &proposer.Option{
					GraffitiConfig: &proposer.GraffitiConfig{
						Graffiti: "default graffiti",
					},
				},
			},
		},
		{
			name: "set with proposer settings but without the specific public key setting",
			proposerSettings: &proposer.Settings{
				ProposeConfig: func() map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option {
					config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
					pk := make([]byte, fieldparams.BLSPubkeyLength)
					config[bytesutil.ToBytes48(pk)] = &proposer.Option{
						GraffitiConfig: &proposer.GraffitiConfig{
							Graffiti: "specific graffiti",
						},
					}
					return config
				}(),
			},
		},
		{
			name:     "set without proposer settings",
			graffiti: "specific graffiti",
			wantProposerSettings: func() *proposer.Settings {
				config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
				config[pubKey] = &proposer.Option{
					GraffitiConfig: &proposer.GraffitiConfig{
						Graffiti: "specific graffiti",
					},
				}
				return &proposer.Settings{ProposeConfig: config}
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &validator{
				db:               testing2.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{pubKey}, false),
				proposerSettings: tt.proposerSettings,
			}
			err := v.SetGraffiti(t.Context(), pubKey, []byte(tt.graffiti))
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
			} else {
				require.NoError(t, err)
				if tt.wantProposerSettings != nil {
					require.DeepEqual(t, tt.wantProposerSettings, v.proposerSettings)
				} else {
					require.Equal(t, v.proposerSettings.ProposeConfig[pubKey].GraffitiConfig.Graffiti, tt.graffiti)
				}

			}
		})
	}
}
