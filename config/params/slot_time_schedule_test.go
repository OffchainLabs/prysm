package params_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"gopkg.in/yaml.v3"
)

func TestSlotTimeSchedule_CurrentSlot(t *testing.T) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	tests := []struct {
		name string
		// Inputs
		sch     *params.SlotSchedule
		genesis time.Time
		// Want
		slot primitives.Slot
	}{
		{
			name: "single entry",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: 12 * time.Second,
				},
			},
			genesis: time.Now().Add(-1 * 33 * time.Duration(slotsPerEpoch) * 12 * time.Second), // Genesis was 33 epochs ago.
			slot:    slots.UnsafeEpochStart(33),
		},
		{
			name: "multiple entries",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: 12 * time.Second,
				}, {
					Epoch:        32,
					SlotDuration: 10 * time.Second,
				}, {
					Epoch:        64,
					SlotDuration: 1 * time.Second,
				},
			},
			genesis: func() time.Time {
				tm := time.Now()
				firstEpochDuration := 32 * time.Duration(params.BeaconConfig().SlotsPerEpoch) * 12 * time.Second
				tm = tm.Add(-1 * firstEpochDuration)
				oneEpochDuration := time.Duration(params.BeaconConfig().SlotsPerEpoch) * 10 * time.Second
				tm = tm.Add(-1 * oneEpochDuration)

				return tm
			}(),
			slot: slots.UnsafeEpochStart(33),
		},
		{
			name: "multiple entries, last entry",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: 12 * time.Second,
				}, {
					Epoch:        32,
					SlotDuration: 10 * time.Second,
				}, {
					Epoch:        64,
					SlotDuration: 1 * time.Second,
				},
			},
			genesis: func() time.Time {
				tm := time.Now()
				firstEpochDuration := 32 * time.Duration(params.BeaconConfig().SlotsPerEpoch) * 12 * time.Second
				tm = tm.Add(-1 * firstEpochDuration)
				secondEpochDuration := 32 * time.Duration(params.BeaconConfig().SlotsPerEpoch) * 10 * time.Second
				tm = tm.Add(-1 * secondEpochDuration)
				remaining := (100 - 64) * time.Duration(params.BeaconConfig().SlotsPerEpoch) * 1 * time.Second
				tm = tm.Add(-1 * remaining)

				return tm
			}(),
			slot: slots.UnsafeEpochStart(100),
		},
		// TODO: Unsorted.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.slot, tt.sch.CurrentSlot(tt.genesis))
		})
	}
}

func TestSlotTimeSchedule_SinceGenesis(t *testing.T) {
	tests := []struct {
		name string
		// Inputs
		sch  *params.SlotSchedule
		slot primitives.Slot
		// Want
		since time.Duration
		error bool
	}{
		{
			name: "single entry",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: 12 * time.Second,
				},
			},
			slot:  slots.UnsafeEpochStart(33),
			since: 12 * time.Second * 33 * time.Duration(params.BeaconConfig().SlotsPerEpoch),
		},
		{
			name: "single entry 1s",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: time.Second,
				},
			},
			slot:  16,
			since: 16 * time.Second,
		},
		{
			name: "multiple entries",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: 12 * time.Second,
				}, {
					Epoch:        32,
					SlotDuration: 10 * time.Second,
				}, {
					Epoch:        64,
					SlotDuration: 1 * time.Second,
				},
			},
			slot: slots.UnsafeEpochStart(33),
			since: func() time.Duration {
				firstEpochDuration := 32 * time.Duration(params.BeaconConfig().SlotsPerEpoch) * 12 * time.Second
				oneEpochDuration := time.Duration(params.BeaconConfig().SlotsPerEpoch) * 10 * time.Second
				return firstEpochDuration + oneEpochDuration
			}(),
		},
		{
			name: "multiple entries, last entry",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: 12 * time.Second,
				}, {
					Epoch:        32,
					SlotDuration: 10 * time.Second,
				}, {
					Epoch:        64,
					SlotDuration: 1 * time.Second,
				},
			},
			slot: slots.UnsafeEpochStart(100),
			since: func() time.Duration {
				firstEntryDuration := 32 * time.Duration(params.BeaconConfig().SlotsPerEpoch) * 12 * time.Second
				secondEntryDuration := 32 * time.Duration(params.BeaconConfig().SlotsPerEpoch) * 10 * time.Second
				remaining := (100 - 64) * time.Duration(params.BeaconConfig().SlotsPerEpoch) * 1 * time.Second
				return firstEntryDuration + secondEntryDuration + remaining
			}(),
		},
		{
			name: "overflow",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: 12 * time.Second,
				}, {
					Epoch:        64,
					SlotDuration: 1 * time.Second,
				},
			},
			slot:  math.MaxUint64,
			error: true,
		},
		// TODO: Unsorted.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.sch.SinceGenesis(tt.slot)
			if !tt.error {
				require.NoError(t, err)
			} else {
				require.Equal(t, true, err != nil, "did not get any error when one was expected")
			}
			require.Equal(t, tt.since, got)
		})
	}
}

