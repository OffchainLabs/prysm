package events

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen/mock"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v6/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/eth/v1"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/r3labs/sse/v2"
	"github.com/sirupsen/logrus"
)

var testEventWriteTimeout = 100 * time.Millisecond

func requireAllEventsReceived(t *testing.T, stn, opn *mockChain.EventFeedWrapper, events []*feed.Event, req *topicRequest, s *Server, w *StreamingResponseWriterRecorder, logs chan *logrus.Entry) {
	// maxBufferSize param copied from sse lib client code
	sseR := sse.NewEventStreamReader(w.Body(), 1<<24)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	expected := make(map[string]bool)
	for i := range events {
		ev := events[i]
		// serialize the event the same way the server will so that we can compare expectation to results.
		top := topicForEvent(ev)
		eb, err := s.lazyReaderForEvent(t.Context(), ev, req)
		require.NoError(t, err)
		exb, err := io.ReadAll(eb())
		require.NoError(t, err)
		exs := string(exb[0 : len(exb)-2]) // remove trailing double newline

		if topicsForOpsFeed[top] {
			if err := opn.WaitForSubscription(ctx); err != nil {
				t.Fatal(err)
			}
			// Send the event on the feed.
			s.OperationNotifier.OperationFeed().Send(ev)
		} else {
			if err := stn.WaitForSubscription(ctx); err != nil {
				t.Fatal(err)
			}
			// Send the event on the feed.
			s.StateNotifier.StateFeed().Send(ev)
		}
		expected[exs] = true
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			ev, err := sseR.ReadEvent()
			if err == io.EOF {
				return
			}
			require.NoError(t, err)
			str := string(ev)
			delete(expected, str)
			if len(expected) == 0 {
				return
			}
		}
	}()
	for {
		select {
		case entry := <-logs:
			errAttr, ok := entry.Data[logrus.ErrorKey]
			if ok {
				t.Errorf("unexpected error in logs: %v", errAttr)
			}
		case <-done:
			require.Equal(t, 0, len(expected), "expected events not seen")
			return
		case <-ctx.Done():
			t.Fatalf("context canceled / timed out waiting for events, err=%v", ctx.Err())
		}
	}
}

