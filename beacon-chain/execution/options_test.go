package execution

import (
	"context"
	"testing"

	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
)

func TestWithRPCClientDialer(t *testing.T) {
	wantErr := errors.New("dialer invoked")
	dialer := func(context.Context) (*rpc.Client, error) {
		return nil, wantErr
	}
	s, err := NewService(t.Context(),
		WithDatabase(dbutil.SetupDB(t)),
		WithRPCClientDialer(dialer),
	)
	require.NoError(t, err)
	require.NotNil(t, s.cfg.rpcClientDialer)
	_, err = s.cfg.rpcClientDialer(t.Context())
	require.ErrorIs(t, err, wantErr)
}