func TestSlotTimeSchedule_SlotDuration(t *testing.T) {
	tests := []struct {
		name string
		// Inputs
		sch  *params.SlotSchedule
		slot primitives.Slot
		// Want
		duration time.Duration
	}{
		{
			name: "single entry",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: 12 * time.Second,
				},
			},
			slot:     slots.UnsafeEpochStart(33),
			duration: 12 * time.Second,
		},
		{
			name: "multiple entries",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: 12 * time.Second,
				}, {
					Epoch:        32,
					SlotDuration: 10 * time.Second,
				}, {
					Epoch:        64,
					SlotDuration: 1 * time.Second,
				},
			},
			slot:     slots.UnsafeEpochStart(33),
			duration: 10 * time.Second,
		},
		{
			name: "multiple entries, last entry",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: 12 * time.Second,
				}, {
					Epoch:        32,
					SlotDuration: 10 * time.Second,
				}, {
					Epoch:        64,
					SlotDuration: 1 * time.Second,
				},
			},
			slot:     slots.UnsafeEpochStart(100),
			duration: 1 * time.Second,
		},
		{
			name: "multiple entries, first entry",
			sch: &params.SlotSchedule{
				{
					Epoch:        0,
					SlotDuration: 12 * time.Second,
				}, {
					Epoch:        32,
					SlotDuration: 10 * time.Second,
				}, {
					Epoch:        64,
					SlotDuration: 1 * time.Second,
				},
			},
			slot:     slots.UnsafeEpochStart(3),
			duration: 12 * time.Second,
		},
		// TODO: Unsorted.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sch.SlotDuration(tt.slot)
			require.Equal(t, tt.duration, got)
		})
	}
}

func TestSlotTimeSchedule_UnmarshalsYAML(t *testing.T) {
	const input = `SLOT_SCHEDULE:
  - EPOCH: 0
    SECONDS_PER_SLOT: 12
  - EPOCH: 1234567890
    SECONDS_PER_SLOT: 6
  - EPOCH: 2345678900
    SECONDS_PER_SLOT: 2.5`

	expected := &params.SlotSchedule{
		{
			Epoch:        0,
			SlotDuration: 12 * time.Second,
		}, {
			Epoch:        1234567890,
			SlotDuration: 6 * time.Second,
		}, {
			Epoch:        2345678900,
			SlotDuration: 2*time.Second + 500*time.Millisecond,
		},
	}

	c := &struct {
		SlotTimeSchedule *params.SlotSchedule `yaml:"SLOT_SCHEDULE"`
	}{}

	require.NoError(t, yaml.Unmarshal([]byte(input), c))

	require.Equal(t, len(*expected), len(*c.SlotTimeSchedule), "Did not get the expected number of slot time entries")
	for i, e := range *expected {
		require.Equal(t, e.Epoch, (*c.SlotTimeSchedule)[i].Epoch)
		require.Equal(t, e.SlotDuration, (*c.SlotTimeSchedule)[i].SlotDuration)
	}
}

func TestSlotTimeSchedule_MarshalsYAML(t *testing.T) {
	const want = `- EPOCH: 0
  SECONDS_PER_SLOT: 12
- EPOCH: 12
  SECONDS_PER_SLOT: 6
`

	input := &params.SlotSchedule{
		{Epoch: 0, SlotDuration: 12 * time.Second},
		{Epoch: 12, SlotDuration: 6 * time.Second},
	}

	out, err := yaml.Marshal(input)
	require.NoError(t, err)
	require.Equal(t, want, string(out))
}

func TestSlotTimeSchedule_Length(t *testing.T) {
	tests := []struct {
		schedule *params.SlotSchedule
		want     int
	}{
		{
			schedule: nil,
			want:     0,
		},
		{
			schedule: &params.SlotSchedule{{}},
			want:     1,
		},
		{
			schedule: &params.SlotSchedule{{}, {}},
			want:     2,
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			require.Equal(t, tt.want, tt.schedule.Length())
		})
	}
}

