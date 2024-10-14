package client

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/prysmaticlabs/prysm/v5/async/event"
	"github.com/prysmaticlabs/prysm/v5/cmd/validator/flags"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	validatorType "github.com/prysmaticlabs/prysm/v5/consensus-types/validator"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	blsmock "github.com/prysmaticlabs/prysm/v5/crypto/bls/common/mock"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	validatormock "github.com/prysmaticlabs/prysm/v5/testing/validator-mock"
	"github.com/prysmaticlabs/prysm/v5/validator/accounts/wallet"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	dbTest "github.com/prysmaticlabs/prysm/v5/validator/db/testing"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager/local"
	remoteweb3signer "github.com/prysmaticlabs/prysm/v5/validator/keymanager/remote-web3signer"
	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/urfave/cli/v2"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)
}

var _ iface.Validator = (*validator)(nil)

const cancelledCtx = "context has been canceled"

var unknownIndex = primitives.ValidatorIndex(^uint64(0))

func genMockKeymanager(t *testing.T, numKeys int) *mockKeymanager {
	pairs := make([]keypair, numKeys)
	for i := 0; i < numKeys; i++ {
		pairs[i] = randKeypair(t)
	}

	return newMockKeymanager(t, pairs...)
}

type keypair struct {
	pub [fieldparams.BLSPubkeyLength]byte
	pri bls.SecretKey
}

func randKeypair(t *testing.T) keypair {
	pri, err := bls.RandKey()
	require.NoError(t, err)
	var pub [fieldparams.BLSPubkeyLength]byte
	copy(pub[:], pri.PublicKey().Marshal())
	return keypair{pub: pub, pri: pri}
}

func newMockKeymanager(t *testing.T, pairs ...keypair) *mockKeymanager {
	m := &mockKeymanager{keysMap: make(map[[fieldparams.BLSPubkeyLength]byte]bls.SecretKey)}
	require.NoError(t, m.add(pairs...))
	return m
}

type mockKeymanager struct {
	lock                sync.RWMutex
	keysMap             map[[fieldparams.BLSPubkeyLength]byte]bls.SecretKey
	keys                [][fieldparams.BLSPubkeyLength]byte
	fetchNoKeys         bool
	accountsChangedFeed *event.Feed
}

var errMockKeyExists = errors.New("key already in mockKeymanager map")

func (m *mockKeymanager) add(pairs ...keypair) error {
	for _, kp := range pairs {
		if _, exists := m.keysMap[kp.pub]; exists {
			return errMockKeyExists
		}
		m.keys = append(m.keys, kp.pub)
		m.keysMap[kp.pub] = kp.pri
	}
	return nil
}

func (m *mockKeymanager) FetchValidatingPublicKeys(_ context.Context) ([][fieldparams.BLSPubkeyLength]byte, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if m.fetchNoKeys {
		m.fetchNoKeys = false
		return [][fieldparams.BLSPubkeyLength]byte{}, nil
	}
	return m.keys, nil
}

func (m *mockKeymanager) Sign(_ context.Context, req *validatorpb.SignRequest) (bls.Signature, error) {
	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], req.PublicKey)
	privKey, ok := m.keysMap[pubKey]
	if !ok {
		return nil, errors.New("not found")
	}
	sig := privKey.Sign(req.SigningRoot)
	return sig, nil
}

func (m *mockKeymanager) SubscribeAccountChanges(pubKeysChan chan [][fieldparams.BLSPubkeyLength]byte) event.Subscription {
	if m.accountsChangedFeed == nil {
		m.accountsChangedFeed = &event.Feed{}
	}
	return m.accountsChangedFeed.Subscribe(pubKeysChan)
}

func (m *mockKeymanager) SimulateAccountChanges(newKeys [][fieldparams.BLSPubkeyLength]byte) {
	m.accountsChangedFeed.Send(newKeys)
}

func (*mockKeymanager) ExtractKeystores(
	_ context.Context, _ []bls.PublicKey, _ string,
) ([]*keymanager.Keystore, error) {
	return nil, errors.New("extracting keys not supported on mock keymanager")
}

func (*mockKeymanager) ListKeymanagerAccounts(
	context.Context, keymanager.ListKeymanagerAccountConfig) error {
	return nil
}

func (*mockKeymanager) DeleteKeystores(context.Context, [][]byte,
) ([]*keymanager.KeyStatus, error) {
	return nil, nil
}

func TestWaitForChainStart_SetsGenesisInfo(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := validatormock.NewMockValidatorClient(ctrl)

			db := dbTest.SetupDB(t, [][fieldparams.BLSPubkeyLength]byte{}, isSlashingProtectionMinimal)
			v := validator{
				validatorClient: client,
				db:              db,
			}

			// Make sure its clean at the start.
			savedGenValRoot, err := db.GenesisValidatorsRoot(context.Background())
			require.NoError(t, err)
			assert.DeepEqual(t, []byte(nil), savedGenValRoot, "Unexpected saved genesis validators root")

			genesis := uint64(time.Unix(1, 0).Unix())
			genesisValidatorsRoot := bytesutil.ToBytes32([]byte("validators"))
			client.EXPECT().WaitForChainStart(
				gomock.Any(),
				&emptypb.Empty{},
			).Return(&ethpb.ChainStartResponse{
				Started:               true,
				GenesisTime:           genesis,
				GenesisValidatorsRoot: genesisValidatorsRoot[:],
			}, nil)
			require.NoError(t, v.WaitForChainStart(context.Background()))
			savedGenValRoot, err = db.GenesisValidatorsRoot(context.Background())
			require.NoError(t, err)

			assert.DeepEqual(t, genesisValidatorsRoot[:], savedGenValRoot, "Unexpected saved genesis validators root")
			assert.Equal(t, genesis, v.genesisTime, "Unexpected chain start time")
			assert.NotNil(t, v.ticker, "Expected ticker to be set, received nil")

			// Make sure there are no errors running if it is the same data.
			client.EXPECT().WaitForChainStart(
				gomock.Any(),
				&emptypb.Empty{},
			).Return(&ethpb.ChainStartResponse{
				Started:               true,
				GenesisTime:           genesis,
				GenesisValidatorsRoot: genesisValidatorsRoot[:],
			}, nil)
			require.NoError(t, v.WaitForChainStart(context.Background()))
		})
	}
}

func TestWaitForChainStart_SetsGenesisInfo_IncorrectSecondTry(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := validatormock.NewMockValidatorClient(ctrl)

			db := dbTest.SetupDB(t, [][fieldparams.BLSPubkeyLength]byte{}, isSlashingProtectionMinimal)
			v := validator{
				validatorClient: client,
				db:              db,
			}
			genesis := uint64(time.Unix(1, 0).Unix())
			genesisValidatorsRoot := bytesutil.ToBytes32([]byte("validators"))
			client.EXPECT().WaitForChainStart(
				gomock.Any(),
				&emptypb.Empty{},
			).Return(&ethpb.ChainStartResponse{
				Started:               true,
				GenesisTime:           genesis,
				GenesisValidatorsRoot: genesisValidatorsRoot[:],
			}, nil)
			require.NoError(t, v.WaitForChainStart(context.Background()))
			savedGenValRoot, err := db.GenesisValidatorsRoot(context.Background())
			require.NoError(t, err)

			assert.DeepEqual(t, genesisValidatorsRoot[:], savedGenValRoot, "Unexpected saved genesis validators root")
			assert.Equal(t, genesis, v.genesisTime, "Unexpected chain start time")
			assert.NotNil(t, v.ticker, "Expected ticker to be set, received nil")

			genesisValidatorsRoot = bytesutil.ToBytes32([]byte("badvalidators"))

			// Make sure there are no errors running if it is the same data.
			client.EXPECT().WaitForChainStart(
				gomock.Any(),
				&emptypb.Empty{},
			).Return(&ethpb.ChainStartResponse{
				Started:               true,
				GenesisTime:           genesis,
				GenesisValidatorsRoot: genesisValidatorsRoot[:],
			}, nil)
			err = v.WaitForChainStart(context.Background())
			require.ErrorContains(t, "does not match root saved", err)
		})
	}
}

func TestWaitForChainStart_ContextCanceled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	v := validator{
		//keyManager:      testKeyManager,
		validatorClient: client,
	}
	genesis := uint64(time.Unix(0, 0).Unix())
	genesisValidatorsRoot := bytesutil.PadTo([]byte("validators"), 32)
	client.EXPECT().WaitForChainStart(
		gomock.Any(),
		&emptypb.Empty{},
	).Return(&ethpb.ChainStartResponse{
		Started:               true,
		GenesisTime:           genesis,
		GenesisValidatorsRoot: genesisValidatorsRoot,
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorContains(t, cancelledCtx, v.WaitForChainStart(ctx))
}

func TestWaitForChainStart_ReceiveErrorFromStream(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	v := validator{
		validatorClient: client,
	}
	client.EXPECT().WaitForChainStart(
		gomock.Any(),
		&emptypb.Empty{},
	).Return(nil, errors.New("fails"))
	err := v.WaitForChainStart(context.Background())
	want := "could not receive ChainStart from stream"
	assert.ErrorContains(t, want, err)
}

func TestCanonicalHeadSlot_FailedRPC(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockChainClient(ctrl)
	v := validator{
		chainClient: client,
		genesisTime: 1,
	}
	client.EXPECT().ChainHead(
		gomock.Any(),
		gomock.Any(),
	).Return(nil, errors.New("failed"))
	_, err := v.CanonicalHeadSlot(context.Background())
	assert.ErrorContains(t, "failed", err)
}

func TestCanonicalHeadSlot_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockChainClient(ctrl)
	v := validator{
		chainClient: client,
	}
	client.EXPECT().ChainHead(
		gomock.Any(),
		gomock.Any(),
	).Return(&ethpb.ChainHead{HeadSlot: 0}, nil)
	headSlot, err := v.CanonicalHeadSlot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(0), headSlot, "Mismatch slots")
}

func TestWaitSync_ContextCanceled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	n := validatormock.NewMockNodeClient(ctrl)

	v := validator{
		nodeClient: n,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	n.EXPECT().SyncStatus(
		gomock.Any(),
		gomock.Any(),
	).Return(&ethpb.SyncStatus{Syncing: true}, nil)

	assert.ErrorContains(t, cancelledCtx, v.WaitForSync(ctx))
}

func TestWaitSync_NotSyncing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	n := validatormock.NewMockNodeClient(ctrl)

	v := validator{
		nodeClient: n,
	}

	n.EXPECT().SyncStatus(
		gomock.Any(),
		gomock.Any(),
	).Return(&ethpb.SyncStatus{Syncing: false}, nil)

	require.NoError(t, v.WaitForSync(context.Background()))
}

func TestWaitSync_Syncing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	n := validatormock.NewMockNodeClient(ctrl)

	v := validator{
		nodeClient: n,
	}

	n.EXPECT().SyncStatus(
		gomock.Any(),
		gomock.Any(),
	).Return(&ethpb.SyncStatus{Syncing: true}, nil)

	n.EXPECT().SyncStatus(
		gomock.Any(),
		gomock.Any(),
	).Return(&ethpb.SyncStatus{Syncing: false}, nil)

	require.NoError(t, v.WaitForSync(context.Background()))
}

func TestUpdateDuties_DoesNothingWhenNotEpochStart_AlreadyExistingAssignments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	slot := primitives.Slot(1)
	v := validator{
		validatorClient: client,
		duties: &ethpb.DutiesResponse{
			CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
				{
					Committee:      []primitives.ValidatorIndex{},
					AttesterSlot:   10,
					CommitteeIndex: 20,
				},
			},
		},
	}
	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Times(0)

	assert.NoError(t, v.UpdateDuties(context.Background(), slot), "Could not update assignments")
}

func TestUpdateDuties_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	v := validator{
		validatorClient: client,
		km:              newMockKeymanager(t, randKeypair(t)),
		duties: &ethpb.DutiesResponse{
			CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
				{
					CommitteeIndex: 1,
				},
			},
		},
	}

	expected := errors.New("bad")

	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Return(nil, expected)

	assert.ErrorContains(t, expected.Error(), v.UpdateDuties(context.Background(), params.BeaconConfig().SlotsPerEpoch))
	assert.Equal(t, (*ethpb.DutiesResponse)(nil), v.duties, "Assignments should have been cleared on failure")
}

