package das

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

var commitments = [][]byte{
	bytesutil.PadTo([]byte("a"), 48),
	bytesutil.PadTo([]byte("b"), 48),
	bytesutil.PadTo([]byte("c"), 48),
	bytesutil.PadTo([]byte("d"), 48),
}

func TestPersist(t *testing.T) {
	t.Run("no sidecars", func(t *testing.T) {
		dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)
		lazilyPersistentStoreColumns := NewLazilyPersistentStoreColumn(dataColumnStorage, enode.ID{}, nil, 0)
		err := lazilyPersistentStoreColumns.Persist(0)
		require.NoError(t, err)
		require.Equal(t, 0, len(lazilyPersistentStoreColumns.cache.entries))
	})

	t.Run("mixed roots", func(t *testing.T) {
		dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)

		dataColumnParamsByBlockRoot := []util.DataColumnParam{
			{Slot: 1, Index: 1},
			{Slot: 2, Index: 2},
		}

		roSidecars, _ := roSidecarsFromDataColumnParamsByBlockRoot(t, dataColumnParamsByBlockRoot)
		lazilyPersistentStoreColumns := NewLazilyPersistentStoreColumn(dataColumnStorage, enode.ID{}, nil, 0)

		err := lazilyPersistentStoreColumns.Persist(0, roSidecars...)
		require.ErrorIs(t, err, errMixedRoots)
		require.Equal(t, 0, len(lazilyPersistentStoreColumns.cache.entries))
	})

	t.Run("outside DA period", func(t *testing.T) {
		dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)

		dataColumnParamsByBlockRoot := []util.DataColumnParam{
			{Slot: 1, Index: 1},
		}

		roSidecars, _ := roSidecarsFromDataColumnParamsByBlockRoot(t, dataColumnParamsByBlockRoot)
		lazilyPersistentStoreColumns := NewLazilyPersistentStoreColumn(dataColumnStorage, enode.ID{}, nil, 0)

		err := lazilyPersistentStoreColumns.Persist(1_000_000, roSidecars...)
		require.NoError(t, err)
		require.Equal(t, 0, len(lazilyPersistentStoreColumns.cache.entries))
	})

	t.Run("nominal", func(t *testing.T) {
		const slot = 42
		dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)

		dataColumnParamsByBlockRoot := []util.DataColumnParam{
			{Slot: slot, Index: 1},
			{Slot: slot, Index: 5},
		}

		roSidecars, roDataColumns := roSidecarsFromDataColumnParamsByBlockRoot(t, dataColumnParamsByBlockRoot)
		lazilyPersistentStoreColumns := NewLazilyPersistentStoreColumn(dataColumnStorage, enode.ID{}, nil, 0)

		err := lazilyPersistentStoreColumns.Persist(slot, roSidecars...)
		require.NoError(t, err)
		require.Equal(t, 1, len(lazilyPersistentStoreColumns.cache.entries))

		key := cacheKey{slot: slot, root: roDataColumns[0].BlockRoot()}
		entry, ok := lazilyPersistentStoreColumns.cache.entries[key]
		require.Equal(t, true, ok)

		// A call to Persist does NOT save the sidecars to disk.
		require.Equal(t, uint64(0), entry.diskSummary.Count())

		require.DeepSSZEqual(t, roDataColumns[0], *entry.scs[1])
		require.DeepSSZEqual(t, roDataColumns[1], *entry.scs[5])

		for i, roDataColumn := range entry.scs {
			if map[int]bool{1: true, 5: true}[i] {
				continue
			}

			require.IsNil(t, roDataColumn)
		}
	})
}

