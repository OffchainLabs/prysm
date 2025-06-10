package types

import (
	"encoding/hex"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	ssz "github.com/prysmaticlabs/fastssz"
)

func generateBlobIdentifiers(n int) []*eth.BlobIdentifier {
	r := make([]*eth.BlobIdentifier, n)
	for i := 0; i < n; i++ {
		r[i] = &eth.BlobIdentifier{
			BlockRoot: bytesutil.PadTo([]byte{byte(i)}, 32),
			Index:     0,
		}
	}
	return r
}

func TestBlobSidecarsByRootReq_MarshalSSZ(t *testing.T) {
	cases := []struct {
		name         string
		ids          []*eth.BlobIdentifier
		marshalErr   error
		unmarshalErr error
		unmarshalMod func([]byte) []byte
	}{
		{
			name: "empty list",
		},
		{
			name: "single item list",
			ids:  generateBlobIdentifiers(1),
		},
		{
			name: "10 item list",
			ids:  generateBlobIdentifiers(10),
		},
		{
			name: "max list",
			ids:  generateBlobIdentifiers(int(params.BeaconConfig().MaxRequestBlobSidecarsElectra)),
		},
		{
			name:         "beyond max list",
			ids:          generateBlobIdentifiers(int(params.BeaconConfig().MaxRequestBlobSidecarsElectra) + 1),
			unmarshalErr: ssz.ErrIncorrectListSize,
		},
		{
			name: "wonky unmarshal size",
			ids:  generateBlobIdentifiers(10),
			unmarshalMod: func(in []byte) []byte {
				in = append(in, byte(0))
				return in
			},
			unmarshalErr: ssz.ErrIncorrectByteSize,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := BlobSidecarsByRootReq(c.ids)
			by, err := r.MarshalSSZ()
			if c.marshalErr != nil {
				require.ErrorIs(t, err, c.marshalErr)
				return
			}
			require.NoError(t, err)
			if c.unmarshalMod != nil {
				by = c.unmarshalMod(by)
			}
			got := &BlobSidecarsByRootReq{}
			err = got.UnmarshalSSZ(by)
			if c.unmarshalErr != nil {
				require.ErrorIs(t, err, c.unmarshalErr)
				return
			}
			require.NoError(t, err)
			for i, gid := range *got {
				require.DeepEqual(t, c.ids[i], gid)
			}
		})
	}
}

func TestBeaconBlockByRootsReq_Limit(t *testing.T) {
	fixedRoots := make([][32]byte, 0)
	for i := uint64(0); i < params.BeaconConfig().MaxRequestBlocks+100; i++ {
		fixedRoots = append(fixedRoots, [32]byte{byte(i)})
	}
	req := BeaconBlockByRootsReq(fixedRoots)

	_, err := req.MarshalSSZ()
	require.ErrorContains(t, "beacon block by roots request exceeds max size", err)

	buf := make([]byte, 0)
	for _, rt := range fixedRoots {
		buf = append(buf, rt[:]...)
	}
	req2 := BeaconBlockByRootsReq(nil)
	require.ErrorContains(t, "expected buffer with length of up to", req2.UnmarshalSSZ(buf))
}

func TestErrorResponse_Limit(t *testing.T) {
	errorMessage := make([]byte, 0)
	// Provide a message of size 6400 bytes.
	for i := uint64(0); i < 200; i++ {
		byteArr := [32]byte{byte(i)}
		errorMessage = append(errorMessage, byteArr[:]...)
	}
	errMsg := ErrorMessage{}
	require.ErrorContains(t, "expected buffer with length of upto", errMsg.UnmarshalSSZ(errorMessage))
}

func TestRoundTripSerialization(t *testing.T) {
	roundTripTestBlocksByRootReq(t)
	roundTripTestErrorMessage(t)
}

func roundTripTestBlocksByRootReq(t *testing.T) {
	fixedRoots := make([][32]byte, 0)
	for i := 0; i < 200; i++ {
		fixedRoots = append(fixedRoots, [32]byte{byte(i)})
	}
	req := BeaconBlockByRootsReq(fixedRoots)

	marshalledObj, err := req.MarshalSSZ()
	require.NoError(t, err)
	newVal := BeaconBlockByRootsReq(nil)

	require.NoError(t, newVal.UnmarshalSSZ(marshalledObj))
	assert.DeepEqual(t, [][32]byte(newVal), fixedRoots)
}

