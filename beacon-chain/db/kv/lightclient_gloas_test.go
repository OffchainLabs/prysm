package kv

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	light_client "github.com/OffchainLabs/prysm/v7/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/golang/snappy"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

func TestLightClientEncoding_ForkCompatibility(t *testing.T) {
	setupLightClientStorageConfig(t)
	for _, tt := range []struct {
		fork int
		key  string
	}{
		{version.Altair, "altair"},
		{version.Bellatrix, "merge"},
		{version.Capella, "capella"},
		{version.Deneb, "deneb"},
		{version.Electra, "electra"},
		{version.Fulu, "fulu"},
		{version.Gloas, "gloas"},
	} {
		t.Run(version.String(tt.fork), func(t *testing.T) {
			update, err := createUpdate(t, tt.fork)
			require.NoError(t, err)
			encoded, err := encodeLightClientUpdate(update)
			require.NoError(t, err)
			ssz, err := update.MarshalSSZ()
			require.NoError(t, err)
			require.DeepEqual(t, snappy.Encode(nil, append([]byte(tt.key), ssz...)), encoded)
			decoded, err := decodeLightClientUpdate(encoded)
			require.NoError(t, err)
			require.Equal(t, tt.fork, decoded.Version())
			require.DeepEqual(t, update.Proto(), decoded.Proto())

			bootstrap, err := createDefaultLightClientBootstrap(update.AttestedHeader().Beacon().Slot)
			require.NoError(t, err)
			require.NoError(t, bootstrap.SetCurrentSyncCommittee(createRandomSyncCommittee()))
			committeeHash, err := bootstrap.CurrentSyncCommittee().HashTreeRoot()
			require.NoError(t, err)
			encoded, err = encodeLightClientBootstrap(bootstrap, committeeHash)
			require.NoError(t, err)
			ssz, err = bootstrap.MarshalSSZ()
			require.NoError(t, err)
			expected := append([]byte(tt.key), committeeHash[:]...)
			expected = append(expected, ssz...)
			require.DeepEqual(t, snappy.Encode(nil, expected), encoded)
			decodedBootstrap, decodedHash, err := decodeLightClientBootstrap(encoded)
			require.NoError(t, err)
			require.Equal(t, tt.fork, decodedBootstrap.Version())
			require.DeepEqual(t, committeeHash[:], decodedHash)
			require.DeepEqual(t, bootstrap.Proto(), decodedBootstrap.Proto())

			t.Run("invalid SSZ", func(t *testing.T) {
				_, err := decodeLightClientUpdate(snappy.Encode(nil, append([]byte(tt.key), 0)))
				require.ErrorContains(t, "could not unmarshal", err)
				_, _, err = decodeLightClientBootstrap(snappy.Encode(nil, append([]byte(tt.key), make([]byte, 33)...)))
				require.ErrorContains(t, "could not unmarshal", err)
			})
		})
	}
	t.Run("unsupported version", func(t *testing.T) {
		for _, fork := range []int{-1, version.Phase0, version.Heze} {
			_, err := keyForLightClientUpdate(fork)
			require.ErrorContains(t, "unsupported light client update version", err)
		}
	})
}