func TestUpdateDuties_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	slot := params.BeaconConfig().SlotsPerEpoch
	resp := &ethpb.DutiesResponse{
		CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
			{
				AttesterSlot:   params.BeaconConfig().SlotsPerEpoch,
				ValidatorIndex: 200,
				CommitteeIndex: 100,
				Committee:      []primitives.ValidatorIndex{0, 1, 2, 3},
				PublicKey:      []byte("testPubKey_1"),
				ProposerSlots:  []primitives.Slot{params.BeaconConfig().SlotsPerEpoch + 1},
			},
		},
	}
	v := validator{
		km:              newMockKeymanager(t, randKeypair(t)),
		validatorClient: client,
	}
	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Return(resp, nil)

	var wg sync.WaitGroup
	wg.Add(1)

	client.EXPECT().SubscribeCommitteeSubnets(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.DutiesResponse_Duty) (*emptypb.Empty, error) {
		wg.Done()
		return nil, nil
	})

	require.NoError(t, v.UpdateDuties(context.Background(), slot), "Could not update assignments")

	util.WaitTimeout(&wg, 2*time.Second)

	assert.Equal(t, params.BeaconConfig().SlotsPerEpoch+1, v.duties.CurrentEpochDuties[0].ProposerSlots[0], "Unexpected validator assignments")
	assert.Equal(t, params.BeaconConfig().SlotsPerEpoch, v.duties.CurrentEpochDuties[0].AttesterSlot, "Unexpected validator assignments")
	assert.Equal(t, resp.CurrentEpochDuties[0].CommitteeIndex, v.duties.CurrentEpochDuties[0].CommitteeIndex, "Unexpected validator assignments")
	assert.Equal(t, resp.CurrentEpochDuties[0].ValidatorIndex, v.duties.CurrentEpochDuties[0].ValidatorIndex, "Unexpected validator assignments")
}

func TestUpdateDuties_OK_FilterBlacklistedPublicKeys(t *testing.T) {
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)
	slot := params.BeaconConfig().SlotsPerEpoch

	numValidators := 10
	km := genMockKeymanager(t, numValidators)
	blacklistedPublicKeys := make(map[[fieldparams.BLSPubkeyLength]byte]bool)
	for _, k := range km.keys {
		blacklistedPublicKeys[k] = true
	}
	v := validator{
		km:                 km,
		validatorClient:    client,
		blacklistedPubkeys: blacklistedPublicKeys,
	}

	resp := &ethpb.DutiesResponse{
		CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{},
	}
	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Return(resp, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	client.EXPECT().SubscribeCommitteeSubnets(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.DutiesResponse_Duty) (*emptypb.Empty, error) {
		wg.Done()
		return nil, nil
	})

	require.NoError(t, v.UpdateDuties(context.Background(), slot), "Could not update assignments")

	util.WaitTimeout(&wg, 2*time.Second)

	for range blacklistedPublicKeys {
		assert.LogsContain(t, hook, "Not including slashable public key")
	}
}

func TestUpdateDuties_AllValidatorsExited(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	slot := params.BeaconConfig().SlotsPerEpoch
	resp := &ethpb.DutiesResponse{
		CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
			{
				AttesterSlot:   params.BeaconConfig().SlotsPerEpoch,
				ValidatorIndex: 200,
				CommitteeIndex: 100,
				Committee:      []primitives.ValidatorIndex{0, 1, 2, 3},
				PublicKey:      []byte("testPubKey_1"),
				ProposerSlots:  []primitives.Slot{params.BeaconConfig().SlotsPerEpoch + 1},
				Status:         ethpb.ValidatorStatus_EXITED,
			},
			{
				AttesterSlot:   params.BeaconConfig().SlotsPerEpoch,
				ValidatorIndex: 201,
				CommitteeIndex: 101,
				Committee:      []primitives.ValidatorIndex{0, 1, 2, 3},
				PublicKey:      []byte("testPubKey_2"),
				ProposerSlots:  []primitives.Slot{params.BeaconConfig().SlotsPerEpoch + 1},
				Status:         ethpb.ValidatorStatus_EXITED,
			},
		},
	}
	v := validator{
		km:              newMockKeymanager(t, randKeypair(t)),
		validatorClient: client,
	}
	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Return(resp, nil)

	err := v.UpdateDuties(context.Background(), slot)
	require.ErrorContains(t, ErrValidatorsAllExited.Error(), err)

}

func TestUpdateDuties_Distributed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	// Start of third epoch.
	slot := 2 * params.BeaconConfig().SlotsPerEpoch
	keys := randKeypair(t)
	resp := &ethpb.DutiesResponse{
		CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
			{
				AttesterSlot:   slot, // First slot in epoch.
				ValidatorIndex: 200,
				CommitteeIndex: 100,
				PublicKey:      keys.pub[:],
				Status:         ethpb.ValidatorStatus_ACTIVE,
			},
		},
		NextEpochDuties: []*ethpb.DutiesResponse_Duty{
			{
				AttesterSlot:   slot + params.BeaconConfig().SlotsPerEpoch, // First slot in next epoch.
				ValidatorIndex: 200,
				CommitteeIndex: 100,
				PublicKey:      keys.pub[:],
				Status:         ethpb.ValidatorStatus_ACTIVE,
			},
		},
	}

	v := validator{
		km:              newMockKeymanager(t, keys),
		validatorClient: client,
		distributed:     true,
	}

	sigDomain := make([]byte, 32)

	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Return(resp, nil)

	client.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Return(
		&ethpb.DomainResponse{SignatureDomain: sigDomain},
		nil, /*err*/
	).Times(2)

	client.EXPECT().AggregatedSelections(
		gomock.Any(),
		gomock.Any(), // fill this properly
	).Return(
		[]iface.BeaconCommitteeSelection{
			{
				SelectionProof: make([]byte, 32),
				Slot:           slot,
				ValidatorIndex: 200,
			},
			{
				SelectionProof: make([]byte, 32),
				Slot:           slot + params.BeaconConfig().SlotsPerEpoch,
				ValidatorIndex: 200,
			},
		},
		nil,
	)

	var wg sync.WaitGroup
	wg.Add(1)

	client.EXPECT().SubscribeCommitteeSubnets(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.DutiesResponse_Duty) (*emptypb.Empty, error) {
		wg.Done()
		return nil, nil
	})

	require.NoError(t, v.UpdateDuties(context.Background(), slot), "Could not update assignments")
	util.WaitTimeout(&wg, 2*time.Second)
	require.Equal(t, 2, len(v.attSelections))
}

func TestRolesAt_OK(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			v, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			v.duties = &ethpb.DutiesResponse{
				CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
					{
						CommitteeIndex:  1,
						AttesterSlot:    1,
						PublicKey:       validatorKey.PublicKey().Marshal(),
						IsSyncCommittee: true,
						PtcSlot:         1,
					},
				},
				NextEpochDuties: []*ethpb.DutiesResponse_Duty{
					{
						CommitteeIndex:  1,
						AttesterSlot:    1,
						PublicKey:       validatorKey.PublicKey().Marshal(),
						IsSyncCommittee: true,
						PtcSlot:         1,
					},
				},
			}

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().SyncSubcommitteeIndex(
				gomock.Any(), // ctx
				&ethpb.SyncSubcommitteeIndexRequest{
					PublicKey: validatorKey.PublicKey().Marshal(),
					Slot:      1,
				},
			).Return(&ethpb.SyncSubcommitteeIndexResponse{}, nil /*err*/)

			roleMap, err := v.RolesAt(context.Background(), 1)
			require.NoError(t, err)

			assert.Equal(t, iface.RoleAttester, roleMap[bytesutil.ToBytes48(validatorKey.PublicKey().Marshal())][0])
			assert.Equal(t, iface.RoleAggregator, roleMap[bytesutil.ToBytes48(validatorKey.PublicKey().Marshal())][1])
			assert.Equal(t, iface.RoleSyncCommittee, roleMap[bytesutil.ToBytes48(validatorKey.PublicKey().Marshal())][2])
			assert.Equal(t, iface.RolePayloadTimelinessCommittee, roleMap[bytesutil.ToBytes48(validatorKey.PublicKey().Marshal())][3])

			// Test sync committee role at epoch boundary.
			v.duties = &ethpb.DutiesResponse{
				CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
					{
						CommitteeIndex:  1,
						AttesterSlot:    1,
						PublicKey:       validatorKey.PublicKey().Marshal(),
						IsSyncCommittee: false,
					},
				},
				NextEpochDuties: []*ethpb.DutiesResponse_Duty{
					{
						CommitteeIndex:  1,
						AttesterSlot:    1,
						PublicKey:       validatorKey.PublicKey().Marshal(),
						IsSyncCommittee: true,
					},
				},
			}

			m.validatorClient.EXPECT().SyncSubcommitteeIndex(
				gomock.Any(), // ctx
				&ethpb.SyncSubcommitteeIndexRequest{
					PublicKey: validatorKey.PublicKey().Marshal(),
					Slot:      31,
				},
			).Return(&ethpb.SyncSubcommitteeIndexResponse{}, nil /*err*/)

			roleMap, err = v.RolesAt(context.Background(), params.BeaconConfig().SlotsPerEpoch-1)
			require.NoError(t, err)
			assert.Equal(t, iface.RoleSyncCommittee, roleMap[bytesutil.ToBytes48(validatorKey.PublicKey().Marshal())][0])
		})
	}
}

func TestRolesAt_DoesNotAssignProposer_Slot0(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			v, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			v.duties = &ethpb.DutiesResponse{
				CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
					{
						CommitteeIndex: 1,
						AttesterSlot:   0,
						ProposerSlots:  []primitives.Slot{0},
						PublicKey:      validatorKey.PublicKey().Marshal(),
					},
				},
			}

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			roleMap, err := v.RolesAt(context.Background(), 0)
			require.NoError(t, err)

			assert.Equal(t, iface.RoleAttester, roleMap[bytesutil.ToBytes48(validatorKey.PublicKey().Marshal())][0])
		})
	}
}

