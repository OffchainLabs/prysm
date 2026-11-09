package logs

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestStreamServer_BackfillsMessages(t *testing.T) {
	ss := NewStreamServer()
	msgs := [][]byte{
		[]byte("foo"),
		[]byte("bar"),
		[]byte("buzz"),
	}
	for _, msg := range msgs {
		_, err := ss.Write(msg)
		require.NoError(t, err)
	}

	recentMessages := ss.GetLastFewLogs()
	require.DeepEqual(t, msgs, recentMessages)
}

func TestStreamServer_Write(t *testing.T) {
	t.Run("does not block on a stalled subscriber", func(t *testing.T) {
		ss := NewStreamServer()

		stalled := make(chan []byte, 1)
		stalledSub := ss.SubscribeLogs(stalled)
		defer stalledSub.Unsubscribe()

		reading := make(chan []byte, 10)
		readingSub := ss.SubscribeLogs(reading)
		defer readingSub.Unsubscribe()

		done := make(chan struct{})
		go func() {
			defer close(done)
			for range 10 {
				_, err := ss.Write([]byte("line"))
				require.NoError(t, err)
			}
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Write blocked on a stalled subscriber")
		}

		// The stalled subscriber keeps only what fit in its buffer, the other one gets everything.
		require.Equal(t, 1, len(stalled))
		require.Equal(t, 10, len(reading))
	})

	t.Run("does not retain the input buffer", func(t *testing.T) {
		ss := NewStreamServer()

		ch := make(chan []byte, 1)
		sub := ss.SubscribeLogs(ch)
		defer sub.Unsubscribe()

		buf := []byte("original")
		_, err := ss.Write(buf)
		require.NoError(t, err)
		copy(buf, "modified")

		require.DeepEqual(t, []byte("original"), <-ch)
		require.DeepEqual(t, [][]byte{[]byte("original")}, ss.GetLastFewLogs())
	})
}

func TestStreamServer_SubscribeLogs(t *testing.T) {
	t.Run("stops delivery after unsubscribe", func(t *testing.T) {
		ss := NewStreamServer()

		ch := make(chan []byte, 10)
		sub := ss.SubscribeLogs(ch)

		_, err := ss.Write([]byte("before"))
		require.NoError(t, err)

		sub.Unsubscribe()
		// Closing the channel after Unsubscribe must be safe, as the stream handlers do it.
		close(ch)

		_, err = ss.Write([]byte("after"))
		require.NoError(t, err)

		require.DeepEqual(t, []byte("before"), <-ch)
		_, ok := <-ch
		require.Equal(t, false, ok)

		_, ok = <-sub.Err()
		require.Equal(t, false, ok)

		// Unsubscribing twice is a no-op.
		sub.Unsubscribe()
	})
}