func TestIsDataAvailable(t *testing.T) {
	newDataColumnsVerifier := func(dataColumnSidecars []blocks.RODataColumn, _ []verification.Requirement) verification.DataColumnsVerifier {
		return &mockDataColumnsVerifier{t: t, dataColumnSidecars: dataColumnSidecars}
	}

	ctx := t.Context()

	t.Run("without commitments", func(t *testing.T) {
		signedBeaconBlockFulu := util.NewBeaconBlockFulu()
		signedRoBlock := newSignedRoBlock(t, signedBeaconBlockFulu)

		dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)
		lazilyPersistentStoreColumns := NewLazilyPersistentStoreColumn(dataColumnStorage, enode.ID{}, newDataColumnsVerifier, 0)

		err := lazilyPersistentStoreColumns.IsDataAvailable(ctx, 0 /*current slot*/, signedRoBlock)
		require.NoError(t, err)
	})

	t.Run("with commitments", func(t *testing.T) {
		signedBeaconBlockFulu := util.NewBeaconBlockFulu()
		signedBeaconBlockFulu.Block.Body.BlobKzgCommitments = commitments
		signedRoBlock := newSignedRoBlock(t, signedBeaconBlockFulu)
		block := signedRoBlock.Block()
		slot := block.Slot()
		proposerIndex := block.ProposerIndex()
		parentRoot := block.ParentRoot()
		stateRoot := block.StateRoot()
		bodyRoot, err := block.Body().HashTreeRoot()
		require.NoError(t, err)

		root := signedRoBlock.Root()

		dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)
		lazilyPersistentStoreColumns := NewLazilyPersistentStoreColumn(dataColumnStorage, enode.ID{}, newDataColumnsVerifier, 0)

		indices := [...]uint64{1, 17, 19, 42, 75, 87, 102, 117}
		dataColumnsParams := make([]util.DataColumnParam, 0, len(indices))
		for _, index := range indices {
			dataColumnParams := util.DataColumnParam{
				Index:          index,
				KzgCommitments: commitments,

				Slot:          slot,
				ProposerIndex: proposerIndex,
				ParentRoot:    parentRoot[:],
				StateRoot:     stateRoot[:],
				BodyRoot:      bodyRoot[:],
			}

			dataColumnsParams = append(dataColumnsParams, dataColumnParams)
		}

		_, verifiedRoDataColumns := util.CreateTestVerifiedRoDataColumnSidecars(t, dataColumnsParams)

		key := cacheKey{root: root}
		entry := lazilyPersistentStoreColumns.cache.ensure(key)
		defer lazilyPersistentStoreColumns.cache.delete(key)

		for _, verifiedRoDataColumn := range verifiedRoDataColumns {
			err := entry.stash(&verifiedRoDataColumn.RODataColumn)
			require.NoError(t, err)
		}

		err = lazilyPersistentStoreColumns.IsDataAvailable(ctx, slot, signedRoBlock)
		require.NoError(t, err)

		actual, err := dataColumnStorage.Get(root, indices[:])
		require.NoError(t, err)

		summary := dataColumnStorage.Summary(root)
		require.Equal(t, uint64(len(indices)), summary.Count())
		require.DeepSSZEqual(t, verifiedRoDataColumns, actual)
	})
}

func TestFullCommitmentsToCheck(t *testing.T) {
	windowSlots, err := slots.EpochEnd(params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		commitments [][]byte
		block       func(*testing.T) blocks.ROBlock
		slot        primitives.Slot
	}{
		{
			name: "Pre-Fulu block",
			block: func(t *testing.T) blocks.ROBlock {
				return newSignedRoBlock(t, util.NewBeaconBlockElectra())
			},
		},
		{
			name: "Commitments outside data availability window",
			block: func(t *testing.T) blocks.ROBlock {
				beaconBlockElectra := util.NewBeaconBlockElectra()

				// Block is from slot 0, "current slot" is window size +1 (so outside the window)
				beaconBlockElectra.Block.Body.BlobKzgCommitments = commitments

				return newSignedRoBlock(t, beaconBlockElectra)
			},
			slot: windowSlots + 1,
		},
		{
			name: "Commitments within data availability window",
			block: func(t *testing.T) blocks.ROBlock {
				signedBeaconBlockFulu := util.NewBeaconBlockFulu()
				signedBeaconBlockFulu.Block.Body.BlobKzgCommitments = commitments
				signedBeaconBlockFulu.Block.Slot = 100

				return newSignedRoBlock(t, signedBeaconBlockFulu)
			},
			commitments: commitments,
			slot:        100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			numberOfColumns := params.BeaconConfig().NumberOfColumns

			b := tc.block(t)
			s := NewLazilyPersistentStoreColumn(nil, enode.ID{}, nil, numberOfColumns)

			commitmentsArray, err := s.fullCommitmentsToCheck(enode.ID{}, b, tc.slot)
			require.NoError(t, err)

			for _, commitments := range commitmentsArray {
				require.DeepEqual(t, tc.commitments, commitments)
			}
		})
	}
}

