package genesis_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/genesis"
	"github.com/OffchainLabs/prysm/v6/config/params"
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
			st, err := genesis.State(tt.name)
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
