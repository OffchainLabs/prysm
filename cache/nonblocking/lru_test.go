// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package nonblocking

import (
	"context"
	"testing"
	"time"
)

func TestLRU_Concurrency(t *testing.T) {
	onEvicted := func(_ int, _ int) {}
	size := 20
	cache, err := NewLRU(size, onEvicted)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()
	for i := 0; i < 100; i++ {
		go func(j int) {
			for {
				if ctx.Err() != nil {
					return
				}
				cache.Add(j, j)
				cache.Get(j)
				time.Sleep(time.Millisecond * 50)
			}
		}(i)
	}
	<-ctx.Done()
}

func TestLRU_Eviction(t *testing.T) {
	evictCounter := 0
	onEvicted := func(_ int, _ int) {
		evictCounter++
	}
	size := 20
	cache, err := NewLRU(size, onEvicted)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for i := 0; i < 20; i++ {
		cache.Add(i, i)
		cache.Get(i)
	}
	cache.Add(20, 20)
	if evictCounter != 1 {
		t.Fatalf("should have evicted 1 element: %d", evictCounter)
	}
}

// Test that Add returns true/false if an eviction occurred
func TestLRU_Add(t *testing.T) {
	evictCounter := 0
	onEvicted := func(_ int, _ int) {
		evictCounter++
	}
	l, err := NewLRU(1, onEvicted)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if l.Add(1, 1) == true || evictCounter != 0 {
		t.Errorf("should not have an eviction")
	}
	if l.Add(2, 2) == false || evictCounter != 1 {
		t.Errorf("should have an eviction")
	}
}

// Test that Resize can upsize and downsize
func TestLRU_Resize(t *testing.T) {
	onEvictCounter := 0
	onEvicted := func(k int, v int) {
		onEvictCounter++
	}
	l, err := NewLRU(2, onEvicted)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Downsize
	l.Add(1, 1)
	l.Add(2, 2)
	evicted := l.Resize(1)
	if evicted != 1 {
		t.Errorf("1 element should have been evicted: %v", evicted)
	}
	if onEvictCounter != 1 {
		t.Errorf("onEvicted should have been called 1 time: %v", onEvictCounter)
	}

	l.Add(3, 3)
	if _, ok := l.Get(1); ok {
		t.Errorf("Element 1 should have been evicted")
	}

	// Upsize
	evicted = l.Resize(2)
	if evicted != 0 {
		t.Errorf("0 elements should have been evicted: %v", evicted)
	}

	l.Add(4, 4)
	if _, ok := l.Get(3); !ok {
		t.Errorf("Cache should have contained 2 elements")
	}
	if _, ok := l.Get(4); !ok {
		t.Errorf("Cache should have contained 2 elements")
	}
}
