package mvslice

import (
	"fmt"
	"math/rand"
	"sync"
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
	s.Init([]int{1, 2, 3})
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
	s.Init([]int{1, 2, 3})

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
	s.Init([]int{0})
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

func TestAppendAfterCopyAndDetach(t *testing.T) {
	for _, shared := range [][]int{nil, {10, 20}} {
		t.Run(fmt.Sprintf("shared=%d", len(shared)), func(t *testing.T) {
			s := &Slice[int]{}
			s.Init(shared)
			a, b, c := &testObject{id: 1}, &testObject{id: 2}, &testObject{id: 3}
			s.Append(a, 30)
			s.Copy(a, b)
			s.Append(a, 40)
			s.Append(a, 50)
			s.Append(b, 40)
			s.Append(b, 60)
			s.Copy(b, c)
			s.Detach(b)
			s.Append(c, 70)
			s.Append(b, 80)
			require.DeepEqual(t, append(append([]int{}, shared...), 30, 40, 50), s.Value(a))
			require.DeepEqual(t, append(append([]int{}, shared...), 80), s.Value(b))
			require.DeepEqual(t, append(append([]int{}, shared...), 30, 40, 60, 70), s.Value(c))
			s.Detach(a)
			s.Detach(b)
			s.Detach(c)
			// Reuse empty appended positions left by detached objects.
			for i := range 8 {
				s.Append(a, i)
				require.Equal(t, len(shared)+i+1, s.Len(a))
				got, err := s.At(a, uint64(len(shared)+i))
				require.NoError(t, err)
				require.Equal(t, i, got)
			}
		})
	}
}

func TestAppendModel(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{10, 20})
	objects := make([]testObject, 8)
	want := make([][]int, len(objects))
	for i := range objects {
		objects[i].id = uint64(i + 1)
		want[i] = []int{10, 20}
	}
	rng := rand.New(rand.NewSource(42))
	for step := range 2000 {
		i := rng.Intn(len(objects))
		switch rng.Intn(5) {
		case 0:
			s.Detach(&objects[i])
			want[i] = []int{10, 20}
		case 1:
			j := (i + 1 + rng.Intn(len(objects)-1)) % len(objects)
			s.Detach(&objects[i])
			s.Copy(&objects[j], &objects[i])
			want[i] = append([]int{}, want[j]...)
		case 2:
			index := rng.Intn(len(want[i]))
			require.NoError(t, s.UpdateAt(&objects[i], uint64(index), step))
			want[i][index] = step
		default:
			value := rng.Intn(4)
			s.Append(&objects[i], value)
			want[i] = append(want[i], value)
		}
		for j := range objects {
			require.Equal(t, len(want[j]), s.Len(&objects[j]))
			require.DeepEqual(t, want[j], s.Value(&objects[j]))
		}
	}
}

func TestAppendConcurrentCopies(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{10, 20})
	original := &testObject{id: 0}
	s.Append(original, 30)
	objects := make([]testObject, 8)
	var wg sync.WaitGroup
	for i := range objects {
		objects[i].id = uint64(i + 1)
		s.Copy(original, &objects[i])
		wg.Go(func() {
			for j := range 512 {
				s.Append(&objects[i], j+int(objects[i].id)%2)
			}
		})
	}
	wg.Wait()
	require.DeepEqual(t, []int{10, 20, 30}, s.Value(original))
	for i := range objects {
		want := []int{10, 20, 30}
		for j := range 512 {
			want = append(want, j+int(objects[i].id)%2)
		}
		require.Equal(t, len(want), s.Len(&objects[i]))
		require.DeepEqual(t, want, s.Value(&objects[i]))
	}
}

func BenchmarkAppendScaling(b *testing.B) {
	for _, count := range []int{1000, 10000, 50000} {
		b.Run(fmt.Sprintf("n=%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				s := &Slice[[121]byte]{}
				s.Init(nil)
				obj := &testObject{id: 1}
				for i := range count {
					var value [121]byte
					value[0] = byte(i)
					s.Append(obj, value)
				}
				if s.Len(obj) != count {
					b.Fatal("incorrect appended length")
				}
			}
		})
	}
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

