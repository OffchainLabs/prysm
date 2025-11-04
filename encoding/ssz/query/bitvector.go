package query

// bitvectorInfo holds information about a SSZ Bitvector type.
type bitvectorInfo struct {
	// length is the fixed length of bits of the Bitvector.
	length uint64
}

func (v *bitvectorInfo) Length() uint64 {
	if v == nil {
		return 0
	}

	return v.length
}

func (v *bitvectorInfo) Size() uint64 {
	if v == nil {
		return 0
	}

	// Size in bytes.
	return v.length / 8
}