func (tr *topicRequest) testHttpRequest(ctx context.Context, _ *testing.T) *http.Request {
	tq := make([]string, 0, len(tr.topics))
	for topic := range tr.topics {
		tq = append(tq, "topics="+topic)
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://example.com/eth/v1/events?%s", strings.Join(tq, "&")), nil)
	return req.WithContext(ctx)
}

func operationEventsFixtures(t *testing.T) (*topicRequest, []*feed.Event) {
	topics, err := newTopicRequest([]string{
		AttestationTopic,
		SingleAttestationTopic,
		VoluntaryExitTopic,
		SyncCommitteeContributionTopic,
		BLSToExecutionChangeTopic,
		BlobSidecarTopic,
		AttesterSlashingTopic,
		ProposerSlashingTopic,
		BlockGossipTopic,
		DataColumnTopic,
	})
	require.NoError(t, err)
	ro, err := blocks.NewROBlob(util.HydrateBlobSidecar(&eth.BlobSidecar{}))
	require.NoError(t, err)
	vblob := blocks.NewVerifiedROBlob(ro)

	// Create a test block for block gossip event
	block := util.NewBeaconBlock()
	block.Block.Slot = 123
	signedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	return topics, []*feed.Event{
		{
			Type: operation.UnaggregatedAttReceived,
			Data: &operation.UnAggregatedAttReceivedData{
				Attestation: util.HydrateAttestation(&eth.Attestation{}),
			},
		},
		{
			Type: operation.AggregatedAttReceived,
			Data: &operation.AggregatedAttReceivedData{
				Attestation: &eth.AggregateAttestationAndProof{
					AggregatorIndex: 0,
					Aggregate:       util.HydrateAttestation(&eth.Attestation{}),
					SelectionProof:  make([]byte, 96),
				},
			},
		},
		{
			Type: operation.SingleAttReceived,
			Data: &operation.SingleAttReceivedData{
				Attestation: util.HydrateSingleAttestation(&eth.SingleAttestation{}),
			},
		},
		{
			Type: operation.ExitReceived,
			Data: &operation.ExitReceivedData{
				Exit: &eth.SignedVoluntaryExit{
					Exit: &eth.VoluntaryExit{
						Epoch:          0,
						ValidatorIndex: 0,
					},
					Signature: make([]byte, 96),
				},
			},
		},
		{
			Type: operation.SyncCommitteeContributionReceived,
			Data: &operation.SyncCommitteeContributionReceivedData{
				Contribution: &eth.SignedContributionAndProof{
					Message: &eth.ContributionAndProof{
						AggregatorIndex: 0,
						Contribution: &eth.SyncCommitteeContribution{
							Slot:              0,
							BlockRoot:         make([]byte, 32),
							SubcommitteeIndex: 0,
							AggregationBits:   make([]byte, 16),
							Signature:         make([]byte, 96),
						},
						SelectionProof: make([]byte, 96),
					},
					Signature: make([]byte, 96),
				},
			},
		},
		{
			Type: operation.BLSToExecutionChangeReceived,
			Data: &operation.BLSToExecutionChangeReceivedData{
				Change: &eth.SignedBLSToExecutionChange{
					Message: &eth.BLSToExecutionChange{
						ValidatorIndex:     0,
						FromBlsPubkey:      make([]byte, 48),
						ToExecutionAddress: make([]byte, 20),
					},
					Signature: make([]byte, 96),
				},
			},
		},
		{
			Type: operation.BlobSidecarReceived,
			Data: &operation.BlobSidecarReceivedData{
				Blob: &vblob,
			},
		},
		{
			Type: operation.AttesterSlashingReceived,
			Data: &operation.AttesterSlashingReceivedData{
				AttesterSlashing: &eth.AttesterSlashing{
					Attestation_1: &eth.IndexedAttestation{
						AttestingIndices: []uint64{0, 1},
						Data: &eth.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature: make([]byte, fieldparams.BLSSignatureLength),
					},
					Attestation_2: &eth.IndexedAttestation{
						AttestingIndices: []uint64{0, 1},
						Data: &eth.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature: make([]byte, fieldparams.BLSSignatureLength),
					},
				},
			},
		},
		{
			Type: operation.AttesterSlashingReceived,
			Data: &operation.AttesterSlashingReceivedData{
				AttesterSlashing: &eth.AttesterSlashingElectra{
					Attestation_1: &eth.IndexedAttestationElectra{
						AttestingIndices: []uint64{0, 1},
						Data: &eth.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature: make([]byte, fieldparams.BLSSignatureLength),
					},
					Attestation_2: &eth.IndexedAttestationElectra{
						AttestingIndices: []uint64{0, 1},
						Data: &eth.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &eth.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature: make([]byte, fieldparams.BLSSignatureLength),
					},
				},
			},
		},
		{
			Type: operation.ProposerSlashingReceived,
			Data: &operation.ProposerSlashingReceivedData{
				ProposerSlashing: &eth.ProposerSlashing{
					Header_1: &eth.SignedBeaconBlockHeader{
						Header: &eth.BeaconBlockHeader{
							ParentRoot: make([]byte, fieldparams.RootLength),
							StateRoot:  make([]byte, fieldparams.RootLength),
							BodyRoot:   make([]byte, fieldparams.RootLength),
						},
						Signature: make([]byte, fieldparams.BLSSignatureLength),
					},
					Header_2: &eth.SignedBeaconBlockHeader{
						Header: &eth.BeaconBlockHeader{
							ParentRoot: make([]byte, fieldparams.RootLength),
							StateRoot:  make([]byte, fieldparams.RootLength),
							BodyRoot:   make([]byte, fieldparams.RootLength),
						},
						Signature: make([]byte, fieldparams.BLSSignatureLength),
					},
				},
			},
		},
		{
			Type: operation.BlockGossipReceived,
			Data: &operation.BlockGossipReceivedData{
				SignedBlock: signedBlock,
			},
		},
		{
			Type: operation.DataColumnReceived,
			Data: &operation.DataColumnReceivedData{
				Slot:           1,
				Index:          2,
				BlockRoot:      [32]byte{'a'},
				KzgCommitments: [][]byte{{'a'}, {'b'}, {'c'}},
			},
		},
	}
}

type streamTestSync struct {
	done   chan struct{}
	cancel func()
	undo   func()
	logs   chan *logrus.Entry
	ctx    context.Context
	t      *testing.T
}

func (s *streamTestSync) cleanup() {
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(10 * time.Millisecond):
		s.t.Fatal("timed out waiting for handler to finish")
	}
	s.undo()
}