func TestReset(t *testing.T) {
	s := setup()
	obj := &testObject{id: 2}

	reset := s.Reset(obj)

	assert.Equal(t, 8, len(reset.sharedItems))
	assert.Equal(t, 123, reset.sharedItems[0])
	assert.Equal(t, 2, reset.sharedItems[1])
	assert.Equal(t, 3, reset.sharedItems[2])
	assert.Equal(t, 123, reset.sharedItems[3])
	assert.Equal(t, 2, reset.sharedItems[4])
	assert.Equal(t, 2, reset.sharedItems[5])
	assert.Equal(t, 3, reset.sharedItems[6])
	assert.Equal(t, 2, reset.sharedItems[7])
	assert.Equal(t, 0, len(reset.individualItems))
	assert.Equal(t, 0, len(reset.appendedItems))
}

func TestFragmentation_IndividualReferences(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{123, 123, 123, 123, 123})
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

	numOfRefs := fragmentationLimit / 2
	for i := 3; i < numOfRefs; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}

	assert.Equal(t, false, s.IsFragmented())

	// Add more references to hit fragmentation limit. Id 1
	// has 2 references above.
	for i := numOfRefs; i < numOfRefs+3; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}
	assert.Equal(t, true, s.IsFragmented())
}

func TestFragmentation_AppendedReferences(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{123, 123, 123, 123, 123})
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
	}
	s.cachedLengths[1] = 7
	s.cachedLengths[2] = 8

	numOfRefs := fragmentationLimit / 2
	for i := 3; i < numOfRefs; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}

	assert.Equal(t, false, s.IsFragmented())

	// Add more references to hit fragmentation limit. Id 1
	// has 2 references above.
	for i := numOfRefs; i < numOfRefs+3; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}
	assert.Equal(t, true, s.IsFragmented())
}

func TestFragmentation_IndividualAndAppendedReferences(t *testing.T) {
	s := &Slice[int]{}
	s.Init([]int{123, 123, 123, 123, 123})
	s.individualItems[2] = &MultiValueItem[int]{
		Values: []*Value[int]{
			{
				val: 3,
				ids: []uint64{1, 2},
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
	}
	s.cachedLengths[1] = 7
	s.cachedLengths[2] = 8

	numOfRefs := fragmentationLimit / 2
	for i := 3; i < numOfRefs; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}

	assert.Equal(t, false, s.IsFragmented())

	// Add more references to hit fragmentation limit. Id 1
	// has 2 references above.
	for i := numOfRefs; i < numOfRefs+3; i++ {
		obj := &testObject{id: uint64(i)}
		s.Copy(&testObject{id: 1}, obj)
	}
	assert.Equal(t, true, s.IsFragmented())
}

func TestNil(t *testing.T) {
	obj := &testObject{}

	s := &Slice[int]{}
	s.Init(nil)
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
	s.Init([]int{123, 123, 123, 123, 123})
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

func BenchmarkValue(b *testing.B) {
	const _100k = 100000
	const _1m = 1000000
	const _10m = 10000000

	b.Run("100,000 shared items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k))
		for b.Loop() {
			s.Value(&testObject{})
		}
	})
	b.Run("100,000 equal individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _100k))
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
		s.Init(make([]int, _100k))
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
		s.Init(make([]int, _100k))
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
		s.Init(make([]int, _100k))
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
		s.Init(make([]int, _1m))
		for b.Loop() {
			s.Value(&testObject{})
		}
	})
	b.Run("1,000,000 equal individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _1m))
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
		s.Init(make([]int, _1m))
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
		s.Init(make([]int, _1m))
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
		s.Init(make([]int, _1m))
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
		s.Init(make([]int, _10m))
		for b.Loop() {
			s.Value(&testObject{})
		}
	})
	b.Run("10,000,000 equal individual items", func(b *testing.B) {
		s := &Slice[int]{}
		s.Init(make([]int, _10m))
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
		s.Init(make([]int, _10m))
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
		s.Init(make([]int, _10m))
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
		s.Init(make([]int, _10m))
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
