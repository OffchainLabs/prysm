package filesystem

import (
	"context"
	"sync"
	"testing"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/db/common"
)

func TestStore_EIPImportBlacklistedPublicKeys(t *testing.T) {
	// Create a new store.
	store, err := NewStore(t.TempDir(), nil)
	require.NoError(t, err, "could not create store")

	var expected = [][fieldparams.BLSPubkeyLength]byte{}
	actual, err := store.EIPImportBlacklistedPublicKeys(context.Background())
	require.NoError(t, err, "could not get blacklisted public keys")
	require.DeepSSZEqual(t, expected, actual, "blacklisted public keys do not match")
}

func TestStore_SaveEIPImportBlacklistedPublicKeys(t *testing.T) {
	// Create a new store.
	store, err := NewStore(t.TempDir(), nil)
	require.NoError(t, err, "could not create store")

	// Save blacklisted public keys.
	err = store.SaveEIPImportBlacklistedPublicKeys(context.Background(), [][fieldparams.BLSPubkeyLength]byte{})
	require.NoError(t, err, "could not save blacklisted public keys")
}

func TestStore_LowestSignedTargetEpoch(t *testing.T) {
	// Define some saved source and target epoch.
	savedSourceEpoch, savedTargetEpoch := 42, 43

	// Create a pubkey.
	pubkey := getPubKeys(t, 1)[0]

	// Create a new store.
	store, err := NewStore(t.TempDir(), nil)
	require.NoError(t, err, "could not create store")

	// Get the lowest signed target epoch.
	_, exists, err := store.LowestSignedTargetEpoch(context.Background(), [fieldparams.BLSPubkeyLength]byte{})
	require.NoError(t, err, "could not get lowest signed target epoch")
	require.Equal(t, false, exists, "lowest signed target epoch should not exist")

	// Create an attestation with both source and target epoch
	attestation := &ethpb.IndexedAttestation{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{Epoch: primitives.Epoch(savedSourceEpoch)},
			Target: &ethpb.Checkpoint{Epoch: primitives.Epoch(savedTargetEpoch)},
		},
	}

	// Save the attestation.
	err = store.SaveAttestationForPubKey(context.Background(), pubkey, [32]byte{}, attestation)
	require.NoError(t, err, "SaveAttestationForPubKey should not return an error")

	// Get the lowest signed target epoch.
	expected := primitives.Epoch(savedTargetEpoch)
	actual, exists, err := store.LowestSignedTargetEpoch(context.Background(), pubkey)
	require.NoError(t, err, "could not get lowest signed target epoch")
	require.Equal(t, true, exists, "lowest signed target epoch should not exist")
	require.Equal(t, expected, actual, "lowest signed target epoch should match")
}

func TestStore_LowestSignedSourceEpoch(t *testing.T) {
	// Create a pubkey.
	pubkey := getPubKeys(t, 1)[0]

	// Create a new store.
	store, err := NewStore(t.TempDir(), nil)
	require.NoError(t, err, "could not create store")

	// Get the lowest signed target epoch.
	_, exists, err := store.LowestSignedSourceEpoch(context.Background(), [fieldparams.BLSPubkeyLength]byte{})
	require.NoError(t, err, "could not get lowest signed source epoch")
	require.Equal(t, false, exists, "lowest signed source epoch should not exist")

	// Create an attestation.
	savedSourceEpoch, savedTargetEpoch := 42, 43
	attestation := &ethpb.IndexedAttestation{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{Epoch: primitives.Epoch(savedSourceEpoch)},
			Target: &ethpb.Checkpoint{Epoch: primitives.Epoch(savedTargetEpoch)},
		},
	}

	// Save the attestation.
	err = store.SaveAttestationForPubKey(context.Background(), pubkey, [32]byte{}, attestation)
	require.NoError(t, err, "SaveAttestationForPubKey should not return an error")

	// Get the lowest signed target epoch.
	expected := primitives.Epoch(savedSourceEpoch)
	actual, exists, err := store.LowestSignedSourceEpoch(context.Background(), pubkey)
	require.NoError(t, err, "could not get lowest signed target epoch")
	require.Equal(t, true, exists, "lowest signed target epoch should exist")
	require.Equal(t, expected, actual, "lowest signed target epoch should match")
}