func TestCheckAndLogValidatorStatus_OK(t *testing.T) {
	nonexistentIndex := primitives.ValidatorIndex(^uint64(0))
	type statusTest struct {
		name   string
		status *validatorStatus
		log    string
		active bool
	}
	pubKeys := [][]byte{bytesutil.Uint64ToBytesLittleEndian(0)}
	tests := []statusTest{
		{
			name: "UNKNOWN_STATUS, no deposit found yet",
			status: &validatorStatus{
				publicKey: pubKeys[0],
				index:     nonexistentIndex,
				status: &ethpb.ValidatorStatusResponse{
					Status: ethpb.ValidatorStatus_UNKNOWN_STATUS,
				},
			},
			log:    "Waiting for deposit to be observed by beacon node",
			active: false,
		},
		{
			name: "DEPOSITED into state",
			status: &validatorStatus{
				publicKey: pubKeys[0],
				index:     30,
				status: &ethpb.ValidatorStatusResponse{
					Status:                    ethpb.ValidatorStatus_DEPOSITED,
					PositionInActivationQueue: 30,
				},
			},
			log:    "Deposit processed, entering activation queue after finalization\" positionInActivationQueue=30 prefix=client pubkey=0x000000000000 status=DEPOSITED validatorIndex=30",
			active: false,
		},
		{
			name: "PENDING",
			status: &validatorStatus{
				publicKey: pubKeys[0],
				index:     50,
				status: &ethpb.ValidatorStatusResponse{
					Status:                    ethpb.ValidatorStatus_PENDING,
					ActivationEpoch:           params.BeaconConfig().FarFutureEpoch,
					PositionInActivationQueue: 6,
				},
			},
			log:    "Waiting to be assigned activation epoch\" expectedWaitingTime=12m48s positionInActivationQueue=6 prefix=client pubkey=0x000000000000 status=PENDING validatorIndex=50",
			active: false,
		},
		{
			name: "PENDING",
			status: &validatorStatus{
				publicKey: pubKeys[0],
				index:     89,
				status: &ethpb.ValidatorStatusResponse{
					Status:                    ethpb.ValidatorStatus_PENDING,
					ActivationEpoch:           60,
					PositionInActivationQueue: 5,
				},
			},
			log:    "Waiting for activation\" activationEpoch=60 prefix=client pubkey=0x000000000000 status=PENDING validatorIndex=89",
			active: false,
		},
		{
			name: "ACTIVE",
			status: &validatorStatus{
				publicKey: pubKeys[0],
				index:     89,
				status: &ethpb.ValidatorStatusResponse{
					Status: ethpb.ValidatorStatus_ACTIVE,
				},
			},
			active: true,
		},
		{
			name: "EXITING",
			status: &validatorStatus{
				publicKey: pubKeys[0],
				index:     89,
				status: &ethpb.ValidatorStatusResponse{
					Status: ethpb.ValidatorStatus_EXITING,
				},
			},
			active: true,
		},
		{
			name: "EXITED",
			status: &validatorStatus{
				publicKey: pubKeys[0],
				status: &ethpb.ValidatorStatusResponse{
					Status: ethpb.ValidatorStatus_EXITED,
				},
			},
			log:    "Validator exited",
			active: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := validatormock.NewMockValidatorClient(ctrl)
			v := validator{
				validatorClient: client,
				duties: &ethpb.DutiesResponse{
					CurrentEpochDuties: []*ethpb.DutiesResponse_Duty{
						{
							CommitteeIndex: 1,
						},
					},
				},
				pubkeyToStatus: make(map[[48]byte]*validatorStatus),
			}
			v.pubkeyToStatus[bytesutil.ToBytes48(test.status.publicKey)] = test.status
			active := v.checkAndLogValidatorStatus(100)
			require.Equal(t, test.active, active)
			if test.log != "" {
				require.LogsContain(t, hook, test.log)
			}
		})
	}
}

type doppelGangerRequestMatcher struct {
	req *ethpb.DoppelGangerRequest
}

var _ gomock.Matcher = (*doppelGangerRequestMatcher)(nil)

func (m *doppelGangerRequestMatcher) Matches(x interface{}) bool {
	r, ok := x.(*ethpb.DoppelGangerRequest)
	if !ok {
		panic("Invalid match type")
	}
	return gomock.InAnyOrder(m.req.ValidatorRequests).Matches(r.ValidatorRequests)
}

func (m *doppelGangerRequestMatcher) String() string {
	return fmt.Sprintf("%#v", m.req.ValidatorRequests)
}

func TestValidator_CheckDoppelGanger(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		flgs := features.Get()
		flgs.EnableDoppelGanger = true
		reset := features.InitWithReset(flgs)
		defer reset()
		tests := []struct {
			name            string
			validatorSetter func(t *testing.T) *validator
			err             string
		}{
			{
				name: "no doppelganger",
				validatorSetter: func(t *testing.T) *validator {
					client := validatormock.NewMockValidatorClient(ctrl)
					km := genMockKeymanager(t, 10)
					keys, err := km.FetchValidatingPublicKeys(context.Background())
					assert.NoError(t, err)
					db := dbTest.SetupDB(t, keys, isSlashingProtectionMinimal)
					req := &ethpb.DoppelGangerRequest{ValidatorRequests: []*ethpb.DoppelGangerRequest_ValidatorRequest{}}
					for _, k := range keys {
						pkey := k
						att := createAttestation(10, 12)
						rt, err := att.Data.HashTreeRoot()
						assert.NoError(t, err)
						assert.NoError(t, db.SaveAttestationForPubKey(context.Background(), pkey, rt, att))
						signedRoot := rt[:]
						if isSlashingProtectionMinimal {
							signedRoot = nil
						}
						req.ValidatorRequests = append(req.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{PublicKey: pkey[:], Epoch: att.Data.Target.Epoch, SignedRoot: signedRoot})
					}
					v := &validator{
						validatorClient: client,
						km:              km,
						db:              db,
					}
					client.EXPECT().CheckDoppelGanger(
						gomock.Any(),                     // ctx
						&doppelGangerRequestMatcher{req}, // request
					).Return(nil, nil /*err*/)

					return v
				},
			},
			{
				name: "multiple doppelganger exists",
				validatorSetter: func(t *testing.T) *validator {
					client := validatormock.NewMockValidatorClient(ctrl)
					km := genMockKeymanager(t, 10)
					keys, err := km.FetchValidatingPublicKeys(context.Background())
					assert.NoError(t, err)
					db := dbTest.SetupDB(t, keys, isSlashingProtectionMinimal)
					req := &ethpb.DoppelGangerRequest{ValidatorRequests: []*ethpb.DoppelGangerRequest_ValidatorRequest{}}
					resp := &ethpb.DoppelGangerResponse{Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{}}
					for i, k := range keys {
						pkey := k
						att := createAttestation(10, 12)
						rt, err := att.Data.HashTreeRoot()
						assert.NoError(t, err)
						assert.NoError(t, db.SaveAttestationForPubKey(context.Background(), pkey, rt, att))
						if i%3 == 0 {
							resp.Responses = append(resp.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{PublicKey: pkey[:], DuplicateExists: true})
						}

						signedRoot := rt[:]
						if isSlashingProtectionMinimal {
							signedRoot = nil
						}

						req.ValidatorRequests = append(req.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{PublicKey: pkey[:], Epoch: att.Data.Target.Epoch, SignedRoot: signedRoot})

					}
					v := &validator{
						validatorClient: client,
						km:              km,
						db:              db,
					}
					client.EXPECT().CheckDoppelGanger(
						gomock.Any(),                     // ctx
						&doppelGangerRequestMatcher{req}, // request
					).Return(resp, nil /*err*/)
					return v
				},
				err: "Duplicate instances exists in the network for validator keys",
			},
			{
				name: "single doppelganger exists",
				validatorSetter: func(t *testing.T) *validator {
					client := validatormock.NewMockValidatorClient(ctrl)
					km := genMockKeymanager(t, 10)
					keys, err := km.FetchValidatingPublicKeys(context.Background())
					assert.NoError(t, err)
					db := dbTest.SetupDB(t, keys, isSlashingProtectionMinimal)
					req := &ethpb.DoppelGangerRequest{ValidatorRequests: []*ethpb.DoppelGangerRequest_ValidatorRequest{}}
					resp := &ethpb.DoppelGangerResponse{Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{}}
					for i, k := range keys {
						pkey := k
						att := createAttestation(10, 12)
						rt, err := att.Data.HashTreeRoot()
						assert.NoError(t, err)
						assert.NoError(t, db.SaveAttestationForPubKey(context.Background(), pkey, rt, att))
						if i%9 == 0 {
							resp.Responses = append(resp.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{PublicKey: pkey[:], DuplicateExists: true})
						}
						signedRoot := rt[:]
						if isSlashingProtectionMinimal {
							signedRoot = nil
						}

						req.ValidatorRequests = append(req.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{PublicKey: pkey[:], Epoch: att.Data.Target.Epoch, SignedRoot: signedRoot})
					}
					v := &validator{
						validatorClient: client,
						km:              km,
						db:              db,
					}
					client.EXPECT().CheckDoppelGanger(
						gomock.Any(),                     // ctx
						&doppelGangerRequestMatcher{req}, // request
					).Return(resp, nil /*err*/)
					return v
				},
				err: "Duplicate instances exists in the network for validator keys",
			},
			{
				name: "multiple attestations saved",
				validatorSetter: func(t *testing.T) *validator {
					client := validatormock.NewMockValidatorClient(ctrl)
					km := genMockKeymanager(t, 10)
					keys, err := km.FetchValidatingPublicKeys(context.Background())
					assert.NoError(t, err)
					db := dbTest.SetupDB(t, keys, isSlashingProtectionMinimal)
					req := &ethpb.DoppelGangerRequest{ValidatorRequests: []*ethpb.DoppelGangerRequest_ValidatorRequest{}}
					resp := &ethpb.DoppelGangerResponse{Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{}}
					attLimit := 5
					for i, k := range keys {
						pkey := k
						for j := 0; j < attLimit; j++ {
							att := createAttestation(10+primitives.Epoch(j), 12+primitives.Epoch(j))
							rt, err := att.Data.HashTreeRoot()
							assert.NoError(t, err)
							assert.NoError(t, db.SaveAttestationForPubKey(context.Background(), pkey, rt, att))

							signedRoot := rt[:]
							if isSlashingProtectionMinimal {
								signedRoot = nil
							}

							if j == attLimit-1 {
								req.ValidatorRequests = append(req.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{PublicKey: pkey[:], Epoch: att.Data.Target.Epoch, SignedRoot: signedRoot})
							}
						}
						if i%3 == 0 {
							resp.Responses = append(resp.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{PublicKey: pkey[:], DuplicateExists: true})
						}
					}
					v := &validator{
						validatorClient: client,
						km:              km,
						db:              db,
					}
					client.EXPECT().CheckDoppelGanger(
						gomock.Any(),                     // ctx
						&doppelGangerRequestMatcher{req}, // request
					).Return(resp, nil /*err*/)
					return v
				},
				err: "Duplicate instances exists in the network for validator keys",
			},
			{
				name: "no history exists",
				validatorSetter: func(t *testing.T) *validator {
					client := validatormock.NewMockValidatorClient(ctrl)
					// Use only 1 key for deterministic order.
					km := genMockKeymanager(t, 1)
					keys, err := km.FetchValidatingPublicKeys(context.Background())
					assert.NoError(t, err)
					db := dbTest.SetupDB(t, keys, isSlashingProtectionMinimal)
					resp := &ethpb.DoppelGangerResponse{Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{}}
					req := &ethpb.DoppelGangerRequest{ValidatorRequests: []*ethpb.DoppelGangerRequest_ValidatorRequest{}}
					for _, k := range keys {
						resp.Responses = append(resp.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{PublicKey: k[:], DuplicateExists: false})
						req.ValidatorRequests = append(req.ValidatorRequests, &ethpb.DoppelGangerRequest_ValidatorRequest{PublicKey: k[:], SignedRoot: make([]byte, 32), Epoch: 0})
					}
					v := &validator{
						validatorClient: client,
						km:              km,
						db:              db,
					}
					client.EXPECT().CheckDoppelGanger(
						gomock.Any(), // ctx
						req,          // request
					).Return(resp, nil /*err*/)
					return v
				},
				err: "",
			},
		}
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s/isSlashingProtectionMinimal:%v", tt.name, isSlashingProtectionMinimal), func(t *testing.T) {
				v := tt.validatorSetter(t)
				if err := v.CheckDoppelGanger(context.Background()); tt.err != "" {
					assert.ErrorContains(t, tt.err, err)
				}
			})
		}
	}
}