func roundTripTestErrorMessage(t *testing.T) {
	errMsg := []byte{'e', 'r', 'r', 'o', 'r'}
	sszErr := make(ErrorMessage, len(errMsg))
	copy(sszErr, errMsg)

	marshalledObj, err := sszErr.MarshalSSZ()
	require.NoError(t, err)
	newVal := ErrorMessage(nil)

	require.NoError(t, newVal.UnmarshalSSZ(marshalledObj))
	assert.DeepEqual(t, []byte(newVal), errMsg)
}

func TestSSZBytes_HashTreeRoot(t *testing.T) {
	tests := []struct {
		name        string
		actualValue []byte
		root        []byte
		wantErr     bool
	}{
		{
			name:        "random1",
			actualValue: hexDecodeOrDie(t, "844e1063e0b396eed17be8eddb7eecd1fe3ea46542a4b72f7466e77325e5aa6d"),
			root:        hexDecodeOrDie(t, "844e1063e0b396eed17be8eddb7eecd1fe3ea46542a4b72f7466e77325e5aa6d"),
			wantErr:     false,
		},
		{
			name:        "random1",
			actualValue: hexDecodeOrDie(t, "7b16162ecd9a28fa80a475080b0e4fff4c27efe19ce5134ce3554b72274d59fd534400ba4c7f699aa1c307cd37c2b103"),
			root:        hexDecodeOrDie(t, "128ed34ee798b9f00716f9ba5c000df5c99443dabc4d3f2e9bb86c77c732e007"),
			wantErr:     false,
		},
		{
			name:        "random2",
			actualValue: []byte{},
			root:        hexDecodeOrDie(t, "0000000000000000000000000000000000000000000000000000000000000000"),
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := SSZBytes(tt.actualValue)
			htr, err := s.HashTreeRoot()
			require.NoError(t, err)
			require.DeepEqual(t, tt.root, htr[:])
		})
	}
}

func TestGoodbyeCodes(t *testing.T) {
	assert.Equal(t, primitives.SSZUint64(1), GoodbyeCodeClientShutdown)
	assert.Equal(t, primitives.SSZUint64(2), GoodbyeCodeWrongNetwork)
	assert.Equal(t, primitives.SSZUint64(3), GoodbyeCodeGenericError)
	assert.Equal(t, primitives.SSZUint64(128), GoodbyeCodeUnableToVerifyNetwork)
	assert.Equal(t, primitives.SSZUint64(129), GoodbyeCodeTooManyPeers)
	assert.Equal(t, primitives.SSZUint64(250), GoodbyeCodeBadScore)
	assert.Equal(t, primitives.SSZUint64(251), GoodbyeCodeBanned)

}

func hexDecodeOrDie(t *testing.T, str string) []byte {
	decoded, err := hex.DecodeString(str)
	require.NoError(t, err)
	return decoded
}

// ====================================
// DataColumnsByRootIdentifiers section
// ====================================
func generateDataColumnIdentifiers(n int) []*eth.DataColumnsByRootIdentifier {
	r := make([]*eth.DataColumnsByRootIdentifier, n)
	for i := 0; i < n; i++ {
		r[i] = &eth.DataColumnsByRootIdentifier{
			BlockRoot: bytesutil.PadTo([]byte{byte(i)}, 32),
			Columns:   []uint64{uint64(i)},
		}
	}
	return r
}