func TestStore_AttestedPublicKeys(t *testing.T) {
	// Create a database path.
	databasePath := t.TempDir()

	// Create some pubkeys.
	pubkeys := getPubKeys(t, 5)

	// Create a new store.
	s, err := NewStore(databasePath, &Config{PubKeys: pubkeys})
	require.NoError(t, err, "NewStore should not return an error")

	// Attest for some pubkeys.
	attestedPubkeys := pubkeys[1:3]
	for _, pubkey := range attestedPubkeys {
		err = s.SaveAttestationForPubKey(context.Background(), pubkey, [32]byte{}, &ethpb.IndexedAttestation{
			Data: &ethpb.AttestationData{
				Source: &ethpb.Checkpoint{Epoch: 42},
				Target: &ethpb.Checkpoint{Epoch: 43},
			},
		})
		require.NoError(t, err, "SaveAttestationForPubKey should not return an error")
	}

	// Check the public keys.
	actual, err := s.AttestedPublicKeys(context.Background())
	require.NoError(t, err, "publicKeys should not return an error")

	// We cannot compare the slices directly because the order is not guaranteed,
	// so we compare sets instead.
	expectedSet := make(map[[fieldparams.BLSPubkeyLength]byte]bool)
	for _, pubkey := range attestedPubkeys {
		expectedSet[pubkey] = true
	}

	actualSet := make(map[[fieldparams.BLSPubkeyLength]byte]bool)
	for _, pubkey := range actual {
		actualSet[pubkey] = true
	}

	require.DeepEqual(t, expectedSet, actualSet)
}

func TestStore_SaveAttestationForPubKey(t *testing.T) {
	// Create a public key.
	pubkey := getPubKeys(t, 1)[0]

	for _, tt := range []struct {
		name            string
		existingAttInDB *ethpb.IndexedAttestation
		incomingAtt     *ethpb.IndexedAttestation
		expectedErr     string
	}{
		{
			name:            "att is nil",
			existingAttInDB: nil,
			incomingAtt:     nil,
			expectedErr:     "incoming attestation does not contain source and/or target epoch",
		},
		{
			name:            "att.Data is nil",
			existingAttInDB: nil,
			incomingAtt:     &ethpb.IndexedAttestation{Data: nil},
			expectedErr:     "incoming attestation does not contain source and/or target epoch",
		},
		{
			name:            "att.Data.Source is nil",
			existingAttInDB: nil,
			incomingAtt: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: nil,
					Target: &ethpb.Checkpoint{Epoch: 43},
				},
			},
			expectedErr: "incoming attestation does not contain source and/or target epoch",
		},
		{
			name:            "att.Data.Target is nil",
			existingAttInDB: nil,
			incomingAtt: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 42},
					Target: nil,
				},
			},
			expectedErr: "incoming attestation does not contain source and/or target epoch",
		},
		{
			name:            "no pre-existing slashing protection",
			existingAttInDB: nil,
			incomingAtt: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 42},
					Target: &ethpb.Checkpoint{Epoch: 43},
				},
			},
			expectedErr: "",
		},
		{
			name: "incoming source epoch lower than saved source epoch",
			existingAttInDB: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 42},
					Target: &ethpb.Checkpoint{Epoch: 43},
				},
			},
			incomingAtt: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 41},
					Target: &ethpb.Checkpoint{Epoch: 45},
				},
			},
			expectedErr: "could not sign attestation with source lower than recorded source epoch",
		},
		{
			name: "incoming target epoch lower than saved target epoch",
			existingAttInDB: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 42},
					Target: &ethpb.Checkpoint{Epoch: 43},
				},
			},
			incomingAtt: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 42},
					Target: &ethpb.Checkpoint{Epoch: 42},
				},
			},
			expectedErr: "could not sign attestation with target lower than or equal to recorded target epoch",
		},
		{
			name: "incoming target epoch equal to saved target epoch",
			existingAttInDB: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 42},
					Target: &ethpb.Checkpoint{Epoch: 43},
				},
			},
			incomingAtt: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 42},
					Target: &ethpb.Checkpoint{Epoch: 43},
				},
			},
			expectedErr: "could not sign attestation with target lower than or equal to recorded target epoch",
		},
		{
			name: "nominal",
			existingAttInDB: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 42},
					Target: &ethpb.Checkpoint{Epoch: 43},
				},
			},
			incomingAtt: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: 43},
					Target: &ethpb.Checkpoint{Epoch: 44},
				},
			},
			expectedErr: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a database path.
			databasePath := t.TempDir()

			// Create a new store.
			store, err := NewStore(databasePath, nil)
			require.NoError(t, err, "NewStore should not return an error")

			if tt.existingAttInDB != nil {
				// Simulate an already existing slashing protection.
				err = store.SaveAttestationForPubKey(context.Background(), pubkey, [32]byte{}, tt.existingAttInDB)
				require.NoError(t, err, "failed to save attestation when simulating an already existing slashing protection")
			}

			if tt.incomingAtt != nil {
				// Attempt to save a new attestation.
				err = store.SaveAttestationForPubKey(context.Background(), pubkey, [32]byte{}, tt.incomingAtt)
				if len(tt.expectedErr) > 0 {
					require.ErrorContains(t, tt.expectedErr, err)
				} else {
					require.NoError(t, err, "call to SaveAttestationForPubKey should not return an error")
				}
			}
		})
	}
}

