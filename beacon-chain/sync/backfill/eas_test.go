package backfill

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
)

type mockEASCustody struct {
	eas       primitives.Slot
	cgc       uint64
	easErr    error
	cgcErr    error
	updateErr error
	attempts  []primitives.Slot
}

var _ easCustody = &mockEASCustody{}

func (m *mockEASCustody) EarliestAvailableSlot(context.Context) (primitives.Slot, error) {
	return m.eas, m.easErr
}

func (m *mockEASCustody) CustodyGroupCount(context.Context) (uint64, error) {
	return m.cgc, m.cgcErr
}

func (m *mockEASCustody) UpdateEarliestAvailableSlot(sl primitives.Slot) error {
	m.attempts = append(m.attempts, sl)
	if m.updateErr != nil {
		return m.updateErr
	}
	m.eas = sl
	return nil
}

func testEASService(db *mockBackfillDB, custody *mockEASCustody, low uint64) *Service {
	return &Service{
		store:                &Store{store: db, bs: &dbval.BackfillStatus{LowSlot: low, OriginSlot: low}},
		custody:              custody,
		easAllowed:           true,
		easCustodyGroupCount: custody.cgc,
	}
}

func disableFulu(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = cfg.FarFutureEpoch
	params.OverrideBeaconConfig(cfg)
}

func TestUpdateEarliestAvailableSlotLowersBoth(t *testing.T) {
	require.Equal(t, true, params.FuluEnabled())
	db := &mockBackfillDB{}
	custody := &mockEASCustody{eas: 100, cgc: 4}
	s := testEASService(db, custody, 100)

	s.updateEarliestAvailableSlot(t.Context(), 50)
	require.DeepEqual(t, []primitives.Slot{50}, db.easUpdates)
	require.DeepEqual(t, []primitives.Slot{50}, custody.attempts)
	require.Equal(t, primitives.Slot(50), custody.eas)
}

func TestUpdateEarliestAvailableSlotFuluDisabled(t *testing.T) {
	disableFulu(t)
	db := &mockBackfillDB{}
	custody := &mockEASCustody{eas: 100, cgc: 4}
	s := testEASService(db, custody, 100)

	s.updateEarliestAvailableSlot(t.Context(), 50)
	require.Equal(t, 0, len(db.easUpdates))
	require.Equal(t, 0, len(custody.attempts))
	require.Equal(t, primitives.Slot(100), custody.eas)
}

func TestUpdateEarliestAvailableSlotNotAllowed(t *testing.T) {
	require.Equal(t, true, params.FuluEnabled())
	db := &mockBackfillDB{}
	custody := &mockEASCustody{eas: 100, cgc: 4}
	s := testEASService(db, custody, 100)
	s.easAllowed = false

	s.updateEarliestAvailableSlot(t.Context(), 50)
	require.Equal(t, 0, len(db.easUpdates))
	require.Equal(t, 0, len(custody.attempts))
}

func TestUpdateEarliestAvailableSlotCustodyGroupCountChanged(t *testing.T) {
	require.Equal(t, true, params.FuluEnabled())
	db := &mockBackfillDB{}
	custody := &mockEASCustody{eas: 100, cgc: 4}
	s := testEASService(db, custody, 100)
	// Simulate a custody group count increase after backfill startup.
	custody.cgc = 8

	s.updateEarliestAvailableSlot(t.Context(), 50)
	require.Equal(t, 0, len(db.easUpdates))
	require.Equal(t, 0, len(custody.attempts))
	require.Equal(t, primitives.Slot(100), custody.eas)
}

func TestUpdateEarliestAvailableSlotNeverMovesForward(t *testing.T) {
	require.Equal(t, true, params.FuluEnabled())
	db := &mockBackfillDB{}
	custody := &mockEASCustody{eas: 40, cgc: 4}
	s := testEASService(db, custody, 100)

	// A slot above the currently advertised earliest available slot must not be published.
	s.updateEarliestAvailableSlot(t.Context(), 50)
	require.Equal(t, 0, len(db.easUpdates))
	require.Equal(t, 0, len(custody.attempts))
	require.Equal(t, primitives.Slot(40), custody.eas)

	// An equal slot is a no-op as well.
	s.updateEarliestAvailableSlot(t.Context(), 40)
	require.Equal(t, 0, len(db.easUpdates))
	require.Equal(t, 0, len(custody.attempts))
}

