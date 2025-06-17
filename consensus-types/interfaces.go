package consensus_types

type SSZunmarshalable interface {
	UnmarshalSSZ(buf []byte) error
}
