package p2p

import (
	"testing"

	dbutil "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func TestCorrect_ActiveValidatorsCount(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig()
	cfg.ConfigName = "test"

	params.OverrideBeaconConfig(cfg)

	db := dbutil.SetupDB(t)
	s := &Service{
		ctx: t.Context(),
		cfg: &Config{DB: db},
	}
	bState, err := util.NewBeaconState(func(state *ethpb.BeaconState) error {
		validators := make([]*ethpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
		for i := 0; i < len(validators); i++ {
			validators[i] = &ethpb.Validator{
				PublicKey:             make([]byte, 48),
				WithdrawalCredentials: make([]byte, 32),
				ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
				Slashed:               false,
			}
		}
		state.Validators = validators
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisData(s.ctx, bState))

	vals, err := s.retrieveActiveValidators()
	assert.NoError(t, err, "genesis state not retrieved")
	assert.Equal(t, int(params.BeaconConfig().MinGenesisActiveValidatorCount), int(vals), "mainnet genesis active count isn't accurate")
	for i := 0; i < 100; i++ {
		require.NoError(t, bState.AppendValidator(&ethpb.Validator{
			PublicKey:             make([]byte, 48),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			Slashed:               false,
		}))
	}
	require.NoError(t, bState.SetSlot(10000))
	require.NoError(t, db.SaveState(s.ctx, bState, [32]byte{'a'}))
	// Reset count
	s.activeValidatorCount = 0

	// Retrieve last archived state.
	vals, err = s.retrieveActiveValidators()
	assert.NoError(t, err, "genesis state not retrieved")
	assert.Equal(t, int(params.BeaconConfig().MinGenesisActiveValidatorCount)+100, int(vals), "mainnet genesis active count isn't accurate")
}

func TestLoggingParameters(_ *testing.T) {
	logGossipParameters("testing", nil)
	logGossipParameters("testing", &pubsub.TopicScoreParams{})
	// Test out actual gossip parameters.
	logGossipParameters("testing", defaultBlockTopicParams())
	p := defaultAggregateSubnetTopicParams(10000)
	logGossipParameters("testing", p)
	p = defaultAggregateTopicParams(10000)
	logGossipParameters("testing", p)
	logGossipParameters("testing", defaultAttesterSlashingTopicParams())
	logGossipParameters("testing", defaultProposerSlashingTopicParams())
	logGossipParameters("testing", defaultVoluntaryExitTopicParams())
	logGossipParameters("testing", defaultLightClientOptimisticUpdateTopicParams())
	logGossipParameters("testing", defaultLightClientFinalityUpdateTopicParams())
}

func TestPeerScoringParams_IPColocationWhitelist(t *testing.T) {
	tests := []struct {
		name      string
		whitelist []string
		wantCount int
	}{
		{
			name:      "empty whitelist",
			whitelist: []string{},
			wantCount: 0,
		},
		{
			name:      "single IP whitelist",
			whitelist: []string{"192.168.1.1/32"},
			wantCount: 1,
		},
		{
			name:      "multiple IPs whitelist",
			whitelist: []string{"192.168.1.0/24", "10.0.0.0/8", "34.42.19.170/32"},
			wantCount: 3,
		},
		{
			name:      "invalid CIDR skipped",
			whitelist: []string{"192.168.1.0/24", "invalid-cidr", "10.0.0.0/8"},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, _ := peerScoringParams(tt.whitelist)
			assert.NotNil(t, params)
			assert.Equal(t, tt.wantCount, len(params.IPColocationFactorWhitelist))

			// Verify the IP colocation parameters are set correctly
			assert.Equal(t, float64(-35.11), params.IPColocationFactorWeight)
			assert.Equal(t, 10, params.IPColocationFactorThreshold)
		})
	}
}
