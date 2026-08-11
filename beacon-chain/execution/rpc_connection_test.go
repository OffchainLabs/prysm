package execution

import (
	"context"
	"testing"
	"time"

	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
)

// inProcDialer returns a dialer backed by an in-process RPC server, plus an invocation counter.
func inProcDialer(t *testing.T) (RPCClientDialer, *int) {
	server, _, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(server.Stop)
	calls := new(int)
	dialer := func(_ context.Context) (*rpc.Client, error) {
		*calls++
		return rpc.DialInProc(server), nil
	}
	return dialer, calls
}

func overrideBackOffPeriod(t *testing.T, d time.Duration) {
	orig := backOffPeriod
	backOffPeriod = d
	t.Cleanup(func() { backOffPeriod = orig })
}

func TestSetupExecutionClientConnections_InjectedDialer(t *testing.T) {
	dialer, calls := inProcDialer(t)
	s, err := NewService(t.Context(),
		WithDatabase(dbutil.SetupDB(t)),
		WithRPCClientDialer(dialer),
	)
	require.NoError(t, err)

	// No endpoint is configured; the dialer alone must be enough to connect.
	require.NoError(t, s.setupExecutionClientConnections(t.Context(), s.cfg.currHttpEndpoint))
	assert.Equal(t, 1, *calls)
	assert.Equal(t, true, s.ExecutionClientConnected())
	assert.NotNil(t, s.depositContractCaller)
	require.NoError(t, s.Stop())
}

func TestSetupExecutionClientConnections_DialerPrecedesEndpoint(t *testing.T) {
	dialer, calls := inProcDialer(t)
	// The configured endpoint accepts no connections; a successful setup proves
	// the dialer took precedence over it.
	s, err := NewService(t.Context(),
		WithDatabase(dbutil.SetupDB(t)),
		WithHttpEndpoint("http://127.0.0.1:1"),
		WithRPCClientDialer(dialer),
	)
	require.NoError(t, err)

	require.NoError(t, s.setupExecutionClientConnections(t.Context(), s.cfg.currHttpEndpoint))
	assert.Equal(t, 1, *calls)
	assert.Equal(t, true, s.ExecutionClientConnected())
	require.NoError(t, s.Stop())
}

func TestSetupExecutionClientConnections_DialerReturnsNilClient(t *testing.T) {
	s, err := NewService(t.Context(),
		WithDatabase(dbutil.SetupDB(t)),
		WithRPCClientDialer(func(context.Context) (*rpc.Client, error) { return nil, nil }),
	)
	require.NoError(t, err)

	err = s.setupExecutionClientConnections(t.Context(), s.cfg.currHttpEndpoint)
	require.ErrorContains(t, "nil client", err)
	assert.Equal(t, false, s.ExecutionClientConnected())
}

func TestRetryExecutionClientConnection_ReinvokesDialer(t *testing.T) {
	overrideBackOffPeriod(t, time.Millisecond)
	dialer, calls := inProcDialer(t)
	s, err := NewService(t.Context(),
		WithDatabase(dbutil.SetupDB(t)),
		WithRPCClientDialer(dialer),
	)
	require.NoError(t, err)
	require.NoError(t, s.setupExecutionClientConnections(t.Context(), s.cfg.currHttpEndpoint))
	prevClient := s.rpcClient

	s.retryExecutionClientConnection(t.Context(), errors.New("connection lost"))

	assert.Equal(t, 2, *calls)
	assert.Equal(t, true, s.ExecutionClientConnected())
	require.NoError(t, s.runError)
	// The reconnected client is usable and the previous client has been closed.
	var res string
	require.NoError(t, s.rpcClient.CallContext(t.Context(), &res, "net_version"))
	err = prevClient.CallContext(t.Context(), &res, "net_version")
	require.NotNil(t, err, "expected previous client to be closed")
	require.NoError(t, s.Stop())
}

func TestPollConnectionStatus_InjectedDialerReconnects(t *testing.T) {
	overrideBackOffPeriod(t, 5*time.Millisecond)
	server, _, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(server.Stop)
	calls := 0
	dialer := func(_ context.Context) (*rpc.Client, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("execution node not ready")
		}
		return rpc.DialInProc(server), nil
	}
	s, err := NewService(t.Context(),
		WithDatabase(dbutil.SetupDB(t)),
		WithRPCClientDialer(dialer),
	)
	require.NoError(t, err)

	// The poll loop keeps re-invoking the dialer until it succeeds.
	s.pollConnectionStatus(t.Context())
	assert.Equal(t, 3, calls)
	assert.Equal(t, true, s.ExecutionClientConnected())
	require.NoError(t, s.Stop())
}
