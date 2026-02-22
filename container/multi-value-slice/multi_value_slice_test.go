package mvslice

import (
	"math/rand"
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

type testObject struct {
	id uint64
}

func (o *testObject) Id() uint64 {
	return o.id
}

func (o *testObject) SetId(id uint64) {
	o.id = id
}

func TestLen(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{1, 2, 3}, 0)
	s.cachedLengths[1] = 123
	t.Run("cached", func(t *testing.T) {
		assert.Equal(t, 123, s.Len(&testObject{id: 1}))
	})
	t.Run("not cached", func(t *testing.T) {
		assert.Equal(t, 3, s.Len(&testObject{id: 999}))
	})
}

func TestCopy(t *testing.T) {
	// What we want to check:
	// - shared value is copied
	// - when the source object has an individual value, it is copied
	// - when the source object does not have an individual value, the shared value is copied
	// - when the source object has an appended value, it is copied
	// - when the source object does not have an appended value, nothing is copied
	// - length of destination object is cached

	s := setup()
	src := &testObject{id: 1}
	dst := &testObject{id: 999}

	s.Copy(src, dst)

	assert.Equal(t, (*MultiValueItem[int])(nil), s.individualItems[0])
	assertIndividualFound(t, s, dst.id, 1, 1)
	assertIndividualFound(t, s, dst.id, 2, 3)
	assertIndividualFound(t, s, dst.id, 3, 1)
	assertIndividualNotFound(t, s, dst.id, 4)
	assertAppendedFound(t, s, dst.id, 0, 1)
	assertAppendedFound(t, s, dst.id, 1, 3)
	assertAppendedNotFound(t, s, dst.id, 2)
	l, ok := s.cachedLengths[999]
	require.Equal(t, true, ok)
	assert.Equal(t, 7, l)
}

func TestValue(t *testing.T) {
	// What we want to check:
	// - correct values are returned for first object
	// - correct values are returned for second object
	// - correct values are returned for an object without appended items

	s := setup()
	first := &testObject{id: 1}
	second := &testObject{id: 2}

	v := s.Value(first)

	require.Equal(t, 7, len(v))
	assert.Equal(t, 123, v[0])
	assert.Equal(t, 1, v[1])
	assert.Equal(t, 3, v[2])
	assert.Equal(t, 1, v[3])
	assert.Equal(t, 123, v[4])
	assert.Equal(t, 1, v[5])
	assert.Equal(t, 3, v[6])

	v = s.Value(second)

	require.Equal(t, 8, len(v))
	assert.Equal(t, 123, v[0])
	assert.Equal(t, 2, v[1])
	assert.Equal(t, 3, v[2])
	assert.Equal(t, 123, v[3])
	assert.Equal(t, 2, v[4])
	assert.Equal(t, 2, v[5])
	assert.Equal(t, 3, v[6])
	assert.Equal(t, 2, v[7])

	s = &Slice[int]{}
	s.Init([]int{1, 2, 3}, 0)

	v = s.Value(&testObject{id: 999})

	require.Equal(t, 3, len(v))
	assert.Equal(t, 1, v[0])
	assert.Equal(t, 2, v[1])
	assert.Equal(t, 3, v[2])
}

func TestAt(t *testing.T) {
	// What we want to check:
	// - correct values are returned for first object
	// - correct values are returned for second object
	// - ERROR when index too large in general
	// - ERROR when index not too large in general, but too large for an object

	s := setup()
	first := &testObject{id: 1}
	second := &testObject{id: 2}

	v, err := s.At(first, 0)
	require.NoError(t, err)
	assert.Equal(t, 123, v)
	v, err = s.At(first, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, v)
	v, err = s.At(first, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, v)
	v, err = s.At(first, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, v)
	v, err = s.At(first, 4)
	require.NoError(t, err)
	assert.Equal(t, 123, v)
	v, err = s.At(first, 5)
	require.NoError(t, err)
	assert.Equal(t, 1, v)
	v, err = s.At(first, 6)
	require.NoError(t, err)
	assert.Equal(t, 3, v)
	_, err = s.At(first, 7)
	assert.ErrorContains(t, "index 7 out of bounds", err)

	v, err = s.At(second, 0)
	require.NoError(t, err)
	assert.Equal(t, 123, v)
	v, err = s.At(second, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, v)
	v, err = s.At(second, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, v)
	v, err = s.At(second, 3)
	require.NoError(t, err)
	assert.Equal(t, 123, v)
	v, err = s.At(second, 4)
	require.NoError(t, err)
	assert.Equal(t, 2, v)
	v, err = s.At(second, 5)
	require.NoError(t, err)
	assert.Equal(t, 2, v)
	v, err = s.At(second, 6)
	require.NoError(t, err)
	assert.Equal(t, 3, v)
	v, err = s.At(second, 7)
	require.NoError(t, err)
	assert.Equal(t, 2, v)
	_, err = s.At(second, 8)
	assert.ErrorContains(t, "index 8 out of bounds", err)
}

