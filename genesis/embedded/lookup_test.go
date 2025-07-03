package embedded_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
<<<<<<<< HEAD:genesis/internal/embedded/lookup_test.go
	"github.com/OffchainLabs/prysm/v6/genesis/internal/embedded"
========
	"github.com/OffchainLabs/prysm/v6/genesis/embedded"
>>>>>>>> fc78ad7c5b (initialize genesis data asap at node start):genesis/embedded/lookup_test.go
)

func TestGenesisState(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: params.MainnetName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := embedded.ByName(tt.name)
			if err != nil {
				t.Fatal(err)
			}
			if st == nil {
				t.Fatal("nil state")
			}
			if st.NumValidators() <= 0 {
				t.Error("No validators present in state")
			}
		})
	}
}
