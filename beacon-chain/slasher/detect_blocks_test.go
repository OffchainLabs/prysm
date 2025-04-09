package slasher

import (
	"context"
	"testing"

	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	dbtest "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	doublylinkedtree "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/doubly-linked-tree"
	slashingsmock "github.com/prysmaticlabs/prysm/v5/beacon-chain/operations/slashings/mock"
	slashertypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/slasher/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stategen"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

type wrapped struct {
	slot             primitives.Slot
	signedBlkHeaders []*slashertypes.SignedBlockHeaderWrapper
}

func Test_processQueuedBlocks_DetectsDoubleProposals(t *testing.T) {
	testCases := []struct {
		name  string
		wraps []wrapped
	}{
		{
			name: "detects double proposals in the same batch",
			wraps: []wrapped{
				{
					4,
					[]*slashertypes.SignedBlockHeaderWrapper{
						createProposalWrapper(t, 4, 1, []byte{1}),
						createProposalWrapper(t, 4, 1, []byte{1}),
						createProposalWrapper(t, 4, 1, []byte{1}),
						createProposalWrapper(t, 4, 1, []byte{2}),
					},
				},
			},
		},
		{
			name: "detects double proposals in the different batches",
			wraps: []wrapped{
				{
					5,
					[]*slashertypes.SignedBlockHeaderWrapper{
						createProposalWrapper(t, 4, 1, []byte{1}),
						createProposalWrapper(t, 5, 1, []byte{1}),
					},
				},
				{
					6,
					[]*slashertypes.SignedBlockHeaderWrapper{
						createProposalWrapper(t, 4, 1, []byte{2}),
					},
				},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			beaconDB := dbtest.SetupDB(t)
			slasherDB := dbtest.SetupSlasherDB(t)
			ctx, cancel := context.WithCancel(context.Background())

			beaconState, err := util.NewBeaconState()
			require.NoError(t, err)

			// Initialize validators in the state.
			numVals := params.BeaconConfig().MinGenesisActiveValidatorCount
			validators := make([]*ethpb.Validator, numVals)
			privKeys := make([]bls.SecretKey, numVals)
			for i := range validators {
				privKey, err := bls.RandKey()
				require.NoError(t, err)
				privKeys[i] = privKey
				validators[i] = &ethpb.Validator{
					PublicKey:             privKey.PublicKey().Marshal(),
					WithdrawalCredentials: make([]byte, 32),
				}
			}
			err = beaconState.SetValidators(validators)
			require.NoError(t, err)
			domain, err := signing.Domain(
				beaconState.Fork(),
				0,
				params.BeaconConfig().DomainBeaconProposer,
				beaconState.GenesisValidatorsRoot(),
			)
			require.NoError(t, err)

			mockChain := &mock.ChainService{
				State: beaconState,
			}
			s := &Service{
				serviceCfg: &ServiceConfig{
					Database:             slasherDB,
					StateNotifier:        &mock.MockStateNotifier{},
					HeadStateFetcher:     mockChain,
					StateGen:             stategen.New(beaconDB, doublylinkedtree.New()),
					SlashingPoolInserter: &slashingsmock.PoolMock{},
					ClockWaiter:          startup.NewClockSynchronizer(),
				},
				params:    DefaultParams(),
				blksQueue: newBlocksQueue(),
			}

			parentRoot := bytesutil.ToBytes32([]byte("parent"))
			err = s.serviceCfg.StateGen.SaveState(ctx, parentRoot, beaconState)
			require.NoError(t, err)

			currentSlotChan := make(chan primitives.Slot)
			s.wg.Add(1)
			go func() {
				s.processQueuedBlocks(ctx, currentSlotChan)
			}()

			for _, wrap := range tt.wraps {
				// Add valid signatures to the block headers we are testing.
				for _, proposalWrapper := range wrap.signedBlkHeaders {
					proposalWrapper.SignedBeaconBlockHeader.Header.ParentRoot = parentRoot[:]
					headerHtr, err := proposalWrapper.SignedBeaconBlockHeader.Header.HashTreeRoot()
					require.NoError(t, err)

					container := &ethpb.SigningData{
						ObjectRoot: headerHtr[:],
						Domain:     domain,
					}

					signingRoot, err := container.HashTreeRoot()
					require.NoError(t, err)

					privKey := privKeys[proposalWrapper.SignedBeaconBlockHeader.Header.ProposerIndex]
					proposalWrapper.SignedBeaconBlockHeader.Signature = privKey.Sign(signingRoot[:]).Marshal()
				}

				s.blksQueue.extend(wrap.signedBlkHeaders)

				currentSlot := primitives.Slot(4)
				currentSlotChan <- currentSlot
			}

			cancel()
			s.wg.Wait()
			require.LogsContain(t, hook, "Proposer slashing detected")
		})
	}
}

func Test_processQueuedBlocks_NotSlashable(t *testing.T) {
	hook := logTest.NewGlobal()
	slasherDB := dbtest.SetupSlasherDB(t)
	ctx, cancel := context.WithCancel(context.Background())

	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	currentSlot := primitives.Slot(4)
	require.NoError(t, beaconState.SetSlot(currentSlot))
	mockChain := &mock.ChainService{
		State: beaconState,
		Slot:  &currentSlot,
	}

	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:         slasherDB,
			StateNotifier:    &mock.MockStateNotifier{},
			HeadStateFetcher: mockChain,
			ClockWaiter:      startup.NewClockSynchronizer(),
		},
		params:    DefaultParams(),
		blksQueue: newBlocksQueue(),
	}
	currentSlotChan := make(chan primitives.Slot)
	s.wg.Add(1)
	go func() {
		s.processQueuedBlocks(ctx, currentSlotChan)
	}()
	s.blksQueue.extend([]*slashertypes.SignedBlockHeaderWrapper{
		createProposalWrapper(t, 4, 1, []byte{1}),
		createProposalWrapper(t, 4, 1, []byte{1}),
	})
	currentSlotChan <- currentSlot
	cancel()
	s.wg.Wait()
	require.LogsDoNotContain(t, hook, "Proposer slashing detected")
}

func createProposalWrapper(t *testing.T, slot primitives.Slot, proposerIndex primitives.ValidatorIndex, signingRoot []byte) *slashertypes.SignedBlockHeaderWrapper {
	header := &ethpb.BeaconBlockHeader{
		Slot:          slot,
		ProposerIndex: proposerIndex,
		ParentRoot:    params.BeaconConfig().ZeroHash[:],
		StateRoot:     bytesutil.PadTo(signingRoot, 32),
		BodyRoot:      params.BeaconConfig().ZeroHash[:],
	}
	headerRoot, err := header.HashTreeRoot()
	require.NoError(t, err)
	fakeSig := make([]byte, 96)
	copy(fakeSig, "hello")
	return &slashertypes.SignedBlockHeaderWrapper{
		SignedBeaconBlockHeader: &ethpb.SignedBeaconBlockHeader{
			Header:    header,
			Signature: fakeSig,
		},
		HeaderRoot: headerRoot,
	}
}