func TestUpdateAt(t *testing.T) {
	// What we want to check:
	// - shared value is updated only for the updated object, creating a new individual value (shared value remains the same)
	// - individual value (different for both objects) is updated to a third value
	// - individual value (different for both objects) is updated to the other object's value
	// - individual value (equal for both objects) is updated
	// - individual value existing only for the updated object is updated
	// - individual value existing only for the other-object appends an item to the individual value
	// - individual value updated to the original shared value removes that individual value
	// - appended value (different for both objects) is updated to a third value
	// - appended value (different for both objects) is updated to the other object's value
	// - appended value (equal for both objects) is updated
	// - appended value existing for one object is updated
	// - ERROR when index too large in general
	// - ERROR when index not too large in general, but too large for an object

	s := setup()
	first := &testObject{id: 1}
	second := &testObject{id: 2}

	require.NoError(t, s.UpdateAt(first, 0, 999))
	assert.Equal(t, 123, s.sharedItems[0])
	assertIndividualFound(t, s, first.id, 0, 999)
	assertIndividualNotFound(t, s, second.id, 0)

	require.NoError(t, s.UpdateAt(first, 1, 999))
	assertIndividualFound(t, s, first.id, 1, 999)
	assertIndividualFound(t, s, second.id, 1, 2)

	require.NoError(t, s.UpdateAt(first, 1, 2))
	assertIndividualFound(t, s, first.id, 1, 2)
	assertIndividualFound(t, s, second.id, 1, 2)

	require.NoError(t, s.UpdateAt(first, 2, 999))
	assertIndividualFound(t, s, first.id, 2, 999)
	assertIndividualFound(t, s, second.id, 2, 3)

	require.NoError(t, s.UpdateAt(first, 3, 999))
	assertIndividualFound(t, s, first.id, 3, 999)
	assertIndividualNotFound(t, s, second.id, 3)

	require.NoError(t, s.UpdateAt(first, 4, 999))
	assertIndividualFound(t, s, first.id, 4, 999)
	assertIndividualFound(t, s, second.id, 4, 2)

	require.NoError(t, s.UpdateAt(first, 4, 123))
	assertIndividualNotFound(t, s, first.id, 4)
	assertIndividualFound(t, s, second.id, 4, 2)

	require.NoError(t, s.UpdateAt(first, 5, 999))
	assertAppendedFound(t, s, first.id, 0, 999)
	assertAppendedFound(t, s, second.id, 0, 2)

	require.NoError(t, s.UpdateAt(first, 5, 2))
	assertAppendedFound(t, s, first.id, 0, 2)
	assertAppendedFound(t, s, second.id, 0, 2)

	require.NoError(t, s.UpdateAt(first, 6, 999))
	assertAppendedFound(t, s, first.id, 1, 999)
	assertAppendedFound(t, s, second.id, 1, 3)

	// we update the second object because there are no more appended items for the first object
	require.NoError(t, s.UpdateAt(second, 7, 999))
	assertAppendedNotFound(t, s, first.id, 2)
	assertAppendedFound(t, s, second.id, 2, 999)

	assert.ErrorContains(t, "index 7 out of bounds", s.UpdateAt(first, 7, 999))
	assert.ErrorContains(t, "index 8 out of bounds", s.UpdateAt(second, 8, 999))
}

