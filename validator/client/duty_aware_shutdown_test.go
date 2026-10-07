package client

import (
	"context"
	"sync"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"
)

const (
	testDutyAwareShutdownSlotDuration  = 500 * time.Millisecond
	testDutyAwareShutdownRestartBudget = 100 * time.Millisecond
	testDutyAwareShutdownMaxWait       = 2 * testDutyAwareShutdownSlotDuration
)

// setupDutyAwareShutdownTest uses short slots, and returns a tracker and a genesis time
// such that the current time is `offset` into a slot.
func setupDutyAwareShutdownTest(t *testing.T, offset time.Duration) (*dutyAwareShutdownTracker, time.Time) {
	cfg := params.BeaconConfig().Copy()
	cfg.SlotDurationMilliseconds = uint64(testDutyAwareShutdownSlotDuration.Milliseconds())
	params.SetActiveTestCleanup(t, cfg)

	tracker := newDutyAwareShutdownTracker()
	tracker.restartBudget = testDutyAwareShutdownRestartBudget
	tracker.maxWait = testDutyAwareShutdownMaxWait

	genesis := time.Now().Add(-10*testDutyAwareShutdownSlotDuration - offset)
	return tracker, genesis
}

// everySlot reports a rewarded duty at every slot.
func everySlot(primitives.Slot) bool { return true }

// waitAsync runs the tracker wait in a goroutine, and returns a channel
// receiving the time at which the wait returned.
func waitAsync(ctx context.Context, tracker *dutyAwareShutdownTracker) <-chan time.Time {
	returned := make(chan time.Time, 1)
	go func() {
		tracker.wait(ctx)
		returned <- time.Now()
	}()

	return returned
}

func receive(t *testing.T, returned <-chan time.Time) time.Time {
	select {
	case at := <-returned:
		return at
	case <-time.After(5 * testDutyAwareShutdownSlotDuration):
		t.Fatal("wait did not return")
		return time.Time{}
	}
}

