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
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

// fakeDynFamily is a test implementation of a dynamic-subnet topic family.
type fakeDynFamily struct {
	baseGossipsubTopicFamily
	topics []string
	name   string
}

func (f *fakeDynFamily) Name() string {
	return f.name
}

func (f *fakeDynFamily) Validator() wrappedVal {
	return nil
}

func (f *fakeDynFamily) Handler() subHandler {
	return noopHandler
}

func (f *fakeDynFamily) Subscribe() {

}

func (f *fakeDynFamily) Unsubscribe() {

}

func (f *fakeDynFamily) GetFullTopicString(subnet uint64) string {
	return fmt.Sprintf("topic-%d", subnet)
}

func (f *fakeDynFamily) GetSubnetsToJoin(_ primitives.Slot) map[uint64]bool {
	return nil
}

func (f *fakeDynFamily) GetSubnetsForBroadcast(_ primitives.Slot) map[uint64]bool {
	return nil
}

func (f *fakeDynFamily) GetTopicsForNode(_ *enode.Node) ([]string, error) {
	return append([]string{}, f.topics...), nil
}

type fakeStaticFamily struct {
	baseGossipsubTopicFamily
	name string
}

func (f *fakeStaticFamily) Name() string {
	return f.name
}

func (f *fakeStaticFamily) Validator() wrappedVal {
	return nil
}

func (f *fakeStaticFamily) Handler() subHandler {
	return noopHandler
}

func (f *fakeStaticFamily) Subscribe() {

}

func (f *fakeStaticFamily) Unsubscribe() {

}

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

func TestGossipsubController_ExtractTopics(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	genesis.StoreEmbeddedDuringTest(t, params.BeaconConfig().ConfigName)

	type tc struct {
		name    string
		setup   func(*GossipsubController)
		ctx     func() context.Context
		node    *enode.Node
		want    []string
		wantErr bool
	}

	dummyNode := new(enode.Node)

	tests := []tc{
		{
			name:    "nil node returns error",
			setup:   func(g *GossipsubController) {},
			ctx:     func() context.Context { return context.Background() },
			node:    nil,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "no families yields empty",
			setup:   func(g *GossipsubController) {},
			ctx:     func() context.Context { return context.Background() },
			node:    dummyNode,
			want:    []string{},
			wantErr: false,
		},
		{
			name: "static family ignored",
			setup: func(g *GossipsubController) {
				g.mu.Lock()
				g.activeTopicFamilies[topicFamilyKey{topicName: "static", forkDigest: [4]byte{1, 2, 3, 4}}] = &fakeStaticFamily{name: "StaticFam"}
				g.mu.Unlock()
			},
			ctx:     func() context.Context { return context.Background() },
			node:    dummyNode,
			want:    []string{},
			wantErr: false,
		},
		{
			name: "single dynamic family topics returned",
			setup: func(g *GossipsubController) {
				fam := &fakeDynFamily{topics: []string{"t1", "t2"}, name: "Dyn1"}
				g.mu.Lock()
				g.activeTopicFamilies[topicFamilyKey{topicName: "dyn1", forkDigest: [4]byte{0}}] = fam
				g.mu.Unlock()
			},
			ctx:     func() context.Context { return context.Background() },
			node:    dummyNode,
			want:    []string{"t1", "t2"},
			wantErr: false,
		},
		{
			name: "multiple dynamic families de-dup",
			setup: func(g *GossipsubController) {
				f1 := &fakeDynFamily{topics: []string{"t1", "t2"}, name: "Dyn1"}
				f2 := &fakeDynFamily{topics: []string{"t2", "t3"}, name: "Dyn2"}
				g.mu.Lock()
				g.activeTopicFamilies[topicFamilyKey{topicName: "static", forkDigest: [4]byte{1, 2, 3, 4}}] = &fakeStaticFamily{name: "StaticFam"}
				g.activeTopicFamilies[topicFamilyKey{topicName: "dyn1", forkDigest: [4]byte{0}}] = f1
				g.activeTopicFamilies[topicFamilyKey{topicName: "dyn2", forkDigest: [4]byte{0}}] = f2
				g.mu.Unlock()
			},
			ctx:     func() context.Context { return context.Background() },
			node:    dummyNode,
			want:    []string{"t1", "t2", "t3"},
			wantErr: false,
		},
		{
			name: "mixed static and dynamic",
			setup: func(g *GossipsubController) {
				f1 := &fakeDynFamily{topics: []string{"a", "b"}, name: "Dyn"}
				s1 := &fakeStaticFamily{name: "Static"}
				g.mu.Lock()
				g.activeTopicFamilies[topicFamilyKey{topicName: "dyn", forkDigest: [4]byte{9}}] = f1
				g.activeTopicFamilies[topicFamilyKey{topicName: "static", forkDigest: [4]byte{9}}] = s1
				g.mu.Unlock()
			},
			ctx:     func() context.Context { return context.Background() },
			node:    dummyNode,
			want:    []string{"a", "b"},
			wantErr: false,
		},
		{
			name: "context cancelled short-circuits",
			setup: func(g *GossipsubController) {
				f1 := &fakeDynFamily{topics: []string{"x"}, name: "Dyn"}
				g.mu.Lock()
				g.activeTopicFamilies[topicFamilyKey{topicName: "dyn", forkDigest: [4]byte{0}}] = f1
				g.mu.Unlock()
			},
			ctx:     func() context.Context { c, cancel := context.WithCancel(context.Background()); cancel(); return c },
			node:    dummyNode,
			want:    nil,
			wantErr: true,
		},
	}

	s := &Service{}
	g := NewGossipsubController(context.Background(), s)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset families for each subtest
			g.mu.Lock()
			g.activeTopicFamilies = make(map[topicFamilyKey]GossipsubTopicFamily)
			g.mu.Unlock()

			tt.setup(g)
			topics, err := g.ExtractTopics(tt.ctx(), tt.node)
			if tt.wantErr {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)

			got := map[string]bool{}
			for _, tpc := range topics {
				got[tpc] = true
			}
			want := map[string]bool{}
			for _, tpc := range tt.want {
				want[tpc] = true
			}
			require.Equal(t, len(want), len(got))
			for k := range want {
				require.Equal(t, true, got[k], "missing topic %s", k)
			}
		})
	}
}