func TestUpdateEarliestAvailableSlotIndependentUpdates(t *testing.T) {
	require.Equal(t, true, params.FuluEnabled())
	t.Run("db error still updates p2p", func(t *testing.T) {
		db := &mockBackfillDB{}
		dbAttempts := make([]primitives.Slot, 0)
		db.updateEarliestAvailableSlot = func(_ context.Context, sl primitives.Slot) error {
			dbAttempts = append(dbAttempts, sl)
			return errors.New("db failure")
		}
		custody := &mockEASCustody{eas: 100, cgc: 4}
		s := testEASService(db, custody, 100)

		s.updateEarliestAvailableSlot(t.Context(), 50)
		require.DeepEqual(t, []primitives.Slot{50}, dbAttempts)
		require.DeepEqual(t, []primitives.Slot{50}, custody.attempts)
		require.Equal(t, primitives.Slot(50), custody.eas)
	})
	t.Run("p2p error still updates db", func(t *testing.T) {
		db := &mockBackfillDB{}
		custody := &mockEASCustody{eas: 100, cgc: 4, updateErr: errors.New("p2p failure")}
		s := testEASService(db, custody, 100)

		s.updateEarliestAvailableSlot(t.Context(), 50)
		require.DeepEqual(t, []primitives.Slot{50}, db.easUpdates)
		require.DeepEqual(t, []primitives.Slot{50}, custody.attempts)
		require.Equal(t, primitives.Slot(100), custody.eas)
	})
}

func TestConfigureEASUpdates(t *testing.T) {
	require.Equal(t, true, params.FuluEnabled())
	// Slot 512 is the start of epoch 16 with mainnet SLOTS_PER_EPOCH.
	const origin = 512
	cases := []struct {
		name    string
		custody *mockEASCustody
		allowed bool
	}{
		{
			name:    "eas at origin enables updates",
			custody: &mockEASCustody{eas: origin, cgc: 4},
			allowed: true,
		},
		{
			name:    "eas below origin enables updates",
			custody: &mockEASCustody{eas: 100, cgc: 4},
			allowed: true,
		},
		{
			name:    "eas within one epoch of origin enables updates",
			custody: &mockEASCustody{eas: origin + 63, cgc: 4},
			allowed: true,
		},
		{
			name:    "eas above origin disables updates",
			custody: &mockEASCustody{eas: origin + 64, cgc: 4},
			allowed: false,
		},
		{
			name:    "eas raised to head disables updates",
			custody: &mockEASCustody{eas: 10000, cgc: 4},
			allowed: false,
		},
		{
			name:    "custody group count error disables updates",
			custody: &mockEASCustody{eas: origin, cgc: 4, cgcErr: errors.New("cgc failure")},
			allowed: false,
		},
		{
			name:    "eas error disables updates",
			custody: &mockEASCustody{eas: origin, cgc: 4, easErr: errors.New("eas failure")},
			allowed: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := &mockBackfillDB{}
			s := testEASService(db, c.custody, origin)
			s.easAllowed = false
			s.easCustodyGroupCount = 0

			s.configureEASUpdates(t.Context())
			require.Equal(t, c.allowed, s.easAllowed)
			if c.allowed {
				require.Equal(t, c.custody.cgc, s.easCustodyGroupCount)
			}
		})
	}
}

func TestConfigureEASUpdatesFuluDisabled(t *testing.T) {
	disableFulu(t)
	db := &mockBackfillDB{}
	custody := &mockEASCustody{eas: 100, cgc: 4}
	s := testEASService(db, custody, 100)
	s.easAllowed = false

	s.configureEASUpdates(t.Context())
	require.Equal(t, false, s.easAllowed)
}

// TestConfigureEASUpdatesPreservesRaisedEAS covers the custody-coverage guard end to end: an
// earliest available slot far above the backfill origin (eg. raised to head by a custody group
// count increase in a previous run) must not be overwritten by subsequent batch imports.
func TestConfigureEASUpdatesPreservesRaisedEAS(t *testing.T) {
	require.Equal(t, true, params.FuluEnabled())
	db := &mockBackfillDB{}
	custody := &mockEASCustody{eas: 10000, cgc: 4}
	s := testEASService(db, custody, 512)
	s.easAllowed = false

	s.configureEASUpdates(t.Context())
	require.Equal(t, false, s.easAllowed)

	s.updateEarliestAvailableSlot(t.Context(), 50)
	require.Equal(t, 0, len(db.easUpdates))
	require.Equal(t, 0, len(custody.attempts))
	require.Equal(t, primitives.Slot(10000), custody.eas)
}

