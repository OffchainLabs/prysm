package query

// containerInfo has
// 1. fields: a field map that maps a field's JSON name to its sszInfo for nested Containers
// 2. order: a list of field names in the order they should be serialized
type containerInfo struct {
	fields map[string]*fieldInfo
	order  []string
}

// Exported alias (or type) for container field map.
type ContainerInfo = containerInfo

// Exported FieldInfo with accessors
type FieldInfo = fieldInfo

func (ci *ContainerInfo) Fields() map[string]*FieldInfo {
	return ci.fields
}

func (ci *ContainerInfo) Order() []string {
	return ci.order
}

type fieldInfo struct {
	// sszInfo contains the SSZ information of the field.
	sszInfo *sszInfo
	// offset is the offset of the field within the parent struct.
	offset uint64
	// goFieldName is the name of the field in Go struct.
	goFieldName string
}

// Exported fields
func (f *FieldInfo) SSZ() *SSZInfo {
	return f.sszInfo
}

func (f *FieldInfo) Offset() uint64 {
	return f.offset
}

func (f *FieldInfo) ActualOffset() uint64 {
	return f.offset
}

func (f *FieldInfo) Name() string {
	return f.goFieldName
}
