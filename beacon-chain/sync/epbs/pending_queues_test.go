package epbs

import (
	"testing"

	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

// TestAdd tests the Add method of the PayloadPendingQueue.
func TestAdd(t *testing.T) {
	q := NewPayloadPendingQueue()
	root := [32]byte{}
	payload := &enginev1.SignedExecutionPayloadEnvelope{
		Message: &enginev1.ExecutionPayloadEnvelope{
			BeaconBlockRoot: root[:],
			Slot:            1,
		},
	}
	q.Add(payload)
	require.Equal(t, q.Get([32]byte(payload.Message.BeaconBlockRoot)), payload)
}

// TestRemove tests the Remove method of the PayloadPendingQueue.
func TestRemove(t *testing.T) {
	q := NewPayloadPendingQueue()
	root := [32]byte{}
	payload := &enginev1.SignedExecutionPayloadEnvelope{
		Message: &enginev1.ExecutionPayloadEnvelope{
			BeaconBlockRoot: root[:],
			Slot:            1,
		},
	}
	q.Add(payload)
	require.NotNil(t, q.Get([32]byte(payload.Message.BeaconBlockRoot)))
	q.Remove([32]byte(payload.Message.BeaconBlockRoot))
	require.IsNil(t, q.Get([32]byte(payload.Message.BeaconBlockRoot)))
}

// TestGet tests the Get method of the PayloadPendingQueue.
func TestGet(t *testing.T) {
	q := NewPayloadPendingQueue()
	root := [32]byte{}
	payload := &enginev1.SignedExecutionPayloadEnvelope{
		Message: &enginev1.ExecutionPayloadEnvelope{
			BeaconBlockRoot: root[:],
			Slot:            1,
		},
	}
	q.Add(payload)
	require.Equal(t, q.Get([32]byte(payload.Message.BeaconBlockRoot)), payload)
}

// TestPrune tests the Prune method of the PayloadPendingQueue.
func TestPrune(t *testing.T) {
	q := NewPayloadPendingQueue()
	root1 := [32]byte{}
	root2 := [32]byte{'a'}
	payload1 := &enginev1.SignedExecutionPayloadEnvelope{
		Message: &enginev1.ExecutionPayloadEnvelope{
			BeaconBlockRoot: root1[:],
			Slot:            1,
		},
	}
	payload2 := &enginev1.SignedExecutionPayloadEnvelope{
		Message: &enginev1.ExecutionPayloadEnvelope{
			BeaconBlockRoot: root2[:],
			Slot:            2,
		},
	}
	q.Add(payload1)
	q.Add(payload2)
	q.Prune(2)
	require.IsNil(t, q.Get([32]byte(payload1.Message.BeaconBlockRoot)))
	require.Equal(t, payload2, q.Get([32]byte(payload2.Message.BeaconBlockRoot)))
}