func TestAppend(t *testing.T) {
	// What we want to check:
	// - appending first item ever to the slice
	// - appending an item to an object when there is no corresponding item for the other object
	// - appending an item to an object when there is a corresponding item with same value for the other object
	// - appending an item to an object when there is a corresponding item with different value for the other object
	// - we also want to check that cached length is properly updated after every append

	// we want to start with the simplest slice possible
	s := &Slice[int]{}
	s.Init([]int{0}, 0)
	first := &testObject{id: 1}
	second := &testObject{id: 2}

	// append first value ever
	s.Append(first, 1)
	require.Equal(t, 1, len(s.appendedItems))
	assertAppendedFound(t, s, first.id, 0, 1)
	assertAppendedNotFound(t, s, second.id, 0)
	l, ok := s.cachedLengths[first.id]
	require.Equal(t, true, ok)
	assert.Equal(t, 2, l)
	_, ok = s.cachedLengths[second.id]
	assert.Equal(t, false, ok)

	// append one more value to the first object, so that we can test two append scenarios for the second object
	s.Append(first, 1)

	// append the first value to the second object, equal to the value for the first object
	s.Append(second, 1)
	require.Equal(t, 2, len(s.appendedItems))
	assertAppendedFound(t, s, first.id, 0, 1)
	assertAppendedFound(t, s, second.id, 0, 1)
	l, ok = s.cachedLengths[first.id]
	require.Equal(t, true, ok)
	assert.Equal(t, 3, l)
	l, ok = s.cachedLengths[second.id]
	assert.Equal(t, true, ok)
	assert.Equal(t, 2, l)

	// append the first value to the second object, different than the value for the first object
	s.Append(second, 2)
	require.Equal(t, 2, len(s.appendedItems))
	assertAppendedFound(t, s, first.id, 1, 1)
	assertAppendedFound(t, s, second.id, 1, 2)
	l, ok = s.cachedLengths[first.id]
	require.Equal(t, true, ok)
	assert.Equal(t, 3, l)
	l, ok = s.cachedLengths[second.id]
	assert.Equal(t, true, ok)
	assert.Equal(t, 3, l)
}

func TestDetach(t *testing.T) {
	// What we want to check:
	// - no individual or appended items left after detaching an object
	// - length removed from cache

	s := setup()
	obj := &testObject{id: 1}

	s.Detach(obj)

	for _, item := range s.individualItems {
		found := false
		for _, v := range item.Values {
			for _, o := range v.ids {
				if o == obj.id {
					found = true
				}
			}
		}
		assert.Equal(t, false, found)
	}
	for _, item := range s.appendedItems {
		found := false
		for _, v := range item.Values {
			for _, o := range v.ids {
				if o == obj.id {
					found = true
				}
			}
		}
		assert.Equal(t, false, found)
	}
	_, ok := s.cachedLengths[obj.id]
	assert.Equal(t, false, ok)
}

func TestNil(t *testing.T) {
	obj := &testObject{}

	s := &Slice[int]{}
	s.Init(nil, 0)
	assert.Equal(t, 0, s.Len(obj))
	assert.DeepEqual(t, []int{}, s.Value(obj))
	_, err := s.At(obj, 0)
	assert.NotNil(t, err)
	s.Append(obj, 1)
	assert.Equal(t, 1, s.Len(obj))
	assert.DeepEqual(t, []int{1}, s.Value(obj))
}

// Share the slice between 2 objects.
// Index 0: Shared value
// Index 1: Different individual value
// Index 2: Same individual value
// Index 3: Individual value ONLY for the first object
// Index 4: Individual value ONLY for the second object
// Index 5: Different appended value
// Index 6: Same appended value
// Index 7: Appended value ONLY for the second object
func setup() *Slice[int] {
	s := &Slice[int]{}
	s.Init([]int{123, 123, 123, 123, 123}, 0)
	s.individualItems[1] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 1,
				ids: []uint64{1},
			},
			{
				val: 2,
				ids: []uint64{2},
			},
		},
	}
	s.individualItems[2] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 3,
				ids: []uint64{1, 2},
			},
		},
	}
	s.individualItems[3] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 1,
				ids: []uint64{1},
			},
		},
	}
	s.individualItems[4] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 2,
				ids: []uint64{2},
			},
		},
	}
	s.appendedItems = []*MultiValueItem[int]{
		{
			Values: []*Value[int]{
				{
					val: 1,
					ids: []uint64{1},
				},
				{
					val: 2,
					ids: []uint64{2},
				},
			},
		},
		{
			Values: []*Value[int]{
				{
					val: 3,
					ids: []uint64{1, 2},
				},
			},
		},
		{
			Values: []*Value[int]{
				{
					val: 2,
					ids: []uint64{2},
				},
			},
		},
	}
	s.cachedLengths[1] = 7
	s.cachedLengths[2] = 8

	return s
}