func TestValidatorAttestationsAreOrdered(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			km := genMockKeymanager(t, 10)
			keys, err := km.FetchValidatingPublicKeys(context.Background())
			assert.NoError(t, err)
			db := dbTest.SetupDB(t, keys, isSlashingProtectionMinimal)

			k := keys[0]
			att := createAttestation(10, 14)
			rt, err := att.Data.HashTreeRoot()
			assert.NoError(t, err)
			assert.NoError(t, db.SaveAttestationForPubKey(context.Background(), k, rt, att))

			att = createAttestation(6, 8)
			rt, err = att.Data.HashTreeRoot()
			assert.NoError(t, err)

			err = db.SaveAttestationForPubKey(context.Background(), k, rt, att)
			if isSlashingProtectionMinimal {
				assert.ErrorContains(t, "could not sign attestation with source lower than recorded source epoch", err)
			} else {
				assert.NoError(t, err)
			}

			att = createAttestation(10, 12)
			rt, err = att.Data.HashTreeRoot()
			assert.NoError(t, err)

			err = db.SaveAttestationForPubKey(context.Background(), k, rt, att)
			if isSlashingProtectionMinimal {
				assert.ErrorContains(t, "could not sign attestation with target lower than or equal to recorded target epoch", err)
			} else {
				assert.NoError(t, err)
			}

			att = createAttestation(2, 3)
			rt, err = att.Data.HashTreeRoot()
			assert.NoError(t, err)

			err = db.SaveAttestationForPubKey(context.Background(), k, rt, att)
			if isSlashingProtectionMinimal {
				assert.ErrorContains(t, "could not sign attestation with source lower than recorded source epoch", err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func createAttestation(source, target primitives.Epoch) *ethpb.IndexedAttestation {
	return &ethpb.IndexedAttestation{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{
				Epoch: source,
				Root:  make([]byte, 32),
			},
			Target: &ethpb.Checkpoint{
				Epoch: target,
				Root:  make([]byte, 32),
			},
			BeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
}

func TestIsSyncCommitteeAggregator_OK(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			v, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			slot := primitives.Slot(1)
			pubKey := validatorKey.PublicKey().Marshal()

			m.validatorClient.EXPECT().SyncSubcommitteeIndex(
				gomock.Any(), // ctx
				&ethpb.SyncSubcommitteeIndexRequest{
					PublicKey: validatorKey.PublicKey().Marshal(),
					Slot:      1,
				},
			).Return(&ethpb.SyncSubcommitteeIndexResponse{}, nil /*err*/)

			aggregator, err := v.isSyncCommitteeAggregator(context.Background(), slot, map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte{
				0: bytesutil.ToBytes48(pubKey),
			})
			require.NoError(t, err)
			require.Equal(t, false, aggregator[0])

			c := params.BeaconConfig().Copy()
			c.TargetAggregatorsPerSyncSubcommittee = math.MaxUint64
			params.OverrideBeaconConfig(c)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().SyncSubcommitteeIndex(
				gomock.Any(), // ctx
				&ethpb.SyncSubcommitteeIndexRequest{
					PublicKey: validatorKey.PublicKey().Marshal(),
					Slot:      1,
				},
			).Return(&ethpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{0}}, nil /*err*/)

			aggregator, err = v.isSyncCommitteeAggregator(context.Background(), slot, map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte{
				0: bytesutil.ToBytes48(pubKey),
			})
			require.NoError(t, err)
			require.Equal(t, true, aggregator[0])
		})
	}
}

func TestIsSyncCommitteeAggregator_Distributed_OK(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			v, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			v.distributed = true
			slot := primitives.Slot(1)
			pubKey := validatorKey.PublicKey().Marshal()

			m.validatorClient.EXPECT().SyncSubcommitteeIndex(
				gomock.Any(), // ctx
				&ethpb.SyncSubcommitteeIndexRequest{
					PublicKey: validatorKey.PublicKey().Marshal(),
					Slot:      1,
				},
			).Return(&ethpb.SyncSubcommitteeIndexResponse{}, nil /*err*/)

			aggregator, err := v.isSyncCommitteeAggregator(context.Background(), slot, map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte{
				0: bytesutil.ToBytes48(pubKey),
			})
			require.NoError(t, err)
			require.Equal(t, false, aggregator[0])

			c := params.BeaconConfig().Copy()
			c.TargetAggregatorsPerSyncSubcommittee = math.MaxUint64
			params.OverrideBeaconConfig(c)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/).Times(2)

			m.validatorClient.EXPECT().SyncSubcommitteeIndex(
				gomock.Any(), // ctx
				&ethpb.SyncSubcommitteeIndexRequest{
					PublicKey: validatorKey.PublicKey().Marshal(),
					Slot:      1,
				},
			).Return(&ethpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{0}}, nil /*err*/)

			sig, err := v.signSyncSelectionData(context.Background(), bytesutil.ToBytes48(pubKey), 0, slot)
			require.NoError(t, err)

			selection := iface.SyncCommitteeSelection{
				SelectionProof:    sig,
				Slot:              1,
				ValidatorIndex:    123,
				SubcommitteeIndex: 0,
			}
			m.validatorClient.EXPECT().AggregatedSyncSelections(
				gomock.Any(), // ctx
				[]iface.SyncCommitteeSelection{selection},
			).Return([]iface.SyncCommitteeSelection{selection}, nil)

			aggregator, err = v.isSyncCommitteeAggregator(context.Background(), slot, map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte{
				123: bytesutil.ToBytes48(pubKey),
			})
			require.NoError(t, err)
			require.Equal(t, true, aggregator[123])
		})
	}
}

func TestValidator_WaitForKeymanagerInitialization_web3Signer(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			ctx := context.Background()
			db := dbTest.SetupDB(t, [][fieldparams.BLSPubkeyLength]byte{}, isSlashingProtectionMinimal)
			root := make([]byte, 32)
			copy(root[2:], "a")
			err := db.SaveGenesisValidatorsRoot(ctx, root)
			require.NoError(t, err)
			app := cli.App{}
			set := flag.NewFlagSet("test", 0)
			newDir := filepath.Join(t.TempDir(), "new")
			require.NoError(t, os.MkdirAll(newDir, 0700))
			set.String(flags.WalletDirFlag.Name, newDir, "")
			w := wallet.NewWalletForWeb3Signer(cli.NewContext(&app, set, nil))
			v := validator{
				db:     db,
				useWeb: false,
				wallet: w,
				web3SignerConfig: &remoteweb3signer.SetupConfig{
					BaseEndpoint:       "http://localhost:8545",
					ProvidedPublicKeys: []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"},
				},
			}
			err = v.WaitForKeymanagerInitialization(context.Background())
			require.NoError(t, err)
			km, err := v.Keymanager()
			require.NoError(t, err)
			require.NotNil(t, km)
		})
	}
}

func TestValidator_WaitForKeymanagerInitialization_Web(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			ctx := context.Background()
			db := dbTest.SetupDB(t, [][fieldparams.BLSPubkeyLength]byte{}, isSlashingProtectionMinimal)
			root := make([]byte, 32)
			copy(root[2:], "a")
			err := db.SaveGenesisValidatorsRoot(ctx, root)
			require.NoError(t, err)
			walletChan := make(chan *wallet.Wallet, 1)
			v := validator{
				db:                    db,
				useWeb:                true,
				walletInitializedFeed: &event.Feed{},
				walletInitializedChan: walletChan,
			}
			wait := make(chan struct{})
			go func() {
				defer close(wait)
				err = v.WaitForKeymanagerInitialization(ctx)
				require.NoError(t, err)
				km, err := v.Keymanager()
				require.NoError(t, err)
				require.NotNil(t, km)
			}()

			walletChan <- wallet.New(&wallet.Config{
				KeymanagerKind: keymanager.Local,
			})
			<-wait
		})
	}
}

func TestValidator_WaitForKeymanagerInitialization_Interop(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			ctx := context.Background()
			db := dbTest.SetupDB(t, [][fieldparams.BLSPubkeyLength]byte{}, isSlashingProtectionMinimal)
			root := make([]byte, 32)
			copy(root[2:], "a")
			err := db.SaveGenesisValidatorsRoot(ctx, root)
			require.NoError(t, err)
			v := validator{
				db:     db,
				useWeb: false,
				interopKeysConfig: &local.InteropKeymanagerConfig{
					NumValidatorKeys: 2,
					Offset:           1,
				},
			}
			err = v.WaitForKeymanagerInitialization(ctx)
			require.NoError(t, err)
			km, err := v.Keymanager()
			require.NoError(t, err)
			require.NotNil(t, km)
		})
	}
}

type PrepareBeaconProposerRequestMatcher struct {
	expectedRecipients []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer
}

func (m *PrepareBeaconProposerRequestMatcher) Matches(x interface{}) bool {
	req, ok := x.(*ethpb.PrepareBeaconProposerRequest)
	if !ok {
		return false
	}

	if len(req.Recipients) != len(m.expectedRecipients) {
		return false
	}

	// Build maps for efficient comparison
	expectedMap := make(map[primitives.ValidatorIndex][]byte)
	for _, recipient := range m.expectedRecipients {
		expectedMap[recipient.ValidatorIndex] = recipient.FeeRecipient
	}

	// Compare the maps
	for _, fc := range req.Recipients {
		expectedFeeRecipient, exists := expectedMap[fc.ValidatorIndex]
		if !exists || !bytes.Equal(expectedFeeRecipient, fc.FeeRecipient) {
			return false
		}
	}
	return true
}

func (m *PrepareBeaconProposerRequestMatcher) String() string {
	return fmt.Sprintf("matches PrepareBeaconProposerRequest with Recipients: %v", m.expectedRecipients)
}

