package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	dbutil "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	mockP2p "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/testutil"
	mockSync "github.com/prysmaticlabs/prysm/v5/beacon-chain/sync/initial-sync/testing"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNodeServer_GetSyncStatus(t *testing.T) {
	mSync := &mockSync.Sync{IsSyncing: false}
	ns := &Server{
		SyncChecker: mSync,
	}
	res, err := ns.GetSyncStatus(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	assert.Equal(t, false, res.Syncing)
	ns.SyncChecker = &mockSync.Sync{IsSyncing: true}
	res, err = ns.GetSyncStatus(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	assert.Equal(t, true, res.Syncing)
}

func TestNodeServer_GetGenesis(t *testing.T) {
	db := dbutil.SetupDB(t)
	ctx := context.Background()
	addr := common.Address{1, 2, 3}
	require.NoError(t, db.SaveDepositContractAddress(ctx, addr))
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	genValRoot := bytesutil.ToBytes32([]byte("I am root"))
	ns := &Server{
		BeaconDB:           db,
		GenesisTimeFetcher: &mock.ChainService{},
		GenesisFetcher: &mock.ChainService{
			State:          st,
			ValidatorsRoot: genValRoot,
		},
	}
	res, err := ns.GetGenesis(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	assert.DeepEqual(t, addr.Bytes(), res.DepositContractAddress)
	pUnix := timestamppb.New(time.Unix(0, 0))
	assert.Equal(t, res.GenesisTime.Seconds, pUnix.Seconds)
	assert.DeepEqual(t, genValRoot[:], res.GenesisValidatorsRoot)

	ns.GenesisTimeFetcher = &mock.ChainService{Genesis: time.Unix(10, 0)}
	res, err = ns.GetGenesis(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	pUnix = timestamppb.New(time.Unix(10, 0))
	assert.Equal(t, res.GenesisTime.Seconds, pUnix.Seconds)
}

func TestNodeServer_GetVersion(t *testing.T) {
	v := version.Version()
	ns := &Server{}
	res, err := ns.GetVersion(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	assert.Equal(t, v, res.Version)
}

func TestNodeServer_GetImplementedServices(t *testing.T) {
	server := grpc.NewServer()
	ns := &Server{
		Server: server,
	}
	ethpb.RegisterNodeServer(server, ns)
	reflection.Register(server)

	res, err := ns.ListImplementedServices(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	// Expecting node service and Server reflect. As of grpc, v1.65.0, there are two version of server reflection
	// Services: [ethereum.eth.v1alpha1.Node grpc.reflection.v1.ServerReflection grpc.reflection.v1alpha.ServerReflection]
	assert.Equal(t, 3, len(res.Services))
}

func TestNodeServer_GetHost(t *testing.T) {
	server := grpc.NewServer()
	peersProvider := &mockP2p.MockPeersProvider{}
	mP2P := mockP2p.NewTestP2P(t)
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	db, err := enode.OpenDB("")
	require.NoError(t, err)
	lNode := enode.NewLocalNode(db, key)
	record := lNode.Node().Record()
	stringENR, err := p2p.SerializeENR(record)
	require.NoError(t, err)
	ns := &Server{
		PeerManager:  &mockP2p.MockPeerManager{BHost: mP2P.BHost, Enr: record, PID: mP2P.BHost.ID()},
		PeersFetcher: peersProvider,
	}
	ethpb.RegisterNodeServer(server, ns)
	reflection.Register(server)
	h, err := ns.GetHost(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	assert.Equal(t, mP2P.PeerID().String(), h.PeerId)
	assert.Equal(t, stringENR, h.Enr)
}

func TestNodeServer_GetPeer(t *testing.T) {
	server := grpc.NewServer()
	peersProvider := &mockP2p.MockPeersProvider{}
	ns := &Server{
		PeersFetcher: peersProvider,
	}
	ethpb.RegisterNodeServer(server, ns)
	reflection.Register(server)

	res, err := ns.GetPeer(context.Background(), &ethpb.PeerRequest{PeerId: mockP2p.MockRawPeerId0})
	require.NoError(t, err)
	assert.Equal(t, "16Uiu2HAkyWZ4Ni1TpvDS8dPxsozmHY85KaiFjodQuV6Tz5tkHVeR" /* first peer's raw id */, res.PeerId, "Unexpected peer ID")
	assert.Equal(t, int(ethpb.PeerDirection_INBOUND), int(res.Direction), "Expected 1st peer to be an inbound connection")
	assert.Equal(t, int(ethpb.ConnectionState_CONNECTED), int(res.ConnectionState), "Expected peer to be connected")
}

func TestNodeServer_ListPeers(t *testing.T) {
	server := grpc.NewServer()
	peersProvider := &mockP2p.MockPeersProvider{}
	ns := &Server{
		PeersFetcher: peersProvider,
	}
	ethpb.RegisterNodeServer(server, ns)
	reflection.Register(server)

	res, err := ns.ListPeers(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	assert.Equal(t, 2, len(res.Peers))

	var (
		firstPeer  *ethpb.Peer
		secondPeer *ethpb.Peer
	)

	for _, p := range res.Peers {
		if p.PeerId == mockP2p.MockRawPeerId0 {
			firstPeer = p
		}
		if p.PeerId == mockP2p.MockRawPeerId1 {
			secondPeer = p
		}
	}

	assert.NotNil(t, firstPeer)
	assert.NotNil(t, secondPeer)
	assert.Equal(t, int(ethpb.PeerDirection_INBOUND), int(firstPeer.Direction))
	assert.Equal(t, int(ethpb.PeerDirection_OUTBOUND), int(secondPeer.Direction))
}

func TestNodeServer_GetETH1ConnectionStatus(t *testing.T) {
	server := grpc.NewServer()
	ep := "foo"
	err := errors.New("error1")
	errStr := "error1"
	mockFetcher := &testutil.MockExecutionChainInfoFetcher{
		CurrEndpoint: ep,
		CurrError:    err,
	}
	ns := &Server{
		POWChainInfoFetcher: mockFetcher,
	}
	ethpb.RegisterNodeServer(server, ns)
	reflection.Register(server)

	res, err := ns.GetETH1ConnectionStatus(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	assert.Equal(t, ep, res.CurrentAddress)
	assert.Equal(t, errStr, res.CurrentConnectionError)
}

func TestNodeServer_GetHealth(t *testing.T) {
	tests := []struct {
		name         string
		input        *mockSync.Sync
		customStatus uint64
		wantedErr    string
	}{
		{
			name:  "happy path",
			input: &mockSync.Sync{IsSyncing: false, IsSynced: true},
		},
		{
			name:      "syncing",
			input:     &mockSync.Sync{IsSyncing: false},
			wantedErr: "service unavailable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := grpc.NewServer()
			ns := &Server{
				SyncChecker: tt.input,
			}
			ethpb.RegisterNodeServer(server, ns)
			reflection.Register(server)
			_, err := ns.GetHealth(context.Background(), &ethpb.HealthRequest{SyncingStatus: tt.customStatus})
			if tt.wantedErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, tt.wantedErr, err)
		})
	}
}