func TestLightClientStorage_Gloas(t *testing.T) {
	setupLightClientStorageConfig(t)
	fuluUpdate, err := createUpdate(t, version.Fulu)
	require.NoError(t, err)
	gloasUpdate, err := createUpdate(t, version.Gloas)
	require.NoError(t, err)
	updates := map[uint64]interfaces.LightClientUpdate{10: fuluUpdate, 12: gloasUpdate}
	committee := createEmptySyncCommittee()
	committee.Pubkeys[0][0] = 42
	committee.AggregatePubkey[0] = 43
	committeeHash, err := committee.HashTreeRoot()
	require.NoError(t, err)
	bootstraps := make([]interfaces.LightClientBootstrap, 4)
	for i, slot := range []primitives.Slot{5 * fieldparams.SlotsPerEpoch, 6 * fieldparams.SlotsPerEpoch, 6*fieldparams.SlotsPerEpoch + 1, 6*fieldparams.SlotsPerEpoch + 2} {
		bootstraps[i], err = createDefaultLightClientBootstrap(slot)
		require.NoError(t, err)
		require.NoError(t, bootstraps[i].SetCurrentSyncCommittee(proto.Clone(committee).(*pb.SyncCommittee)))
	}
	bootstraps[3].CurrentSyncCommittee().Pubkeys[0][0] = 99
	beforeSave := make([]proto.Message, len(bootstraps))
	for i, bootstrap := range bootstraps {
		beforeSave[i] = proto.Clone(bootstrap.Proto())
	}
	dir := t.TempDir()
	for _, phase := range []string{"save", "reopen"} {
		t.Run(phase, func(t *testing.T) {
			db, err := NewKVStore(t.Context(), dir)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			if phase == "save" {
				for period, update := range updates {
					require.NoError(t, db.SaveLightClientUpdate(t.Context(), period, update))
				}
				for i, bootstrap := range bootstraps {
					require.NoError(t, db.SaveLightClientBootstrap(t.Context(), lightClientTestRoots(4)[i], bootstrap))
					require.Equal(t, true, proto.Equal(beforeSave[i], bootstrap.Proto()), "saving must not mutate the caller's bootstrap")
				}
			}
			for period, expected := range updates {
				got, err := db.LightClientUpdate(t.Context(), period)
				require.NoError(t, err)
				require.Equal(t, expected.Version(), got.Version())
				require.DeepEqual(t, expected.Proto(), got.Proto())
			}
			got, err := db.LightClientUpdates(t.Context(), 10, 12)
			require.NoError(t, err)
			require.Equal(t, len(updates), len(got))
			for period, expected := range updates {
				require.NotNil(t, got[period])
				require.DeepEqual(t, expected.Proto(), got[period].Proto())
			}
			got, err = db.LightClientUpdates(t.Context(), 11, 12)
			require.NoError(t, err)
			require.Equal(t, 1, len(got))
			require.NotNil(t, got[12])
			require.DeepEqual(t, gloasUpdate.Proto(), got[12].Proto())
			for i, expected := range bootstraps {
				got, err := db.LightClientBootstrap(t.Context(), lightClientTestRoots(4)[i])
				require.NoError(t, err)
				require.Equal(t, expected.Version(), got.Version())
				require.DeepEqual(t, expected.Proto(), got.Proto())
			}
			require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
				committees := tx.Bucket(lightClientSyncCommitteeBucket)
				require.Equal(t, 2, committees.Stats().KeyN, "shared committees should be deduplicated across forks")
				committeeSSZ, err := committee.MarshalSSZ()
				require.NoError(t, err)
				require.DeepEqual(t, committeeSSZ, committees.Get(committeeHash[:]))
				encoded := tx.Bucket(lightClientBootstrapBucket).Get(lightClientTestRoots(4)[1])
				stored, hash, err := decodeLightClientBootstrap(encoded)
				require.NoError(t, err)
				require.DeepEqual(t, committeeHash[:], hash)
				require.DeepEqual(t, createEmptySyncCommittee(), stored.CurrentSyncCommittee())
				return nil
			}))
		})
	}
}