func TestValidator_PushSettings(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		ctrl := gomock.NewController(t)
		ctx := context.Background()
		db := dbTest.SetupDB(t, [][fieldparams.BLSPubkeyLength]byte{}, isSlashingProtectionMinimal)
		client := validatormock.NewMockValidatorClient(ctrl)
		nodeClient := validatormock.NewMockNodeClient(ctrl)
		defaultFeeHex := "0x046Fb65722E7b2455043BFEBf6177F1D2e9738D9"
		byteValueAddress, err := hexutil.Decode("0x046Fb65722E7b2455043BFEBf6177F1D2e9738D9")
		require.NoError(t, err)

		type ExpectedValidatorRegistration struct {
			FeeRecipient []byte
			GasLimit     uint64
			Timestamp    uint64
			Pubkey       []byte
		}

		tests := []struct {
			name                 string
			validatorSetter      func(t *testing.T) *validator
			feeRecipientMap      map[primitives.ValidatorIndex]string
			mockExpectedRequests []ExpectedValidatorRegistration
			err                  string
			logDelay             time.Duration
			logMessages          []string
			doesntContainLogs    bool
		}{
			{
				name: "Happy Path proposer config not nil",
				validatorSetter: func(t *testing.T) *validator {

					v := validator{
						validatorClient:              client,
						nodeClient:                   nodeClient,
						db:                           db,
						pubkeyToStatus:               make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
						signedValidatorRegistrations: make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
						useWeb:                       false,
						interopKeysConfig: &local.InteropKeymanagerConfig{
							NumValidatorKeys: 2,
							Offset:           1,
						},
					}
					err := v.WaitForKeymanagerInitialization(ctx)
					require.NoError(t, err)
					config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
					km, err := v.Keymanager()
					require.NoError(t, err)
					keys, err := km.FetchValidatingPublicKeys(ctx)
					require.NoError(t, err)
					v.pubkeyToStatus[keys[0]] = &validatorStatus{
						publicKey: keys[0][:],
						status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
						index:     primitives.ValidatorIndex(1),
					}
					v.pubkeyToStatus[keys[1]] = &validatorStatus{
						publicKey: keys[1][:],
						status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
						index:     primitives.ValidatorIndex(2),
					}
					client.EXPECT().MultipleValidatorStatus(
						gomock.Any(),
						gomock.Any()).Return(
						&ethpb.MultipleValidatorStatusResponse{
							Statuses:   []*ethpb.ValidatorStatusResponse{{Status: ethpb.ValidatorStatus_ACTIVE}, {Status: ethpb.ValidatorStatus_ACTIVE}},
							PublicKeys: [][]byte{keys[0][:], keys[1][:]},
							Indices:    []primitives.ValidatorIndex{1, 2},
						}, nil)
					client.EXPECT().PrepareBeaconProposer(gomock.Any(), &PrepareBeaconProposerRequestMatcher{
						expectedRecipients: []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
							{FeeRecipient: common.HexToAddress("0x055Fb65722E7b2455043BFEBf6177F1D2e9738D9").Bytes(), ValidatorIndex: 1},
							{FeeRecipient: common.HexToAddress(defaultFeeHex).Bytes(), ValidatorIndex: 2},
						},
					}).Return(nil, nil)
					config[keys[0]] = &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x055Fb65722E7b2455043BFEBf6177F1D2e9738D9"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: 40000000,
						},
					}
					err = v.SetProposerSettings(context.Background(), &proposer.Settings{
						ProposeConfig: config,
						DefaultConfig: &proposer.Option{
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress(defaultFeeHex),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: 35000000,
							},
						},
					})
					require.NoError(t, err)
					client.EXPECT().SubmitValidatorRegistrations(
						gomock.Any(),
						gomock.Any(),
					).Return(&empty.Empty{}, nil)
					return &v
				},
				feeRecipientMap: map[primitives.ValidatorIndex]string{
					1: "0x055Fb65722E7b2455043BFEBf6177F1D2e9738D9",
					2: defaultFeeHex,
				},
				mockExpectedRequests: []ExpectedValidatorRegistration{

					{
						FeeRecipient: common.HexToAddress("0x055Fb65722E7b2455043BFEBf6177F1D2e9738D9").Bytes(),
						GasLimit:     40000000,
					},
					{
						FeeRecipient: byteValueAddress,
						GasLimit:     35000000,
					},
				},
			},
			{
				name: " Happy Path default doesn't send validator registration",
				validatorSetter: func(t *testing.T) *validator {

					v := validator{
						validatorClient:              client,
						nodeClient:                   nodeClient,
						db:                           db,
						pubkeyToStatus:               make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
						signedValidatorRegistrations: make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
						useWeb:                       false,
						interopKeysConfig: &local.InteropKeymanagerConfig{
							NumValidatorKeys: 2,
							Offset:           1,
						},
					}
					err := v.WaitForKeymanagerInitialization(ctx)
					require.NoError(t, err)
					config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
					km, err := v.Keymanager()
					require.NoError(t, err)
					keys, err := km.FetchValidatingPublicKeys(ctx)
					require.NoError(t, err)
					v.pubkeyToStatus[keys[0]] = &validatorStatus{
						publicKey: keys[0][:],
						status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
						index:     primitives.ValidatorIndex(1),
					}
					v.pubkeyToStatus[keys[1]] = &validatorStatus{
						publicKey: keys[1][:],
						status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
						index:     primitives.ValidatorIndex(2),
					}
					client.EXPECT().MultipleValidatorStatus(
						gomock.Any(),
						gomock.Any()).Return(
						&ethpb.MultipleValidatorStatusResponse{
							Statuses:   []*ethpb.ValidatorStatusResponse{{Status: ethpb.ValidatorStatus_ACTIVE}, {Status: ethpb.ValidatorStatus_ACTIVE}},
							PublicKeys: [][]byte{keys[0][:], keys[1][:]},
							Indices:    []primitives.ValidatorIndex{1, 2},
						}, nil)
					client.EXPECT().PrepareBeaconProposer(gomock.Any(), &PrepareBeaconProposerRequestMatcher{
						expectedRecipients: []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
							{FeeRecipient: common.HexToAddress("0x055Fb65722E7b2455043BFEBf6177F1D2e9738D9").Bytes(), ValidatorIndex: 1},
							{FeeRecipient: common.HexToAddress(defaultFeeHex).Bytes(), ValidatorIndex: 2},
						},
					}).Return(nil, nil)
					config[keys[0]] = &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x055Fb65722E7b2455043BFEBf6177F1D2e9738D9"),
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: 40000000,
						},
					}
					err = v.SetProposerSettings(context.Background(), &proposer.Settings{
						ProposeConfig: config,
						DefaultConfig: &proposer.Option{
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress(defaultFeeHex),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  false,
								GasLimit: 35000000,
							},
						},
					})
					require.NoError(t, err)
					client.EXPECT().SubmitValidatorRegistrations(
						gomock.Any(),
						gomock.Any(),
					).Return(&empty.Empty{}, nil)
					return &v
				},
				feeRecipientMap: map[primitives.ValidatorIndex]string{
					1: "0x055Fb65722E7b2455043BFEBf6177F1D2e9738D9",
					2: defaultFeeHex,
				},
				mockExpectedRequests: []ExpectedValidatorRegistration{

					{
						FeeRecipient: common.HexToAddress("0x055Fb65722E7b2455043BFEBf6177F1D2e9738D9").Bytes(),
						GasLimit:     uint64(40000000),
					},
				},
			},
			{
				name: " Happy Path default doesn't send any validator registrations",
				validatorSetter: func(t *testing.T) *validator {
					v := validator{
						validatorClient:              client,
						nodeClient:                   nodeClient,
						db:                           db,
						pubkeyToStatus:               make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
						signedValidatorRegistrations: make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
						useWeb:                       false,
						interopKeysConfig: &local.InteropKeymanagerConfig{
							NumValidatorKeys: 2,
							Offset:           1,
						},
					}
					err := v.WaitForKeymanagerInitialization(ctx)
					require.NoError(t, err)
					config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
					km, err := v.Keymanager()
					require.NoError(t, err)
					keys, err := km.FetchValidatingPublicKeys(ctx)
					require.NoError(t, err)
					v.pubkeyToStatus[keys[0]] = &validatorStatus{
						publicKey: keys[0][:],
						status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
						index:     primitives.ValidatorIndex(1),
					}
					v.pubkeyToStatus[keys[1]] = &validatorStatus{
						publicKey: keys[1][:],
						status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
						index:     primitives.ValidatorIndex(2),
					}
					client.EXPECT().MultipleValidatorStatus(
						gomock.Any(),
						gomock.Any()).Return(
						&ethpb.MultipleValidatorStatusResponse{
							Statuses:   []*ethpb.ValidatorStatusResponse{{Status: ethpb.ValidatorStatus_ACTIVE}, {Status: ethpb.ValidatorStatus_ACTIVE}},
							PublicKeys: [][]byte{keys[0][:], keys[1][:]},
							Indices:    []primitives.ValidatorIndex{1, 2},
						}, nil)
					client.EXPECT().PrepareBeaconProposer(gomock.Any(), &PrepareBeaconProposerRequestMatcher{
						expectedRecipients: []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
							{FeeRecipient: common.HexToAddress("0x055Fb65722E7b2455043BFEBf6177F1D2e9738D9").Bytes(), ValidatorIndex: 1},
							{FeeRecipient: common.HexToAddress(defaultFeeHex).Bytes(), ValidatorIndex: 2},
						},
					}).Return(nil, nil)
					config[keys[0]] = &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x055Fb65722E7b2455043BFEBf6177F1D2e9738D9"),
						},
					}
					err = v.SetProposerSettings(context.Background(), &proposer.Settings{
						ProposeConfig: config,
						DefaultConfig: &proposer.Option{
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress(defaultFeeHex),
							},
						},
					})
					require.NoError(t, err)
					return &v
				},
				feeRecipientMap: map[primitives.ValidatorIndex]string{
					1: "0x055Fb65722E7b2455043BFEBf6177F1D2e9738D9",
					2: defaultFeeHex,
				},
				logMessages:       []string{"will not be included in builder validator registration"},
				doesntContainLogs: true,
			},
			{
				name: " Happy Path",
				validatorSetter: func(t *testing.T) *validator {

					v := validator{
						validatorClient:              client,
						nodeClient:                   nodeClient,
						db:                           db,
						pubkeyToStatus:               make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
						signedValidatorRegistrations: make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
						useWeb:                       false,
						interopKeysConfig: &local.InteropKeymanagerConfig{
							NumValidatorKeys: 1,
							Offset:           1,
						},
						genesisTime: 0,
					}
					// set bellatrix as current epoch
					params.BeaconConfig().BellatrixForkEpoch = 0
					err := v.WaitForKeymanagerInitialization(ctx)
					require.NoError(t, err)
					km, err := v.Keymanager()
					require.NoError(t, err)
					keys, err := km.FetchValidatingPublicKeys(ctx)
					require.NoError(t, err)
					err = v.SetProposerSettings(context.Background(), &proposer.Settings{
						ProposeConfig: nil,
						DefaultConfig: &proposer.Option{
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress(defaultFeeHex),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: validatorType.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
							},
						},
					})
					require.NoError(t, err)
					v.pubkeyToStatus[keys[0]] = &validatorStatus{
						publicKey: keys[0][:],
						status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
						index:     primitives.ValidatorIndex(1),
					}
					client.EXPECT().MultipleValidatorStatus(
						gomock.Any(),
						gomock.Any()).Return(
						&ethpb.MultipleValidatorStatusResponse{
							Statuses:   []*ethpb.ValidatorStatusResponse{{Status: ethpb.ValidatorStatus_ACTIVE}},
							PublicKeys: [][]byte{keys[0][:]},
							Indices:    []primitives.ValidatorIndex{1},
						}, nil)

					client.EXPECT().SubmitValidatorRegistrations(
						gomock.Any(),
						gomock.Any(),
					).Return(&empty.Empty{}, nil)
					client.EXPECT().PrepareBeaconProposer(gomock.Any(), &ethpb.PrepareBeaconProposerRequest{
						Recipients: []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
							{FeeRecipient: common.HexToAddress(defaultFeeHex).Bytes(), ValidatorIndex: 1},
						},
					}).Return(nil, nil)
					return &v
				},
				feeRecipientMap: map[primitives.ValidatorIndex]string{
					1: defaultFeeHex,
				},
				mockExpectedRequests: []ExpectedValidatorRegistration{
					{
						FeeRecipient: byteValueAddress,
						GasLimit:     params.BeaconConfig().DefaultBuilderGasLimit,
					},
				},
			},
			{
				name: " Happy Path validator index not found in cache",
				validatorSetter: func(t *testing.T) *validator {

					v := validator{
						validatorClient:              client,
						nodeClient:                   nodeClient,
						db:                           db,
						pubkeyToStatus:               make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
						signedValidatorRegistrations: make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
						useWeb:                       false,
						interopKeysConfig: &local.InteropKeymanagerConfig{
							NumValidatorKeys: 1,
							Offset:           1,
						},
					}
					err := v.WaitForKeymanagerInitialization(ctx)
					require.NoError(t, err)
					err = v.SetProposerSettings(context.Background(), &proposer.Settings{
						ProposeConfig: nil,
						DefaultConfig: &proposer.Option{
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress(defaultFeeHex),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: 40000000,
							},
						},
					})
					require.NoError(t, err)
					km, err := v.Keymanager()
					require.NoError(t, err)
					keys, err := km.FetchValidatingPublicKeys(ctx)
					require.NoError(t, err)
					v.pubkeyToStatus[keys[0]] = &validatorStatus{
						publicKey: keys[0][:],
						status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
						index:     primitives.ValidatorIndex(1),
					}
					client.EXPECT().MultipleValidatorStatus(
						gomock.Any(),
						gomock.Any()).Return(
						&ethpb.MultipleValidatorStatusResponse{
							Statuses:   []*ethpb.ValidatorStatusResponse{{Status: ethpb.ValidatorStatus_ACTIVE}},
							PublicKeys: [][]byte{keys[0][:]},
							Indices:    []primitives.ValidatorIndex{1},
						}, nil)
					client.EXPECT().SubmitValidatorRegistrations(
						gomock.Any(),
						gomock.Any(),
					).Return(&empty.Empty{}, nil)
					client.EXPECT().PrepareBeaconProposer(gomock.Any(), &ethpb.PrepareBeaconProposerRequest{
						Recipients: []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
							{FeeRecipient: common.HexToAddress(defaultFeeHex).Bytes(), ValidatorIndex: 1},
						},
					}).Return(nil, nil)
					return &v
				},
				feeRecipientMap: map[primitives.ValidatorIndex]string{
					1: defaultFeeHex,
				},
				mockExpectedRequests: []ExpectedValidatorRegistration{
					{
						FeeRecipient: byteValueAddress,
						GasLimit:     params.BeaconConfig().DefaultBuilderGasLimit,
					},
				},
			},
			{
				name: " proposer config not nil but fee recipient empty",
				validatorSetter: func(t *testing.T) *validator {

					v := validator{
						validatorClient:              client,
						nodeClient:                   nodeClient,
						db:                           db,
						pubkeyToStatus:               make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
						signedValidatorRegistrations: make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
						useWeb:                       false,
						interopKeysConfig: &local.InteropKeymanagerConfig{
							NumValidatorKeys: 1,
							Offset:           1,
						},
					}
					err := v.WaitForKeymanagerInitialization(ctx)
					require.NoError(t, err)
					config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
					km, err := v.Keymanager()
					require.NoError(t, err)
					keys, err := km.FetchValidatingPublicKeys(ctx)
					require.NoError(t, err)
					v.pubkeyToStatus[keys[0]] = &validatorStatus{
						publicKey: keys[0][:],
						status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
						index:     primitives.ValidatorIndex(1),
					}
					client.EXPECT().MultipleValidatorStatus(
						gomock.Any(),
						gomock.Any()).Return(
						&ethpb.MultipleValidatorStatusResponse{
							Statuses:   []*ethpb.ValidatorStatusResponse{{Status: ethpb.ValidatorStatus_ACTIVE}},
							PublicKeys: [][]byte{keys[0][:]},
							Indices:    []primitives.ValidatorIndex{1},
						}, nil)
					client.EXPECT().PrepareBeaconProposer(gomock.Any(), &ethpb.PrepareBeaconProposerRequest{
						Recipients: []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
							{FeeRecipient: common.HexToAddress("0x0").Bytes(), ValidatorIndex: 1},
						},
					}).Return(nil, nil)
					config[keys[0]] = &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.Address{},
						},
					}
					err = v.SetProposerSettings(context.Background(), &proposer.Settings{
						ProposeConfig: config,
						DefaultConfig: &proposer.Option{
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress(defaultFeeHex),
							},
						},
					})
					require.NoError(t, err)
					return &v
				},
			},
			{
				name: "Validator index not found with proposeconfig",
				validatorSetter: func(t *testing.T) *validator {

					v := validator{
						validatorClient:              client,
						db:                           db,
						pubkeyToStatus:               make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
						signedValidatorRegistrations: make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
						useWeb:                       false,
						interopKeysConfig: &local.InteropKeymanagerConfig{
							NumValidatorKeys: 1,
							Offset:           1,
						},
					}
					err := v.WaitForKeymanagerInitialization(ctx)
					require.NoError(t, err)
					config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
					km, err := v.Keymanager()
					require.NoError(t, err)
					keys, err := km.FetchValidatingPublicKeys(ctx)
					require.NoError(t, err)
					config[keys[0]] = &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.HexToAddress("0x046Fb65722E7b2455043BFEBf6177F1D2e9738D9"),
						},
					}
					client.EXPECT().MultipleValidatorStatus(
						gomock.Any(),
						gomock.Any()).Return(
						&ethpb.MultipleValidatorStatusResponse{
							Statuses:   []*ethpb.ValidatorStatusResponse{{Status: ethpb.ValidatorStatus_UNKNOWN_STATUS}},
							PublicKeys: [][]byte{keys[0][:]},
							Indices:    []primitives.ValidatorIndex{unknownIndex},
						}, nil)
					err = v.SetProposerSettings(context.Background(), &proposer.Settings{
						ProposeConfig: config,
						DefaultConfig: &proposer.Option{
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress(defaultFeeHex),
							},
						},
					})
					require.NoError(t, err)
					return &v
				},
			},
			{
				name: "register validator batch failed",
				validatorSetter: func(t *testing.T) *validator {
					v := validator{
						validatorClient:              client,
						nodeClient:                   nodeClient,
						db:                           db,
						pubkeyToStatus:               make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
						signedValidatorRegistrations: make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
						useWeb:                       false,
						interopKeysConfig: &local.InteropKeymanagerConfig{
							NumValidatorKeys: 1,
							Offset:           1,
						},
					}
					err := v.WaitForKeymanagerInitialization(ctx)
					require.NoError(t, err)
					config := make(map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option)
					km, err := v.Keymanager()
					require.NoError(t, err)
					keys, err := km.FetchValidatingPublicKeys(ctx)
					require.NoError(t, err)
					v.pubkeyToStatus[keys[0]] = &validatorStatus{
						publicKey: keys[0][:],
						status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
						index:     primitives.ValidatorIndex(1),
					}
					client.EXPECT().MultipleValidatorStatus(
						gomock.Any(),
						gomock.Any()).Return(
						&ethpb.MultipleValidatorStatusResponse{
							Statuses:   []*ethpb.ValidatorStatusResponse{{Status: ethpb.ValidatorStatus_ACTIVE}},
							PublicKeys: [][]byte{keys[0][:]},
							Indices:    []primitives.ValidatorIndex{1},
						}, nil)

					config[keys[0]] = &proposer.Option{
						FeeRecipientConfig: &proposer.FeeRecipientConfig{
							FeeRecipient: common.Address{},
						},
						BuilderConfig: &proposer.BuilderConfig{
							Enabled:  true,
							GasLimit: 40000000,
						},
					}
					err = v.SetProposerSettings(context.Background(), &proposer.Settings{
						ProposeConfig: config,
						DefaultConfig: &proposer.Option{
							FeeRecipientConfig: &proposer.FeeRecipientConfig{
								FeeRecipient: common.HexToAddress(defaultFeeHex),
							},
							BuilderConfig: &proposer.BuilderConfig{
								Enabled:  true,
								GasLimit: 40000000,
							},
						},
					})
					require.NoError(t, err)
					client.EXPECT().PrepareBeaconProposer(gomock.Any(), &ethpb.PrepareBeaconProposerRequest{
						Recipients: []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
							{FeeRecipient: common.HexToAddress("0x0").Bytes(), ValidatorIndex: 1},
						},
					}).Return(nil, nil)
					client.EXPECT().SubmitValidatorRegistrations(
						gomock.Any(),
						gomock.Any(),
					).Return(&empty.Empty{}, errors.New("request failed"))
					return &v
				},
				logMessages: []string{"request failed"},
				logDelay:    1 * time.Second,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				hook := logTest.NewGlobal()
				v := tt.validatorSetter(t)
				km, err := v.Keymanager()
				require.NoError(t, err)
				pubkeys, err := km.FetchValidatingPublicKeys(ctx)
				require.NoError(t, err)
				if tt.feeRecipientMap != nil {
					feeRecipients, err := v.buildPrepProposerReqs(pubkeys)
					require.NoError(t, err)
					signedRegisterValidatorRequests := v.buildSignedRegReqs(ctx, pubkeys, km.Sign, 0, false)
					for _, recipient := range feeRecipients {
						require.Equal(t, strings.ToLower(tt.feeRecipientMap[recipient.ValidatorIndex]), strings.ToLower(hexutil.Encode(recipient.FeeRecipient)))
					}
					require.Equal(t, len(tt.feeRecipientMap), len(feeRecipients))
					for i, request := range tt.mockExpectedRequests {
						require.Equal(t, tt.mockExpectedRequests[i].GasLimit, request.GasLimit)
						require.Equal(t, hexutil.Encode(tt.mockExpectedRequests[i].FeeRecipient), hexutil.Encode(request.FeeRecipient))
					}
					// check if Pubkeys are always unique
					var unique = make(map[string]bool)
					for _, request := range signedRegisterValidatorRequests {
						require.Equal(t, unique[common.BytesToAddress(request.Message.Pubkey).Hex()], false)
						unique[common.BytesToAddress(request.Message.Pubkey).Hex()] = true
					}
					require.Equal(t, len(tt.mockExpectedRequests), len(signedRegisterValidatorRequests))
					require.Equal(t, len(signedRegisterValidatorRequests), len(v.signedValidatorRegistrations))
				}
				if err := v.PushProposerSettings(ctx, km, 0, false); tt.err != "" {
					assert.ErrorContains(t, tt.err, err)
				}
				if len(tt.logMessages) > 0 {
					if tt.logDelay > 0 {
						time.Sleep(tt.logDelay)
					}
					for _, message := range tt.logMessages {
						if tt.doesntContainLogs {
							assert.LogsDoNotContain(t, hook, message)
						} else {
							assert.LogsContain(t, hook, message)
						}
					}

				}
			})
		}
	}
}