func assertIndividualFound(t *testing.T, slice *Slice[int], id uint64, itemIndex uint64, expected int) {
	found := false
	for _, v := range slice.individualItems[itemIndex].Values {
		for _, o := range v.ids {
			if o == id {
				found = true
				assert.Equal(t, expected, v.val)
			}
		}
	}
	assert.Equal(t, true, found)
}

func assertIndividualNotFound(t *testing.T, slice *Slice[int], id uint64, itemIndex uint64) {
	found := false
	for _, v := range slice.individualItems[itemIndex].Values {
		for _, o := range v.ids {
			if o == id {
				found = true
			}
		}
	}
	assert.Equal(t, false, found)
}

func assertAppendedFound(t *testing.T, slice *Slice[int], id uint64, itemIndex uint64, expected int) {
	found := false
	for _, v := range slice.appendedItems[itemIndex].Values {
		for _, o := range v.ids {
			if o == id {
				found = true
				assert.Equal(t, expected, v.val)
			}
		}
	}
	assert.Equal(t, true, found)
}

func assertAppendedNotFound(t *testing.T, slice *Slice[int], id uint64, itemIndex uint64) {
	found := false
	for _, v := range slice.appendedItems[itemIndex].Values {
		for _, o := range v.ids {
			if o == id {
				found = true
			}
		}
	}
	assert.Equal(t, false, found)
}

func TestActiveIds_InitCopyDetach(t *testing.T) {
	o1 := &testObject{id: 10}
	o2 := &testObject{id: 20}

	// Init registers the owner ID.
	s := &Slice[int]{}
	s.Init([]int{1, 2, 3}, o1.id)
	_, ok := s.activeIds[10]
	assert.Equal(t, true, ok)

	// Copy adds both src and dst.
	s.Copy(o1, o2)
	_, ok = s.activeIds[10]
	assert.Equal(t, true, ok)
	_, ok = s.activeIds[20]
	assert.Equal(t, true, ok)

	// Detach removes ID.
	s.Detach(o2)
	_, ok = s.activeIds[20]
	assert.Equal(t, false, ok)
	// Owner still present.
	_, ok = s.activeIds[10]
	assert.Equal(t, true, ok)
}

func TestPromoteToHead_NoOp(t *testing.T) {
	// Head has no overrides - nothing should change.
	head := &testObject{id: 1}
	other := &testObject{id: 2}
	s := &Slice[int]{}
	s.Init([]int{10, 20, 30}, head.id)
	s.Copy(head, other)

	s.PromoteToHead(head)

	assert.DeepEqual(t, []int{10, 20, 30}, s.sharedItems)
	assert.Equal(t, 0, len(s.individualItems))
	assert.Equal(t, 0, len(s.appendedItems))
}

func TestPromoteToHead_Basic(t *testing.T) {
	// Head has overrides at indices 0 and 2. After promotion,
	// sharedItems should reflect head's values.
	head := &testObject{id: 1}
	s := &Slice[int]{}
	s.Init([]int{10, 20, 30}, head.id)

	require.NoError(t, s.UpdateAt(head, 0, 99))
	require.NoError(t, s.UpdateAt(head, 2, 77))

	s.PromoteToHead(head)

	// Shared items should now have head's values.
	assert.Equal(t, 99, s.sharedItems[0])
	assert.Equal(t, 20, s.sharedItems[1])
	assert.Equal(t, 77, s.sharedItems[2])

	// Head should have no individual items.
	for _, item := range s.individualItems {
		for _, v := range item.Values {
			assert.Equal(t, false, slices.Contains(v.ids, head.id))
		}
	}

	// Head should read correct values directly.
	v, err := s.At(head, 0)
	require.NoError(t, err)
	assert.Equal(t, 99, v)
	v, err = s.At(head, 1)
	require.NoError(t, err)
	assert.Equal(t, 20, v)
	v, err = s.At(head, 2)
	require.NoError(t, err)
	assert.Equal(t, 77, v)
}

