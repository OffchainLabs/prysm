package testutil

import (
	"crypto/rand"
	"fmt"
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
	ssz "github.com/prysmaticlabs/fastssz"
)

// DummyData is a struct that holds dummy data for testing purposes.
type DummyData struct {
	Root      []byte
	Pubkey    []byte
	Signature []byte
}

// RandomDummyData generates random dummy data for testing purposes.
func RandomDummyData(t *testing.T) *DummyData {
	dummyRoot := make([]byte, 32)
	_, err := rand.Read(dummyRoot)
	require.NoError(t, err)

	dummyPubkey := make([]byte, 48)
	_, err = rand.Read(dummyPubkey)
	require.NoError(t, err)

	dummySignature := make([]byte, 96)
	_, err = rand.Read(dummySignature)
	require.NoError(t, err)

	return &DummyData{
		Root:      dummyRoot,
		Pubkey:    dummyPubkey,
		Signature: dummySignature,
	}
}

// marshalAny marshals any value into SSZ format.
func marshalAny(value any) ([]byte, error) {
	// First check if it implements ssz.Marshaler (this catches custom types like primitives.Epoch)
	if marshaler, ok := value.(ssz.Marshaler); ok {
		return marshaler.MarshalSSZ()
	}

	// Handle custom type aliases by checking if they're based on primitive types
	valueType := reflect.TypeOf(value)
	if valueType.PkgPath() != "" {
		switch valueType.Kind() {
		case reflect.Uint64:
			return ssz.MarshalUint64(make([]byte, 0), reflect.ValueOf(value).Uint()), nil
		case reflect.Uint32:
			return ssz.MarshalUint32(make([]byte, 0), uint32(reflect.ValueOf(value).Uint())), nil
		case reflect.Bool:
			return ssz.MarshalBool(make([]byte, 0), reflect.ValueOf(value).Bool()), nil
		}
	}

	switch v := value.(type) {
	case []byte:
		return v, nil
	case []uint64:
		buf := make([]byte, len(v)*8)
		for i, val := range v {
			buf = ssz.MarshalUint64(buf[i*8:], val)
		}
		return buf, nil
	case uint64:
		return ssz.MarshalUint64(make([]byte, 0), v), nil
	case bool:
		return ssz.MarshalBool(make([]byte, 0), v), nil

	default:
		return nil, fmt.Errorf("unsupported type for SSZ marshalling: %T", value)
	}
}