func (s *streamTestSync) markDone() {
	close(s.done)
}

func newStreamTestSync(t *testing.T) *streamTestSync {
	logChan := make(chan *logrus.Entry, 100)
	cew := util.NewChannelEntryWriter(logChan)
	undo := util.RegisterHookWithUndo(logger, cew)
	ctx, cancel := context.WithCancel(t.Context())
	return &streamTestSync{
		t:      t,
		ctx:    ctx,
		cancel: cancel,
		logs:   logChan,
		undo:   undo,
		done:   make(chan struct{}),
	}
}

func TestStreamEvents_OperationsEvents(t *testing.T) {
	t.Run("operations", func(t *testing.T) {
		testSync := newStreamTestSync(t)
		defer testSync.cleanup()
		stn := mockChain.NewEventFeedWrapper()
		opn := mockChain.NewEventFeedWrapper()
		s := &Server{
			StateNotifier:     &mockChain.SimpleNotifier{Feed: stn},
			OperationNotifier: &mockChain.SimpleNotifier{Feed: opn},
			EventWriteTimeout: testEventWriteTimeout,
		}

		topics, events := operationEventsFixtures(t)
		request := topics.testHttpRequest(testSync.ctx, t)
		w := NewStreamingResponseWriterRecorder(testSync.ctx)

		go func() {
			s.StreamEvents(w, request)
			testSync.markDone()
		}()

		requireAllEventsReceived(t, stn, opn, events, topics, s, w, testSync.logs)
	})
	t.Run("state", func(t *testing.T) {
		testSync := newStreamTestSync(t)
		defer testSync.cleanup()

		stn := mockChain.NewEventFeedWrapper()
		opn := mockChain.NewEventFeedWrapper()
		s := &Server{
			StateNotifier:     &mockChain.SimpleNotifier{Feed: stn},
			OperationNotifier: &mockChain.SimpleNotifier{Feed: opn},
			EventWriteTimeout: testEventWriteTimeout,
		}

		topics, err := newTopicRequest([]string{
			HeadTopic,
			FinalizedCheckpointTopic,
			ChainReorgTopic,
			BlockTopic,
		})
		require.NoError(t, err)
		request := topics.testHttpRequest(testSync.ctx, t)
		w := NewStreamingResponseWriterRecorder(testSync.ctx)

		b, err := blocks.NewSignedBeaconBlock(util.HydrateSignedBeaconBlock(&eth.SignedBeaconBlock{}))
		require.NoError(t, err)
		events := []*feed.Event{
			{
				Type: statefeed.BlockProcessed,
				Data: &statefeed.BlockProcessedData{
					Slot:        0,
					BlockRoot:   [32]byte{},
					SignedBlock: b,
					Verified:    true,
					Optimistic:  false,
				},
			},
			{
				Type: statefeed.NewHead,
				Data: &ethpb.EventHead{
					Slot:                      0,
					Block:                     make([]byte, 32),
					State:                     make([]byte, 32),
					EpochTransition:           true,
					PreviousDutyDependentRoot: make([]byte, 32),
					CurrentDutyDependentRoot:  make([]byte, 32),
					ExecutionOptimistic:       false,
				},
			},
			{
				Type: statefeed.Reorg,
				Data: &ethpb.EventChainReorg{
					Slot:                0,
					Depth:               0,
					OldHeadBlock:        make([]byte, 32),
					NewHeadBlock:        make([]byte, 32),
					OldHeadState:        make([]byte, 32),
					NewHeadState:        make([]byte, 32),
					Epoch:               0,
					ExecutionOptimistic: false,
				},
			},
			{
				Type: statefeed.FinalizedCheckpoint,
				Data: &ethpb.EventFinalizedCheckpoint{
					Block:               make([]byte, 32),
					State:               make([]byte, 32),
					Epoch:               0,
					ExecutionOptimistic: false,
				},
			},
		}

		go func() {
			s.StreamEvents(w, request)
			testSync.markDone()
		}()

		requireAllEventsReceived(t, stn, opn, events, topics, s, w, testSync.logs)
	})
	t.Run("payload attributes", func(t *testing.T) {
		type testCase struct {
			name                      string
			getState                  func() state.BeaconState
			getBlock                  func() interfaces.SignedBeaconBlock
			SetTrackedValidatorsCache func(*cache.TrackedValidatorsCache)
		}
		testCases := []testCase{
			{
				name: "bellatrix",
				getState: func() state.BeaconState {
					st, err := util.NewBeaconStateBellatrix()
					require.NoError(t, err)
					return st
				},
				getBlock: func() interfaces.SignedBeaconBlock {
					b, err := blocks.NewSignedBeaconBlock(util.HydrateSignedBeaconBlockBellatrix(&eth.SignedBeaconBlockBellatrix{}))
					require.NoError(t, err)
					return b
				},
			},
			{
				name: "capella",
				getState: func() state.BeaconState {
					st, err := util.NewBeaconStateCapella()
					require.NoError(t, err)
					return st
				},
				getBlock: func() interfaces.SignedBeaconBlock {
					b, err := blocks.NewSignedBeaconBlock(util.HydrateSignedBeaconBlockCapella(&eth.SignedBeaconBlockCapella{}))
					require.NoError(t, err)
					return b
				},
			},
			{
				name: "deneb",
				getState: func() state.BeaconState {
					st, err := util.NewBeaconStateDeneb()
					require.NoError(t, err)
					return st
				},
				getBlock: func() interfaces.SignedBeaconBlock {
					b, err := blocks.NewSignedBeaconBlock(util.HydrateSignedBeaconBlockDeneb(&eth.SignedBeaconBlockDeneb{}))
					require.NoError(t, err)
					return b
				},
			},
			{
				name: "electra",
				getState: func() state.BeaconState {
					st, err := util.NewBeaconStateElectra()
					require.NoError(t, err)
					return st
				},
				getBlock: func() interfaces.SignedBeaconBlock {
					b, err := blocks.NewSignedBeaconBlock(util.HydrateSignedBeaconBlockElectra(&eth.SignedBeaconBlockElectra{}))
					require.NoError(t, err)
					return b
				},
				SetTrackedValidatorsCache: func(c *cache.TrackedValidatorsCache) {
					c.Set(cache.TrackedValidator{
						Active:       true,
						Index:        0,
						FeeRecipient: primitives.ExecutionAddress(common.HexToAddress("0xd2DBd02e4efe087d7d195de828b9Dd25f19A89C9").Bytes()),
					})
				},
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				testSync := newStreamTestSync(t)
				defer testSync.cleanup()

				st := tc.getState()
				v := &eth.Validator{ExitEpoch: math.MaxUint64, EffectiveBalance: params.BeaconConfig().MinActivationBalance, WithdrawalCredentials: make([]byte, 32)}
				require.NoError(t, st.SetValidators([]*eth.Validator{v}))
				require.NoError(t, st.SetBalances([]uint64{0}))
				currentSlot := primitives.Slot(0)
				// to avoid slot processing
				require.NoError(t, st.SetSlot(currentSlot+1))
				b := tc.getBlock()
				genesis := time.Now()
				require.NoError(t, st.SetGenesisTime(genesis))
				mockChainService := &mockChain.ChainService{
					Root:    make([]byte, 32),
					State:   st,
					Block:   b,
					Slot:    &currentSlot,
					Genesis: genesis,
				}
				headRoot, err := b.Block().HashTreeRoot()
				require.NoError(t, err)

				stn := mockChain.NewEventFeedWrapper()
				opn := mockChain.NewEventFeedWrapper()
				stategen := mock.NewService()
				stategen.AddStateForRoot(st, headRoot)
				s := &Server{
					StateNotifier:          &mockChain.SimpleNotifier{Feed: stn},
					OperationNotifier:      &mockChain.SimpleNotifier{Feed: opn},
					HeadFetcher:            mockChainService,
					ChainInfoFetcher:       mockChainService,
					TrackedValidatorsCache: cache.NewTrackedValidatorsCache(),
					EventWriteTimeout:      testEventWriteTimeout,
					StateGen:               stategen,
				}
				if tc.SetTrackedValidatorsCache != nil {
					tc.SetTrackedValidatorsCache(s.TrackedValidatorsCache)
				}
				topics, err := newTopicRequest([]string{PayloadAttributesTopic})
				require.NoError(t, err)
				request := topics.testHttpRequest(testSync.ctx, t)
				w := NewStreamingResponseWriterRecorder(testSync.ctx)
				events := []*feed.Event{
					{
						Type: statefeed.PayloadAttributes,
						Data: payloadattribute.EventData{
							ProposerIndex:     0,
							ProposalSlot:      mockChainService.CurrentSlot() + 1,
							ParentBlockNumber: 0,
							ParentBlockHash:   make([]byte, 32),
							HeadBlock:         b,
							HeadRoot:          headRoot,
						},
					},
				}

				go func() {
					s.StreamEvents(w, request)
					testSync.markDone()
				}()
				requireAllEventsReceived(t, stn, opn, events, topics, s, w, testSync.logs)
			})
		}
	})
}