func TestDutyAwareShutdownTrackerWait(t *testing.T) {
	t.Run("nil tracker", func(t *testing.T) {
		var tracker *dutyAwareShutdownTracker
		tracker.start(time.Now(), everySlot)
		tracker.stop()
		tracker.wait(t.Context())
	})

	t.Run("runner inactive", func(t *testing.T) {
		tracker, _ := setupDutyAwareShutdownTest(t, 0)

		start := time.Now()
		tracker.wait(t.Context())
		require.Equal(t, true, time.Since(start) < testDutyAwareShutdownSlotDuration/10)
	})

	t.Run("done before cutoff", func(t *testing.T) {
		hook := logTest.NewGlobal()
		tracker, genesis := setupDutyAwareShutdownTest(t, 50*time.Millisecond)
		tracker.start(genesis, everySlot)
		slot := slots.CurrentSlot(genesis)

		returned := waitAsync(t.Context(), tracker)

		// Not done yet.
		select {
		case <-returned:
			t.Fatal("wait returned before the duties were done")
		case <-time.After(50 * time.Millisecond):
		}

		tracker.markDone(slot)
		at := receive(t, returned)

		require.Equal(t, slot, slots.At(genesis, at))
		require.Equal(t, true, at.Before(slots.UnsafeStartTime(genesis, slot+1).Add(-testDutyAwareShutdownRestartBudget)))
		require.LogsContain(t, hook, "Waiting for the rewarded duties of the slot")
	})

	t.Run("already done", func(t *testing.T) {
		tracker, genesis := setupDutyAwareShutdownTest(t, 50*time.Millisecond)
		tracker.start(genesis, everySlot)
		slot := slots.CurrentSlot(genesis)
		tracker.markDone(slot)

		at := receive(t, waitAsync(t.Context(), tracker))
		require.Equal(t, slot, slots.At(genesis, at))
	})

	t.Run("done after cutoff", func(t *testing.T) {
		tracker, genesis := setupDutyAwareShutdownTest(t, 50*time.Millisecond)
		tracker.start(genesis, everySlot)
		slot := slots.CurrentSlot(genesis)

		returned := waitAsync(t.Context(), tracker)

		// The duties of the current slot are done after the cutoff.
		cutoff := slots.UnsafeStartTime(genesis, slot+1).Add(-testDutyAwareShutdownRestartBudget)
		time.Sleep(time.Until(cutoff.Add(testDutyAwareShutdownRestartBudget / 2)))
		tracker.markDone(slot)

		select {
		case <-returned:
			t.Fatal("wait returned after the cutoff")
		case <-time.After(testDutyAwareShutdownRestartBudget):
		}

		// The duties of the next slot are done before the cutoff.
		time.Sleep(time.Until(slots.UnsafeStartTime(genesis, slot+1).Add(50 * time.Millisecond)))
		tracker.markDone(slot + 1)

		at := receive(t, returned)
		require.Equal(t, slot+1, slots.At(genesis, at))
	})

	t.Run("requested after cutoff", func(t *testing.T) {
		hook := logTest.NewGlobal()
		tracker, genesis := setupDutyAwareShutdownTest(t, testDutyAwareShutdownSlotDuration-testDutyAwareShutdownRestartBudget/2)
		tracker.start(genesis, everySlot)
		slot := slots.CurrentSlot(genesis)
		tracker.markDone(slot)

		returned := waitAsync(t.Context(), tracker)

		select {
		case <-returned:
			t.Fatal("wait returned after the cutoff")
		case <-time.After(testDutyAwareShutdownRestartBudget):
		}

		tracker.markDone(slot + 1)
		at := receive(t, returned)
		require.Equal(t, slot+1, slots.At(genesis, at))

		// The wait for the rewarded duties of the next slot is logged only once.
		require.LogsContain(t, hook, "Too late in the slot to restart before the next one")
		require.LogsDoNotContain(t, hook, "Waiting for the rewarded duties of the slot")
	})

	t.Run("current slot without rewarded duty", func(t *testing.T) {
		tracker, genesis := setupDutyAwareShutdownTest(t, 50*time.Millisecond)
		slot := slots.CurrentSlot(genesis)
		tracker.start(genesis, func(s primitives.Slot) bool { return s != slot })

		// The slot is not marked as done.
		at := receive(t, waitAsync(t.Context(), tracker))
		require.Equal(t, slot, slots.At(genesis, at))
	})

	t.Run("requested after cutoff, next slot without rewarded duty", func(t *testing.T) {
		tracker, genesis := setupDutyAwareShutdownTest(t, testDutyAwareShutdownSlotDuration-testDutyAwareShutdownRestartBudget/2)
		slot := slots.CurrentSlot(genesis)
		tracker.start(genesis, func(s primitives.Slot) bool { return s != slot+1 })
		tracker.markDone(slot)

		at := receive(t, waitAsync(t.Context(), tracker))
		require.Equal(t, slot, slots.At(genesis, at))
	})

	t.Run("give up", func(t *testing.T) {
		tracker, genesis := setupDutyAwareShutdownTest(t, 50*time.Millisecond)
		tracker.start(genesis, everySlot)

		start := time.Now()
		at := receive(t, waitAsync(t.Context(), tracker))

		elapsed := at.Sub(start)
		require.Equal(t, true, elapsed >= testDutyAwareShutdownMaxWait)
		require.Equal(t, true, elapsed < testDutyAwareShutdownMaxWait+testDutyAwareShutdownSlotDuration/2)
	})

	t.Run("context canceled", func(t *testing.T) {
		tracker, genesis := setupDutyAwareShutdownTest(t, 50*time.Millisecond)
		tracker.start(genesis, everySlot)

		ctx, cancel := context.WithCancel(t.Context())
		returned := waitAsync(ctx, tracker)
		cancel()

		at := receive(t, returned)
		require.Equal(t, slots.CurrentSlot(genesis), slots.At(genesis, at))
	})

	t.Run("runner stops", func(t *testing.T) {
		tracker, genesis := setupDutyAwareShutdownTest(t, 50*time.Millisecond)
		tracker.start(genesis, everySlot)

		returned := waitAsync(t.Context(), tracker)
		tracker.stop()

		at := receive(t, returned)
		require.Equal(t, slots.CurrentSlot(genesis), slots.At(genesis, at))
	})
}

func TestDutyAwareShutdownTrackerMarkDone(t *testing.T) {
	t.Run("nil tracker", func(t *testing.T) {
		var tracker *dutyAwareShutdownTracker
		tracker.markDone(1)
	})

	t.Run("prunes old slots", func(t *testing.T) {
		tracker := newDutyAwareShutdownTracker()
		for slot := range primitives.Slot(10) {
			tracker.markDone(slot)
		}

		require.Equal(t, 2, len(tracker.done))
	})
}

