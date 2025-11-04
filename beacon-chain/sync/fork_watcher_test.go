package sync

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/genesis"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func defaultClockWithTimeAtEpoch(epoch primitives.Epoch) *startup.Clock {
	now := genesis.Time().Add(params.EpochsDuration(epoch, params.BeaconConfig()))
	return startup.NewClock(genesis.Time(), genesis.ValidatorsRoot(), startup.WithTimeAsNow(now))
}

func testForkWatcherService(t *testing.T, current primitives.Epoch) *Service {
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
	return r
}

func TestRegisterSubscriptions_Idempotent(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	genesis.StoreEmbeddedDuringTest(t, params.BeaconConfig().ConfigName)
	fulu := params.BeaconConfig().ElectraForkEpoch + 4096*2
	params.BeaconConfig().FuluForkEpoch = fulu
	params.BeaconConfig().InitializeForkSchedule()

	current := fulu - 1
	s := testForkWatcherService(t, current)
	next := params.GetNetworkScheduleEntry(fulu)
	wg := attachSpawner(s)
	require.Equal(t, true, s.registerSubscribers(next))
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for subscriptions to be registered")
	case <-done:
	}
	// the goal of this callback is just to assert that spawn is never called.
	s.subscriptionSpawner = func(func()) { t.Error("registration routines spawned twice for the same digest") }
	require.NoError(t, s.ensureRegistrationsForEpoch(fulu))
}

func TestService_CheckForNextEpochFork(t *testing.T) {
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
				rpcMap := make(map[string]bool)
				for _, p := range s.cfg.p2p.Host().Mux().Protocols() {
					rpcMap[string(p)] = true
				}
				assert.Equal(t, true, rpcMap[p2p.RPCBlocksByRangeTopicV2+s.cfg.p2p.Encoding().ProtocolSuffix()], "topic doesn't exist")
				assert.Equal(t, true, rpcMap[p2p.RPCBlocksByRootTopicV2+s.cfg.p2p.Encoding().ProtocolSuffix()], "topic doesn't exist")
				assert.Equal(t, true, rpcMap[p2p.RPCMetaDataTopicV2+s.cfg.p2p.Encoding().ProtocolSuffix()], "topic doesn't exist")
				expected := fmt.Sprintf(p2p.SyncContributionAndProofSubnetTopicFormat+s.cfg.p2p.Encoding().ProtocolSuffix(), digest)
				assert.Equal(t, true, s.subHandler.topicExists(expected), "subnet topic doesn't exist")
				// TODO: we should check subcommittee indices here but we need to work with the committee cache to do it properly
				/*
					subIndices := mapFromCount(params.BeaconConfig().SyncCommitteeSubnetCount)
					for idx := range subIndices {
						topic := fmt.Sprintf(p2p.SyncCommitteeSubnetTopicFormat, digest, idx)
						expected := topic + s.cfg.p2p.Encoding().ProtocolSuffix()
						assert.Equal(t, true, s.subHandler.topicExists(expected), fmt.Sprintf("subnet topic %s doesn't exist", expected))
					}
				*/
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
				rpcMap := make(map[string]bool)
				for _, p := range s.cfg.p2p.Host().Mux().Protocols() {
					rpcMap[string(p)] = true
				}
				subIndices := mapFromCount(params.BeaconConfig().BlobsidecarSubnetCount)
				for idx := range subIndices {
					topic := fmt.Sprintf(p2p.BlobSubnetTopicFormat, digest, idx)
					expected := topic + s.cfg.p2p.Encoding().ProtocolSuffix()
					assert.Equal(t, true, s.subHandler.topicExists(expected), fmt.Sprintf("subnet topic %s doesn't exist", expected))
				}
				assert.Equal(t, true, rpcMap[p2p.RPCBlobSidecarsByRangeTopicV1+s.cfg.p2p.Encoding().ProtocolSuffix()], "topic doesn't exist")
				assert.Equal(t, true, rpcMap[p2p.RPCBlobSidecarsByRootTopicV1+s.cfg.p2p.Encoding().ProtocolSuffix()], "topic doesn't exist")
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
			name: "fulu fork in the next epoch",
			checkRegistration: func(t *testing.T, s *Service) {
				rpcMap := make(map[string]bool)
				for _, p := range s.cfg.p2p.Host().Mux().Protocols() {
					rpcMap[string(p)] = true
				}
				assert.Equal(t, true, rpcMap[p2p.RPCMetaDataTopicV3+s.cfg.p2p.Encoding().ProtocolSuffix()], "topic doesn't exist")
			},
			forkEpoch:           params.BeaconConfig().FuluForkEpoch,
			nextForkEpoch:       params.BeaconConfig().FuluForkEpoch,
			epochAtRegistration: func(e primitives.Epoch) primitives.Epoch { return e - 1 },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := tt.epochAtRegistration(tt.forkEpoch)
			s := testForkWatcherService(t, current)
			wg := attachSpawner(s)
			require.NoError(t, s.ensureRegistrationsForEpoch(s.cfg.clock.CurrentEpoch()))
			wg.Wait()
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
			wg = attachSpawner(s)
			s.cfg.clock = defaultClockWithTimeAtEpoch(tt.nextForkEpoch - 1)
			require.NoError(t, s.ensureRegistrationsForEpoch(s.cfg.clock.CurrentEpoch()))
			wg.Wait()

			require.NoError(t, s.ensureDeregistrationForEpoch(tt.nextForkEpoch))
			assert.Equal(t, true, s.subHandler.digestExists(digest))
			// deregister as if it is the epoch after the next fork epoch
			require.NoError(t, s.ensureDeregistrationForEpoch(tt.nextForkEpoch+1))
			assert.Equal(t, false, s.subHandler.digestExists(digest))
			assert.Equal(t, true, s.subHandler.digestExists(nextDigest))
		})
	}
}

func attachSpawner(s *Service) *sync.WaitGroup {
	wg := new(sync.WaitGroup)
	s.subscriptionSpawner = func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
	}
	return wg
}

// oneEpoch returns the duration of one epoch.
func oneEpoch() time.Duration {
	return time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot)) * time.Second
}
