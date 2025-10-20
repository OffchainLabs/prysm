package flags

import (
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestValidateStateDiffExponents(t *testing.T) {
	tests := []struct {
		idx       int
		exponents []int
		wantErr   bool
		errMsg    string
	}{
		{idx: 1, exponents: []int{0, 1, 2}, wantErr: true, errMsg: "at least 5"},
		{idx: 2, exponents: []int{1, 2, 3}, wantErr: true, errMsg: "at least 5"},
		{idx: 3, exponents: []int{9, 8, 4}, wantErr: true, errMsg: "at least 5"},
		{idx: 4, exponents: []int{3, 4, 5}, wantErr: true, errMsg: "decreasing"},
		{idx: 5, exponents: []int{15, 14, 14, 12, 11}, wantErr: true, errMsg: "decreasing"},
		{idx: 6, exponents: []int{15, 14, 13, 12, 11}, wantErr: false},
		{idx: 7, exponents: []int{21, 18, 16, 13, 11, 9, 5}, wantErr: false},
		{idx: 8, exponents: []int{30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 18, 16, 13, 11, 9, 5}, wantErr: true, errMsg: "between 1 and 15 values"},
		{idx: 9, exponents: []int{}, wantErr: true, errMsg: "between 1 and 15 values"},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.idx), func(t *testing.T) {
			err := validateStateDiffExponents(tt.exponents)
			if tt.wantErr {
				require.ErrorContains(t, tt.errMsg, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
