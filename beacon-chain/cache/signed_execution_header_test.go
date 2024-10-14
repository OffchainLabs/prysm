package cache

import (
	"testing"

	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func Test_SaveSignedExecutionPayloadHeader(t *testing.T) {
	t.Run("First header should be added to cache", func(t *testing.T) {
		c := NewExecutionPayloadHeaders()
		header := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            1,
				ParentBlockHash: []byte("parent1"),
				Value:           100,
			},
		}
		c.SaveSignedExecutionPayloadHeader(header)
		require.Equal(t, 1, len(c.headers))
		require.Equal(t, header, c.headers[1][0])
	})

	t.Run("Second header with higher slot should be added, and both slots should be in cache", func(t *testing.T) {
		c := NewExecutionPayloadHeaders()
		header1 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            1,
				ParentBlockHash: []byte("parent1"),
				Value:           100,
			},
		}
		header2 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            2,
				ParentBlockHash: []byte("parent2"),
				Value:           100,
			},
		}
		c.SaveSignedExecutionPayloadHeader(header1)
		c.SaveSignedExecutionPayloadHeader(header2)
		require.Equal(t, 2, len(c.headers))
		require.Equal(t, header1, c.headers[1][0])
		require.Equal(t, header2, c.headers[2][0])
	})

	t.Run("Third header with higher slot should replace the oldest slot", func(t *testing.T) {
		c := NewExecutionPayloadHeaders()
		header1 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            1,
				ParentBlockHash: []byte("parent1"),
				Value:           100,
			},
		}
		header2 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            2,
				ParentBlockHash: []byte("parent2"),
				Value:           100,
			},
		}
		header3 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            3,
				ParentBlockHash: []byte("parent3"),
				Value:           100,
			},
		}
		c.SaveSignedExecutionPayloadHeader(header1)
		c.SaveSignedExecutionPayloadHeader(header2)
		c.SaveSignedExecutionPayloadHeader(header3)
		require.Equal(t, 2, len(c.headers))
		require.Equal(t, header2, c.headers[2][0])
		require.Equal(t, header3, c.headers[3][0])
	})

	t.Run("Header with same slot but higher value should replace the existing one", func(t *testing.T) {
		c := NewExecutionPayloadHeaders()
		header1 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            2,
				ParentBlockHash: []byte("parent2"),
				Value:           100,
			},
		}
		header2 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            2,
				ParentBlockHash: []byte("parent2"),
				Value:           200,
			},
		}
		c.SaveSignedExecutionPayloadHeader(header1)
		c.SaveSignedExecutionPayloadHeader(header2)
		require.Equal(t, 1, len(c.headers[2]))
		require.Equal(t, header2, c.headers[2][0])
	})

	t.Run("Header with different parent block hash should be appended to the same slot", func(t *testing.T) {
		c := NewExecutionPayloadHeaders()
		header1 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            2,
				ParentBlockHash: []byte("parent1"),
				Value:           100,
			},
		}
		header2 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            2,
				ParentBlockHash: []byte("parent2"),
				Value:           200,
			},
		}
		c.SaveSignedExecutionPayloadHeader(header1)
		c.SaveSignedExecutionPayloadHeader(header2)
		require.Equal(t, 2, len(c.headers[2]))
		require.Equal(t, header1, c.headers[2][0])
		require.Equal(t, header2, c.headers[2][1])
	})
}

func TestSignedExecutionPayloadHeader(t *testing.T) {
	t.Run("Return header when slot and parentBlockHash match", func(t *testing.T) {
		c := NewExecutionPayloadHeaders()
		header := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            1,
				ParentBlockHash: []byte("parent1"),
				ParentBlockRoot: []byte("root1"),
				Value:           100,
			},
		}
		c.SaveSignedExecutionPayloadHeader(header)
		result := c.SignedExecutionPayloadHeader(1, []byte("parent1"), []byte("root1"))
		require.NotNil(t, result)
		require.Equal(t, header, result)
	})

	t.Run("Return nil when no matching slot and parentBlockHash", func(t *testing.T) {
		c := NewExecutionPayloadHeaders()
		header := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            1,
				ParentBlockHash: []byte("parent1"),
				ParentBlockRoot: []byte("root1"),
				Value:           100,
			},
		}
		c.SaveSignedExecutionPayloadHeader(header)
		result := c.SignedExecutionPayloadHeader(2, []byte("parent2"), []byte("root1"))
		require.IsNil(t, result)
	})

	t.Run("Return nil when no matching slot and parentBlockRoot", func(t *testing.T) {
		c := NewExecutionPayloadHeaders()
		header := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            1,
				ParentBlockHash: []byte("parent1"),
				ParentBlockRoot: []byte("root1"),
				Value:           100,
			},
		}
		c.SaveSignedExecutionPayloadHeader(header)
		result := c.SignedExecutionPayloadHeader(2, []byte("parent1"), []byte("root2"))
		require.IsNil(t, result)
	})

	t.Run("Return header when there are two slots in the cache and a match is found", func(t *testing.T) {
		c := NewExecutionPayloadHeaders()
		header1 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            1,
				ParentBlockHash: []byte("parent1"),
				Value:           100,
			},
		}
		header2 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            2,
				ParentBlockHash: []byte("parent2"),
				Value:           200,
			},
		}
		c.SaveSignedExecutionPayloadHeader(header1)
		c.SaveSignedExecutionPayloadHeader(header2)

		// Check for the first header
		result1 := c.SignedExecutionPayloadHeader(1, []byte("parent1"), []byte{})
		require.NotNil(t, result1)
		require.Equal(t, header1, result1)

		// Check for the second header
		result2 := c.SignedExecutionPayloadHeader(2, []byte("parent2"), []byte{})
		require.NotNil(t, result2)
		require.Equal(t, header2, result2)
	})

	t.Run("Return nil when slot is evicted from cache", func(t *testing.T) {
		c := NewExecutionPayloadHeaders()
		header1 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            1,
				ParentBlockHash: []byte("parent1"),
				Value:           100,
			},
		}
		header2 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            2,
				ParentBlockHash: []byte("parent2"),
				Value:           200,
			},
		}
		header3 := &enginev1.SignedExecutionPayloadHeader{
			Message: &enginev1.ExecutionPayloadHeaderEPBS{
				Slot:            3,
				ParentBlockHash: []byte("parent3"),
				Value:           300,
			},
		}
		c.SaveSignedExecutionPayloadHeader(header1)
		c.SaveSignedExecutionPayloadHeader(header2)
		c.SaveSignedExecutionPayloadHeader(header3)

		// The first slot should be evicted, so result should be nil
		result := c.SignedExecutionPayloadHeader(1, []byte("parent1"), []byte{})
		require.IsNil(t, result)

		// The second slot should still be present
		result = c.SignedExecutionPayloadHeader(2, []byte("parent2"), []byte{})
		require.NotNil(t, result)
		require.Equal(t, header2, result)

		// The third slot should be present
		result = c.SignedExecutionPayloadHeader(3, []byte("parent3"), []byte{})
		require.NotNil(t, result)
		require.Equal(t, header3, result)
	})
}