func TestDataColumnSidecarsByRootReq_Marshal(t *testing.T) {
	/*
		SSZ encoding of DataColumnsByRootIdentifiers is tested in spectests.
		However, encoding a list of DataColumnsByRootIdentifier is not.
		We are testing it here.

		Python code to generate the expected value

		# pip install eth2spec

		from eth2spec.utils.ssz import ssz_typing

		Container = ssz_typing.Container
		List = ssz_typing.List

		Root = ssz_typing.Bytes32
		ColumnIndex = ssz_typing.uint64

		NUMBER_OF_COLUMNS=128

		class DataColumnsByRootIdentifier(Container):
			block_root: Root
			columns: List[ColumnIndex, NUMBER_OF_COLUMNS]

		first = DataColumnsByRootIdentifier(block_root="0x0100000000000000000000000000000000000000000000000000000000000000", columns=[3,5,7])
		second = DataColumnsByRootIdentifier(block_root="0x0200000000000000000000000000000000000000000000000000000000000000", columns=[])
		third = DataColumnsByRootIdentifier(block_root="0x0300000000000000000000000000000000000000000000000000000000000000", columns=[6, 4])

		expected = List[DataColumnsByRootIdentifier, 42](first, second, third).encode_bytes().hex()
	*/

	const expected = "0c000000480000006c00000001000000000000000000000000000000000000000000000000000000000000002400000003000000000000000500000000000000070000000000000002000000000000000000000000000000000000000000000000000000000000002400000003000000000000000000000000000000000000000000000000000000000000002400000006000000000000000400000000000000"
	identifiers := &DataColumnsByRootIdentifiers{
		{
			BlockRoot: bytesutil.PadTo([]byte{1}, fieldparams.RootLength),
			Columns:   []uint64{3, 5, 7},
		},
		{
			BlockRoot: bytesutil.PadTo([]byte{2}, fieldparams.RootLength),
			Columns:   []uint64{},
		},
		{
			BlockRoot: bytesutil.PadTo([]byte{3}, fieldparams.RootLength),
			Columns:   []uint64{6, 4},
		},
	}

	marshalled, err := identifiers.MarshalSSZ()
	require.NoError(t, err)

	actual := hex.EncodeToString(marshalled)
	require.Equal(t, expected, actual)
}

func TestDataColumnSidecarsByRootReq_MarshalUnmarshal(t *testing.T) {
	cases := []struct {
		name         string
		ids          []*eth.DataColumnsByRootIdentifier
		marshalErr   error
		unmarshalErr string
		unmarshalMod func([]byte) []byte
	}{
		{
			name: "empty list",
		},
		{
			name: "single item list",
			ids:  generateDataColumnIdentifiers(1),
		},
		{
			name: "10 item list",
			ids:  generateDataColumnIdentifiers(10),
		},
		{
			name: "wonky unmarshal size",
			ids:  generateDataColumnIdentifiers(10),
			unmarshalMod: func(in []byte) []byte {
				in = append(in, byte(0))
				return in
			},
			unmarshalErr: "a is not evenly divisble by b",
		},
		{
			name: "size too big",
			ids:  generateDataColumnIdentifiers(1),
			unmarshalMod: func(in []byte) []byte {
				maxLen := params.BeaconConfig().MaxRequestDataColumnSidecars * uint64(dataColumnIdSize)
				add := make([]byte, maxLen)
				in = append(in, add...)
				return in
			},
			unmarshalErr: "a/b is greater than max",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := DataColumnsByRootIdentifiers(c.ids)
			bytes, err := req.MarshalSSZ()
			if c.marshalErr != nil {
				require.ErrorIs(t, err, c.marshalErr)
				return
			}

			require.NoError(t, err)
			if c.unmarshalMod != nil {
				bytes = c.unmarshalMod(bytes)
			}

			got := &DataColumnsByRootIdentifiers{}
			err = got.UnmarshalSSZ(bytes)
			if c.unmarshalErr != "" {
				require.ErrorContains(t, c.unmarshalErr, err)
				return
			}
			require.NoError(t, err)

			require.Equal(t, len(c.ids), len(*got))

			for i, expected := range c.ids {
				actual := (*got)[i]
				require.DeepEqual(t, expected, actual)
			}
		})
	}

	// Test MarshalSSZTo
	req := DataColumnsByRootIdentifiers(generateDataColumnIdentifiers(10))
	buf := make([]byte, 0)
	buf, err := req.MarshalSSZTo(buf)
	require.NoError(t, err)
	require.Equal(t, len(buf), int(req.SizeSSZ()))

	var unmarshalled DataColumnsByRootIdentifiers
	err = unmarshalled.UnmarshalSSZ(buf)
	require.NoError(t, err)
	require.DeepEqual(t, req, unmarshalled)
}