func TestSlotSchedule_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		schedule  *params.SlotSchedule
		wantError string
	}{
		{
			name:      "nil schedule",
			schedule:  nil,
			wantError: "empty schedule",
		},
		{
			name:      "empty schedule",
			schedule:  &params.SlotSchedule{},
			wantError: "empty schedule",
		},
		{
			name: "valid single entry",
			schedule: &params.SlotSchedule{
				{Epoch: 0, SlotDuration: 12 * time.Second},
			},
			wantError: "",
		},
		{
			name: "first entry not epoch 0",
			schedule: &params.SlotSchedule{
				{Epoch: 1, SlotDuration: 12 * time.Second},
			},
			wantError: "first entry must start with epoch 0",
		},
		{
			name: "duplicate epochs",
			schedule: &params.SlotSchedule{
				{Epoch: 0, SlotDuration: 12 * time.Second},
				{Epoch: 1, SlotDuration: 10 * time.Second},
				{Epoch: 1, SlotDuration: 8 * time.Second}, // Duplicate
			},
			wantError: "duplicate epoch 1",
		},
		{
			name: "unsorted epochs",
			schedule: &params.SlotSchedule{
				{Epoch: 0, SlotDuration: 12 * time.Second},
				{Epoch: 2, SlotDuration: 10 * time.Second},
				{Epoch: 1, SlotDuration: 8 * time.Second}, // Out of order
			},
			wantError: "must be sorted by epoch",
		},
		{
			name: "duration too small",
			schedule: &params.SlotSchedule{
				{Epoch: 0, SlotDuration: 500 * time.Millisecond}, // Less than 1 second
			},
			wantError: "less than minimum 1 second",
		},
		{
			name: "zero duration",
			schedule: &params.SlotSchedule{
				{Epoch: 0, SlotDuration: 0},
			},
			wantError: "less than minimum 1 second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.schedule.IsValid()
			if tt.wantError == "" {
				require.NoError(t, err)
			} else {
				require.NotNil(t, err, "expected error but got nil")
				if err != nil && tt.wantError != "" {
					require.Equal(t, true, strings.Contains(err.Error(), tt.wantError),
						fmt.Sprintf("error message '%s' does not contain '%s'", err.Error(), tt.wantError))
				}
			}
		})
	}
}

func TestSlotSchedule_AlwaysSorted(t *testing.T) {
	// Test that schedule is automatically sorted during unmarshaling
	const input = `SLOT_SCHEDULE:
  - EPOCH: 100
    SECONDS_PER_SLOT: 6
  - EPOCH: 0
    SECONDS_PER_SLOT: 12
  - EPOCH: 50
    SECONDS_PER_SLOT: 10`

	c := &struct {
		SlotTimeSchedule *params.SlotSchedule `yaml:"SLOT_SCHEDULE"`
	}{}

	require.NoError(t, yaml.Unmarshal([]byte(input), c))
	
	// Verify the schedule is sorted despite being out of order in YAML
	require.Equal(t, 3, c.SlotTimeSchedule.Length())
	require.Equal(t, primitives.Epoch(0), (*c.SlotTimeSchedule)[0].Epoch)
	require.Equal(t, primitives.Epoch(50), (*c.SlotTimeSchedule)[1].Epoch)
	require.Equal(t, primitives.Epoch(100), (*c.SlotTimeSchedule)[2].Epoch)
	
	// Verify operations work correctly without additional sorting
	// These should not panic or give wrong results
	slot := c.SlotTimeSchedule.CurrentSlot(time.Now().Add(-24 * time.Hour))
	_ = slot // slot is always >= 0 for primitives.Slot type
	
	// Calculate slots based on slots per epoch (32 for mainnet)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	
	// Slot in epoch 10 (< 50)
	duration := c.SlotTimeSchedule.SlotDuration(10 * slotsPerEpoch)
	require.Equal(t, 12*time.Second, duration, "should use epoch 0 duration")
	
	// Slot in epoch 75 (between 50 and 100)
	duration = c.SlotTimeSchedule.SlotDuration(75 * slotsPerEpoch)
	require.Equal(t, 10*time.Second, duration, "should use epoch 50 duration")
	
	// Slot in epoch 150 (> 100)
	duration = c.SlotTimeSchedule.SlotDuration(150 * slotsPerEpoch)
	require.Equal(t, 6*time.Second, duration, "should use epoch 100 duration")
}

