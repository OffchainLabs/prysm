package kv

import (
	"context"
	"sort"
	"testing"

	c "github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestKV_Aggregated_AggregateUnaggregatedAttestations(t *testing.T) {
	cache := NewAttCaches()
	priv, err := bls.RandKey()
	require.NoError(t, err)
	sig1 := priv.Sign([]byte{'a'})
	sig2 := priv.Sign([]byte{'b'})
	att1 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1001}, Signature: sig1.Marshal()})
	att2 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1010}, Signature: sig1.Marshal()})
	att3 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1100}, Signature: sig1.Marshal()})
	att4 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1001}, Signature: sig2.Marshal()})
	att5 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b1001}, Signature: sig1.Marshal()})
	att6 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b1010}, Signature: sig1.Marshal()})
	att7 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b1100}, Signature: sig1.Marshal()})
	att8 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b1001}, Signature: sig2.Marshal()})
	atts := []ethpb.Att{att1, att2, att3, att4, att5, att6, att7, att8}
	require.NoError(t, cache.SaveUnaggregatedAttestations(atts))
	require.NoError(t, cache.AggregateUnaggregatedAttestations(context.Background()))

	require.Equal(t, 1, len(cache.AggregatedAttestationsBySlotIndex(context.Background(), 1, 0)), "Did not aggregate correctly")
	require.Equal(t, 1, len(cache.AggregatedAttestationsBySlotIndex(context.Background(), 2, 0)), "Did not aggregate correctly")
}

func TestKV_Aggregated_SaveAggregatedAttestation(t *testing.T) {
	tests := []struct {
		name          string
		att           ethpb.Att
		count         int
		wantErrString string
	}{
		{
			name:          "nil attestation",
			att:           nil,
			wantErrString: "attestation is nil",
		},
		{
			name:          "nil attestation data",
			att:           &ethpb.Attestation{},
			wantErrString: "attestation is nil",
		},
		{
			name: "not aggregated",
			att: util.HydrateAttestation(&ethpb.Attestation{
				Data: &ethpb.AttestationData{}, AggregationBits: bitfield.Bitlist{0b10100}}),
			wantErrString: "attestation is not aggregated",
		},
		{
			name: "invalid hash",
			att: &ethpb.Attestation{
				Data: util.HydrateAttestationData(&ethpb.AttestationData{
					BeaconBlockRoot: []byte{0b0},
				}),
				AggregationBits: bitfield.Bitlist{0b10111},
			},
			wantErrString: "could not create attestation ID",
		},
		{
			name: "already seen",
			att: util.HydrateAttestation(&ethpb.Attestation{
				Data: &ethpb.AttestationData{
					Slot: 100,
				},
				AggregationBits: bitfield.Bitlist{0b11101001},
			}),
			count: 0,
		},
		{
			name: "normal save",
			att: util.HydrateAttestation(&ethpb.Attestation{
				Data: &ethpb.AttestationData{
					Slot: 1,
				},
				AggregationBits: bitfield.Bitlist{0b1101},
			}),
			count: 1,
		},
	}
	id, err := attestation.NewId(util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 100}}), attestation.Data)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewAttCaches()
			cache.seenAtt.Set(id.String(), []bitfield.Bitlist{{0xff}}, c.DefaultExpiration)
			assert.Equal(t, 0, len(cache.unAggregatedAtt), "Invalid start pool, atts: %d", len(cache.unAggregatedAtt))

			err := cache.SaveAggregatedAttestation(tt.att)
			if tt.wantErrString != "" {
				assert.ErrorContains(t, tt.wantErrString, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.count, len(cache.aggregatedAtt), "Wrong attestation count")
			assert.Equal(t, tt.count, cache.AggregatedAttestationCount(), "Wrong attestation count")
		})
	}
}

func TestKV_Aggregated_SaveAggregatedAttestations(t *testing.T) {
	tests := []struct {
		name          string
		atts          []ethpb.Att
		count         int
		wantErrString string
	}{
		{
			name: "no duplicates",
			atts: []ethpb.Att{
				util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1},
					AggregationBits: bitfield.Bitlist{0b1101}}),
				util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1},
					AggregationBits: bitfield.Bitlist{0b1101}}),
			},
			count: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewAttCaches()
			assert.Equal(t, 0, len(cache.aggregatedAtt), "Invalid start pool, atts: %d", len(cache.unAggregatedAtt))
			err := cache.SaveAggregatedAttestations(tt.atts)
			if tt.wantErrString != "" {
				assert.ErrorContains(t, tt.wantErrString, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.count, len(cache.aggregatedAtt), "Wrong attestation count")
			assert.Equal(t, tt.count, cache.AggregatedAttestationCount(), "Wrong attestation count")
		})
	}
}