func pointerFromInt(i uint64) *uint64 {
	return &i
}

func TestStore_SaveAttestationsForPubKey2(t *testing.T) {
	// Get the context.
	ctx := context.Background()

	// Create a public key.
	pubkey := getPubKeys(t, 1)[0]

	for _, tt := range []struct {
		name                            string
		existingAttInDB                 *ethpb.IndexedAttestation
		incomingAtts                    []*ethpb.IndexedAttestation
		expectedSavedSlashingProtection *ValidatorSlashingProtection
	}{
		{
			name:                            "no atts",
			existingAttInDB:                 nil,
			incomingAtts:                    nil,
			expectedSavedSlashingProtection: nil,
		},
		{
			//               40 ==========> 45   <----- Will be recorded into DB
			//      30 ==========> 40
			name:            "no pre-existing slashing protection",
			existingAttInDB: nil,
			incomingAtts: []*ethpb.IndexedAttestation{
				{
					Data: &ethpb.AttestationData{
						Source: &ethpb.Checkpoint{Epoch: primitives.Epoch(40)},
						Target: &ethpb.Checkpoint{Epoch: primitives.Epoch(45)},
					},
				},
				{
					Data: &ethpb.AttestationData{
						Source: &ethpb.Checkpoint{Epoch: primitives.Epoch(30)},
						Target: &ethpb.Checkpoint{Epoch: primitives.Epoch(40)},
					},
				},
			},
			expectedSavedSlashingProtection: &ValidatorSlashingProtection{
				LastSignedAttestationSourceEpoch: 40,
				LastSignedAttestationTargetEpoch: pointerFromInt(45),
			},
		},
		{
			name: "surrounded incoming attestation",
			//               40 ==========> 45   <----- Already recorded into DB
			//                   42 => 43        <----- Incoming attestation
			// ------------------------------------------------------------------------------------------------
			//                   42 ======> 45   <----- Will be recorded into DB (max source and target epochs)
			existingAttInDB: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: primitives.Epoch(40)},
					Target: &ethpb.Checkpoint{Epoch: primitives.Epoch(45)},
				},
			},
			incomingAtts: []*ethpb.IndexedAttestation{
				{
					Data: &ethpb.AttestationData{
						Source: &ethpb.Checkpoint{Epoch: primitives.Epoch(42)},
						Target: &ethpb.Checkpoint{Epoch: primitives.Epoch(43)},
					},
				},
			},
			expectedSavedSlashingProtection: &ValidatorSlashingProtection{
				LastSignedAttestationSourceEpoch: 42,
				LastSignedAttestationTargetEpoch: pointerFromInt(45),
			},
		},
		{
			name: "surrounding incoming attestation",
			// We create a surrounding attestation
			//                   42 ======> 45          <----- Already recorded into DB
			//              40 ==================> 50   <----- Incoming attestation
			// ------------------------------------------------------------------------------------------------------
			//                   42 =============> 50   <----- Will be recorded into DB (max source and target epochs)
			existingAttInDB: &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{Epoch: primitives.Epoch(42)},
					Target: &ethpb.Checkpoint{Epoch: primitives.Epoch(45)},
				},
			},
			incomingAtts: []*ethpb.IndexedAttestation{
				{
					Data: &ethpb.AttestationData{
						Source: &ethpb.Checkpoint{Epoch: primitives.Epoch(40)},
						Target: &ethpb.Checkpoint{Epoch: primitives.Epoch(50)},
					},
				},
			},
			expectedSavedSlashingProtection: &ValidatorSlashingProtection{
				LastSignedAttestationSourceEpoch: 42,
				LastSignedAttestationTargetEpoch: pointerFromInt(50),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a database path.
			databasePath := t.TempDir()

			// Create a new store.
			store, err := NewStore(databasePath, nil)
			require.NoError(t, err, "NewStore should not return an error")

			// Simulate an already existing slashing protection.
			if tt.existingAttInDB != nil {
				err = store.SaveAttestationForPubKey(ctx, pubkey, [32]byte{}, tt.existingAttInDB)
				require.NoError(t, err, "failed to save attestation when simulating an already existing slashing protection")
			}

			// Save attestations.
			err = store.SaveAttestationsForPubKey(ctx, pubkey, [][]byte{}, tt.incomingAtts)
			require.NoError(t, err, "SaveAttestationsForPubKey should not return an error")

			// Check the correct source / target epochs are saved.
			actualValidatorSlashingProtection, err := store.validatorSlashingProtection(pubkey)
			require.NoError(t, err, "validatorSlashingProtection should not return an error")
			require.DeepEqual(t, tt.expectedSavedSlashingProtection, actualValidatorSlashingProtection)
		})
	}
}