func roSidecarsFromDataColumnParamsByBlockRoot(t *testing.T, parameters []util.DataColumnParam) ([]blocks.ROSidecar, []blocks.RODataColumn) {
	roDataColumns, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, parameters)

	roSidecars := make([]blocks.ROSidecar, 0, len(roDataColumns))
	for _, roDataColumn := range roDataColumns {
		roSidecars = append(roSidecars, blocks.NewSidecarFromDataColumnSidecar(roDataColumn))
	}

	return roSidecars, roDataColumns
}

func newSignedRoBlock(t *testing.T, signedBeaconBlock interface{}) blocks.ROBlock {
	sb, err := blocks.NewSignedBeaconBlock(signedBeaconBlock)
	require.NoError(t, err)

	rb, err := blocks.NewROBlock(sb)
	require.NoError(t, err)

	return rb
}

type mockDataColumnsVerifier struct {
	t                                                                        *testing.T
	dataColumnSidecars                                                       []blocks.RODataColumn
	validCalled, SidecarInclusionProvenCalled, SidecarKzgProofVerifiedCalled bool
}

var _ verification.DataColumnsVerifier = &mockDataColumnsVerifier{}

func (m *mockDataColumnsVerifier) VerifiedRODataColumns() ([]blocks.VerifiedRODataColumn, error) {
	require.Equal(m.t, true, m.validCalled && m.SidecarInclusionProvenCalled && m.SidecarKzgProofVerifiedCalled)

	verifiedDataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, len(m.dataColumnSidecars))
	for _, dataColumnSidecar := range m.dataColumnSidecars {
		verifiedDataColumnSidecar := blocks.NewVerifiedRODataColumn(dataColumnSidecar)
		verifiedDataColumnSidecars = append(verifiedDataColumnSidecars, verifiedDataColumnSidecar)
	}

	return verifiedDataColumnSidecars, nil
}

func (m *mockDataColumnsVerifier) SatisfyRequirement(verification.Requirement) {}

func (m *mockDataColumnsVerifier) ValidFields() error {
	m.validCalled = true
	return nil
}

func (m *mockDataColumnsVerifier) CorrectSubnet(dataColumnSidecarSubTopic string, expectedTopics []string) error {
	return nil
}
func (m *mockDataColumnsVerifier) NotFromFutureSlot() error                         { return nil }
func (m *mockDataColumnsVerifier) SlotAboveFinalized() error                        { return nil }
func (m *mockDataColumnsVerifier) ValidProposerSignature(ctx context.Context) error { return nil }

func (m *mockDataColumnsVerifier) SidecarParentSeen(parentSeen func([fieldparams.RootLength]byte) bool) error {
	return nil
}

func (m *mockDataColumnsVerifier) SidecarParentValid(badParent func([fieldparams.RootLength]byte) bool) error {
	return nil
}

func (m *mockDataColumnsVerifier) SidecarParentSlotLower() error       { return nil }
func (m *mockDataColumnsVerifier) SidecarDescendsFromFinalized() error { return nil }

func (m *mockDataColumnsVerifier) SidecarInclusionProven() error {
	m.SidecarInclusionProvenCalled = true
	return nil
}

func (m *mockDataColumnsVerifier) SidecarKzgProofVerified() error {
	m.SidecarKzgProofVerifiedCalled = true
	return nil
}

func (m *mockDataColumnsVerifier) SidecarProposerExpected(ctx context.Context) error { return nil }