func TestKV_Aggregated_SaveAggregatedAttestations_SomeGoodSomeBad(t *testing.T) {
	tests := []struct {
		name          string
		atts          []ethpb.Att
		count         int
		wantErrString string
	}{
		{
			name: "the first attestation is bad",
			atts: []ethpb.Att{
				util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1},
					AggregationBits: bitfield.Bitlist{0b1100}}),
				util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1},
					AggregationBits: bitfield.Bitlist{0b1101}}),
			},
			count: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewAttCaches()
			assert.Equal(t, 0, len(cache.aggregatedAtt), "Invalid start pool, atts: %d", len(cache.unAggregatedAtt))
			err := cache.SaveAggregatedAttestations(tt.atts)
			if tt.wantErrString != "" {
				assert.ErrorContains(t, tt.wantErrString, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.count, len(cache.aggregatedAtt), "Wrong attestation count")
			assert.Equal(t, tt.count, cache.AggregatedAttestationCount(), "Wrong attestation count")
		})
	}
}

func TestKV_Aggregated_AggregatedAttestations(t *testing.T) {
	cache := NewAttCaches()

	att1 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1101}})
	att2 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b1101}})
	att3 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 3}, AggregationBits: bitfield.Bitlist{0b1101}})
	atts := []ethpb.Att{att1, att2, att3}

	for _, att := range atts {
		require.NoError(t, cache.SaveAggregatedAttestation(att))
	}

	returned := cache.AggregatedAttestations()
	sort.Slice(returned, func(i, j int) bool {
		return returned[i].GetData().Slot < returned[j].GetData().Slot
	})
	assert.DeepSSZEqual(t, atts, returned)
}

func TestKV_Aggregated_DeleteAggregatedAttestation(t *testing.T) {
	t.Run("nil attestation", func(t *testing.T) {
		cache := NewAttCaches()
		assert.ErrorContains(t, "attestation is nil", cache.DeleteAggregatedAttestation(nil))
		att := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10101}, Data: &ethpb.AttestationData{Slot: 2}})
		assert.NoError(t, cache.DeleteAggregatedAttestation(att))
	})

	t.Run("non aggregated attestation", func(t *testing.T) {
		cache := NewAttCaches()
		att := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b1001}, Data: &ethpb.AttestationData{Slot: 2}})
		err := cache.DeleteAggregatedAttestation(att)
		assert.ErrorContains(t, "attestation is not aggregated", err)
	})

	t.Run("invalid hash", func(t *testing.T) {
		cache := NewAttCaches()
		att := &ethpb.Attestation{
			AggregationBits: bitfield.Bitlist{0b1111},
			Data: &ethpb.AttestationData{
				Slot:   2,
				Source: &ethpb.Checkpoint{},
				Target: &ethpb.Checkpoint{},
			},
		}
		err := cache.DeleteAggregatedAttestation(att)
		wantErr := "could not create attestation ID"
		assert.ErrorContains(t, wantErr, err)
	})

	t.Run("nonexistent attestation", func(t *testing.T) {
		cache := NewAttCaches()
		att := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b1111}, Data: &ethpb.AttestationData{Slot: 2}})
		assert.NoError(t, cache.DeleteAggregatedAttestation(att))
	})

	t.Run("non-filtered deletion", func(t *testing.T) {
		cache := NewAttCaches()
		att1 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b11010}})
		att2 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b11010}})
		att3 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 3}, AggregationBits: bitfield.Bitlist{0b11010}})
		att4 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 3}, AggregationBits: bitfield.Bitlist{0b10101}})
		atts := []ethpb.Att{att1, att2, att3, att4}
		require.NoError(t, cache.SaveAggregatedAttestations(atts))
		require.NoError(t, cache.DeleteAggregatedAttestation(att1))
		require.NoError(t, cache.DeleteAggregatedAttestation(att3))

		returned := cache.AggregatedAttestations()
		wanted := []ethpb.Att{att2}
		assert.DeepEqual(t, wanted, returned)
	})

	t.Run("filtered deletion", func(t *testing.T) {
		cache := NewAttCaches()
		att1 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b110101}})
		att2 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b110111}})
		att3 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b110100}})
		att4 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b110101}})
		atts := []ethpb.Att{att1, att2, att3, att4}
		require.NoError(t, cache.SaveAggregatedAttestations(atts))

		assert.Equal(t, 2, cache.AggregatedAttestationCount(), "Unexpected number of atts")
		require.NoError(t, cache.DeleteAggregatedAttestation(att4))

		returned := cache.AggregatedAttestations()
		wanted := []ethpb.Att{att1, att2}
		sort.Slice(returned, func(i, j int) bool {
			return string(returned[i].GetAggregationBits()) < string(returned[j].GetAggregationBits())
		})
		assert.DeepEqual(t, wanted, returned)
	})
}

