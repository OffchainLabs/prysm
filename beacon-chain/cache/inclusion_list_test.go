package cache

import (
	"bytes"
	"testing"
)

func TestInclusionLists(t *testing.T) {
	il := NewInclusionLists()

	tests := []struct {
		name          string
		actions       func()
		expectedGet   [][]byte
		expectedTwice bool
	}{
		{
			name: "Add single validator with unique transactions",
			actions: func() {
				il.Add(1, 1, [][]byte{[]byte("tx1"), []byte("tx2")})
			},
			expectedGet:   [][]byte{[]byte("tx1"), []byte("tx2")},
			expectedTwice: false,
		},
		{
			name: "Add duplicate transactions for second validator",
			actions: func() {
				il.Add(1, 2, [][]byte{[]byte("tx1"), []byte("tx3")})
			},
			expectedGet:   [][]byte{[]byte("tx1"), []byte("tx2"), []byte("tx3")},
			expectedTwice: false,
		},
		{
			name: "Mark validator as seen twice",
			actions: func() {
				il.Add(1, 1, [][]byte{[]byte("tx4")})
			},
			expectedGet:   [][]byte{[]byte("tx1"), []byte("tx3")},
			expectedTwice: true,
		},
		{
			name: "Delete a slot",
			actions: func() {
				il.Delete(1)
			},
			expectedGet:   nil,
			expectedTwice: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.actions()

			// Check Get results
			got := il.Get(1)
			if !compareTransactions(got, tt.expectedGet) {
				t.Errorf("unexpected Get result: got %v, want %v", got, tt.expectedGet)
			}

			// Check SeenTwice result for validator 1
			gotTwice := il.SeenTwice(1, 1)
			if gotTwice != tt.expectedTwice {
				t.Errorf("unexpected SeenTwice result: got %v, want %v", gotTwice, tt.expectedTwice)
			}
		})
	}
}

// compareTransactions compares two slices of byte slices for equality.
func compareTransactions(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}