func TestSlotTimeSchedule_SlotAt(t *testing.T) {
	// Test SlotAt with variable slot durations
	schedule := &params.SlotSchedule{
		{Epoch: 0, SlotDuration: 10 * time.Second}, // Epochs 0-15: 10s slots
		{Epoch: 16, SlotDuration: 4 * time.Second}, // Epochs 16+: 4s slots
	}

	genesis := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		tm   time.Time
		want primitives.Slot
	}{
		{
			name: "slot 0",
			tm:   genesis,
			want: 0,
		},
		{
			name: "slot 95 (last slot of epoch 15)",
			tm:   genesis.Add(95 * 10 * time.Second),
			want: 95,
		},
		{
			name: "slot 96 (first slot of epoch 16, 4s duration)",
			tm:   genesis.Add(96 * 10 * time.Second), // 16 epochs * 6 slots * 10s = 960s
			want: 96,
		},
		{
			name: "slot past epoch boundary - tests epoch diff calculation",
			// 16 epochs at 10s/slot = 16*32*10 = 5120s
			// Plus 10 more slots at 4s/slot = 40s
			// Total: 5160s, which should be slot 16*32 + 10 = 522
			tm:   genesis.Add(5160 * time.Second),
			want: 522,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schedule.SlotAt(genesis, tt.tm)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSlotSchedule_Immutability(t *testing.T) {
	// Test that NewSlotSchedule creates an immutable schedule
	entries := []params.SlotScheduleEntry{
		{Epoch: 100, SlotDuration: 6 * time.Second},  // Out of order
		{Epoch: 0, SlotDuration: 12 * time.Second},
		{Epoch: 50, SlotDuration: 10 * time.Second},
	}

	schedule, err := params.NewSlotSchedule(entries)
	require.NoError(t, err)
	require.NotNil(t, schedule)

	// Verify the schedule is sorted
	require.Equal(t, 3, schedule.Length())
	require.Equal(t, primitives.Epoch(0), (*schedule)[0].Epoch)
	require.Equal(t, primitives.Epoch(50), (*schedule)[1].Epoch)
	require.Equal(t, primitives.Epoch(100), (*schedule)[2].Epoch)

	// Original entries should not be modified
	require.Equal(t, primitives.Epoch(100), entries[0].Epoch)
	require.Equal(t, primitives.Epoch(0), entries[1].Epoch)
	require.Equal(t, primitives.Epoch(50), entries[2].Epoch)
}

func TestSlotSchedule_ThreadSafety(t *testing.T) {
	// Create a schedule that will be accessed concurrently
	schedule := &params.SlotSchedule{
		{Epoch: 0, SlotDuration: 12 * time.Second},
		{Epoch: 32, SlotDuration: 10 * time.Second},
		{Epoch: 64, SlotDuration: 6 * time.Second},
	}

	genesis := time.Now().Add(-24 * time.Hour)

	// Run multiple goroutines concurrently accessing the schedule
	// Since the schedule is immutable, no synchronization is needed
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(idx int) {
			// Each goroutine performs various read operations
			slot := primitives.Slot(idx * 32)
			
			// These operations are safe without locks because schedule is immutable
			_ = schedule.CurrentSlot(genesis)
			_ = schedule.SlotDuration(slot)
			_, _ = schedule.SinceGenesis(slot)
			_ = schedule.SlotAt(genesis, time.Now())
			_ = schedule.CurrentSlotDuration(genesis)
			
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestNewSlotSchedule_Validation(t *testing.T) {
	tests := []struct {
		name      string
		entries   []params.SlotScheduleEntry
		wantError string
	}{
		{
			name:      "empty schedule",
			entries:   []params.SlotScheduleEntry{},
			wantError: "empty schedule",
		},
		{
			name: "missing epoch 0",
			entries: []params.SlotScheduleEntry{
				{Epoch: 1, SlotDuration: 12 * time.Second},
			},
			wantError: "first entry must start with epoch 0",
		},
		{
			name: "duration too small",
			entries: []params.SlotScheduleEntry{
				{Epoch: 0, SlotDuration: 500 * time.Millisecond},
			},
			wantError: "less than minimum 1 second",
		},
		{
			name: "valid unsorted entries get sorted",
			entries: []params.SlotScheduleEntry{
				{Epoch: 50, SlotDuration: 10 * time.Second},
				{Epoch: 0, SlotDuration: 12 * time.Second},
			},
			wantError: "", // Should succeed after sorting
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule, err := params.NewSlotSchedule(tt.entries)
			if tt.wantError == "" {
				require.NoError(t, err)
				require.NotNil(t, schedule)
			} else {
				require.NotNil(t, err)
				require.Equal(t, true, strings.Contains(err.Error(), tt.wantError),
					fmt.Sprintf("error message '%s' does not contain '%s'", err.Error(), tt.wantError))
			}
		})
	}
}