func TestKV_Aggregated_HasAggregatedAttestation(t *testing.T) {
	tests := []struct {
		name     string
		existing []ethpb.Att
		input    *ethpb.Attestation
		want     bool
		err      error
	}{
		{
			name:  "nil attestation",
			input: nil,
			want:  false,
			err:   errors.New("is nil"),
		},
		{
			name: "nil attestation data",
			input: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b1111},
			},
			want: false,
			err:  errors.New("is nil"),
		},
		{
			name: "empty cache aggregated",
			input: util.HydrateAttestation(&ethpb.Attestation{
				Data: &ethpb.AttestationData{
					Slot: 1,
				},
				AggregationBits: bitfield.Bitlist{0b1111}}),
			want: false,
		},
		{
			name: "empty cache unaggregated",
			input: util.HydrateAttestation(&ethpb.Attestation{
				Data: &ethpb.AttestationData{
					Slot: 1,
				},
				AggregationBits: bitfield.Bitlist{0b1001}}),
			want: false,
		},
		{
			name: "single attestation in cache with exact match",
			existing: []ethpb.Att{&ethpb.Attestation{
				Data: util.HydrateAttestationData(&ethpb.AttestationData{
					Slot: 1,
				}),
				AggregationBits: bitfield.Bitlist{0b1111}},
			},
			input: &ethpb.Attestation{
				Data: util.HydrateAttestationData(&ethpb.AttestationData{
					Slot: 1,
				}),
				AggregationBits: bitfield.Bitlist{0b1111}},
			want: true,
		},
		{
			name: "single attestation in cache with subset aggregation",
			existing: []ethpb.Att{&ethpb.Attestation{
				Data: util.HydrateAttestationData(&ethpb.AttestationData{
					Slot: 1,
				}),
				AggregationBits: bitfield.Bitlist{0b1111}},
			},
			input: &ethpb.Attestation{
				Data: util.HydrateAttestationData(&ethpb.AttestationData{
					Slot: 1,
				}),
				AggregationBits: bitfield.Bitlist{0b1110}},
			want: true,
		},
		{
			name: "single attestation in cache with superset aggregation",
			existing: []ethpb.Att{&ethpb.Attestation{
				Data: util.HydrateAttestationData(&ethpb.AttestationData{
					Slot: 1,
				}),
				AggregationBits: bitfield.Bitlist{0b1110}},
			},
			input: &ethpb.Attestation{
				Data: util.HydrateAttestationData(&ethpb.AttestationData{
					Slot: 1,
				}),
				AggregationBits: bitfield.Bitlist{0b1111}},
			want: false,
		},
		{
			name: "multiple attestations with same data in cache with overlapping aggregation, input is subset",
			existing: []ethpb.Att{
				&ethpb.Attestation{
					Data: util.HydrateAttestationData(&ethpb.AttestationData{
						Slot: 1,
					}),
					AggregationBits: bitfield.Bitlist{0b1111000},
				},
				&ethpb.Attestation{
					Data: util.HydrateAttestationData(&ethpb.AttestationData{
						Slot: 1,
					}),
					AggregationBits: bitfield.Bitlist{0b1100111},
				},
			},
			input: &ethpb.Attestation{
				Data: util.HydrateAttestationData(&ethpb.AttestationData{
					Slot: 1,
				}),
				AggregationBits: bitfield.Bitlist{0b1100000}},
			want: true,
		},
		{
			name: "multiple attestations with same data in cache with overlapping aggregation and input is superset",
			existing: []ethpb.Att{
				&ethpb.Attestation{
					Data: util.HydrateAttestationData(&ethpb.AttestationData{
						Slot: 1,
					}),
					AggregationBits: bitfield.Bitlist{0b1111000},
				},
				&ethpb.Attestation{
					Data: util.HydrateAttestationData(&ethpb.AttestationData{
						Slot: 1,
					}),
					AggregationBits: bitfield.Bitlist{0b1100111},
				},
			},
			input: &ethpb.Attestation{
				Data: util.HydrateAttestationData(&ethpb.AttestationData{
					Slot: 1,
				}),
				AggregationBits: bitfield.Bitlist{0b1111111}},
			want: false,
		},
		{
			name: "multiple attestations with different data in cache",
			existing: []ethpb.Att{
				&ethpb.Attestation{
					Data: util.HydrateAttestationData(&ethpb.AttestationData{
						Slot: 2,
					}),
					AggregationBits: bitfield.Bitlist{0b1111000},
				},
				&ethpb.Attestation{
					Data: util.HydrateAttestationData(&ethpb.AttestationData{
						Slot: 3,
					}),
					AggregationBits: bitfield.Bitlist{0b1100111},
				},
			},
			input: &ethpb.Attestation{
				Data: util.HydrateAttestationData(&ethpb.AttestationData{
					Slot: 1,
				}),
				AggregationBits: bitfield.Bitlist{0b1111111}},
			want: false,
		},
		{
			name: "attestations with different bitlist lengths",
			existing: []ethpb.Att{
				&ethpb.Attestation{
					Data: util.HydrateAttestationData(&ethpb.AttestationData{
						Slot: 2,
					}),
					AggregationBits: bitfield.Bitlist{0b1111000},
				},
			},
			input: &ethpb.Attestation{
				Data: util.HydrateAttestationData(&ethpb.AttestationData{
					Slot: 2,
				}),
				AggregationBits: bitfield.Bitlist{0b1111},
			},
			want: false,
			err:  bitfield.ErrBitlistDifferentLength,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewAttCaches()
			require.NoError(t, cache.SaveAggregatedAttestations(tt.existing))

			if tt.input != nil && tt.input.Signature == nil {
				tt.input.Signature = make([]byte, 96)
			}

			if tt.err != nil {
				_, err := cache.HasAggregatedAttestation(tt.input)
				require.ErrorContains(t, tt.err.Error(), err)
			} else {
				result, err := cache.HasAggregatedAttestation(tt.input)
				require.NoError(t, err)
				assert.Equal(t, tt.want, result)

				// Same test for block attestations
				cache = NewAttCaches()

				for _, att := range tt.existing {
					require.NoError(t, cache.SaveBlockAttestation(att))
				}
				result, err = cache.HasAggregatedAttestation(tt.input)
				require.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestKV_Aggregated_DuplicateAggregatedAttestations(t *testing.T) {
	cache := NewAttCaches()

	att1 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1101}})
	att2 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1111}})
	atts := []*ethpb.Attestation{att1, att2}

	for _, att := range atts {
		require.NoError(t, cache.SaveAggregatedAttestation(att))
	}

	returned := cache.AggregatedAttestations()

	// It should have only returned att2.
	assert.DeepSSZEqual(t, att2, returned[0], "Did not receive correct aggregated atts")
	assert.Equal(t, 1, len(returned), "Did not receive correct aggregated atts")
}

