package query

// containerInfo maps a field's JSON name to its sszInfo for nested Containers.
type containerInfo = map[string]*fieldInfo

type fieldInfo struct {
	// sszInfo contains the SSZ information of the field.
	sszInfo *sszInfo
	// offset is the offset of the field within the parent struct.
	offset uint64
}
