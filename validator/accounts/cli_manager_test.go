package accounts

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/config/features"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	testGenesisTime           = 1590832934
	testGenesisValidatorsRoot = "0x0102030405060708091011121314151617181920212223242526272829303132"
	testDepositContract       = "0x00000000219ab540356cbb839cbe05303d7705fa"
)

// genesisRESTServer serves the two endpoints the beacon-api node client reads for Genesis,
// and counts requests so tests can prove whether REST was used at all.
func genesisRESTServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var body string
		switch r.URL.Path {
		case "/eth/v1/beacon/genesis":
			body = `{"data":{"genesis_time":"1590832934","genesis_validators_root":"` +
				testGenesisValidatorsRoot + `","genesis_fork_version":"0x00000000"}}`
		case "/eth/v1/config/deposit_contract":
			body = `{"data":{"chain_id":"1","address":"` + testDepositContract + `"}}`
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", api.JsonMediaType)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

type genesisNodeServer struct {
	ethpb.UnimplementedNodeServer
}

func (*genesisNodeServer) GetGenesis(_ context.Context, _ *emptypb.Empty) (*ethpb.Genesis, error) {
	return &ethpb.Genesis{
		GenesisTime:            &timestamppb.Timestamp{Seconds: testGenesisTime},
		GenesisValidatorsRoot:  hexutil.MustDecode(testGenesisValidatorsRoot),
		DepositContractAddress: hexutil.MustDecode(testDepositContract),
	}, nil
}

// genesisGRPCServer serves the same genesis over gRPC and returns its address, so the
// gRPC and REST paths can be asserted against identical expectations.
func genesisGRPCServer(t *testing.T) string {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	ethpb.RegisterNodeServer(srv, &genesisNodeServer{})
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Log(err)
		}
	}()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func TestPrepareBeaconClients_Genesis(t *testing.T) {
	t.Run("REST enabled reads genesis over HTTP", func(t *testing.T) {
		srv, hits := genesisRESTServer(t)
		reset := features.InitWithReset(&features.Flags{EnableBeaconRESTApi: true})
		defer reset()

		acm := &CLIManager{
			dialOpts:          []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
			beaconRPCProvider: "127.0.0.1:1", // unreachable: nothing may fall back to gRPC
			beaconApiEndpoint: srv.URL,
			beaconApiTimeout:  30 * time.Second,
		}
		_, nodeClient, err := acm.PrepareBeaconClients(context.Background())
		require.NoError(t, err)

		genesis, err := nodeClient.Genesis(context.Background(), &emptypb.Empty{})
		require.NoError(t, err)
		require.Equal(t, int64(testGenesisTime), genesis.GenesisTime.Seconds)
		require.DeepEqual(t, hexutil.MustDecode(testGenesisValidatorsRoot), genesis.GenesisValidatorsRoot)
		require.DeepEqual(t, hexutil.MustDecode(testDepositContract), genesis.DepositContractAddress)
		require.NotEqual(t, int64(0), hits.Load())
	})

	t.Run("REST disabled reads the same genesis over gRPC", func(t *testing.T) {
		restSrv, hits := genesisRESTServer(t)
		grpcAddr := genesisGRPCServer(t)
		reset := features.InitWithReset(&features.Flags{})
		defer reset()

		acm := &CLIManager{
			dialOpts:          []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
			beaconRPCProvider: grpcAddr,
			beaconApiEndpoint: restSrv.URL, // configured but must stay unused
			beaconApiTimeout:  30 * time.Second,
		}
		_, nodeClient, err := acm.PrepareBeaconClients(context.Background())
		require.NoError(t, err)

		genesis, err := nodeClient.Genesis(context.Background(), &emptypb.Empty{})
		require.NoError(t, err)
		require.Equal(t, int64(testGenesisTime), genesis.GenesisTime.Seconds)
		require.DeepEqual(t, hexutil.MustDecode(testGenesisValidatorsRoot), genesis.GenesisValidatorsRoot)
		require.DeepEqual(t, hexutil.MustDecode(testDepositContract), genesis.DepositContractAddress)
		require.Equal(t, int64(0), hits.Load(), "genesis must not be read over REST when the flag is off")
	})
}