func pubkeyFromString(t *testing.T, stringPubkey string) [fieldparams.BLSPubkeyLength]byte {
	pubkeyTemp, err := hexutil.Decode(stringPubkey)
	require.NoError(t, err)

	var pubkey [fieldparams.BLSPubkeyLength]byte
	copy(pubkey[:], pubkeyTemp)

	return pubkey
}

func feeRecipientFromString(t *testing.T, stringFeeRecipient string) common.Address {
	feeRecipientTemp, err := hexutil.Decode(stringFeeRecipient)
	require.NoError(t, err)

	var feeRecipient common.Address
	copy(feeRecipient[:], feeRecipientTemp)

	return feeRecipient
}

func TestValidator_buildPrepProposerReqs_WithoutDefaultConfig(t *testing.T) {
	// pubkey1 => feeRecipient1 (already in `v.validatorIndex`)
	// pubkey2 => feeRecipient2 (NOT in `v.validatorIndex`, index found by beacon node)
	// pubkey3 => feeRecipient3 (NOT in `v.validatorIndex`, index NOT found by beacon node)
	// pubkey4 => Nothing (already in `v.validatorIndex`)

	// Public keys
	pubkey1 := pubkeyFromString(t, "0x111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")
	pubkey2 := pubkeyFromString(t, "0x222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222")
	pubkey3 := pubkeyFromString(t, "0x333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333")
	pubkey4 := pubkeyFromString(t, "0x444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444")

	// Fee recipients
	feeRecipient1 := feeRecipientFromString(t, "0x1111111111111111111111111111111111111111")
	feeRecipient2 := feeRecipientFromString(t, "0x0000000000000000000000000000000000000000")
	feeRecipient3 := feeRecipientFromString(t, "0x3333333333333333333333333333333333333333")
	feeRecipient4 := common.Address{}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	client := validatormock.NewMockValidatorClient(ctrl)

	client.EXPECT().MultipleValidatorStatus(
		gomock.Any(),
		gomock.Any()).Return(
		&ethpb.MultipleValidatorStatusResponse{
			Statuses:   []*ethpb.ValidatorStatusResponse{{Status: ethpb.ValidatorStatus_ACTIVE}, {Status: ethpb.ValidatorStatus_ACTIVE}, {Status: ethpb.ValidatorStatus_UNKNOWN_STATUS}, {Status: ethpb.ValidatorStatus_ACTIVE}},
			PublicKeys: [][]byte{pubkey1[:], pubkey2[:], pubkey3[:], pubkey4[:]},
			Indices:    []primitives.ValidatorIndex{1, 2, unknownIndex, 4},
		}, nil)
	v := validator{
		validatorClient: client,
		proposerSettings: &proposer.Settings{
			DefaultConfig: nil,
			ProposeConfig: map[[48]byte]*proposer.Option{
				pubkey1: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient1,
					},
				},
				pubkey2: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient2,
					},
				},
				pubkey3: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient3,
					},
				},
				pubkey4: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient4,
					},
				},
			},
		},
		pubkeyToStatus: map[[48]byte]*validatorStatus{
			pubkey1: {
				publicKey: pubkey1[:],
				status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
				index:     1,
			},
			pubkey4: {
				publicKey: pubkey4[:],
				status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
				index:     4,
			},
		},
	}

	pubkeys := [][fieldparams.BLSPubkeyLength]byte{pubkey1, pubkey2, pubkey3, pubkey4}

	expected := []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
		{
			ValidatorIndex: 1,
			FeeRecipient:   feeRecipient1[:],
		},
		{
			ValidatorIndex: 2,
			FeeRecipient:   feeRecipient2[:],
		},
		{
			ValidatorIndex: 4,
			FeeRecipient:   feeRecipient4[:],
		},
	}
	filteredKeys, err := v.filterAndCacheActiveKeys(ctx, pubkeys, 0)
	require.NoError(t, err)
	actual, err := v.buildPrepProposerReqs(filteredKeys)
	require.NoError(t, err)
	sort.Slice(actual, func(i, j int) bool {
		return actual[i].ValidatorIndex < actual[j].ValidatorIndex
	})
	assert.DeepEqual(t, expected, actual)
}

