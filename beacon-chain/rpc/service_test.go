package rpc

import (
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	mockExecution "github.com/OffchainLabs/prysm/v6/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)
}

func TestLifecycle_OK(t *testing.T) {
	hook := logTest.NewGlobal()
	chainService := &mock.ChainService{
		Genesis: time.Now(),
	}
	rpcService := NewService(t.Context(), &Config{
		Port:                  "7348",
		SyncService:           &mockSync.Sync{IsSyncing: false},
		BlockReceiver:         chainService,
		AttestationReceiver:   chainService,
		HeadFetcher:           chainService,
		GenesisTimeFetcher:    chainService,
		ExecutionChainService: &mockExecution.Chain{},
		StateNotifier:         chainService.StateNotifier(),
		Router:                http.NewServeMux(),
		ClockWaiter:           startup.NewClockSynchronizer(),
	})

	rpcService.Start()

	require.LogsContain(t, hook, "Beacon chain gRPC server listening")
	assert.NoError(t, rpcService.Stop())
	require.LogsContain(t, hook, "Completed graceful stop of beacon-chain gRPC server")
}

func TestStatus_CredentialError(t *testing.T) {
	credentialErr := errors.New("credentialError")
	s := &Service{
		cfg: &Config{SyncService: &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mock.ChainService{Optimistic: false}},
		credentialError: credentialErr,
	}

	assert.ErrorContains(t, s.credentialError.Error(), s.Status())
}

func TestStatus_Optimistic(t *testing.T) {
	s := &Service{
		cfg: &Config{SyncService: &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mock.ChainService{Optimistic: true}},
	}

	assert.ErrorContains(t, "service is optimistic", s.Status())
}

func TestRPC_InsecureEndpoint(t *testing.T) {
	hook := logTest.NewGlobal()
	chainService := &mock.ChainService{Genesis: time.Now()}
	rpcService := NewService(t.Context(), &Config{
		Port:                  "7777",
		SyncService:           &mockSync.Sync{IsSyncing: false},
		BlockReceiver:         chainService,
		GenesisTimeFetcher:    chainService,
		AttestationReceiver:   chainService,
		HeadFetcher:           chainService,
		ExecutionChainService: &mockExecution.Chain{},
		StateNotifier:         chainService.StateNotifier(),
		Router:                http.NewServeMux(),
		ClockWaiter:           startup.NewClockSynchronizer(),
	})

	rpcService.Start()

	require.LogsContain(t, hook, "Beacon chain gRPC server listening")
	require.LogsContain(t, hook, "You are using an insecure gRPC server")
	assert.NoError(t, rpcService.Stop())
}