func TestStore_AttestationHistoryForPubKey(t *testing.T) {
	// Get a database path.
	databasePath := t.TempDir()

	// Create a public key.
	pubkey := getPubKeys(t, 1)[0]

	// Create a new store.
	store, err := NewStore(databasePath, nil)
	require.NoError(t, err, "NewStore should not return an error")

	// Get the attestation history.
	actual, err := store.AttestationHistoryForPubKey(context.Background(), pubkey)
	require.NoError(t, err, "AttestationHistoryForPubKey should not return an error")
	require.DeepEqual(t, []*common.AttestationRecord{}, actual)

	// Create an attestation.
	savedSourceEpoch, savedTargetEpoch := 42, 43
	attestation := &ethpb.IndexedAttestation{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{Epoch: primitives.Epoch(savedSourceEpoch)},
			Target: &ethpb.Checkpoint{Epoch: primitives.Epoch(savedTargetEpoch)},
		},
	}

	// Save the attestation.
	err = store.SaveAttestationForPubKey(context.Background(), pubkey, [32]byte{}, attestation)
	require.NoError(t, err, "SaveAttestationForPubKey should not return an error")

	// Get the attestation history.
	expected := []*common.AttestationRecord{
		{
			PubKey: pubkey,
			Source: primitives.Epoch(savedSourceEpoch),
			Target: primitives.Epoch(savedTargetEpoch),
		},
	}

	actual, err = store.AttestationHistoryForPubKey(context.Background(), pubkey)
	require.NoError(t, err, "AttestationHistoryForPubKey should not return an error")
	require.DeepEqual(t, expected, actual)
}

func BenchmarkStore_SaveAttestationForPubKey(b *testing.B) {
	var wg sync.WaitGroup
	ctx := context.Background()

	// Create pubkeys
	pubkeys := make([][fieldparams.BLSPubkeyLength]byte, 2000)
	for i := range pubkeys {
		validatorKey, err := bls.RandKey()
		require.NoError(b, err, "RandKey should not return an error")

		copy(pubkeys[i][:], validatorKey.PublicKey().Marshal())
	}

	signingRoot := [32]byte{1}
	attestation := &ethpb.IndexedAttestation{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{
				Epoch: 42,
			},
			Target: &ethpb.Checkpoint{
				Epoch: 43,
			},
		},
	}

	validatorDB, err := NewStore(b.TempDir(), &Config{PubKeys: pubkeys})
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		err := validatorDB.ClearDB()
		require.NoError(b, err)

		for _, pubkey := range pubkeys {
			wg.Add(1)

			go func(pk [fieldparams.BLSPubkeyLength]byte) {
				defer wg.Done()

				err := validatorDB.SaveAttestationForPubKey(ctx, pk, signingRoot, attestation)
				require.NoError(b, err)
			}(pubkey)
		}

		b.StartTimer()
		wg.Wait()
	}

	err = validatorDB.Close()
	require.NoError(b, err)
}