func TestPromoteToHead_ReverseOverrides(t *testing.T) {
	// Two states share a slice. Head modifies index 0.
	// After promotion, other state should still read the old value.
	head := &testObject{id: 1}
	other := &testObject{id: 2}
	s := &Slice[int]{}
	s.Init([]int{10, 20, 30}, head.id)
	s.Copy(head, other)

	require.NoError(t, s.UpdateAt(head, 0, 99))

	// Before promotion: head reads 99, other reads 10.
	v, err := s.At(head, 0)
	require.NoError(t, err)
	assert.Equal(t, 99, v)
	v, err = s.At(other, 0)
	require.NoError(t, err)
	assert.Equal(t, 10, v)

	s.PromoteToHead(head)

	// After promotion: head still reads 99, other still reads 10.
	v, err = s.At(head, 0)
	require.NoError(t, err)
	assert.Equal(t, 99, v)
	v, err = s.At(other, 0)
	require.NoError(t, err)
	assert.Equal(t, 10, v)

	// sharedItems[0] should now be 99.
	assert.Equal(t, 99, s.sharedItems[0])

	// other should have a reverse-override at index 0.
	assertIndividualFound(t, s, other.id, 0, 10)
}

func TestPromoteToHead_MultipleStatesWithOverrides(t *testing.T) {
	// Three states: head, s2, s3.
	// Head overrides index 1. s2 overrides index 1 with a different value. s3 reads shared.
	head := &testObject{id: 1}
	s2 := &testObject{id: 2}
	s3 := &testObject{id: 3}
	s := &Slice[int]{}
	s.Init([]int{10, 20, 30}, head.id)
	s.Copy(head, s2)
	s.Copy(head, s3)

	require.NoError(t, s.UpdateAt(head, 1, 99))
	require.NoError(t, s.UpdateAt(s2, 1, 55))

	// Before promotion:
	// head reads 99, s2 reads 55, s3 reads 20 (shared).
	v, _ := s.At(head, 1)
	assert.Equal(t, 99, v)
	v, _ = s.At(s2, 1)
	assert.Equal(t, 55, v)
	v, _ = s.At(s3, 1)
	assert.Equal(t, 20, v)

	s.PromoteToHead(head)

	// After promotion: same values visible.
	v, _ = s.At(head, 1)
	assert.Equal(t, 99, v)
	v, _ = s.At(s2, 1)
	assert.Equal(t, 55, v)
	v, _ = s.At(s3, 1)
	assert.Equal(t, 20, v)

	// sharedItems[1] should be 99 now.
	assert.Equal(t, 99, s.sharedItems[1])
}

func TestPromoteToHead_AppendedItemsIgnored(t *testing.T) {
	// PromoteToHead only promotes individual overrides, not appended items.
	// Appended items stay in appendedItems.
	head := &testObject{id: 1}
	other := &testObject{id: 2}
	s := &Slice[int]{}
	s.Init([]int{10, 20}, head.id)
	s.Copy(head, other)

	s.Append(head, 100)
	s.Append(head, 200)

	// Head also overrides an individual item.
	require.NoError(t, s.UpdateAt(head, 0, 99))

	// Before promotion: head has length 4, other has length 2.
	assert.Equal(t, 4, s.Len(head))
	assert.Equal(t, 2, s.Len(other))

	s.PromoteToHead(head)

	// sharedItems should NOT include appended values (only individual overrides promoted).
	assert.Equal(t, 2, len(s.sharedItems))
	assert.Equal(t, 99, s.sharedItems[0]) // individual override promoted
	assert.Equal(t, 20, s.sharedItems[1]) // unchanged

	// Head still reads all values correctly.
	v, err := s.At(head, 0)
	require.NoError(t, err)
	assert.Equal(t, 99, v)
	v, err = s.At(head, 2)
	require.NoError(t, err)
	assert.Equal(t, 100, v)
	v, err = s.At(head, 3)
	require.NoError(t, err)
	assert.Equal(t, 200, v)

	// Other state reads correctly.
	v, err = s.At(other, 0)
	require.NoError(t, err)
	assert.Equal(t, 10, v) // reverse-override from promotion
}

