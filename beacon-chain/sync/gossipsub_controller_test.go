package sync

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/genesis"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
)

func testGossipsubControllerService(t *testing.T, current primitives.Epoch) *Service {
	closedChan := make(chan struct{})
	close(closedChan)
	peer2peer := p2ptest.NewTestP2P(t)
	chainService := &mockChain.ChainService{
		Genesis:        genesis.Time(),
		ValidatorsRoot: genesis.ValidatorsRoot(),
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	r := &Service{
		ctx:    ctx,
		cancel: cancel,
		cfg: &config{
			p2p:         peer2peer,
			chain:       chainService,
			clock:       defaultClockWithTimeAtEpoch(current),
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		chainStarted:        abool.New(),
		subHandler:          newSubTopicHandler(),
		initialSyncComplete: closedChan,
	}
	r.gossipsubController = NewGossipsubController(context.Background(), r)
	return r
}

func TestGossipsubController_CheckForNextEpochForkSubscriptions(t *testing.T) {
	closedChan := make(chan struct{})
	close(closedChan)
	params.SetupTestConfigCleanup(t)
	
	genesis.StoreEmbeddedDuringTest(t, params.BeaconConfig().ConfigName)
	params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().ElectraForkEpoch + 4096*2
	params.BeaconConfig().InitializeForkSchedule()

	tests := []struct {
		name                string
		svcCreator          func(t *testing.T) *Service
		checkRegistration   func(t *testing.T, s *Service)
		forkEpoch           primitives.Epoch
		epochAtRegistration func(primitives.Epoch) primitives.Epoch
		nextForkEpoch       primitives.Epoch
	}{
		{
			name:                "no fork in the next epoch",
			forkEpoch:           params.BeaconConfig().AltairForkEpoch,
			epochAtRegistration: func(e primitives.Epoch) primitives.Epoch { return e - 2 },
			nextForkEpoch:       params.BeaconConfig().BellatrixForkEpoch,
			checkRegistration:   func(t *testing.T, s *Service) {},
		},
		{
			name:                "altair fork in the next epoch",
			forkEpoch:           params.BeaconConfig().AltairForkEpoch,
			epochAtRegistration: func(e primitives.Epoch) primitives.Epoch { return e - 1 },
			nextForkEpoch:       params.BeaconConfig().BellatrixForkEpoch,
			checkRegistration: func(t *testing.T, s *Service) {
				digest := params.ForkDigest(params.BeaconConfig().AltairForkEpoch)
				expected := fmt.Sprintf(p2p.SyncContributionAndProofSubnetTopicFormat+s.cfg.p2p.Encoding().ProtocolSuffix(), digest)
				assert.Equal(t, true, s.subHandler.topicExists(expected), "subnet topic doesn't exist")
			},
		},
		{
			name: "capella fork in the next epoch",
			checkRegistration: func(t *testing.T, s *Service) {
				digest := params.ForkDigest(params.BeaconConfig().CapellaForkEpoch)
				rpcMap := make(map[string]bool)
				for _, p := range s.cfg.p2p.Host().Mux().Protocols() {
					rpcMap[string(p)] = true
				}

				expected := fmt.Sprintf(p2p.BlsToExecutionChangeSubnetTopicFormat+s.cfg.p2p.Encoding().ProtocolSuffix(), digest)
				assert.Equal(t, true, s.subHandler.topicExists(expected), "subnet topic doesn't exist")
			},
			forkEpoch:           params.BeaconConfig().CapellaForkEpoch,
			nextForkEpoch:       params.BeaconConfig().DenebForkEpoch,
			epochAtRegistration: func(e primitives.Epoch) primitives.Epoch { return e - 1 },
		},
		{
			name: "deneb fork in the next epoch",
			checkRegistration: func(t *testing.T, s *Service) {
				digest := params.ForkDigest(params.BeaconConfig().DenebForkEpoch)
				subIndices := mapFromCount(params.BeaconConfig().BlobsidecarSubnetCount)
				for idx := range subIndices {
					topic := fmt.Sprintf(p2p.BlobSubnetTopicFormat, digest, idx)
					expected := topic + s.cfg.p2p.Encoding().ProtocolSuffix()
					assert.Equal(t, true, s.subHandler.topicExists(expected), fmt.Sprintf("subnet topic %s doesn't exist", expected))
				}
			},
			forkEpoch:           params.BeaconConfig().DenebForkEpoch,
			nextForkEpoch:       params.BeaconConfig().ElectraForkEpoch,
			epochAtRegistration: func(e primitives.Epoch) primitives.Epoch { return e - 1 },
		},
		{
			name: "electra fork in the next epoch",
			checkRegistration: func(t *testing.T, s *Service) {
				digest := params.ForkDigest(params.BeaconConfig().ElectraForkEpoch)
				subIndices := mapFromCount(params.BeaconConfig().BlobsidecarSubnetCountElectra)
				for idx := range subIndices {
					topic := fmt.Sprintf(p2p.BlobSubnetTopicFormat, digest, idx)
					expected := topic + s.cfg.p2p.Encoding().ProtocolSuffix()
					assert.Equal(t, true, s.subHandler.topicExists(expected), fmt.Sprintf("subnet topic %s doesn't exist", expected))
				}
			},
			forkEpoch:           params.BeaconConfig().ElectraForkEpoch,
			nextForkEpoch:       params.BeaconConfig().FuluForkEpoch,
			epochAtRegistration: func(e primitives.Epoch) primitives.Epoch { return e - 1 },
		},
		{
			name: "fulu fork in the next epoch; should not have blob topics",
			checkRegistration: func(t *testing.T, s *Service) {
				// Advance to two epochs after Fulu activation and assert no blob topics remain.
				fulu := params.BeaconConfig().FuluForkEpoch
				target := fulu + 2
				s.cfg.clock = defaultClockWithTimeAtEpoch(target)
				s.gossipsubController.updateActiveTopicFamilies(s.cfg.clock.CurrentEpoch())

				for _, topic := range s.subHandler.allTopics() {
					if strings.Contains(topic, "/"+p2p.GossipBlobSidecarMessage) {
						t.Fatalf("blob topic still exists after Fulu+2: %s", topic)
					}
				}
			},
			forkEpoch:           params.BeaconConfig().FuluForkEpoch,
			nextForkEpoch:       params.BeaconConfig().FuluForkEpoch,
			epochAtRegistration: func(e primitives.Epoch) primitives.Epoch { return e - 1 },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := tt.epochAtRegistration(tt.forkEpoch)
			s := testGossipsubControllerService(t, current)
			s.gossipsubController.updateActiveTopicFamilies(s.cfg.clock.CurrentEpoch())
			tt.checkRegistration(t, s)

			if current != tt.forkEpoch-1 {
				return
			}

			// Ensure the topics were registered for the upcoming fork
			digest := params.ForkDigest(tt.forkEpoch)
			assert.Equal(t, true, s.subHandler.digestExists(digest))

			// After this point we are checking deregistration, which doesn't apply if there isn't a higher
			// nextForkEpoch.
			if tt.forkEpoch >= tt.nextForkEpoch {
				return
			}

			nextDigest := params.ForkDigest(tt.nextForkEpoch)
			// Move the clock to just before the next fork epoch and ensure deregistration is correct
			s.cfg.clock = defaultClockWithTimeAtEpoch(tt.nextForkEpoch - 1)
			s.gossipsubController.updateActiveTopicFamilies(s.cfg.clock.CurrentEpoch())

			s.gossipsubController.updateActiveTopicFamilies(tt.nextForkEpoch)
			assert.Equal(t, true, s.subHandler.digestExists(digest))
			// deregister as if it is the epoch after the next fork epoch
			s.gossipsubController.updateActiveTopicFamilies(tt.nextForkEpoch + 1)
			assert.Equal(t, false, s.subHandler.digestExists(digest))
			assert.Equal(t, true, s.subHandler.digestExists(nextDigest))
		})
	}
}
