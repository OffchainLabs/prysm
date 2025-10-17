package sync

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	dbtest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v6/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/prysmaticlabs/go-bitfield"
)

func TestService_validateCommitteeIndexBeaconAttestation(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	db := dbtest.SetupDB(t)
	chain := &mockChain.ChainService{
		// 1 slot ago.
		Genesis:          time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second),
		ValidatorsRoot:   [32]byte{'A'},
		ValidAttestation: true,
		DB:               db,
		Optimistic:       true,
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := &Service{
		ctx: ctx,
		cfg: &config{
			initialSync:         &mockSync.Sync{IsSyncing: false},
			p2p:                 p,
			beaconDB:            db,
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attestationNotifier: (&mockChain.ChainService{}).OperationNotifier(),
		},
		blkRootToPendingAtts:             make(map[[32]byte][]any),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	s.initCaches()
	go s.verifierRoutine()

	invalidRoot := [32]byte{'A', 'B', 'C', 'D'}
	s.setBadBlock(ctx, invalidRoot)

	digest, err := s.currentForkDigest()
	require.NoError(t, err)

	blk := util.NewBeaconBlock()
	blk.Block.Slot = 1
	util.SaveBlock(t, ctx, db, blk)

	validBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	chain.FinalizedCheckPoint = &ethpb.Checkpoint{
		Root:  validBlockRoot[:],
		Epoch: 0,
	}

	validators := uint64(64)
	savedState, keys := util.DeterministicGenesisState(t, validators)
	require.NoError(t, savedState.SetSlot(1))
	require.NoError(t, db.SaveState(t.Context(), savedState, validBlockRoot))
	chain.State = savedState

	tests := []struct {
		name                      string
		msg                       ethpb.Att
		topic                     string
		validAttestationSignature bool
		want                      bool
	}{
		{
			name: "valid attestation signature",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target: &ethpb.Checkpoint{
						Epoch: 0,
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      true,
		},
		{
			name: "valid attestation signature with nil topic",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target: &ethpb.Checkpoint{
						Epoch: 0,
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     "",
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "bad target epoch",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target: &ethpb.Checkpoint{
						Epoch: 10,
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "already seen",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "invalid beacon block",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: invalidRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "committee index exceeds committee length",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  4,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_2", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "wrong committee index",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  2,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_2", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "already aggregated",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b1011},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  1,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "missing block",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: bytesutil.PadTo([]byte("missing"), fieldparams.RootLength),
					CommitteeIndex:  1,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "invalid attestation",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  1,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest),
			validAttestationSignature: false,
			want:                      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helpers.ClearCache()
			chain.ValidAttestation = tt.validAttestationSignature
			if tt.validAttestationSignature {
				com, err := helpers.BeaconCommitteeFromState(t.Context(), savedState, tt.msg.GetData().Slot, tt.msg.GetData().CommitteeIndex)
				require.NoError(t, err)
				domain, err := signing.Domain(savedState.Fork(), tt.msg.GetData().Target.Epoch, params.BeaconConfig().DomainBeaconAttester, savedState.GenesisValidatorsRoot())
				require.NoError(t, err)
				attRoot, err := signing.ComputeSigningRoot(tt.msg.GetData(), domain)
				require.NoError(t, err)
				for i := 0; ; i++ {
					if tt.msg.GetAggregationBits().BitAt(uint64(i)) {
						tt.msg.SetSignature(keys[com[i]].Sign(attRoot[:]).Marshal())
						break
					}
				}
			} else {
				tt.msg.SetSignature(make([]byte, 96))
			}
			buf := new(bytes.Buffer)
			_, err := p.Encoding().EncodeGossip(buf, tt.msg)
			require.NoError(t, err)
			m := &pubsub.Message{
				Message: &pubsubpb.Message{
					Data:  buf.Bytes(),
					Topic: &tt.topic,
				},
			}
			if tt.topic == "" {
				m.Message.Topic = nil
			}

			res, err := s.validateCommitteeIndexBeaconAttestation(ctx, "", m)
			received := res == pubsub.ValidationAccept
			if received != tt.want {
				t.Fatalf("Did not received wanted validation. Got %v, wanted %v", !tt.want, tt.want)
			}
			if tt.want && err != nil {
				t.Errorf("Non nil error returned: %v", err)
			}
			if tt.want && m.ValidatorData == nil {
				t.Error("Expected validator data to be set")
			}
		})
	}
}

func TestService_validateCommitteeIndexBeaconAttestationElectra(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()

	p := p2ptest.NewTestP2P(t)
	db := dbtest.SetupDB(t)
	currentSlot := 1 + (primitives.Slot(params.BeaconConfig().ElectraForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
	genesisOffset := time.Duration(currentSlot) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	chain := &mockChain.ChainService{
		Genesis:          time.Now().Add(-1 * genesisOffset),
		ValidatorsRoot:   params.BeaconConfig().GenesisValidatorsRoot,
		ValidAttestation: true,
		DB:               db,
		Optimistic:       true,
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := &Service{
		ctx: ctx,
		cfg: &config{
			initialSync:         &mockSync.Sync{IsSyncing: false},
			p2p:                 p,
			beaconDB:            db,
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attestationNotifier: (&mockChain.ChainService{}).OperationNotifier(),
		},
		blkRootToPendingAtts:             make(map[[32]byte][]any),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	require.Equal(t, currentSlot, s.cfg.clock.CurrentSlot())
	s.initCaches()
	go s.verifierRoutine()

	digest, err := s.currentForkDigest()
	require.NoError(t, err)

	blk := util.NewBeaconBlock()
	blk.Block.Slot = s.cfg.clock.CurrentSlot()
	util.SaveBlock(t, ctx, db, blk)

	validBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	chain.FinalizedCheckPoint = &ethpb.Checkpoint{
		Root:  validBlockRoot[:],
		Epoch: 0,
	}

	validators := uint64(64)
	savedState, keys := util.DeterministicGenesisState(t, validators)
	require.NoError(t, savedState.SetSlot(s.cfg.clock.CurrentSlot()))
	require.NoError(t, db.SaveState(t.Context(), savedState, validBlockRoot))
	chain.State = savedState
	committee, err := helpers.BeaconCommitteeFromState(ctx, savedState, s.cfg.clock.CurrentSlot(), 0)
	require.NoError(t, err)

	tests := []struct {
		name string
		msg  ethpb.Att
		want bool
	}{
		{
			name: "valid",
			msg: &ethpb.SingleAttestation{
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            s.cfg.clock.CurrentSlot(),
					Target: &ethpb.Checkpoint{
						Epoch: s.cfg.clock.CurrentEpoch(),
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
				AttesterIndex: committee[0],
			},
			want: true,
		},
		{
			name: "non-zero committee index in att data",
			msg: &ethpb.SingleAttestation{
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  1,
					Slot:            s.cfg.clock.CurrentSlot(),
					Target: &ethpb.Checkpoint{
						Epoch: s.cfg.clock.CurrentEpoch(),
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
				AttesterIndex: committee[0],
			},
			want: false,
		},
		{
			name: "attesting index not in committee",
			msg: &ethpb.SingleAttestation{
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  1,
					Slot:            s.cfg.clock.CurrentSlot(),
					Target: &ethpb.Checkpoint{
						Epoch: s.cfg.clock.CurrentEpoch(),
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
				AttesterIndex: 999999,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helpers.ClearCache()
			com, err := helpers.BeaconCommitteeFromState(t.Context(), savedState, tt.msg.GetData().Slot, tt.msg.GetData().CommitteeIndex)
			require.NoError(t, err)
			domain, err := signing.Domain(savedState.Fork(), tt.msg.GetData().Target.Epoch, params.BeaconConfig().DomainBeaconAttester, savedState.GenesisValidatorsRoot())
			require.NoError(t, err)
			attRoot, err := signing.ComputeSigningRoot(tt.msg.GetData(), domain)
			require.NoError(t, err)
			tt.msg.SetSignature(keys[com[0]].Sign(attRoot[:]).Marshal())
			buf := new(bytes.Buffer)
			_, err = p.Encoding().EncodeGossip(buf, tt.msg)
			require.NoError(t, err)
			topic := fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest)
			m := &pubsub.Message{
				Message: &pubsubpb.Message{
					Data:  buf.Bytes(),
					Topic: &topic,
				},
			}

			res, err := s.validateCommitteeIndexBeaconAttestation(ctx, "", m)
			received := res == pubsub.ValidationAccept
			if received != tt.want {
				t.Fatalf("Did not received wanted validation. Got %v, wanted %v", !tt.want, tt.want)
			}
			if tt.want && err != nil {
				t.Errorf("Non nil error returned: %v", err)
			}
			if tt.want && m.ValidatorData == nil {
				t.Error("Expected validator data to be set")
			}
		})
	}
}

func TestService_setSeenUnaggregatedAtt(t *testing.T) {
	s := NewService(t.Context(), WithP2P(p2ptest.NewTestP2P(t)))

	// Helper function to generate key and handle errors in tests
	generateKey := func(t *testing.T, att ethpb.Att) string {
		key, err := generateUnaggregatedAttCacheKey(att)
		require.NoError(t, err)
		return key
	}

	t.Run("phase0", func(t *testing.T) {
		s.initCaches()

		s0c0a0 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 0, CommitteeIndex: 0},
			AggregationBits: bitfield.Bitlist{0b1001},
		}
		s0c0a1 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 0, CommitteeIndex: 0},
			AggregationBits: bitfield.Bitlist{0b1010},
		}
		s0c0a2 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 0, CommitteeIndex: 0},
			AggregationBits: bitfield.Bitlist{0b1100},
		}
		s0c1a0 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 0, CommitteeIndex: 1},
			AggregationBits: bitfield.Bitlist{0b1001},
		}
		s0c2a0 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 0, CommitteeIndex: 2},
			AggregationBits: bitfield.Bitlist{0b1001},
		}
		s1c0a0 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 1, CommitteeIndex: 0},
			AggregationBits: bitfield.Bitlist{0b1001},
		}
		s2c0a0 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 2, CommitteeIndex: 0},
			AggregationBits: bitfield.Bitlist{0b1001},
		}

		t.Run("empty cache", func(t *testing.T) {
			key := generateKey(t, s0c0a0)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key))
		})
		t.Run("ok", func(t *testing.T) {
			key := generateKey(t, s0c0a0)
			s.setSeenUnaggregatedAtt(key)
			assert.Equal(t, true, s.hasSeenUnaggregatedAtt(key))
		})
		t.Run("different slot", func(t *testing.T) {
			key1 := generateKey(t, s1c0a0)
			key2 := generateKey(t, s2c0a0)
			s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
		})
		t.Run("different committee index", func(t *testing.T) {
			key1 := generateKey(t, s0c1a0)
			key2 := generateKey(t, s0c2a0)
			s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
		})
		t.Run("different bit", func(t *testing.T) {
			key1 := generateKey(t, s0c0a1)
			key2 := generateKey(t, s0c0a2)
			s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
		})
		t.Run("0 bits set is considered not seen", func(t *testing.T) {
			a := &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b1000}}
			_, err := generateUnaggregatedAttCacheKey(a)
			require.Equal(t, err != nil, true, "Should error because no bits set is invalid")
		})
		t.Run("multiple bits set is considered not seen", func(t *testing.T) {
			a := &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b1111}}
			_, err := generateUnaggregatedAttCacheKey(a)
			require.Equal(t, err != nil, true, "Should error because no bits set is invalid")
		})
	})
	t.Run("electra", func(t *testing.T) {
		s.initCaches()

		s0c0a0 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 0},
			CommitteeId:   0,
			AttesterIndex: 0,
		}
		s0c0a1 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 0},
			CommitteeId:   0,
			AttesterIndex: 1,
		}
		s0c0a2 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 0},
			CommitteeId:   0,
			AttesterIndex: 2,
		}
		s0c1a0 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 0},
			CommitteeId:   1,
			AttesterIndex: 0,
		}
		s0c2a0 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 0},
			CommitteeId:   2,
			AttesterIndex: 0,
		}
		s1c0a0 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 1},
			CommitteeId:   0,
			AttesterIndex: 0,
		}
		s2c0a0 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 2},
			CommitteeId:   0,
			AttesterIndex: 0,
		}

		t.Run("empty cache", func(t *testing.T) {
			key := generateKey(t, s0c0a0)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key))
		})
		t.Run("ok", func(t *testing.T) {
			key := generateKey(t, s0c0a0)
			s.setSeenUnaggregatedAtt(key)
			assert.Equal(t, true, s.hasSeenUnaggregatedAtt(key))
		})
		t.Run("different slot", func(t *testing.T) {
			key1 := generateKey(t, s1c0a0)
			key2 := generateKey(t, s2c0a0)
			s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
		})
		t.Run("different committee index", func(t *testing.T) {
			key1 := generateKey(t, s0c1a0)
			key2 := generateKey(t, s0c2a0)
			s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
		})
		t.Run("different attester", func(t *testing.T) {
			key1 := generateKey(t, s0c0a1)
			key2 := generateKey(t, s0c0a2)
			s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
		})
		t.Run("single attestation is considered not seen", func(t *testing.T) {
			a := &ethpb.AttestationElectra{}
			_, err := generateUnaggregatedAttCacheKey(a)
			require.Equal(t, err != nil, true, "Should error because no bits set is invalid")
		})
	})
}