func TestFillEventData(t *testing.T) {
	ctx := t.Context()
	t.Run("AlreadyFilledData_ShouldShortCircuitWithoutError", func(t *testing.T) {
		b, err := blocks.NewSignedBeaconBlock(util.HydrateSignedBeaconBlockBellatrix(&eth.SignedBeaconBlockBellatrix{}))
		require.NoError(t, err)
		attributor, err := payloadattribute.New(&enginev1.PayloadAttributes{
			Timestamp: uint64(time.Now().Unix()),
		})
		require.NoError(t, err)
		alreadyFilled := payloadattribute.EventData{
			HeadBlock:       b,
			HeadRoot:        [32]byte{1, 2, 3},
			Attributer:      attributor,
			ParentBlockHash: []byte{4, 5, 6},
		}
		srv := &Server{} // No real HeadFetcher needed here since it won't be called.
		result, err := srv.fillEventData(ctx, alreadyFilled)
		require.NoError(t, err)
		require.DeepEqual(t, alreadyFilled, result)
	})
	t.Run("Electra PartialData_ShouldFetchHeadStateAndBlock", func(t *testing.T) {
		st, err := util.NewBeaconStateElectra()
		require.NoError(t, err)
		valCount := 10
		setActiveValidators(t, st, valCount)
		inactivityScores := make([]uint64, valCount)
		for i := range inactivityScores {
			inactivityScores[i] = 10
		}
		require.NoError(t, st.SetInactivityScores(inactivityScores))
		b, err := blocks.NewSignedBeaconBlock(util.HydrateSignedBeaconBlockElectra(&eth.SignedBeaconBlockElectra{}))
		require.NoError(t, err)
		attributor, err := payloadattribute.New(&enginev1.PayloadAttributes{
			Timestamp: uint64(time.Now().Unix()),
		})
		require.NoError(t, err)
		headRoot, err := b.Block().HashTreeRoot()
		require.NoError(t, err)
		// Create an event data object missing certain fields:
		partial := payloadattribute.EventData{
			ProposalSlot: 42,         // different epoch from current slot
			Attributer:   attributor, // Must be Bellatrix or later
			HeadBlock:    b,
			HeadRoot:     headRoot,
		}
		currentSlot := primitives.Slot(0)
		// to avoid slot processing
		require.NoError(t, st.SetSlot(currentSlot+1))
		mockChainService := &mockChain.ChainService{
			Root:  make([]byte, 32),
			State: st,
			Block: b,
			Slot:  &currentSlot,
		}

		stategen := mock.NewService()
		stategen.AddStateForRoot(st, headRoot)
		stn := mockChain.NewEventFeedWrapper()
		opn := mockChain.NewEventFeedWrapper()
		srv := &Server{
			StateNotifier:          &mockChain.SimpleNotifier{Feed: stn},
			OperationNotifier:      &mockChain.SimpleNotifier{Feed: opn},
			HeadFetcher:            mockChainService,
			ChainInfoFetcher:       mockChainService,
			TrackedValidatorsCache: cache.NewTrackedValidatorsCache(),
			EventWriteTimeout:      testEventWriteTimeout,
			StateGen:               stategen,
		}

		filled, err := srv.fillEventData(ctx, partial)
		require.NoError(t, err, "expected successful fill of partial event data")

		// Verify that fields have been updated from the mock data:
		require.NotNil(t, filled.HeadBlock, "HeadBlock should be assigned")
		require.NotEqual(t, [32]byte{}, filled.HeadRoot, "HeadRoot should no longer be zero")
		require.NotEmpty(t, filled.ParentBlockHash, "ParentBlockHash should be filled")
		require.Equal(t, uint64(0), filled.ParentBlockNumber, "ParentBlockNumber must match mock block")

		// Check that a valid Attributer was set:
		require.NotNil(t, filled.Attributer, "Should have a valid payload attributes object")
		require.Equal(t, false, filled.Attributer.IsEmpty(), "Attributer should not be empty after fill")
	})
}

