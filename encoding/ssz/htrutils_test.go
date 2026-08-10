package ssz_test

import (
	"reflect"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestUint64Root(t *testing.T) {
	uintVal := uint64(1234567890)
	expected := [32]byte{210, 2, 150, 73, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	result := ssz.Uint64Root(uintVal)
	assert.Equal(t, expected, result)
}

func TestByteArrayRootWithLimit(t *testing.T) {
	testHistoricalRoots := [][]byte{{123}, {234}}
	expected := [32]byte{70, 204, 150, 196, 89, 138, 190, 205, 65, 207, 120, 166, 179, 247, 147, 20, 29, 133, 117, 116, 151, 234, 129, 32, 22, 15, 79, 178, 98, 73, 132, 152}

	result, err := ssz.ByteArrayRootWithLimit(testHistoricalRoots, 16777216)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestSlashingsRoot(t *testing.T) {
	testSlashingsRoot := []uint64{123, 234}
	expected := [32]byte{123, 0, 0, 0, 0, 0, 0, 0, 234, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	result, err := ssz.SlashingsRoot(testSlashingsRoot)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestByteSliceRoot(t *testing.T) {
	tests := []struct {
		name      string
		slice     []byte
		maxLength uint64
		want      [32]byte
		wantErr   bool
	}{
		{
			name:  "nil",
			slice: nil,
			want:  [32]byte{245, 165, 253, 66, 209, 106, 32, 48, 39, 152, 239, 110, 211, 9, 151, 155, 67, 0, 61, 35, 32, 217, 240, 232, 234, 152, 49, 169, 39, 89, 251, 75},
		},
		{
			name:  "empty",
			slice: []byte{},
			want:  [32]byte{245, 165, 253, 66, 209, 106, 32, 48, 39, 152, 239, 110, 211, 9, 151, 155, 67, 0, 61, 35, 32, 217, 240, 232, 234, 152, 49, 169, 39, 89, 251, 75},
		},
		{
			name:  "byte slice 3 values",
			slice: []byte{1, 2, 3},
			want:  [32]byte{20, 159, 26, 252, 247, 204, 44, 159, 161, 135, 211, 195, 106, 59, 220, 149, 199, 163, 228, 155, 113, 118, 64, 126, 173, 223, 102, 1, 241, 158, 164, 185},
		},
		{
			name:  "byte slice 32 values",
			slice: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
			want:  [32]byte{7, 30, 46, 77, 237, 240, 59, 126, 232, 232, 232, 6, 145, 210, 31, 18, 117, 12, 217, 40, 204, 141, 90, 236, 241, 128, 221, 45, 126, 39, 39, 202},
		},
		{
			name:    "over max length",
			slice:   make([]byte, fieldparams.RootLength+1),
			want:    [32]byte{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.maxLength == 0 {
				tt.maxLength = fieldparams.RootLength
			}
			got, err := ssz.ByteSliceRoot(tt.slice, tt.maxLength)
			if (err != nil) != tt.wantErr {
				t.Errorf("ByteSliceRoot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ByteSliceRoot() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPackByChunk_SingleList(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  [][32]byte
	}{
		{
			name:  "nil",
			input: nil,
			want:  [][32]byte{{}},
		},
		{
			name:  "empty",
			input: []byte{},
			want:  [][32]byte{{}},
		},
		{
			name:  "one",
			input: []byte{1},
			want:  [][32]byte{{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		},
		{
			name:  "one, two",
			input: []byte{1, 2},
			want:  [][32]byte{{1, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ssz.PackByChunk([][]byte{tt.input})
			require.NoError(t, err)
			require.DeepSSZEqual(t, tt.want, got)
		})
	}
}
