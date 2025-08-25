package query

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// sszInfo holds the all necessary data for analyzing SSZ data types.
type sszInfo struct {
	// Type of the SSZ structure (Basic, Container, List, etc.).
	sszType SSZType
	// Type in Go. Need this for unmarshaling.
	typ reflect.Type

	// isVariable is true if the struct contains any variable-size fields.
	isVariable bool
	// fixedSize is the total size of the struct's fixed part.
	fixedSize uint64

	// For Container types.
	containerInfo containerInfo
}

func (info *sszInfo) FixedSize() uint64 {
	if info == nil {
		return 0
	}
	return info.fixedSize
}

func (info *sszInfo) Size() uint64 {
	if info == nil {
		return 0
	}

	// Easy case: if the type is not variable, we can return the fixed size.
	if !info.isVariable {
		return info.fixedSize
	}

	// NOTE: Handle variable-sized types.
	return 0
}

func (info *sszInfo) ContainerInfo() (containerInfo, error) {
	if info == nil {
		return nil, fmt.Errorf("sszInfo is nil")
	}

	if info.sszType != Container {
		return nil, fmt.Errorf("sszInfo is not a Container type, got %s", info.sszType)
	}

	if info.containerInfo == nil {
		return nil, fmt.Errorf("sszInfo.containerInfo is nil")
	}

	return info.containerInfo, nil
}

// Print returns a string representation of the sszInfo, which is useful for debugging.
func (info *sszInfo) Print() string {
	if info == nil {
		return "<nil>"
	}
	var builder strings.Builder
	printRecursive(info, &builder, "")
	return builder.String()
}

func printRecursive(info *sszInfo, builder *strings.Builder, prefix string) {
	var sizeDesc string
	if info.isVariable {
		sizeDesc = "Variable-size"
	} else {
		sizeDesc = "Fixed-size"
	}

	switch info.sszType {
	case Container:
		builder.WriteString(fmt.Sprintf("%s: %s (%s / fixed size: %d, total size: %d)\n", info.sszType, info.typ.Name(), sizeDesc, info.FixedSize(), info.Size()))
	default:
		builder.WriteString(fmt.Sprintf("%s (%s / size: %d)\n", info.sszType, sizeDesc, info.Size()))
	}

	keys := make([]string, 0, len(info.containerInfo))
	for k := range info.containerInfo {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, key := range keys {
		connector := "├─"
		nextPrefix := prefix + "│  "
		if i == len(keys)-1 {
			connector = "└─"
			nextPrefix = prefix + "   "
		}

		builder.WriteString(fmt.Sprintf("%s%s %s (offset: %d) ", prefix, connector, key, info.containerInfo[key].offset))

		if nestedInfo := info.containerInfo[key].sszInfo; nestedInfo != nil {
			printRecursive(nestedInfo, builder, nextPrefix)
		} else {
			builder.WriteString("\n")
		}
	}
}
