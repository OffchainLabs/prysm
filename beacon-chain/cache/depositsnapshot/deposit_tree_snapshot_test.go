package depositsnapshot

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestDepositTreeSnapshot_CalculateRoot(t *testing.T) {
	tests := []struct {
		name         string
		finalized    int
		depositCount uint64
		want         [32]byte
	}{
		{
			name:         "empty",
			finalized:    0,
			depositCount: 0,
			want:         [32]byte{215, 10, 35, 71, 49, 40, 92, 104, 4, 194, 164, 245, 103, 17, 221, 184, 200, 44, 153, 116, 15, 32, 120, 84, 137, 16, 40, 175, 52, 226, 126, 94},
		},
		{
			name:         "1 Finalized",
			finalized:    1,
			depositCount: 2,
			want:         [32]byte{36, 118, 154, 57, 217, 109, 145, 116, 238, 1, 207, 59, 187, 28, 69, 187, 70, 55, 153, 180, 15, 150, 37, 72, 140, 36, 109, 154, 212, 202, 47, 59},
		},
		{
			name:         "many finalised",
			finalized:    6,
			depositCount: 20,
			want:         [32]byte{210, 63, 57, 119, 12, 5, 3, 25, 139, 20, 244, 59, 114, 119, 35, 88, 222, 88, 122, 106, 239, 20, 45, 140, 99, 92, 222, 166, 133, 159, 128, 72},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var finalized [][32]byte
			for i := 0; i < tt.finalized; i++ {
				finalized = append(finalized, hexString(t, fmt.Sprintf("%064d", i)))
			}
			ds := &DepositTreeSnapshot{
				finalized:    finalized,
				depositCount: tt.depositCount,
			}
			root, err := ds.CalculateRoot()
			require.NoError(t, err)
			if got := root; !reflect.DeepEqual(got, tt.want) {
				require.DeepEqual(t, tt.want, got)
			}
		})
	}
}
