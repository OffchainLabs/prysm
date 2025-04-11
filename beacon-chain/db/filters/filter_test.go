package filters

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestQueryFilter_ChainsCorrectly(t *testing.T) {
	f := NewFilter().
		SetStartSlot(2).
		SetEndSlot(4).
		SetParentRoot([]byte{3, 4, 5})

	filterSet := f.Filters()
	assert.Equal(t, 3, len(filterSet), "Unexpected number of filters")
	for k, v := range filterSet {
		switch k {
		case StartSlot:
			t.Log(v.(primitives.Slot))
		case EndSlot:
			t.Log(v.(primitives.Slot))
		case ParentRoot:
			t.Log(v.([]byte))
		default:
			t.Log("Unknown filter type")
		}
	}
}

func TestSimpleSlotRange(t *testing.T) {
	type tc struct {
		name        string
		applFilters []func(*QueryFilter) *QueryFilter
		isSimple    bool
		start       primitives.Slot
		end         primitives.Slot
	}
	cases := []tc{
		{
			name:        "no filters",
			applFilters: []func(*QueryFilter) *QueryFilter{},
			isSimple:    false,
		},
		{
			name: "start slot",
			applFilters: []func(*QueryFilter) *QueryFilter{
				func(f *QueryFilter) *QueryFilter {
					return f.SetStartSlot(3)
				},
			},
			isSimple: false,
		},
		{
			name: "end slot",
			applFilters: []func(*QueryFilter) *QueryFilter{
				func(f *QueryFilter) *QueryFilter {
					return f.SetEndSlot(3)
				},
			},
			isSimple: false,
		},
		{
			name: "end slot",
			applFilters: []func(*QueryFilter) *QueryFilter{
				func(f *QueryFilter) *QueryFilter {
					return f.SetStartSlot(3)
				},
				func(f *QueryFilter) *QueryFilter {
					return f.SetEndSlot(7)
				},
			},
			start:    3,
			end:      7,
			isSimple: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := NewFilter()
			for _, filt := range c.applFilters {
				f = filt(f)
			}
			start, end, isSimple := f.SimpleSlotRange()
			require.Equal(t, c.isSimple, isSimple)
			require.Equal(t, c.start, start)
			require.Equal(t, c.end, end)
		})
	}
}