func TestKV_Aggregated_AggregatedAttestationsBySlotIndex(t *testing.T) {
	cache := NewAttCaches()

	att1 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b1011}})
	att2 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 2}, AggregationBits: bitfield.Bitlist{0b1101}})
	att3 := util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b1101}})
	atts := []*ethpb.Attestation{att1, att2, att3}

	for _, att := range atts {
		require.NoError(t, cache.SaveAggregatedAttestation(att))
	}
	ctx := context.Background()
	returned := cache.AggregatedAttestationsBySlotIndex(ctx, 1, 1)
	assert.DeepEqual(t, []*ethpb.Attestation{att1}, returned)
	returned = cache.AggregatedAttestationsBySlotIndex(ctx, 1, 2)
	assert.DeepEqual(t, []*ethpb.Attestation{att2}, returned)
	returned = cache.AggregatedAttestationsBySlotIndex(ctx, 2, 1)
	assert.DeepEqual(t, []*ethpb.Attestation{att3}, returned)
}

func TestKV_Aggregated_AggregatedAttestationsBySlotIndexElectra(t *testing.T) {
	cache := NewAttCaches()

	committeeBits := primitives.NewAttestationCommitteeBits()
	committeeBits.SetBitAt(1, true)
	att1 := util.HydrateAttestationElectra(&ethpb.AttestationElectra{Data: &ethpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1011}, CommitteeBits: committeeBits})
	committeeBits = primitives.NewAttestationCommitteeBits()
	committeeBits.SetBitAt(2, true)
	att2 := util.HydrateAttestationElectra(&ethpb.AttestationElectra{Data: &ethpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1101}, CommitteeBits: committeeBits})
	committeeBits = primitives.NewAttestationCommitteeBits()
	committeeBits.SetBitAt(1, true)
	att3 := util.HydrateAttestationElectra(&ethpb.AttestationElectra{Data: &ethpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b1101}, CommitteeBits: committeeBits})
	atts := []*ethpb.AttestationElectra{att1, att2, att3}

	for _, att := range atts {
		require.NoError(t, cache.SaveAggregatedAttestation(att))
	}
	ctx := context.Background()
	returned := cache.AggregatedAttestationsBySlotIndexElectra(ctx, 1, 1)
	assert.DeepEqual(t, []*ethpb.AttestationElectra{att1}, returned)
	returned = cache.AggregatedAttestationsBySlotIndexElectra(ctx, 1, 2)
	assert.DeepEqual(t, []*ethpb.AttestationElectra{att2}, returned)
	returned = cache.AggregatedAttestationsBySlotIndexElectra(ctx, 2, 1)
	assert.DeepEqual(t, []*ethpb.AttestationElectra{att3}, returned)
}