// easImportFixture assembles the pieces needed to drive Service.importBatches directly with two
// importable batches of bounds [448, 512) and [384, 448).
type easImportFixture struct {
	svc     *Service
	db      *mockBackfillDB
	custody *mockEASCustody
	batches []batch
}

func newEASImportFixture(t *testing.T) *easImportFixture {
	require.Equal(t, true, params.FuluEnabled())
	const high = 512
	db := &mockBackfillDB{}
	custody := &mockEASCustody{eas: high, cgc: 4}
	clock := startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(high+1))
	sn, err := das.NewSyncNeeds(clock.CurrentSlot, nil, 0)
	require.NoError(t, err)
	seq := newBatchSequencer(2, high, 64, sn.Currently)
	batches, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 2, len(batches))
	for i := range batches {
		blk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, batches[i].begin, 0)
		batches[i].blocks = verifiedROBlocks{blk}
		batches[i].state = batchImportable
		seq.update(batches[i])
	}
	s := testEASService(db, custody, high)
	s.clock = clock
	s.batchSeq = seq
	return &easImportFixture{svc: s, db: db, custody: custody, batches: batches}
}

func TestImportBatchesUpdatesEAS(t *testing.T) {
	fx := newEASImportFixture(t)
	// Return a LowSlot above the requested batch begin to simulate skipped slots at the low end
	// of the batch: the updates must use the importer-returned value, not the batch boundary.
	fx.svc.batchImporter = func(_ context.Context, _ primitives.Slot, b batch, _ *Store) (*dbval.BackfillStatus, error) {
		return &dbval.BackfillStatus{LowSlot: uint64(b.begin) + 3}, nil
	}

	fx.svc.importBatches(t.Context())
	want := []primitives.Slot{451, 387}
	require.DeepEqual(t, want, fx.custody.attempts)
	require.DeepEqual(t, want, fx.db.easUpdates)
	require.Equal(t, primitives.Slot(387), fx.custody.eas)
	require.NotEqual(t, fx.batches[0].begin, fx.custody.attempts[0])
}

func TestImportBatchesImporterFailureNoEASUpdate(t *testing.T) {
	fx := newEASImportFixture(t)
	fx.svc.batchImporter = func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
		return nil, errors.New("import failure")
	}

	fx.svc.importBatches(t.Context())
	require.Equal(t, 0, len(fx.custody.attempts))
	require.Equal(t, 0, len(fx.db.easUpdates))
	require.Equal(t, primitives.Slot(512), fx.custody.eas)
}

// TestImportBatchesEASUpdaterErrorsDoNotFailImport asserts that failures in both earliest
// available slot updaters leave the already-committed batches imported: the import loop keeps
// going, the sequencer advances, and no batch is re-imported or marked as errored.
func TestImportBatchesEASUpdaterErrorsDoNotFailImport(t *testing.T) {
	fx := newEASImportFixture(t)
	dbAttempts := make([]primitives.Slot, 0)
	fx.db.updateEarliestAvailableSlot = func(_ context.Context, sl primitives.Slot) error {
		dbAttempts = append(dbAttempts, sl)
		return errors.New("db failure")
	}
	fx.custody.updateErr = errors.New("p2p failure")
	importerCalls := 0
	fx.svc.batchImporter = func(_ context.Context, _ primitives.Slot, b batch, _ *Store) (*dbval.BackfillStatus, error) {
		importerCalls++
		return &dbval.BackfillStatus{LowSlot: uint64(b.begin)}, nil
	}

	fx.svc.importBatches(t.Context())
	require.Equal(t, 2, importerCalls)
	// Both updaters were attempted for every imported batch despite the failures.
	want := []primitives.Slot{448, 384}
	require.DeepEqual(t, want, dbAttempts)
	require.DeepEqual(t, want, fx.custody.attempts)
	// The batches stay imported: nothing importable or errored remains, and a second pass does
	// not re-import them.
	require.Equal(t, 0, len(fx.svc.batchSeq.importable()))
	require.Equal(t, 0, fx.svc.batchSeq.countWithState(batchErrRetryable))
	require.Equal(t, 0, fx.svc.batchSeq.countWithState(batchErrFatal))
	fx.svc.importBatches(t.Context())
	require.Equal(t, 2, importerCalls)
}