func TestLightClientEncoding_CorruptGloas(t *testing.T) {
	setupLightClientStorageConfig(t)
	t.Run("invalid compression", func(t *testing.T) {
		_, err := decodeLightClientUpdate([]byte{0xff})
		require.ErrorContains(t, "could not snappy decode light client update", err)
		_, _, err = decodeLightClientBootstrap([]byte{0xff})
		require.ErrorContains(t, "could not snappy decode light client bootstrap", err)
	})
	t.Run("unknown or empty payload", func(t *testing.T) {
		for _, payload := range [][]byte{nil, []byte("unknown"), []byte("gloas")} {
			_, err := decodeLightClientUpdate(snappy.Encode(nil, payload))
			require.ErrorContains(t, "unsupported", err)
			_, _, err = decodeLightClientBootstrap(snappy.Encode(nil, payload))
			require.ErrorContains(t, "unsupported", err)
		}
	})
	t.Run("truncated committee hash", func(t *testing.T) {
		for size := 1; size < 32; size++ {
			_, _, err := decodeLightClientBootstrap(snappy.Encode(nil, append([]byte("gloas"), make([]byte, size)...)))
			require.ErrorContains(t, "missing sync committee hash", err)
		}
	})
	t.Run("truncated SSZ", func(t *testing.T) {
		update, err := createUpdate(t, version.Gloas)
		require.NoError(t, err)
		ssz, err := update.MarshalSSZ()
		require.NoError(t, err)
		_, err = decodeLightClientUpdate(snappy.Encode(nil, append([]byte("gloas"), ssz[:len(ssz)-1]...)))
		require.ErrorContains(t, "could not unmarshal Gloas light client update", err)
		bootstrap, err := createDefaultLightClientBootstrap(update.AttestedHeader().Beacon().Slot)
		require.NoError(t, err)
		ssz, err = bootstrap.MarshalSSZ()
		require.NoError(t, err)
		for _, payload := range [][]byte{nil, ssz[:len(ssz)-1]} {
			encoded := append([]byte("gloas"), make([]byte, 32)...)
			_, _, err = decodeLightClientBootstrap(snappy.Encode(nil, append(encoded, payload...)))
			require.ErrorContains(t, "could not unmarshal Gloas light client bootstrap", err)
		}
	})
}

func setupLightClientStorageConfig(t *testing.T) {
	t.Helper()
	cfg := params.MainnetTestConfig()
	if fieldparams.Preset == "minimal" {
		cfg = params.MinimalSpecConfig()
	}
	cfg.AltairForkEpoch, cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch = 0, 1, 2
	cfg.DenebForkEpoch, cfg.ElectraForkEpoch, cfg.FuluForkEpoch, cfg.GloasForkEpoch = 3, 4, 5, 6
	params.SetActiveTestCleanup(t, cfg)
}

func lightClientTestRoots(count int) [][]byte {
	roots := make([][]byte, count)
	for i := range roots {
		roots[i] = make([]byte, fieldparams.RootLength)
		roots[i][0] = byte(i + 1)
	}
	return roots
}

func createGloasLightClientHeader(slot primitives.Slot) *pb.LightClientHeaderGloas {
	roots := lightClientTestRoots(4)
	return &pb.LightClientHeaderGloas{
		Beacon: &pb.BeaconBlockHeader{
			Slot: slot, ProposerIndex: 42,
			ParentRoot: roots[0], StateRoot: roots[1], BodyRoot: roots[2],
		},
		ExecutionBlockHash: roots[3],
		ExecutionBranch:    lightClientTestRoots(fieldparams.ExecutionBranchDepthGloas),
	}
}

func createGloasLightClientUpdate(slot primitives.Slot) (interfaces.LightClientUpdate, error) {
	committee := createEmptySyncCommittee()
	committee.Pubkeys[0][0] = 42
	bits := make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	bits[0] = 1
	return light_client.NewWrappedUpdate(&pb.LightClientUpdateGloas{
		AttestedHeader:          createGloasLightClientHeader(slot),
		FinalizedHeader:         createGloasLightClientHeader(slot - 2),
		NextSyncCommittee:       committee,
		NextSyncCommitteeBranch: lightClientTestRoots(fieldparams.SyncCommitteeBranchDepthGloas),
		FinalityBranch:          lightClientTestRoots(fieldparams.FinalityBranchDepthGloas),
		SyncAggregate: &pb.SyncAggregate{
			SyncCommitteeBits: bits, SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		},
		SignatureSlot: slot + 1,
	})
}
