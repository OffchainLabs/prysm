package fifo

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

type testObject struct {
	key   string
	value int
}

func testKeyFunc(obj interface{}) (string, error) {
	if t, ok := obj.(*testObject); ok {
		return t.key, nil
	}
	return "", fmt.Errorf("not a testObject")
}

func TestFIFO_AddAndGetByKey(t *testing.T) {
	f := New(testKeyFunc)

	obj := &testObject{key: "test1", value: 42}
	if err := f.Add(obj); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	item, exists, err := f.GetByKey("test1")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if !exists {
		t.Fatal("Item should exist")
	}

	got := item.(*testObject)
	if got.value != 42 {
		t.Errorf("Expected value 42, got %d", got.value)
	}
}

func TestFIFO_AddIfNotPresent(t *testing.T) {
	f := New(testKeyFunc)

	obj1 := &testObject{key: "test1", value: 1}
	obj2 := &testObject{key: "test1", value: 2}

	if err := f.AddIfNotPresent(obj1); err != nil {
		t.Fatalf("AddIfNotPresent failed: %v", err)
	}
	if err := f.AddIfNotPresent(obj2); err != nil {
		t.Fatalf("AddIfNotPresent failed: %v", err)
	}

	item, exists, err := f.GetByKey("test1")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if !exists {
		t.Fatal("Item should exist")
	}

	got := item.(*testObject)
	if got.value != 1 {
		t.Errorf("Expected value 1 (first item), got %d", got.value)
	}
}

func TestFIFO_Delete(t *testing.T) {
	f := New(testKeyFunc)

	obj := &testObject{key: "test1", value: 42}
	if err := f.Add(obj); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if err := f.Delete(obj); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, exists, err := f.GetByKey("test1")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if exists {
		t.Fatal("Item should not exist after delete")
	}
}

func TestFIFO_ListKeys(t *testing.T) {
	f := New(testKeyFunc)

	for i := 0; i < 5; i++ {
		obj := &testObject{key: fmt.Sprintf("key%d", i), value: i}
		if err := f.Add(obj); err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}

	keys := f.ListKeys()
	if len(keys) != 5 {
		t.Errorf("Expected 5 keys, got %d", len(keys))
	}
}

func TestFIFO_Pop(t *testing.T) {
	f := New(testKeyFunc)

	// Add items in order
	for i := 0; i < 3; i++ {
		obj := &testObject{key: fmt.Sprintf("key%d", i), value: i}
		if err := f.Add(obj); err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}

	// Pop should return in FIFO order
	for i := 0; i < 3; i++ {
		item, err := f.Pop(func(_ interface{}, _ bool) error { return nil })
		if err != nil {
			t.Fatalf("Pop failed: %v", err)
		}
		got := item.(*testObject)
		if got.value != i {
			t.Errorf("Expected value %d, got %d", i, got.value)
		}
	}

	if f.Len() != 0 {
		t.Errorf("Expected empty FIFO, got %d items", f.Len())
	}
}

func TestFIFO_PopBlocks(t *testing.T) {
	f := New(testKeyFunc)

	done := make(chan bool)
	go func() {
		obj := &testObject{key: "test1", value: 42}
		time.Sleep(50 * time.Millisecond)
		f.Add(obj)
	}()

	go func() {
		item, err := f.Pop(func(_ interface{}, _ bool) error { return nil })
		if err != nil {
			t.Errorf("Pop failed: %v", err)
		}
		got := item.(*testObject)
		if got.value != 42 {
			t.Errorf("Expected value 42, got %d", got.value)
		}
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Pop did not unblock after Add")
	}
}

func TestFIFO_Close(t *testing.T) {
	f := New(testKeyFunc)
	f.Close()

	if !f.IsClosed() {
		t.Fatal("FIFO should be closed")
	}

	obj := &testObject{key: "test1", value: 42}
	if err := f.Add(obj); err != ErrClosed {
		t.Errorf("Expected ErrClosed, got %v", err)
	}
}

func TestFIFO_Update(t *testing.T) {
	f := New(testKeyFunc)

	obj1 := &testObject{key: "test1", value: 1}
	obj2 := &testObject{key: "test1", value: 2}

	if err := f.Add(obj1); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := f.Update(obj2); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	item, _, _ := f.GetByKey("test1")
	got := item.(*testObject)
	if got.value != 2 {
		t.Errorf("Expected updated value 2, got %d", got.value)
	}

	// Queue should still have only one entry
	if f.Len() != 1 {
		t.Errorf("Expected 1 item, got %d", f.Len())
	}
}

func TestFIFO_Replace(t *testing.T) {
	f := New(testKeyFunc)

	// Add initial items
	for i := 0; i < 3; i++ {
		f.Add(&testObject{key: fmt.Sprintf("old%d", i), value: i})
	}

	// Replace with new items
	newItems := []interface{}{
		&testObject{key: "new0", value: 100},
		&testObject{key: "new1", value: 101},
	}
	if err := f.Replace(newItems, ""); err != nil {
		t.Fatalf("Replace failed: %v", err)
	}

	if f.Len() != 2 {
		t.Errorf("Expected 2 items, got %d", f.Len())
	}

	_, exists, _ := f.GetByKey("old0")
	if exists {
		t.Error("Old item should not exist")
	}

	_, exists, _ = f.GetByKey("new0")
	if !exists {
		t.Error("New item should exist")
	}
}

func TestFIFO_ConcurrentAccess(t *testing.T) {
	f := New(testKeyFunc)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			obj := &testObject{key: fmt.Sprintf("key%d", n), value: n}
			f.Add(obj)
		}(i)
	}

	wg.Wait()

	if f.Len() != 100 {
		t.Errorf("Expected 100 items, got %d", f.Len())
	}
}

func TestFIFO_List(t *testing.T) {
	f := New(testKeyFunc)

	for i := 0; i < 3; i++ {
		f.Add(&testObject{key: fmt.Sprintf("key%d", i), value: i})
	}

	items := f.List()
	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}
}
