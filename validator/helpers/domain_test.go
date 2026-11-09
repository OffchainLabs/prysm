package helpers

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestComputeDomainData(t *testing.T) {
	cfg := params.BeaconConfig()
	gvr := make([]byte, 32)
	for i := range gvr {
		gvr[i] = byte(i)
	}

	preDenebGenesis := time.Now()       // current epoch 0, before Deneb
	postDenebGenesis := time.Unix(0, 0) // current epoch far past Deneb

	tests := []struct {
		name        string
		genesisTime time.Time
		epoch       primitives.Epoch
		domainType  [4]byte
		wantVersion []byte
	}{
		{
			name:        "proposer domain uses fork at epoch",
			genesisTime: postDenebGenesis,
			epoch:       cfg.AltairForkEpoch,
			domainType:  cfg.DomainBeaconProposer,
			wantVersion: cfg.AltairForkVersion,
		},
		{
			name:        "sync committee domain uses fork at epoch",
			genesisTime: postDenebGenesis,
			epoch:       cfg.DenebForkEpoch,
			domainType:  cfg.DomainSyncCommittee,
			wantVersion: cfg.DenebForkVersion,
		},
		{
			name:        "pre-Deneb exit uses fork at exit epoch",
			genesisTime: preDenebGenesis,
			epoch:       cfg.AltairForkEpoch,
			domainType:  cfg.DomainVoluntaryExit,
			wantVersion: cfg.AltairForkVersion,
		},
		{
			name:        "post-Deneb exit uses Capella version",
			genesisTime: postDenebGenesis,
			epoch:       cfg.AltairForkEpoch,
			domainType:  cfg.DomainVoluntaryExit,
			wantVersion: cfg.CapellaForkVersion,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ComputeDomainData(tt.epoch, tt.domainType, gvr, tt.genesisTime)
			require.NoError(t, err)

			want, err := signing.ComputeDomain(tt.domainType, tt.wantVersion, gvr)
			require.NoError(t, err)
			require.DeepEqual(t, want, resp.SignatureDomain)
			require.DeepEqual(t, tt.domainType[:], resp.SignatureDomain[:4])
		})
	}

	t.Run("invalid genesis validators root length", func(t *testing.T) {
		_, err := ComputeDomainData(0, cfg.DomainBeaconProposer, gvr[:31], postDenebGenesis)
		require.ErrorContains(t, "invalid genesis validators root length", err)
	})
}
