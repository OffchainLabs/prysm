package query

// containerInfo has
// 1. fields: a field map that maps a field's JSON name to its sszInfo for nested Containers
// 2. order: a list of field names in the order they should be serialized
type containerInfo struct {
	fields map[string]*fieldInfo
	order  []string
}

type fieldInfo struct {
	// sszInfo contains the SSZ information of the field.
	sszInfo *sszInfo
	// offset is the offset of the field within the parent struct.
	offset uint64
	// goFieldName is the name of the field in Go struct.
	goFieldName string
}
