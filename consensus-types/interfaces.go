package consensus_types

type SSZunmarshalable interface {
	UnmarshalSSZ(buf []byte) error
}

type SSZmarshalable interface {
	MarshalSSZ() ([]byte, error)
}
