package execution

import (
	"context"
	"math/big"
	"strconv"
	"testing"
	"time"

	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
)

// inProcTestRPC serves eth_chainId and net_version for in-process dialer tests.
type inProcTestRPC struct {
	chainID uint64
}

func (r *inProcTestRPC) ChainId(_ context.Context) *hexutil.Big {
	return (*hexutil.Big)(new(big.Int).SetUint64(r.chainID))
}

func (r *inProcTestRPC) Version(_ context.Context) string {
	return strconv.FormatUint(r.chainID, 10)
}

// newInProcServer returns an in-process RPC server reporting the given chain ID.
func newInProcServer(t *testing.T, chainID uint64) *rpc.Server {
	srv := rpc.NewServer()
	api := &inProcTestRPC{chainID: chainID}
	require.NoError(t, srv.RegisterName("eth", api))
	require.NoError(t, srv.RegisterName("net", api))
	t.Cleanup(srv.Stop)
	return srv
}

// inProcDialer returns a dialer backed by an in-process RPC server, plus an invocation counter.
func inProcDialer(t *testing.T) (RPCClientDialer, *int) {
	srv := newInProcServer(t, params.BeaconConfig().DepositChainID)
	calls := new(int)
	dialer := func(_ context.Context) (*rpc.Client, error) {
		*calls++
		return rpc.DialInProc(srv), nil
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

func TestSetupExecutionClientConnections_FailedValidationKeepsPreviousClient(t *testing.T) {
	goodServer := newInProcServer(t, params.BeaconConfig().DepositChainID)
	badServer := newInProcServer(t, params.BeaconConfig().DepositChainID+1)
	calls := 0
	dialer := func(_ context.Context) (*rpc.Client, error) {
		calls++
		if calls == 2 {
			return rpc.DialInProc(badServer), nil
		}
		return rpc.DialInProc(goodServer), nil
	}
	s, err := NewService(t.Context(),
		WithDatabase(dbutil.SetupDB(t)),
		WithRPCClientDialer(dialer),
	)
	require.NoError(t, err)
	require.NoError(t, s.setupExecutionClientConnections(t.Context(), s.cfg.currHttpEndpoint))
	firstClient := s.rpcClient

	// The second dial reaches a node on the wrong chain: setup must fail without
	// replacing or closing the previously attached client.
	err = s.setupExecutionClientConnections(t.Context(), s.cfg.currHttpEndpoint)
	require.ErrorContains(t, "wanted chain ID", err)
	assert.Equal(t, true, firstClient == s.rpcClient, "previous client was replaced")
	var res string
	require.NoError(t, s.rpcClient.CallContext(t.Context(), &res, "net_version"))

	// A subsequent successful dial swaps the client as usual.
	require.NoError(t, s.setupExecutionClientConnections(t.Context(), s.cfg.currHttpEndpoint))
	assert.Equal(t, 3, calls)
	assert.Equal(t, true, firstClient != s.rpcClient, "client was not swapped after successful dial")
	require.NoError(t, s.Stop())
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
	srv := newInProcServer(t, params.BeaconConfig().DepositChainID)
	calls := 0
	dialer := func(_ context.Context) (*rpc.Client, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("execution node not ready")
		}
		return rpc.DialInProc(srv), nil
	}
	// Bound the test so a broken poll loop fails fast instead of hanging.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	s, err := NewService(ctx,
		WithDatabase(dbutil.SetupDB(t)),
		WithRPCClientDialer(dialer),
	)
	require.NoError(t, err)

	// The poll loop keeps re-invoking the dialer until it succeeds.
	s.pollConnectionStatus(ctx)
	assert.Equal(t, 3, calls)
	assert.Equal(t, true, s.ExecutionClientConnected())
	require.NoError(t, s.Stop())
}
