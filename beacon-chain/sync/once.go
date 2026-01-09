package sync

import "sync"

// oncePerDigest represents an action that should only be performed once per fork digest.
type oncePerDigest uint8

const (
	doneZero             oncePerDigest = 0
	registerGossipOnce   oncePerDigest = 1 << 0
	unregisterGossipOnce oncePerDigest = 1 << 1
	registerRpcOnce      oncePerDigest = 1 << 2
	unregisterRpcOnce    oncePerDigest = 1 << 3
)

// perDigestSet keeps track of which oncePerDigest actions
// have been performed for each fork digest.
type perDigestSet struct {
	sync.Mutex
	history map[[4]byte]oncePerDigest
}

// digestActionDone marks the action as done for the given digest, returning true if it was already done.
func (s *Service) digestActionDone(digest [4]byte, action oncePerDigest) bool {
	s.digestActions.Lock()
	defer s.digestActions.Unlock()
	// lazy initialize registrationHistory; the lock is not a reference type so it is ready to go
	if s.digestActions.history == nil {
		s.digestActions.history = make(map[[4]byte]oncePerDigest)
	}

	prev := s.digestActions.history[digest]
	// Return true if the bit was already set
	if prev&action != 0 {
		return true
	}

	s.digestActions.history[digest] = prev | action
	return false
}
