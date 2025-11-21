package flags

import (
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestValidateStateDiffExponents(t *testing.T) {
	tests := []struct {
		exponents []int
		wantErr   bool
		errMsg    string
	}{
		{exponents: []int{0, 1, 2}, wantErr: true, errMsg: "at least 5"},
		{exponents: []int{1, 2, 3}, wantErr: true, errMsg: "at least 5"},
		{exponents: []int{9, 8, 4}, wantErr: true, errMsg: "at least 5"},
		{exponents: []int{3, 4, 5}, wantErr: true, errMsg: "decreasing"},
		{exponents: []int{15, 14, 14, 12, 11}, wantErr: true, errMsg: "decreasing"},
		{exponents: []int{15, 14, 13, 12, 11}, wantErr: false},
		{exponents: []int{21, 18, 16, 13, 11, 9, 5}, wantErr: false},
		{exponents: []int{30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 18, 16, 13, 11, 9, 5}, wantErr: true, errMsg: "between 1 and 15 values"},
		{exponents: []int{}, wantErr: true, errMsg: "between 1 and 15 values"},
		{exponents: []int{30, 18, 16, 13, 11, 9, 5}, wantErr: false},
		{exponents: []int{31, 18, 16, 13, 11, 9, 5}, wantErr: true, errMsg: "<= 30"},
	}

	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			err := validateStateDiffExponents(tt.exponents)
			if tt.wantErr {
				require.ErrorContains(t, tt.errMsg, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