func setActiveValidators(t *testing.T, st state.BeaconState, count int) {
	balances := make([]uint64, count)
	validators := make([]*eth.Validator, 0, count)
	for i := 0; i < count; i++ {
		pubKey := make([]byte, params.BeaconConfig().BLSPubkeyLength)
		binary.LittleEndian.PutUint64(pubKey, uint64(i))
		balances[i] = uint64(i)
		validators = append(validators, &eth.Validator{
			PublicKey:             pubKey,
			ActivationEpoch:       0,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			WithdrawalCredentials: make([]byte, 32),
		})
	}

	require.NoError(t, st.SetValidators(validators))
	require.NoError(t, st.SetBalances(balances))
}

func TestStuckReaderScenarios(t *testing.T) {
	cases := []struct {
		name       string
		queueDepth func([]*feed.Event) int
	}{
		{
			name: "slow reader - queue overflows",
			queueDepth: func(events []*feed.Event) int {
				return len(events) - 1
			},
		},
		{
			name: "slow reader - all queued, but writer is stuck, write timeout",
			queueDepth: func(events []*feed.Event) int {
				return len(events) + 1
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wedgedWriterTestCase(t, c.queueDepth)
		})
	}
}

func wedgedWriterTestCase(t *testing.T, queueDepth func([]*feed.Event) int) {
	topics, events := operationEventsFixtures(t)
	require.Equal(t, 12, len(events))

	// set eventFeedDepth to a number lower than the events we intend to send to force the server to drop the reader.
	stn := mockChain.NewEventFeedWrapper()
	opn := mockChain.NewEventFeedWrapper()
	s := &Server{
		EventWriteTimeout: 10 * time.Millisecond,
		StateNotifier:     &mockChain.SimpleNotifier{Feed: stn},
		OperationNotifier: &mockChain.SimpleNotifier{Feed: opn},
		EventFeedDepth:    queueDepth(events),
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	eventsWritten := make(chan struct{})
	go func() {
		for i := range events {
			ev := events[i]
			top := topicForEvent(ev)
			if topicsForOpsFeed[top] {
				err := opn.WaitForSubscription(ctx)
				require.NoError(t, err)
				s.OperationNotifier.OperationFeed().Send(ev)
			} else {
				err := stn.WaitForSubscription(ctx)
				require.NoError(t, err)
				s.StateNotifier.StateFeed().Send(ev)
			}
		}
		close(eventsWritten)
	}()

	request := topics.testHttpRequest(ctx, t)
	w := NewStreamingResponseWriterRecorder(ctx)

	handlerFinished := make(chan struct{})
	go func() {
		s.StreamEvents(w, request)
		close(handlerFinished)
	}()

	// Make sure that the stream writer shut down when the reader failed to clear the write buffer.
	select {
	case <-handlerFinished:
		// We expect the stream handler to max out the queue buffer and exit gracefully.
		return
	case <-ctx.Done():
		t.Fatalf("context canceled / timed out waiting for handler completion, err=%v", ctx.Err())
	}

	// Also make sure all the events were written.
	select {
	case <-eventsWritten:
		// We expect the stream handler to max out the queue buffer and exit gracefully.
		return
	case <-ctx.Done():
		t.Fatalf("context canceled / timed out waiting to write all events, err=%v", ctx.Err())
	}
}
