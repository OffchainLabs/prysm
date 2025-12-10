package sync

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	testingdb "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/execproof"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestValidateExecutionProof(t *testing.T) {
	beaconDB := testingdb.SetupDB(t)
	p2pService := mockp2p.NewTestP2P(t)
	ctx := context.Background()

	fcp := &ethpb.Checkpoint{
		Epoch: 1,
	}

	require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{
		Root: params.BeaconConfig().ZeroHash[:],
		Slot: 0,
	}))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, fcp))

	defaultTopic := p2p.ExecutionProofSubnetTopicFormat + "/" + encoder.ProtocolSuffixSSZSnappy
	fakeDigest := []byte{0xAB, 0x00, 0xCC, 0x9E}

	chainService := &mock.ChainService{
		Genesis:             time.Now(),
		ValidatorsRoot:      [32]byte{'A'},
		FinalizedCheckPoint: fcp,
	}

	currentSlot := primitives.Slot(100)
	genesisTime := time.Now().Add(-time.Duration(uint64(currentSlot)*params.BeaconConfig().SecondsPerSlot) * time.Second)

	tests := []struct {
		name         string
		setupService func() *Service
		proof        *ethpb.ExecutionProof
		topic        *string
		pid          peer.ID
		want         pubsub.ValidationResult
		wantErr      bool
	}{
		{
			name: "Ignore when syncing",
			setupService: func() *Service {
				s := &Service{
					cfg: &config{
						p2p:         p2pService,
						initialSync: &mockSync.Sync{IsSyncing: true},
						chain:       chainService,
						clock:       startup.NewClock(genesisTime, [32]byte{'A'}),
					},
				}
				s.initCaches()
				return s
			},
			proof: &ethpb.ExecutionProof{
				Slot:      currentSlot,
				ProofId:   primitives.ExecutionProofId(1),
				BlockRoot: make([]byte, 32),
				BlockHash: make([]byte, 32),
				ProofData: make([]byte, 100),
			},
			topic: func() *string {
				t := fmt.Sprintf(defaultTopic, fakeDigest)
				return &t
			}(),
			pid:     "random-peer",
			want:    pubsub.ValidationIgnore,
			wantErr: false,
		},
		{
			name: "Reject nil topic",
			setupService: func() *Service {
				s := &Service{
					cfg: &config{
						p2p:         p2pService,
						initialSync: &mockSync.Sync{IsSyncing: false},
						chain:       chainService,
						clock:       startup.NewClock(genesisTime, [32]byte{'A'}),
					},
				}
				s.initCaches()
				return s
			},
			proof: &ethpb.ExecutionProof{
				Slot:      currentSlot,
				ProofId:   primitives.ExecutionProofId(1),
				BlockRoot: make([]byte, 32),
				BlockHash: make([]byte, 32),
				ProofData: make([]byte, 100),
			},
			topic:   nil,
			pid:     "random-peer",
			want:    pubsub.ValidationReject,
			wantErr: true,
		},
		{
			name: "Reject proof from future slot",
			setupService: func() *Service {
				s := &Service{
					cfg: &config{
						p2p:           p2pService,
						initialSync:   &mockSync.Sync{IsSyncing: false},
						chain:         chainService,
						clock:         startup.NewClock(genesisTime, [32]byte{'A'}),
						beaconDB:      beaconDB,
						stateGen:      stategen.New(beaconDB, doublylinkedtree.New()),
						execProofPool: execproof.NewPool(),
					},
				}
				s.initCaches()
				return s
			},
			proof: &ethpb.ExecutionProof{
				Slot:      currentSlot + 1000, // Far future slot
				ProofId:   primitives.ExecutionProofId(1),
				BlockRoot: make([]byte, 32),
				BlockHash: make([]byte, 32),
				ProofData: make([]byte, 100),
			},
			topic: func() *string {
				t := fmt.Sprintf(defaultTopic, fakeDigest)
				return &t
			}(),
			pid:     "random-peer",
			want:    pubsub.ValidationReject,
			wantErr: true,
		},
		{
			name: "Reject proof below finalized slot",
			setupService: func() *Service {
				s := &Service{
					cfg: &config{
						p2p:           p2pService,
						initialSync:   &mockSync.Sync{IsSyncing: false},
						chain:         chainService,
						clock:         startup.NewClock(genesisTime, [32]byte{'A'}),
						beaconDB:      beaconDB,
						stateGen:      stategen.New(beaconDB, doublylinkedtree.New()),
						execProofPool: execproof.NewPool(),
					},
				}
				s.initCaches()
				return s
			},
			proof: &ethpb.ExecutionProof{
				Slot:      primitives.Slot(5), // Before finalized epoch 1
				ProofId:   1,
				BlockRoot: make([]byte, 32),
				BlockHash: make([]byte, 32),
				ProofData: make([]byte, 100),
			},
			topic: func() *string {
				t := fmt.Sprintf(defaultTopic, fakeDigest)
				return &t
			}(),
			pid:     "random-peer",
			want:    pubsub.ValidationReject,
			wantErr: true,
		},
		{
			name: "Ignore already seen proof",
			setupService: func() *Service {
				s := &Service{
					cfg: &config{
						p2p:           p2pService,
						initialSync:   &mockSync.Sync{IsSyncing: false},
						chain:         chainService,
						clock:         startup.NewClock(genesisTime, [32]byte{'A'}),
						beaconDB:      beaconDB,
						stateGen:      stategen.New(beaconDB, doublylinkedtree.New()),
						execProofPool: execproof.NewPool(),
					},
				}
				s.initCaches()
				// Mark proof as seen
				s.setSeenExecutionProofIndex(1, currentSlot)
				return s
			},
			proof: &ethpb.ExecutionProof{
				Slot:      currentSlot,
				ProofId:   primitives.ExecutionProofId(1),
				BlockRoot: make([]byte, 32),
				BlockHash: make([]byte, 32),
				ProofData: make([]byte, 100),
			},
			topic: func() *string {
				t := fmt.Sprintf(defaultTopic, fakeDigest)
				return &t
			}(),
			pid:     "random-peer",
			want:    pubsub.ValidationIgnore,
			wantErr: false,
		},
		{
			name: "Ignore proof already in pool",
			setupService: func() *Service {
				pool := execproof.NewPool()
				pool.InsertExecutionProof(&ethpb.ExecutionProof{
					Slot:      currentSlot,
					ProofId:   primitives.ExecutionProofId(1),
					BlockRoot: make([]byte, 32),
					BlockHash: make([]byte, 32),
					ProofData: make([]byte, 100),
				})

				s := &Service{
					cfg: &config{
						p2p:           p2pService,
						initialSync:   &mockSync.Sync{IsSyncing: false},
						chain:         chainService,
						clock:         startup.NewClock(genesisTime, [32]byte{'A'}),
						beaconDB:      beaconDB,
						stateGen:      stategen.New(beaconDB, doublylinkedtree.New()),
						execProofPool: pool,
					},
				}
				s.initCaches()
				return s
			},
			proof: &ethpb.ExecutionProof{
				Slot:      currentSlot,
				ProofId:   primitives.ExecutionProofId(1),
				BlockRoot: make([]byte, 32),
				BlockHash: make([]byte, 32),
				ProofData: make([]byte, 100),
			},
			topic: func() *string {
				t := fmt.Sprintf(defaultTopic, fakeDigest)
				return &t
			}(),
			pid:     "random-peer",
			want:    pubsub.ValidationIgnore,
			wantErr: false,
		},
		{
			name: "Accept valid proof",
			setupService: func() *Service {
				s := &Service{
					cfg: &config{
						p2p:           p2pService,
						initialSync:   &mockSync.Sync{IsSyncing: false},
						chain:         chainService,
						clock:         startup.NewClock(genesisTime, [32]byte{'A'}),
						beaconDB:      beaconDB,
						stateGen:      stategen.New(beaconDB, doublylinkedtree.New()),
						execProofPool: execproof.NewPool(),
					},
				}
				s.initCaches()
				return s
			},
			proof: &ethpb.ExecutionProof{
				Slot:      currentSlot,
				ProofId:   primitives.ExecutionProofId(1),
				BlockRoot: make([]byte, 32),
				BlockHash: make([]byte, 32),
				ProofData: make([]byte, 100),
			},
			topic: func() *string {
				t := fmt.Sprintf(defaultTopic, fakeDigest)
				return &t
			}(),
			pid:     "random-peer",
			want:    pubsub.ValidationAccept,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.setupService()

			// Create pubsub message
			buf := new(bytes.Buffer)
			_, err := p2pService.Encoding().EncodeGossip(buf, tt.proof)
			require.NoError(t, err)

			msg := &pubsub.Message{
				Message: &pubsubpb.Message{
					Data:  buf.Bytes(),
					Topic: tt.topic,
				},
			}

			// Validate
			result, err := s.validateExecutionProof(ctx, tt.pid, msg)

			if tt.wantErr {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.want, result)

			// If validation accepted, check that ValidatorData is set
			if result == pubsub.ValidationAccept {
				assert.NotNil(t, msg.ValidatorData)
				validatedProof, ok := msg.ValidatorData.(*ethpb.ExecutionProof)
				assert.Equal(t, true, ok)

				// Check that the validated proof matches the original
				assert.Equal(t, tt.proof.ProofId, validatedProof.ProofId)
				assert.Equal(t, tt.proof.Slot, validatedProof.Slot)
				assert.DeepEqual(t, tt.proof.BlockRoot, validatedProof.BlockRoot)
				assert.DeepEqual(t, tt.proof.BlockHash, validatedProof.BlockHash)
				assert.DeepEqual(t, tt.proof.ProofData, validatedProof.ProofData)
			}
		})
	}
}