func Test_validateCommitteeIndexAndCount_Boundary(t *testing.T) {
	ctx := t.Context()

	// Create a minimal state with a known number of validators.
	validators := uint64(64)
	bs, _ := util.DeterministicGenesisState(t, validators)
	require.NoError(t, bs.SetSlot(1))

	s := &Service{}

	// Build a minimal Phase0 attestation (unaggregated path).
	att := &ethpb.Attestation{
		Data: &ethpb.AttestationData{
			Slot:           1,
			CommitteeIndex: 0,
		},
	}

	// First call to obtain the active validator count used to derive committees per slot.
	_, valCount, res, err := s.validateCommitteeIndexAndCount(ctx, att, bs)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)

	count := helpers.SlotCommitteeCount(valCount)

	// committee_index == count - 1 should be accepted.
	att.Data.CommitteeIndex = primitives.CommitteeIndex(count - 1)
	_, _, res, err = s.validateCommitteeIndexAndCount(ctx, att, bs)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)

	// committee_index == count should be rejected (out of range).
	att.Data.CommitteeIndex = primitives.CommitteeIndex(count)
	_, _, res, err = s.validateCommitteeIndexAndCount(ctx, att, bs)
	require.ErrorContains(t, "committee index", err)
	require.Equal(t, pubsub.ValidationReject, res)
}