func TestHasRewardedDutyAt(t *testing.T) {
	// The duties are for epoch 0.
	const (
		attesterSlot = primitives.Slot(3)
		proposerSlot = primitives.Slot(5)
		otherSlot    = primitives.Slot(7)
	)
	epochEnd := params.BeaconConfig().SlotsPerEpoch - 1

	attester := &ethpb.ValidatorDuty{PublicKey: []byte{1}, ValidatorIndex: 1, AttesterSlot: attesterSlot, ProposerSlots: []primitives.Slot{proposerSlot}}
	syncMember := &ethpb.ValidatorDuty{PublicKey: []byte{2}, ValidatorIndex: 2, AttesterSlot: attesterSlot, IsSyncCommittee: true}

	t.Run("not initialized", func(t *testing.T) {
		v := &validator{duties: &dutyStore{}}
		require.Equal(t, true, v.hasRewardedDutyAt(otherSlot))
	})

	t.Run("other epoch", func(t *testing.T) {
		v := &validator{duties: testDutyStore(attester)}
		require.Equal(t, true, v.hasRewardedDutyAt(epochEnd+1))
	})

	t.Run("attestation", func(t *testing.T) {
		v := &validator{duties: testDutyStore(attester)}
		require.Equal(t, true, v.hasRewardedDutyAt(attesterSlot))
	})

	t.Run("block proposal", func(t *testing.T) {
		v := &validator{duties: testDutyStore(attester)}
		require.Equal(t, true, v.hasRewardedDutyAt(proposerSlot))
	})

	t.Run("no duty", func(t *testing.T) {
		v := &validator{duties: testDutyStore(attester)}
		require.Equal(t, false, v.hasRewardedDutyAt(otherSlot))
	})

	t.Run("sync committee", func(t *testing.T) {
		v := &validator{duties: testDutyStore(syncMember)}
		require.Equal(t, true, v.hasRewardedDutyAt(otherSlot))
	})

	t.Run("sync committee, last slot of the epoch", func(t *testing.T) {
		// The current sync committee does not produce messages at the last slot of the epoch.
		v := &validator{duties: testDutyStore(syncMember)}
		require.Equal(t, false, v.hasRewardedDutyAt(epochEnd))
	})

	t.Run("next sync committee, last slot of the epoch", func(t *testing.T) {
		v := &validator{duties: testDutyStore(attester)}
		v.duties.data.syncNextMap[attester.ValidatorIndex] = true
		require.Equal(t, true, v.hasRewardedDutyAt(epochEnd))
	})

	t.Run("doppelganger pending", func(t *testing.T) {
		v := &validator{duties: testDutyStore(attester)}
		v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{bytesutil.ToBytes48(attester.PublicKey): {}}
		v.doppelGanger.pendingCount.Store(1)
		require.Equal(t, false, v.hasRewardedDutyAt(attesterSlot))
	})
}

func TestIsRewardedRole(t *testing.T) {
	tests := []struct {
		name     string
		role     validatorRole
		expected bool
	}{
		{name: "attester", role: roleAttester, expected: true},
		{name: "proposer", role: roleProposer, expected: true},
		{name: "sync committee", role: roleSyncCommittee, expected: true},
		{name: "aggregator", role: roleAggregator, expected: false},
		{name: "sync committee aggregator", role: roleSyncCommitteeAggregator, expected: false},
		{name: "PTC member", role: rolePTCMember, expected: false},
		{name: "unknown", role: roleUnknown, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, isRewardedRole(tt.role))
		})
	}
}

func TestPerformRoles_MarksDutyAwareShutdownDone(t *testing.T) {
	const slot = primitives.Slot(1)

	tests := []struct {
		waitedFor    bool
		role         validatorRole
		blockingCall func(m *mocks) *gomock.Call
		name         string
	}{
		{
			name: "attester",
			role: roleAttester,
			blockingCall: func(m *mocks) *gomock.Call {
				return m.validatorClient.EXPECT().AttestationData(gomock.Any(), gomock.Any())
			},
			waitedFor: true,
		},
		{
			name: "sync committee",
			role: roleSyncCommittee,
			blockingCall: func(m *mocks) *gomock.Call {
				return m.validatorClient.EXPECT().SyncMessageBlockRoot(gomock.Any(), gomock.Any())
			},
			waitedFor: true,
		},
		{
			name: "sync committee aggregator",
			role: roleSyncCommitteeAggregator,
			blockingCall: func(m *mocks) *gomock.Call {
				return m.validatorClient.EXPECT().SyncSubcommitteeIndex(gomock.Any(), gomock.Any())
			},
			waitedFor: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, m, validatorKey, finish := setup(t, false)
			defer finish()
			v.dutyAwareShutdown = newDutyAwareShutdownTracker()
			pubKey := bytesutil.ToBytes48(validatorKey.PublicKey().Marshal())
			v.duties = testDutyStore(&ethpb.ValidatorDuty{
				PublicKey:       pubKey[:],
				ValidatorIndex:  1,
				CommitteeLength: 1,
			})

			// The duty blocks until released.
			release := make(chan struct{})
			tt.blockingCall(m).DoAndReturn(func(context.Context, any) (any, error) {
				<-release
				return nil, errors.New("stop")
			}).Times(1)

			var wg sync.WaitGroup
			span := &endNotifyingSpan{Span: trace.SpanFromContext(t.Context()), ended: make(chan struct{})}
			performRoles(t.Context(), map[[fieldparams.BLSPubkeyLength]byte][]validatorRole{pubKey: {tt.role}}, v, slot, &wg, span)

			done := v.dutyAwareShutdown.doneChan(slot)
			if tt.waitedFor {
				select {
				case <-done:
					t.Fatal("slot marked as done while a rewarded duty is in progress")
				case <-time.After(100 * time.Millisecond):
				}
			} else {
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("slot not marked as done while only a non rewarded duty is in progress")
				}
			}

			close(release)
			wg.Wait()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("slot not marked as done")
			}

			// Wait for the logging goroutine, which reads the beacon config, before the config is reset.
			<-span.ended
		})
	}
}

// endNotifyingSpan is a span closing `ended` when it ends.
type endNotifyingSpan struct {
	trace.Span
	ended chan struct{}
}

func (s *endNotifyingSpan) End(options ...trace.SpanEndOption) {
	s.Span.End(options...)
	close(s.ended)
}