func TestValidator_filterAndCacheActiveKeys(t *testing.T) {
	// Public keys
	pubkey1 := pubkeyFromString(t, "0x111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")
	pubkey2 := pubkeyFromString(t, "0x222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222")
	pubkey3 := pubkeyFromString(t, "0x333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333")
	pubkey4 := pubkeyFromString(t, "0x444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444")

	t.Run("refetch all keys at start of epoch", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ctx := context.Background()
		client := validatormock.NewMockValidatorClient(ctrl)

		client.EXPECT().MultipleValidatorStatus(
			gomock.Any(),
			gomock.Any()).Return(
			&ethpb.MultipleValidatorStatusResponse{
				Statuses:   []*ethpb.ValidatorStatusResponse{{Status: ethpb.ValidatorStatus_ACTIVE}, {Status: ethpb.ValidatorStatus_ACTIVE}, {Status: ethpb.ValidatorStatus_UNKNOWN_STATUS}, {Status: ethpb.ValidatorStatus_ACTIVE}},
				PublicKeys: [][]byte{pubkey1[:], pubkey2[:], pubkey3[:], pubkey4[:]},
				Indices:    []primitives.ValidatorIndex{1, 2, unknownIndex, 4},
			}, nil)
		v := validator{
			validatorClient: client,
			pubkeyToStatus:  make(map[[48]byte]*validatorStatus),
		}
		keys, err := v.filterAndCacheActiveKeys(ctx, [][48]byte{pubkey1, pubkey2, pubkey3, pubkey4}, 0)
		require.NoError(t, err)
		// one key is unknown status
		require.Equal(t, 3, len(keys))
	})
	t.Run("refetch all keys at start of epoch, even with cache", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ctx := context.Background()
		client := validatormock.NewMockValidatorClient(ctrl)

		client.EXPECT().MultipleValidatorStatus(
			gomock.Any(),
			gomock.Any()).Return(
			&ethpb.MultipleValidatorStatusResponse{
				Statuses:   []*ethpb.ValidatorStatusResponse{{Status: ethpb.ValidatorStatus_ACTIVE}, {Status: ethpb.ValidatorStatus_ACTIVE}, {Status: ethpb.ValidatorStatus_UNKNOWN_STATUS}, {Status: ethpb.ValidatorStatus_ACTIVE}},
				PublicKeys: [][]byte{pubkey1[:], pubkey2[:], pubkey3[:], pubkey4[:]},
				Indices:    []primitives.ValidatorIndex{1, 2, unknownIndex, 4},
			}, nil)
		v := validator{
			validatorClient: client,
			pubkeyToStatus: map[[48]byte]*validatorStatus{
				pubkey1: {
					publicKey: pubkey1[:],
					status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
					index:     1,
				},
				pubkey2: {
					publicKey: pubkey2[:],
					status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
					index:     2,
				},
				pubkey3: {
					publicKey: pubkey3[:],
					status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}, // gets overridden
					index:     3,
				},
				pubkey4: {
					publicKey: pubkey4[:],
					status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
					index:     4,
				},
			},
		}
		keys, err := v.filterAndCacheActiveKeys(ctx, [][48]byte{pubkey1, pubkey2, pubkey3, pubkey4}, 0)
		require.NoError(t, err)
		// one key is unknown status
		require.Equal(t, 3, len(keys))
	})
	t.Run("cache used mid epoch, no new keys added", func(t *testing.T) {
		ctx := context.Background()
		v := validator{
			pubkeyToStatus: map[[48]byte]*validatorStatus{
				pubkey1: {
					publicKey: pubkey1[:],
					status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
					index:     1,
				},
				pubkey4: {
					publicKey: pubkey4[:],
					status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
					index:     4,
				},
			},
		}
		keys, err := v.filterAndCacheActiveKeys(ctx, [][48]byte{pubkey1, pubkey4}, 5)
		require.NoError(t, err)
		// one key is unknown status
		require.Equal(t, 2, len(keys))
	})

}

func TestValidator_buildPrepProposerReqs_WithDefaultConfig(t *testing.T) {
	// pubkey1 => feeRecipient1 - Status: active
	// pubkey2 => feeRecipient2 - Status: active
	// pubkey3 => feeRecipient3 - Status: unknown
	// pubkey4 => Nothing       - Status: active
	// pubkey5 => Nothing       - Status: exited
	// pubkey6 => Nothing       - Status: pending - ActivationEpoch: 35 (current slot: 641 - current epoch: 20)
	// pubkey7 => Nothing       - Status: pending - ActivationEpoch: 20 (current slot: 641 - current epoch: 20)
	// pubkey8 => feeRecipient8 - Status: exiting

	// Public keys
	pubkey1 := pubkeyFromString(t, "0x111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")
	pubkey2 := pubkeyFromString(t, "0x222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222")
	pubkey3 := pubkeyFromString(t, "0x333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333")
	pubkey4 := pubkeyFromString(t, "0x444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444")
	pubkey5 := pubkeyFromString(t, "0x555555555555555555555555555555555555555555555555555555555555555555555555555555555555555555555555")
	pubkey6 := pubkeyFromString(t, "0x666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666666")
	pubkey7 := pubkeyFromString(t, "0x777777777777777777777777777777777777777777777777777777777777777777777777777777777777777777777777")
	pubkey8 := pubkeyFromString(t, "0x888888888888888888888888888888888888888888888888888888888888888888888888888888888888888888888888")

	// Fee recipients
	feeRecipient1 := feeRecipientFromString(t, "0x1111111111111111111111111111111111111111")
	feeRecipient2 := feeRecipientFromString(t, "0x0000000000000000000000000000000000000000")
	feeRecipient3 := feeRecipientFromString(t, "0x3333333333333333333333333333333333333333")
	feeRecipient8 := feeRecipientFromString(t, "0x8888888888888888888888888888888888888888")

	defaultFeeRecipient := feeRecipientFromString(t, "0xdddddddddddddddddddddddddddddddddddddddd")

	pubkeyToStatus := map[[fieldparams.BLSPubkeyLength]byte]ethpb.ValidatorStatus{
		pubkey1: ethpb.ValidatorStatus_ACTIVE,
		pubkey2: ethpb.ValidatorStatus_ACTIVE,
		pubkey3: ethpb.ValidatorStatus_UNKNOWN_STATUS,
		pubkey4: ethpb.ValidatorStatus_ACTIVE,
		pubkey5: ethpb.ValidatorStatus_EXITED,
		pubkey6: ethpb.ValidatorStatus_PENDING,
		pubkey7: ethpb.ValidatorStatus_PENDING,
		pubkey8: ethpb.ValidatorStatus_EXITING,
	}

	pubkeyToActivationEpoch := map[[fieldparams.BLSPubkeyLength]byte]primitives.Epoch{
		pubkey1: 0,
		pubkey2: 0,
		pubkey3: 0,
		pubkey4: 0,
		pubkey5: 0,
		pubkey6: 35,
		pubkey7: 20,
		pubkey8: 0,
	}

	pubkeyToIndex := map[[fieldparams.BLSPubkeyLength]byte]primitives.ValidatorIndex{
		pubkey1: 1,
		pubkey2: 2,
		pubkey3: unknownIndex,
		pubkey4: 4,
		pubkey5: 5,
		pubkey6: 6,
		pubkey7: 7,
		pubkey8: 8,
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	client := validatormock.NewMockValidatorClient(ctrl)

	client.EXPECT().MultipleValidatorStatus(
		gomock.Any(),
		gomock.Any()).DoAndReturn(func(ctx context.Context, val *ethpb.MultipleValidatorStatusRequest) (*ethpb.MultipleValidatorStatusResponse, error) {
		resp := &ethpb.MultipleValidatorStatusResponse{}

		for _, k := range val.PublicKeys {
			resp.PublicKeys = append(resp.PublicKeys, bytesutil.SafeCopyBytes(k))
			resp.Statuses = append(resp.Statuses, &ethpb.ValidatorStatusResponse{
				Status:          pubkeyToStatus[bytesutil.ToBytes48(k)],
				ActivationEpoch: pubkeyToActivationEpoch[bytesutil.ToBytes48(k)],
			})
			index := pubkeyToIndex[bytesutil.ToBytes48(k)]
			resp.Indices = append(resp.Indices, index)
		}
		return resp, nil
	})

	v := validator{
		validatorClient: client,
		proposerSettings: &proposer.Settings{
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{
					FeeRecipient: defaultFeeRecipient,
				},
			},
			ProposeConfig: map[[fieldparams.BLSPubkeyLength]byte]*proposer.Option{
				pubkey1: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient1,
					},
				},
				pubkey2: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient2,
					},
				},
				pubkey3: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient3,
					},
				},
				pubkey8: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient8,
					},
				},
			},
		},
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			pubkey1: {
				publicKey: pubkey1[:],
				status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
				index:     1,
			},
			pubkey2: {
				publicKey: pubkey2[:],
				status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
				index:     2,
			},
			pubkey3: {
				publicKey: pubkey3[:],
				status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_UNKNOWN_STATUS},
				index:     unknownIndex,
			},
			pubkey4: {
				publicKey: pubkey4[:],
				status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
				index:     4,
			},
			pubkey5: {
				publicKey: pubkey5[:],
				status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
				index:     5,
			},
			pubkey6: {
				publicKey: pubkey6[:],
				status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
				index:     6,
			},
			pubkey7: {
				publicKey: pubkey7[:],
				status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
				index:     7,
			},
			pubkey8: {
				publicKey: pubkey8[:],
				status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
				index:     8,
			},
		},
	}

	pubkeys := [][fieldparams.BLSPubkeyLength]byte{
		pubkey1,
		pubkey2,
		pubkey3,
		pubkey4,
		pubkey5,
		pubkey6,
		pubkey7,
		pubkey8,
	}

	expected := []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
		{
			ValidatorIndex: 1,
			FeeRecipient:   feeRecipient1[:],
		},
		{
			ValidatorIndex: 2,
			FeeRecipient:   feeRecipient2[:],
		},
		{
			ValidatorIndex: 4,
			FeeRecipient:   defaultFeeRecipient[:],
		},
		{
			ValidatorIndex: 7,
			FeeRecipient:   defaultFeeRecipient[:],
		},
		{
			ValidatorIndex: 8,
			FeeRecipient:   feeRecipient8[:],
		},
	}
	filteredKeys, err := v.filterAndCacheActiveKeys(ctx, pubkeys, 640)
	require.NoError(t, err)
	actual, err := v.buildPrepProposerReqs(filteredKeys)
	require.NoError(t, err)
	sort.Slice(actual, func(i, j int) bool {
		return actual[i].ValidatorIndex < actual[j].ValidatorIndex
	})
	assert.DeepEqual(t, expected, actual)
}

func TestValidator_buildSignedRegReqs_DefaultConfigDisabled(t *testing.T) {
	// pubkey1 => feeRecipient1, builder enabled
	// pubkey2 => feeRecipient2, builder disabled
	// pubkey3 => Nothing, builder enabled

	// Public keys
	pubkey1 := pubkeyFromString(t, "0x111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")
	pubkey2 := pubkeyFromString(t, "0x222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222")
	pubkey3 := pubkeyFromString(t, "0x333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333")

	// Fee recipients
	feeRecipient1 := feeRecipientFromString(t, "0x0000000000000000000000000000000000000000")
	feeRecipient2 := feeRecipientFromString(t, "0x2222222222222222222222222222222222222222")

	defaultFeeRecipient := feeRecipientFromString(t, "0xdddddddddddddddddddddddddddddddddddddddd")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	client := validatormock.NewMockValidatorClient(ctrl)

	signature := blsmock.NewMockSignature(ctrl)
	signature.EXPECT().Marshal().Return([]byte{})

	v := validator{
		signedValidatorRegistrations: map[[48]byte]*ethpb.SignedValidatorRegistrationV1{},
		validatorClient:              client,
		proposerSettings: &proposer.Settings{
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{
					FeeRecipient: defaultFeeRecipient,
				},
				BuilderConfig: &proposer.BuilderConfig{
					Enabled:  false,
					GasLimit: 9999,
				},
			},
			ProposeConfig: map[[48]byte]*proposer.Option{
				pubkey1: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient1,
					},
					BuilderConfig: &proposer.BuilderConfig{
						Enabled:  true,
						GasLimit: 1111,
					},
				},
				pubkey2: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient2,
					},
					BuilderConfig: &proposer.BuilderConfig{
						Enabled:  false,
						GasLimit: 2222,
					},
				},
				pubkey3: {
					FeeRecipientConfig: nil,
					BuilderConfig: &proposer.BuilderConfig{
						Enabled:  true,
						GasLimit: 3333,
					},
				},
			},
		},
		pubkeyToStatus: make(map[[48]byte]*validatorStatus),
	}

	pubkeys := [][fieldparams.BLSPubkeyLength]byte{pubkey1, pubkey2, pubkey3}

	var signer = func(_ context.Context, _ *validatorpb.SignRequest) (bls.Signature, error) {
		return signature, nil
	}
	v.pubkeyToStatus[pubkey1] = &validatorStatus{
		publicKey: pubkey1[:],
		status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
		index:     1,
	}
	v.pubkeyToStatus[pubkey2] = &validatorStatus{
		publicKey: pubkey2[:],
		status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
		index:     2,
	}
	v.pubkeyToStatus[pubkey3] = &validatorStatus{
		publicKey: pubkey3[:],
		status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
		index:     3,
	}
	actual := v.buildSignedRegReqs(ctx, pubkeys, signer, 0, false)

	assert.Equal(t, 1, len(actual))
	assert.DeepEqual(t, feeRecipient1[:], actual[0].Message.FeeRecipient)
	assert.Equal(t, uint64(1111), actual[0].Message.GasLimit)
	assert.DeepEqual(t, pubkey1[:], actual[0].Message.Pubkey)

}

