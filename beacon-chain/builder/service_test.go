package builder

import (
	"net"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/client/builder"
	buildertesting "github.com/OffchainLabs/prysm/v7/api/client/builder/testing"
	blockchainTesting "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func Test_NewServiceWithBuilder(t *testing.T) {
	s, err := NewService(t.Context(), WithBuilderClient(&buildertesting.MockClient{}))
	require.NoError(t, err)
	assert.Equal(t, true, s.Configured())
}

func Test_NewServiceWithoutBuilder(t *testing.T) {
	s, err := NewService(t.Context())
	require.NoError(t, err)
	assert.Equal(t, false, s.Configured())
}

func Test_NewServiceWithUnresponsiveBuilder(t *testing.T) {
	builderStatusTimeout = 100 * time.Millisecond
	defer func() { builderStatusTimeout = 10 * time.Second }()

	// The listener never accepts: the kernel completes the handshake from the backlog,
	// so the client connects and then waits for a response that never comes.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, ln.Close()) }()

	client, err := builder.NewClient("http://" + ln.Addr().String())
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := NewService(t.Context(), WithBuilderClient(client))
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("NewService is still blocked in the builder status probe")
	}
}

func Test_RegisterValidator(t *testing.T) {
	ctx := t.Context()
	headFetcher := &blockchainTesting.ChainService{}
	builder := buildertesting.NewClient()
	s, err := NewService(ctx, WithHeadFetcher(headFetcher), WithBuilderClient(&builder))
	require.NoError(t, err)
	pubkey := bytesutil.ToBytes48([]byte("pubkey"))
	var feeRecipient [20]byte
	reg := &eth.ValidatorRegistrationV1{Pubkey: pubkey[:], Timestamp: uint64(time.Now().UTC().Unix()), FeeRecipient: feeRecipient[:]}
	require.NoError(t, s.RegisterValidator(ctx, []*eth.SignedValidatorRegistrationV1{{Message: reg}}))
	assert.Equal(t, true, builder.RegisteredVals[pubkey])
	registration, err := s.registrationCache.RegistrationByIndex(0)
	require.NoError(t, err)
	require.DeepEqual(t, reg, registration)
}

func Test_BuilderMethodsWithouClient(t *testing.T) {
	s, err := NewService(t.Context())
	require.NoError(t, err)
	assert.Equal(t, false, s.Configured())

	_, err = s.GetHeader(t.Context(), 0, [32]byte{}, [48]byte{})
	assert.ErrorContains(t, ErrNoBuilder.Error(), err)

	_, _, err = s.SubmitBlindedBlock(t.Context(), nil)
	assert.ErrorContains(t, ErrNoBuilder.Error(), err)

	err = s.RegisterValidator(t.Context(), nil)
	assert.ErrorContains(t, ErrNoBuilder.Error(), err)

	// With no signed auths there's nothing to query; multiplex returns no bids and no error.
	bids, err := s.GetExecutionPayloadBid(t.Context(), 0, [32]byte{}, [32]byte{}, [48]byte{}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, len(bids))

	err = s.SubmitSignedBeaconBlock(t.Context(), "", nil)
	assert.ErrorContains(t, ErrNoBuilder.Error(), err)
}
