package sync

import (
	"fmt"
	"slices"
	"testing"
)

func TestDigestActionDone(t *testing.T) {
	digests := [][4]byte{
		{0, 0, 0, 0},
		{1, 2, 3, 4},
		{4, 3, 2, 1},
	}
	actions := []oncePerDigest{
		registerGossipOnce,
		unregisterGossipOnce,
		registerRpcOnce,
		unregisterRpcOnce,
	}
	testCombos := func(d [][4]byte, a []oncePerDigest) {
		s := &Service{}
		for _, digest := range d {
			for _, action := range a {
				t.Run(fmt.Sprintf("digest=%#x/action=%d", digest, action), func(t *testing.T) {
					if s.digestActionDone(digest, action) {
						t.Fatal("expected first call to return false")
					}
					if !s.digestActionDone(digest, action) {
						t.Fatal("expected second call to return true")
					}
				})
			}
		}
	}
	testCombos(digests, actions)
	slices.Reverse(digests)
	slices.Reverse(actions)
	testCombos(digests, actions)
}