func TestValidator_buildSignedRegReqs_DefaultConfigEnabled(t *testing.T) {
	// pubkey1 => feeRecipient1, builder enabled
	// pubkey2 => feeRecipient2, builder disabled
	// pubkey3 => Nothing, builder enabled
	// pubkey4 => added after builder requests built once, used in mid epoch test

	// Public keys
	pubkey1 := pubkeyFromString(t, "0x111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")
	pubkey2 := pubkeyFromString(t, "0x222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222")
	pubkey3 := pubkeyFromString(t, "0x333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333")
	pubkey4 := pubkeyFromString(t, "0x444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444444")

	// Fee recipients
	feeRecipient1 := feeRecipientFromString(t, "0x0000000000000000000000000000000000000000")
	feeRecipient2 := feeRecipientFromString(t, "0x2222222222222222222222222222222222222222")

	defaultFeeRecipient := feeRecipientFromString(t, "0xdddddddddddddddddddddddddddddddddddddddd")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	client := validatormock.NewMockValidatorClient(ctrl)

	signature := blsmock.NewMockSignature(ctrl)
	signature.EXPECT().Marshal().Return([]byte{}).AnyTimes()
	v := validator{
		signedValidatorRegistrations: map[[48]byte]*ethpb.SignedValidatorRegistrationV1{},
		validatorClient:              client,
		proposerSettings: &proposer.Settings{
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{
					FeeRecipient: defaultFeeRecipient,
				},
				BuilderConfig: &proposer.BuilderConfig{
					Enabled:  true,
					GasLimit: 9999,
				},
			},
			ProposeConfig: map[[48]byte]*proposer.Option{
				pubkey1: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient1,
					},
					BuilderConfig: &proposer.BuilderConfig{
						Enabled:  true,
						GasLimit: 1111,
					},
				},
				pubkey2: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient2,
					},
					BuilderConfig: &proposer.BuilderConfig{
						Enabled:  false,
						GasLimit: 2222,
					},
				},
				pubkey3: {
					FeeRecipientConfig: nil,
					BuilderConfig: &proposer.BuilderConfig{
						Enabled:  true,
						GasLimit: 3333,
					},
				},
			},
		},
		pubkeyToStatus: make(map[[48]byte]*validatorStatus),
	}

	pubkeys := [][fieldparams.BLSPubkeyLength]byte{pubkey1, pubkey2, pubkey3}

	var signer = func(_ context.Context, _ *validatorpb.SignRequest) (bls.Signature, error) {
		return signature, nil
	}
	v.pubkeyToStatus[pubkey1] = &validatorStatus{
		publicKey: pubkey1[:],
		status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
		index:     1,
	}
	v.pubkeyToStatus[pubkey2] = &validatorStatus{
		publicKey: pubkey2[:],
		status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
		index:     2,
	}
	v.pubkeyToStatus[pubkey3] = &validatorStatus{
		publicKey: pubkey3[:],
		status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
		index:     3,
	}
	actual := v.buildSignedRegReqs(ctx, pubkeys, signer, 0, false)

	assert.Equal(t, 2, len(actual))

	assert.DeepEqual(t, feeRecipient1[:], actual[0].Message.FeeRecipient)
	assert.Equal(t, uint64(1111), actual[0].Message.GasLimit)
	assert.DeepEqual(t, pubkey1[:], actual[0].Message.Pubkey)

	assert.DeepEqual(t, defaultFeeRecipient[:], actual[1].Message.FeeRecipient)
	assert.Equal(t, uint64(9999), actual[1].Message.GasLimit)
	assert.DeepEqual(t, pubkey3[:], actual[1].Message.Pubkey)

	t.Run("mid epoch only pushes newly added key", func(t *testing.T) {
		v.pubkeyToStatus[pubkey4] = &validatorStatus{
			publicKey: pubkey4[:],
			status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
			index:     4,
		}
		pubkeys = append(pubkeys, pubkey4)
		actual = v.buildSignedRegReqs(ctx, pubkeys, signer, 5, false)
		assert.Equal(t, 1, len(actual))

		assert.DeepEqual(t, defaultFeeRecipient[:], actual[0].Message.FeeRecipient)
		assert.Equal(t, uint64(9999), actual[0].Message.GasLimit)
		assert.DeepEqual(t, pubkey4[:], actual[0].Message.Pubkey)
	})

	t.Run("force push all keys mid epoch", func(t *testing.T) {
		actual = v.buildSignedRegReqs(ctx, pubkeys, signer, 5, true)
		assert.Equal(t, 3, len(actual))
	})
}

func TestValidator_buildSignedRegReqs_SignerOnError(t *testing.T) {
	// Public keys
	pubkey1 := pubkeyFromString(t, "0x111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")

	// Fee recipients
	defaultFeeRecipient := feeRecipientFromString(t, "0xdddddddddddddddddddddddddddddddddddddddd")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	client := validatormock.NewMockValidatorClient(ctrl)

	v := validator{
		signedValidatorRegistrations: map[[48]byte]*ethpb.SignedValidatorRegistrationV1{},
		validatorClient:              client,
		proposerSettings: &proposer.Settings{
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{
					FeeRecipient: defaultFeeRecipient,
				},
				BuilderConfig: &proposer.BuilderConfig{
					Enabled:  true,
					GasLimit: 9999,
				},
			},
		},
	}

	pubkeys := [][fieldparams.BLSPubkeyLength]byte{pubkey1}

	var signer = func(_ context.Context, _ *validatorpb.SignRequest) (bls.Signature, error) {
		return nil, errors.New("custom error")
	}

	actual := v.buildSignedRegReqs(ctx, pubkeys, signer, 0, false)
	assert.Equal(t, 0, len(actual))
}

func TestValidator_buildSignedRegReqs_TimestampBeforeGenesis(t *testing.T) {
	// Public keys
	pubkey1 := pubkeyFromString(t, "0x111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")

	// Fee recipients
	feeRecipient1 := feeRecipientFromString(t, "0x0000000000000000000000000000000000000000")

	defaultFeeRecipient := feeRecipientFromString(t, "0xdddddddddddddddddddddddddddddddddddddddd")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	client := validatormock.NewMockValidatorClient(ctrl)

	signature := blsmock.NewMockSignature(ctrl)

	v := validator{
		signedValidatorRegistrations: map[[48]byte]*ethpb.SignedValidatorRegistrationV1{},
		validatorClient:              client,
		genesisTime:                  uint64(time.Now().UTC().Unix() + 1000),
		proposerSettings: &proposer.Settings{
			DefaultConfig: &proposer.Option{
				FeeRecipientConfig: &proposer.FeeRecipientConfig{
					FeeRecipient: defaultFeeRecipient,
				},
				BuilderConfig: &proposer.BuilderConfig{
					Enabled:  true,
					GasLimit: 9999,
				},
			},
			ProposeConfig: map[[48]byte]*proposer.Option{
				pubkey1: {
					FeeRecipientConfig: &proposer.FeeRecipientConfig{
						FeeRecipient: feeRecipient1,
					},
					BuilderConfig: &proposer.BuilderConfig{
						Enabled:  true,
						GasLimit: 1111,
					},
				},
			},
		},
		pubkeyToStatus: make(map[[48]byte]*validatorStatus),
	}

	pubkeys := [][fieldparams.BLSPubkeyLength]byte{pubkey1}

	var signer = func(_ context.Context, _ *validatorpb.SignRequest) (bls.Signature, error) {
		return signature, nil
	}
	v.pubkeyToStatus[pubkey1] = &validatorStatus{
		publicKey: pubkey1[:],
		status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
		index:     1,
	}
	actual := v.buildSignedRegReqs(ctx, pubkeys, signer, 0, false)
	assert.Equal(t, 0, len(actual))
}

func TestValidator_Host(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := validatormock.NewMockValidatorClient(ctrl)
	v := validator{
		validatorClient: client,
	}

	client.EXPECT().Host().Return("host").Times(1)
	require.Equal(t, "host", v.Host())
}

func TestValidator_ChangeHost(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := validatormock.NewMockValidatorClient(ctrl)
	v := validator{
		validatorClient:  client,
		beaconNodeHosts:  []string{"http://localhost:8080", "http://localhost:8081"},
		currentHostIndex: 0,
	}

	client.EXPECT().SetHost(v.beaconNodeHosts[1])
	client.EXPECT().SetHost(v.beaconNodeHosts[0])
	v.ChangeHost()
	assert.Equal(t, uint64(1), v.currentHostIndex)
	v.ChangeHost()
	assert.Equal(t, uint64(0), v.currentHostIndex)
}

func TestUpdateValidatorStatusCache(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pubkeys := [][fieldparams.BLSPubkeyLength]byte{
		{0x01},
		{0x02},
	}
	statusRequestKeys := [][]byte{
		pubkeys[0][:],
		pubkeys[1][:],
	}

	client := validatormock.NewMockValidatorClient(ctrl)
	mockResponse := &ethpb.MultipleValidatorStatusResponse{
		PublicKeys: statusRequestKeys,
		Statuses: []*ethpb.ValidatorStatusResponse{
			{
				Status: ethpb.ValidatorStatus_ACTIVE,
			}, {
				Status: ethpb.ValidatorStatus_EXITING,
			}},
		Indices: []primitives.ValidatorIndex{1, 2},
	}
	client.EXPECT().MultipleValidatorStatus(
		gomock.Any(),
		gomock.Any()).Return(mockResponse, nil)

	v := &validator{
		validatorClient:  client,
		beaconNodeHosts:  []string{"http://localhost:8080", "http://localhost:8081"},
		currentHostIndex: 0,
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			[fieldparams.BLSPubkeyLength]byte{0x03}: &validatorStatus{ // add non existant key and status to cache, should be fully removed on update
				publicKey: []byte{0x03},
				status: &ethpb.ValidatorStatusResponse{
					Status: ethpb.ValidatorStatus_ACTIVE,
				},
				index: 3,
			},
		},
	}

	err := v.updateValidatorStatusCache(ctx, pubkeys)
	assert.NoError(t, err)

	// make sure the nonexistant key is fully removed
	_, ok := v.pubkeyToStatus[[fieldparams.BLSPubkeyLength]byte{0x03}]
	require.Equal(t, false, ok)
	// make sure we only have the added values
	assert.Equal(t, 2, len(v.pubkeyToStatus))
	for i, pk := range pubkeys {
		status, exists := v.pubkeyToStatus[pk]
		require.Equal(t, true, exists)
		require.DeepEqual(t, pk[:], status.publicKey)
		require.Equal(t, mockResponse.Statuses[i], status.status)
		require.Equal(t, mockResponse.Indices[i], status.index)
	}
}
