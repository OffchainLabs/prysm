package enginev2

// PayloadStatus.status enum values. Encoded as a single uint8 byte on the wire
// (see engine_v2.proto). Numbering starts at 0 so a zero-valued PayloadStatus
// deserialises as VALID. ACCEPTED is only valid on POST /payloads.
const (
	PayloadStatusValid    uint8 = 0
	PayloadStatusInvalid  uint8 = 1
	PayloadStatusSyncing  uint8 = 2
	PayloadStatusAccepted uint8 = 3
)

// StatusByte returns the one-byte SSZ encoding of a status enum value.
func StatusByte(v uint8) []byte {
	return []byte{v}
}

// Enum returns the status enum value. An absent/empty status decodes as VALID.
func (s *PayloadStatus) Enum() uint8 {
	if s == nil || len(s.Status) == 0 {
		return PayloadStatusValid
	}
	return s.Status[0]
}

// PresentBytes wraps a value as a present Optional[T] (List[T, 1]). Use this
// for the Optional[T] == List[T, 1] convention; an empty list means absent.
func PresentBytes(v []byte) [][]byte {
	return [][]byte{v}
}

// OptionalBytes returns the single element of an Optional[T] list and whether a
// value is present.
func OptionalBytes(list [][]byte) (value []byte, present bool) {
	if len(list) == 0 {
		return nil, false
	}
	return list[0], true
}
