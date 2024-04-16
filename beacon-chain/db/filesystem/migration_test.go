package filesystem

import (
	"os"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/spf13/afero"
)

func ezIdent(t *testing.T, rootStr string, epoch primitives.Epoch, index uint64) blobIdent {
	r, err := stringToRoot(rootStr)
	require.NoError(t, err)
	return blobIdent{root: r, epoch: epoch, index: index}
}

func setupTestBlobFile(t *testing.T, ident blobIdent, offset primitives.Slot, fs afero.Fs, l fsLayout) {
	slot, err := slots.EpochStart(ident.epoch)
	require.NoError(t, err)
	slot += offset
	_, sc := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, slot, 1)
	scb, err := sc[0].MarshalSSZ()
	require.NoError(t, err)
	dir := l.dir(ident)
	require.NoError(t, fs.MkdirAll(dir, directoryPermissions()))
	p := l.sszPath(ident)
	require.NoError(t, afero.WriteFile(fs, p, scb, 0666))
	_, err = fs.Stat(p)
	require.NoError(t, err)
}

type migrationTestTarget struct {
	ident      blobIdent
	slotOffset primitives.Slot
	migrated   bool
}

func testAssertFsMigrated(t *testing.T, fs afero.Fs, ident blobIdent, before, after fsLayout) {
	// Assert the pre-migration path is gone.
	_, err := fs.Stat(before.sszPath(ident))
	require.ErrorIs(t, err, os.ErrNotExist)
	dir := before.dir(ident)
	_, err = listDir(fs, dir)
	require.ErrorIs(t, err, os.ErrNotExist)

	// Assert the post-migration path present.
	_, err = fs.Stat(after.sszPath(ident))
	require.NoError(t, err)
}

func TestMigrations(t *testing.T) {
	cases := []struct {
		name           string
		forwardLayout  string
		backwardLayout string
		targets        []migrationTestTarget
	}{
		{
			name:           "all need migration",
			backwardLayout: LayoutNameFlat,
			forwardLayout:  LayoutNameByEpoch,
			targets: []migrationTestTarget{
				{
					ident: ezIdent(t, "0x0125e54c64c925018c9296965a5b622d9f5ab626c10917860dcfb6aa09a0a00b", 1234, 0),
				},
				{
					ident:      ezIdent(t, "0x0127dba6fd30fdbb47e73e861d5c6e602b38ac3ddc945bb6a2fc4e10761e9a86", 5330, 0),
					slotOffset: 31,
				},
				{
					ident:      ezIdent(t, "0x0127dba6fd30fdbb47e73e861d5c6e602b38ac3ddc945bb6a2fc4e10761e9a86", 5330, 1),
					slotOffset: 31,
				},
				{
					ident:      ezIdent(t, "0x0232521756a0b965eab2c2245d7ad85feaeaf5f427cd14d1a7531f9d555b415c", 16777216, 0),
					slotOffset: 16,
				},
			},
		},
		{
			name:           "mix old and new",
			backwardLayout: LayoutNameFlat,
			forwardLayout:  LayoutNameByEpoch,
			targets: []migrationTestTarget{
				{
					ident: ezIdent(t, "0x0125e54c64c925018c9296965a5b622d9f5ab626c10917860dcfb6aa09a0a00b", 1234, 0),
				},
				{
					ident:      ezIdent(t, "0x0127dba6fd30fdbb47e73e861d5c6e602b38ac3ddc945bb6a2fc4e10761e9a86", 5330, 0),
					slotOffset: 31,
				},
				{
					ident:      ezIdent(t, "0x0127dba6fd30fdbb47e73e861d5c6e602b38ac3ddc945bb6a2fc4e10761e9a86", 5330, 1),
					slotOffset: 31,
				},
				{
					ident:      ezIdent(t, "0x0232521756a0b965eab2c2245d7ad85feaeaf5f427cd14d1a7531f9d555b415c", 16777216, 0),
					slotOffset: 16,
					migrated:   true,
				},
				{
					ident:      ezIdent(t, "0x0232521756a0b965eab2c2245d7ad85feaeaf5f427cd14d1a7531f9d555b415c", 16777216, 1),
					slotOffset: 16,
					migrated:   true,
				},
				{
					ident:      ezIdent(t, "0x42eabe3d2c125410cd226de6f2825fb7575ab896c3f52e43de1fa29e4c809aba", 16777217, 0),
					slotOffset: 16,
					migrated:   true,
				},
				{
					ident:    ezIdent(t, "0x666cea5034e22bd3b849cb33914cad59afd88ee08e4d5bc0e997411c945fbc1d", 11235, 1),
					migrated: true,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Run("forward", func(t *testing.T) {
				testMigration(t, c.forwardLayout, c.backwardLayout, c.targets)
			})
			// run the same test in reverse - to cover both directions while making the test table smaller.
			t.Run("backward", func(t *testing.T) {
				testMigration(t, c.forwardLayout, c.backwardLayout, c.targets)
			})
		})
	}
}

func testMigration(t *testing.T, forwardName, backwardName string, targets []migrationTestTarget) {
	fs := afero.NewMemMapFs()
	cache := newBlobStorageCache()
	forward, err := newLayout(forwardName, fs, cache, nil)
	require.NoError(t, err)
	backward, err := newLayout(backwardName, fs, cache, nil)
	require.NoError(t, err)
	for _, tar := range targets {
		if tar.migrated {
			setupTestBlobFile(t, tar.ident, tar.slotOffset, fs, forward)
		} else {
			setupTestBlobFile(t, tar.ident, tar.slotOffset, fs, backward)
		}
	}
	require.NoError(t, migrateLayout(fs, backward, forward, cache))
	for _, tar := range targets {
		// Make sure the file wound up in the right spot, according to the forward layout
		// and that the old file is gone, according to the backward layout.
		testAssertFsMigrated(t, fs, tar.ident, backward, forward)
		entry, ok := cache.get(tar.ident.root)
		// we only expect cache to be populated here by files that needed to be moved.
		if !tar.migrated {
			require.Equal(t, true, ok)
			require.Equal(t, true, entry.HasIndex(tar.ident.index))
			require.Equal(t, tar.ident.epoch, entry.epoch)
		}
	}

	// Run migration in reverse - testing "undo"
	cache = newBlobStorageCache()
	forward, err = newLayout(forwardName, fs, cache, nil)
	require.NoError(t, err)
	backward, err = newLayout(backwardName, fs, cache, nil)
	require.NoError(t, err)
	// forward and backward are flipped compared to the above
	require.NoError(t, migrateLayout(fs, forward, backward, cache))
	for _, tar := range targets {
		// just like the above, but forward and backward are flipped
		testAssertFsMigrated(t, fs, tar.ident, forward, backward)
		entry, ok := cache.get(tar.ident.root)
		require.Equal(t, true, ok)
		require.Equal(t, true, entry.HasIndex(tar.ident.index))
		require.Equal(t, tar.ident.epoch, entry.epoch)
	}
}
