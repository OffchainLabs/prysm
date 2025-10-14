package query

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// sszInfo holds the all necessary data for analyzing SSZ data types.
type sszInfo struct {
	// Type of the SSZ structure (Basic, Container, List, etc.).
	sszType SSZType
	// Type in Go. Need this for unmarshaling.
	typ reflect.Type
	// Original object being analyzed
	source SSZObject

	// isVariable is true if the struct contains any variable-size fields.
	isVariable bool
	// fixedSize is the total size of the struct's fixed part.
	fixedSize uint64

	// For Container types.
	containerInfo *containerInfo

	// For List types.
	listInfo *listInfo

	// For Vector types.
	vectorInfo *vectorInfo

	// For Bitlist types.
	bitlistInfo *bitlistInfo

	// For Bitvector types.
	bitvectorInfo *bitvectorInfo
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

	switch info.sszType {
	case List:
		return info.listInfo.Size()

	case Bitlist:
		return info.bitlistInfo.Size()

	case Container:
		size := info.fixedSize
		for _, fieldInfo := range info.containerInfo.fields {
			if !fieldInfo.sszInfo.isVariable {
				continue
			}

			// Include offset bytes inside nested lists.
			if fieldInfo.sszInfo.sszType == List {
				size += fieldInfo.sszInfo.listInfo.OffsetBytes()
			}

			size += fieldInfo.sszInfo.Size()
		}
		return size

	default:
		return 0
	}
}

func (info *sszInfo) ContainerInfo() (*containerInfo, error) {
	if info == nil {
		return nil, errors.New("sszInfo is nil")
	}

	if info.sszType != Container {
		return nil, fmt.Errorf("sszInfo is not a Container type, got %s", info.sszType)
	}

	if info.containerInfo == nil {
		return nil, errors.New("sszInfo.containerInfo is nil")
	}

	return info.containerInfo, nil
}

func (info *sszInfo) ListInfo() (*listInfo, error) {
	if info == nil {
		return nil, errors.New("sszInfo is nil")
	}

	if info.sszType != List {
		return nil, fmt.Errorf("sszInfo is not a List type, got %s", info.sszType)
	}

	return info.listInfo, nil
}

func (info *sszInfo) VectorInfo() (*vectorInfo, error) {
	if info == nil {
		return nil, errors.New("sszInfo is nil")
	}

	if info.sszType != Vector {
		return nil, fmt.Errorf("sszInfo is not a Vector type, got %s", info.sszType)
	}

	return info.vectorInfo, nil
}

func (info *sszInfo) BitlistInfo() (*bitlistInfo, error) {
	if info == nil {
		return nil, errors.New("sszInfo is nil")
	}

	if info.sszType != Bitlist {
		return nil, fmt.Errorf("sszInfo is not a Bitlist type, got %s", info.sszType)
	}

	return info.bitlistInfo, nil
}

func (info *sszInfo) BitvectorInfo() (*bitvectorInfo, error) {
	if info == nil {
		return nil, errors.New("sszInfo is nil")
	}

	if info.sszType != Bitvector {
		return nil, fmt.Errorf("sszInfo is not a Bitvector type, got %s", info.sszType)
	}

	return info.bitvectorInfo, nil
}

// String implements the Stringer interface for sszInfo.
// This follows the notation used in the consensus specs.
func (info *sszInfo) String() string {
	if info == nil {
		return "<nil>"
	}

	switch info.sszType {
	case List:
		return fmt.Sprintf("List[%s, %d]", info.listInfo.element, info.listInfo.limit)
	case Vector:
		if info.vectorInfo.element.String() == "uint8" {
			// Handle byte vectors as BytesN
			// See Aliases section in SSZ spec:
			// https://github.com/ethereum/consensus-specs/blob/master/ssz/simple-serialize.md#aliases
			return fmt.Sprintf("Bytes%d", info.vectorInfo.length)
		}
		return fmt.Sprintf("Vector[%s, %d]", info.vectorInfo.element, info.vectorInfo.length)
	case Bitlist:
		return fmt.Sprintf("Bitlist[%d]", info.bitlistInfo.limit)
	case Bitvector:
		return fmt.Sprintf("Bitvector[%d]", info.bitvectorInfo.length)
	default:
		return info.typ.Name()
	}
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
		builder.WriteString(fmt.Sprintf("%s (%s / fixed size: %d, total size: %d)\n", info, sizeDesc, info.FixedSize(), info.Size()))

		for i, key := range info.containerInfo.order {
			connector := "├─"
			nextPrefix := prefix + "│  "
			if i == len(info.containerInfo.order)-1 {
				connector = "└─"
				nextPrefix = prefix + "   "
			}

			builder.WriteString(fmt.Sprintf("%s%s %s (offset: %d) ", prefix, connector, key, info.containerInfo.fields[key].offset))

			if nestedInfo := info.containerInfo.fields[key].sszInfo; nestedInfo != nil {
				printRecursive(nestedInfo, builder, nextPrefix)
			} else {
				builder.WriteString("\n")
			}
		}

	case List:
		builder.WriteString(fmt.Sprintf("%s (%s / length: %d, size: %d)\n", info, sizeDesc, info.listInfo.length, info.Size()))

	case Bitlist:
		builder.WriteString(fmt.Sprintf("%s (%s / length (bit): %d, size: %d)\n", info, sizeDesc, info.bitlistInfo.length, info.Size()))

	default:
		builder.WriteString(fmt.Sprintf("%s (%s / size: %d)\n", info, sizeDesc, info.Size()))
	}
}