func TestPromoteToHead_Correctness(t *testing.T) {
	// Comprehensive correctness: snapshot all values before promotion,
	// verify they match after promotion via At() and Value().
	s := setup()
	s.activeIds = map[uint64]struct{}{1: {}, 2: {}}

	first := &testObject{id: 1}
	second := &testObject{id: 2}

	// Snapshot values before promotion.
	valsBefore1 := s.Value(first)
	valsBefore2 := s.Value(second)

	s.PromoteToHead(first)

	// Verify all values match via Value().
	valsAfter1 := s.Value(first)
	valsAfter2 := s.Value(second)
	assert.DeepEqual(t, valsBefore1, valsAfter1)
	assert.DeepEqual(t, valsBefore2, valsAfter2)

	// Verify via At().
	for i, expected := range valsBefore1 {
		v, err := s.At(first, uint64(i))
		require.NoError(t, err)
		assert.Equal(t, expected, v)
	}
	for i, expected := range valsBefore2 {
		v, err := s.At(second, uint64(i))
		require.NoError(t, err)
		assert.Equal(t, expected, v)
	}

	// Verify via ForEach.
	idx := 0
	err := s.ForEach(first, func(i int, val *int) error {
		assert.Equal(t, valsBefore1[i], *val)
		idx++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, len(valsBefore1), idx)
}

func TestPromoteToHead_ThenCopy(t *testing.T) {
	// After promotion, copying the head should work correctly.
	head := &testObject{id: 1}
	s := &Slice[int]{}
	s.Init([]int{10, 20, 30}, head.id)

	require.NoError(t, s.UpdateAt(head, 0, 99))

	s.PromoteToHead(head)

	// Now copy head to a new state.
	newState := &testObject{id: 3}
	s.Copy(head, newState)

	// New state should read same values as head (from shared).
	v, err := s.At(newState, 0)
	require.NoError(t, err)
	assert.Equal(t, 99, v)
	v, err = s.At(newState, 1)
	require.NoError(t, err)
	assert.Equal(t, 20, v)
}

func TestPromoteToHead_ThenDetach(t *testing.T) {
	// After promotion, detaching non-head states cleans up reverse-overrides.
	head := &testObject{id: 1}
	other := &testObject{id: 2}
	s := &Slice[int]{}
	s.Init([]int{10, 20, 30}, head.id)
	s.Copy(head, other)

	require.NoError(t, s.UpdateAt(head, 0, 99))

	s.PromoteToHead(head)

	// other has a reverse-override at index 0.
	assertIndividualFound(t, s, other.id, 0, 10)

	// Detach other.
	s.Detach(other)

	// No individual items should remain (head has none, other was detached).
	assert.Equal(t, 0, len(s.individualItems))

	// Head still works.
	v, err := s.At(head, 0)
	require.NoError(t, err)
	assert.Equal(t, 99, v)
}

func TestPromoteToHead_AppendedWithBothStates(t *testing.T) {
	// Both head and other have appended items. PromoteToHead only
	// promotes individual overrides, appended items stay untouched.
	head := &testObject{id: 1}
	other := &testObject{id: 2}
	s := &Slice[int]{}
	s.Init([]int{10}, head.id)
	s.Copy(head, other)

	s.Append(head, 100)
	s.Append(other, 200)

	// Also add an individual override for head.
	require.NoError(t, s.UpdateAt(head, 0, 99))

	// Before: head sees [99, 100], other sees [10, 200].
	assert.DeepEqual(t, []int{99, 100}, s.Value(head))
	assert.DeepEqual(t, []int{10, 200}, s.Value(other))

	s.PromoteToHead(head)

	// sharedItems should reflect head's individual override only.
	assert.Equal(t, 1, len(s.sharedItems))
	assert.Equal(t, 99, s.sharedItems[0])

	// After: head still sees [99, 100].
	assert.DeepEqual(t, []int{99, 100}, s.Value(head))
	// Other still sees [10, 200].
	assert.DeepEqual(t, []int{10, 200}, s.Value(other))
}

func BenchmarkValue(b *testing.B) {
	const _100k = 100000
	const _1m = 1000000
	const _10m = 10000000

	b.Run("100,000 shared items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k), 0)
		for b.Loop() {
			s.Value(&testObject{})
		}
	})
	b.Run("100,000 equal individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k), 0)
		s.individualItems[0] = &MultiValueItem[int]{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}
		objs := make([]*testObject, _100k)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[0].Values[0].ids = append(s.individualItems[0].Values[0].ids, uint64(i))
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_100k)])
		}
	})
	b.Run("100,000 different individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k), 0)
		objs := make([]*testObject, _100k)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[uint64(i)] = &MultiValueItem[int]{Values: []*Value[int]{{val: i, ids: []uint64{uint64(i)}}}}
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_100k)])
		}
	})
	b.Run("100,000 shared items and 100,000 equal appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k), 0)
		s.appendedItems = []*MultiValueItem[int]{{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}}
		objs := make([]*testObject, _100k)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems[0].Values[0].ids = append(s.appendedItems[0].Values[0].ids, uint64(i))
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_100k)])
		}
	})
	b.Run("100,000 shared items and 100,000 different appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k), 0)
		s.appendedItems = []*MultiValueItem[int]{}
		objs := make([]*testObject, _100k)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems = append(s.appendedItems, &MultiValueItem[int]{Values: []*Value[int]{{val: i, ids: []uint64{uint64(i)}}}})
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_100k)])
		}
	})
	b.Run("1,000,000 shared items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _1m), 0)
		for b.Loop() {
			s.Value(&testObject{})
		}
	})
	b.Run("1,000,000 equal individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _1m), 0)
		s.individualItems[0] = &MultiValueItem[int]{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}
		objs := make([]*testObject, _1m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[0].Values[0].ids = append(s.individualItems[0].Values[0].ids, uint64(i))
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_1m)])
		}
	})
	b.Run("1,000,000 different individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _1m), 0)
		objs := make([]*testObject, _1m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[uint64(i)] = &MultiValueItem[int]{Values: []*Value[int]{{val: i, ids: []uint64{uint64(i)}}}}
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_1m)])
		}
	})
	b.Run("1,000,000 shared items and 1,000,000 equal appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _1m), 0)
		s.appendedItems = []*MultiValueItem[int]{{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}}
		objs := make([]*testObject, _1m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems[0].Values[0].ids = append(s.appendedItems[0].Values[0].ids, uint64(i))
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_1m)])
		}
	})
	b.Run("1,000,000 shared items and 1,000,000 different appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _1m), 0)
		s.appendedItems = []*MultiValueItem[int]{}
		objs := make([]*testObject, _1m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems = append(s.appendedItems, &MultiValueItem[int]{Values: []*Value[int]{{val: i, ids: []uint64{uint64(i)}}}})
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_1m)])
		}
	})
	b.Run("10,000,000 shared items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _10m), 0)
		for b.Loop() {
			s.Value(&testObject{})
		}
	})
	b.Run("10,000,000 equal individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _10m), 0)
		s.individualItems[0] = &MultiValueItem[int]{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}
		objs := make([]*testObject, _10m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[0].Values[0].ids = append(s.individualItems[0].Values[0].ids, uint64(i))
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_10m)])
		}
	})
	b.Run("10,000,000 different individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _10m), 0)
		objs := make([]*testObject, _10m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.individualItems[uint64(i)] = &MultiValueItem[int]{Values: []*Value[int]{{val: i, ids: []uint64{uint64(i)}}}}
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_10m)])
		}
	})
	b.Run("10,000,000 shared items and 10,000,000 equal appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _10m), 0)
		s.appendedItems = []*MultiValueItem[int]{{Values: []*Value[int]{{val: 999, ids: []uint64{}}}}}
		objs := make([]*testObject, _10m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems[0].Values[0].ids = append(s.appendedItems[0].Values[0].ids, uint64(i))
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_10m)])
		}
	})
	b.Run("10,000,000 shared items and 10,000,000 different appended items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _10m), 0)
		s.appendedItems = []*MultiValueItem[int]{}
		objs := make([]*testObject, _10m)
		for i := range objs {
			objs[i] = &testObject{id: uint64(i)}
			s.appendedItems = append(s.appendedItems, &MultiValueItem[int]{Values: []*Value[int]{{val: i, ids: []uint64{uint64(i)}}}})
		}
		for b.Loop() {
			s.Value(objs[rand.Intn(_10m)])
		}
	})
}
