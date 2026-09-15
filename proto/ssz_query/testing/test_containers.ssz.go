//go:build !minimal

package testing

import (
	binary "encoding/binary"
	"fmt"
	go_bitfield "github.com/OffchainLabs/go-bitfield"
	ssz "github.com/OffchainLabs/methodical-ssz/ssz"
)

func (c *FixedNestedContainer) SizeSSZ() int {
	size := 40

	return size
}

func (c *FixedNestedContainer) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *FixedNestedContainer) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Value1
	dst = binary.LittleEndian.AppendUint64(dst, c.Value1)

	// Field 1: Value2
	if len(c.Value2) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Value2...)

	return dst, err
}

func (c *FixedNestedContainer) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 40 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.Value1
	sszSlice1 := buf[8:40] // c.Value2

	// Field 0: Value1
	c.Value1 = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: Value2
	c.Value2 = make([]byte, 0, 32)
	c.Value2 = append(c.Value2, sszSlice1...)
	return err
}

func (c *FixedNestedContainer) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *FixedNestedContainer) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Value1
	hh.PutUint64(c.Value1)
	// Field 1: Value2
	if len(c.Value2) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Value2)
	hh.Merkleize(indx)
	return nil
}

func (c *FixedTestContainer) SizeSSZ() int {
	size := 565

	return size
}

func (c *FixedTestContainer) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *FixedTestContainer) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: FieldUint32
	dst = binary.LittleEndian.AppendUint32(dst, c.FieldUint32)

	// Field 1: FieldUint64
	dst = binary.LittleEndian.AppendUint64(dst, c.FieldUint64)

	// Field 2: FieldBool
	if c.FieldBool {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}

	// Field 3: FieldBytes32
	if len(c.FieldBytes32) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FieldBytes32...)

	// Field 4: Nested
	if c.Nested == nil {
		c.Nested = new(FixedNestedContainer)
	}
	if dst, err = c.Nested.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Nested: %w", err)
	}

	// Field 5: VectorField
	if len(c.VectorField) != 24 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.VectorField {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 6: TwoDimensionBytesField
	if len(c.TwoDimensionBytesField) != 5 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.TwoDimensionBytesField {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 7: Bitvector64Field
	if len([]byte(c.Bitvector64Field)) != 8 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.Bitvector64Field)...)

	// Field 8: Bitvector512Field
	if len([]byte(c.Bitvector512Field)) != 64 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.Bitvector512Field)...)

	// Field 9: TrailingField
	if len(c.TrailingField) != 56 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.TrailingField...)

	return dst, err
}

func (c *FixedTestContainer) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 565 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:4]     // c.FieldUint32
	sszSlice1 := buf[4:12]    // c.FieldUint64
	sszSlice2 := buf[12:13]   // c.FieldBool
	sszSlice3 := buf[13:45]   // c.FieldBytes32
	sszSlice4 := buf[45:85]   // c.Nested
	sszSlice5 := buf[85:277]  // c.VectorField
	sszSlice6 := buf[277:437] // c.TwoDimensionBytesField
	sszSlice7 := buf[437:445] // c.Bitvector64Field
	sszSlice8 := buf[445:509] // c.Bitvector512Field
	sszSlice9 := buf[509:565] // c.TrailingField

	// Field 0: FieldUint32
	c.FieldUint32 = binary.LittleEndian.Uint32(sszSlice0)

	// Field 1: FieldUint64
	c.FieldUint64 = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: FieldBool
	if sszSlice2[0] > 1 {
		return ssz.ErrInvalidSerialization
	}
	if sszSlice2[0] == 1 {
		c.FieldBool = true
	} else {
		c.FieldBool = false
	}

	// Field 3: FieldBytes32
	c.FieldBytes32 = make([]byte, 0, 32)
	c.FieldBytes32 = append(c.FieldBytes32, sszSlice3...)

	// Field 4: Nested
	c.Nested = new(FixedNestedContainer)
	if err = c.Nested.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("Nested: %w", err)
	}

	// Field 5: VectorField
	{
		c.VectorField = make([]uint64, 24)
		for i := 0; i < 24; i++ {
			var tmp uint64

			tmpSlice := sszSlice5[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.VectorField[i] = tmp
		}
	}

	// Field 6: TwoDimensionBytesField
	{
		c.TwoDimensionBytesField = make([][]byte, 5)
		for i := 0; i < 5; i++ {
			var tmp []byte

			tmpSlice := sszSlice6[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.TwoDimensionBytesField[i] = tmp
		}
	}

	// Field 7: Bitvector64Field
	c.Bitvector64Field = make([]byte, 0, 8)
	c.Bitvector64Field = append(c.Bitvector64Field, go_bitfield.Bitvector64(sszSlice7)...)

	// Field 8: Bitvector512Field
	c.Bitvector512Field = make([]byte, 0, 64)
	c.Bitvector512Field = append(c.Bitvector512Field, go_bitfield.Bitvector512(sszSlice8)...)

	// Field 9: TrailingField
	c.TrailingField = make([]byte, 0, 56)
	c.TrailingField = append(c.TrailingField, sszSlice9...)
	return err
}

func (c *FixedTestContainer) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *FixedTestContainer) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: FieldUint32
	hh.PutUint32(c.FieldUint32)
	// Field 1: FieldUint64
	hh.PutUint64(c.FieldUint64)
	// Field 2: FieldBool
	hh.PutBool(c.FieldBool)
	// Field 3: FieldBytes32
	if len(c.FieldBytes32) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FieldBytes32)
	// Field 4: Nested
	if err := c.Nested.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Nested: %w", err)
	}
	// Field 5: VectorField
	{
		if len(c.VectorField) != 24 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.VectorField {
			hh.AppendUint64(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 6: TwoDimensionBytesField
	{
		if len(c.TwoDimensionBytesField) != 5 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.TwoDimensionBytesField {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 7: Bitvector64Field
	if len([]byte(c.Bitvector64Field)) != 8 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.Bitvector64Field))
	// Field 8: Bitvector512Field
	if len([]byte(c.Bitvector512Field)) != 64 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.Bitvector512Field))
	// Field 9: TrailingField
	if len(c.TrailingField) != 56 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.TrailingField)
	hh.Merkleize(indx)
	return nil
}

func (c *VariableNestedContainer) SizeSSZ() int {
	size := 16
	size += len(c.FieldListUint64) * 8
	for _, o := range c.NestedListField {
		size += 4
		size += len(o)
	}
	return size
}

func (c *VariableNestedContainer) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *VariableNestedContainer) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 16

	// Field 0: Value1
	dst = binary.LittleEndian.AppendUint64(dst, c.Value1)

	// Field 1: FieldListUint64
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.FieldListUint64) * 8

	// Field 2: NestedListField
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.NestedListField {
		offset += 4
		offset += len(o)
	}

	// Field 1: FieldListUint64
	if len(c.FieldListUint64) > 100 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.FieldListUint64 {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 2: NestedListField
	if len(c.NestedListField) > 100 {
		return nil, ssz.ErrListTooBig
	}
	{
		offset = 4 * len(c.NestedListField)
		for _, o := range c.NestedListField {
			dst = ssz.WriteOffset(dst, offset)
			offset += len(o)
		}
	}
	for _, o := range c.NestedListField {
		if len(o) > 50 {
			return nil, ssz.ErrListTooBig
		}
		dst = append(dst, o...)
	}
	return dst, err
}

func (c *VariableNestedContainer) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 16 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8] // c.Value1

	sszVarOffset1 := ssz.ReadOffset(buf[8:12]) // c.FieldListUint64
	if sszVarOffset1 != 16 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset1 > size {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[12:16]) // c.NestedListField
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.FieldListUint64
	sszSlice2 := buf[sszVarOffset2:]              // c.NestedListField

	// Field 0: Value1
	c.Value1 = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: FieldListUint64
	{
		if len(sszSlice1)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.FieldListUint64 length is %d, which is not a multiple of 8: %w", len(sszSlice1), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice1) / 8
		if numElem > 100 {
			return fmt.Errorf("ssz-max exceeded: c.FieldListUint64 has %d elements, ssz-max is 100: %w", numElem, ssz.ErrListTooBig)
		}
		c.FieldListUint64 = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice1[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.FieldListUint64[i] = tmp
		}
	}

	// Field 2: NestedListField
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice2) > 3 {
			startOffset := ssz.ReadOffset(sszSlice2[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.NestedListField")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.NestedListField, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			if listLen > 100 {
				return fmt.Errorf("ssz-max exceeded: c.NestedListField has %d elements, ssz-max is 100: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice2))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.NestedListField")
			}
			c.NestedListField = make([][]byte, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp []byte

				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice2[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.NestedListField", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.NestedListField", endOffset, startOffset)
				}
				tmpSlice = sszSlice2[startOffset:endOffset]
				tmp = append([]byte{}, tmpSlice...)
				c.NestedListField[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice2) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.NestedListField")
			}
			c.NestedListField = make([][]byte, 0)
		}
	}
	return err
}

func (c *VariableNestedContainer) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *VariableNestedContainer) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Value1
	hh.PutUint64(c.Value1)
	// Field 1: FieldListUint64
	{
		if len(c.FieldListUint64) > 100 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.FieldListUint64 {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		numItems := uint64(len(c.FieldListUint64))
		hh.MerkleizeWithMixin(subIndx, numItems, ssz.CalculateLimit(100, numItems, 8))
	}
	// Field 2: NestedListField
	{
		if len(c.NestedListField) > 100 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.NestedListField {

			{
				if len(o) > 50 {
					return ssz.ErrBytesLength
				}
				subIndx := hh.Index()
				hh.AppendBytes32(o)
				numItems := uint64(len(o))
				hh.MerkleizeWithMixin(subIndx, numItems, (50*1+31)/32)
			}

		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.NestedListField)), 100)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *VariableOuterContainer) SizeSSZ() int {
	size := 8
	if c.Inner_1 == nil {
		c.Inner_1 = new(VariableNestedContainer)
	}
	size += c.Inner_1.SizeSSZ()
	if c.Inner_2 == nil {
		c.Inner_2 = new(VariableNestedContainer)
	}
	size += c.Inner_2.SizeSSZ()
	return size
}

func (c *VariableOuterContainer) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *VariableOuterContainer) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 8

	// Field 0: Inner_1
	if c.Inner_1 == nil {
		c.Inner_1 = new(VariableNestedContainer)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Inner_1.SizeSSZ()

	// Field 1: Inner_2
	if c.Inner_2 == nil {
		c.Inner_2 = new(VariableNestedContainer)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Inner_2.SizeSSZ()

	// Field 0: Inner_1
	if dst, err = c.Inner_1.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Inner_1: %w", err)
	}

	// Field 1: Inner_2
	if dst, err = c.Inner_2.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Inner_2: %w", err)
	}
	return dst, err
}

func (c *VariableOuterContainer) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 8 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Inner_1
	if sszVarOffset0 != 8 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.Inner_2
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.Inner_1
	sszSlice1 := buf[sszVarOffset1:]              // c.Inner_2

	// Field 0: Inner_1
	c.Inner_1 = new(VariableNestedContainer)
	if err = c.Inner_1.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Inner_1: %w", err)
	}

	// Field 1: Inner_2
	c.Inner_2 = new(VariableNestedContainer)
	if err = c.Inner_2.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Inner_2: %w", err)
	}
	return err
}

func (c *VariableOuterContainer) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *VariableOuterContainer) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Inner_1
	if err := c.Inner_1.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Inner_1: %w", err)
	}
	// Field 1: Inner_2
	if err := c.Inner_2.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Inner_2: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *VariableTestContainer) SizeSSZ() int {
	size := 116
	size += len(c.FieldListUint64) * 8
	size += len(c.FieldListContainer) * 40
	size += len(c.FieldListBytes32) * 32
	if c.Nested == nil {
		c.Nested = new(VariableNestedContainer)
	}
	size += c.Nested.SizeSSZ()
	for _, o := range c.VariableContainerList {
		size += 4
		size += o.SizeSSZ()
	}
	size += len(c.BitlistField)
	for _, o := range c.NestedListField {
		size += 4
		size += len(o)
	}
	return size
}

func (c *VariableTestContainer) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *VariableTestContainer) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 116

	// Field 0: LeadingField
	if len(c.LeadingField) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LeadingField...)

	// Field 1: FieldListUint64
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.FieldListUint64) * 8

	// Field 2: FieldListContainer
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.FieldListContainer) * 40

	// Field 3: FieldListBytes32
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.FieldListBytes32) * 32

	// Field 4: Nested
	if c.Nested == nil {
		c.Nested = new(VariableNestedContainer)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Nested.SizeSSZ()

	// Field 5: VariableContainerList
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.VariableContainerList {
		offset += 4
		offset += o.SizeSSZ()
	}

	// Field 6: BitlistField
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BitlistField)

	// Field 7: NestedListField
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.NestedListField {
		offset += 4
		offset += len(o)
	}

	// Field 8: TrailingField
	if len(c.TrailingField) != 56 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.TrailingField...)

	// Field 1: FieldListUint64
	if len(c.FieldListUint64) > 2048 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.FieldListUint64 {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 2: FieldListContainer
	if len(c.FieldListContainer) > 128 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.FieldListContainer {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("FieldListContainer: %w", err)
		}
	}

	// Field 3: FieldListBytes32
	if len(c.FieldListBytes32) > 100 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.FieldListBytes32 {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 4: Nested
	if dst, err = c.Nested.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Nested: %w", err)
	}

	// Field 5: VariableContainerList
	if len(c.VariableContainerList) > 10 {
		return nil, ssz.ErrListTooBig
	}
	{
		offset = 4 * len(c.VariableContainerList)
		for _, o := range c.VariableContainerList {
			dst = ssz.WriteOffset(dst, offset)
			offset += o.SizeSSZ()
		}
	}
	for _, o := range c.VariableContainerList {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("VariableContainerList: %w", err)
		}
	}

	// Field 6: BitlistField
	if len(c.BitlistField) > 2048 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.BitlistField...)

	// Field 7: NestedListField
	if len(c.NestedListField) > 100 {
		return nil, ssz.ErrListTooBig
	}
	{
		offset = 4 * len(c.NestedListField)
		for _, o := range c.NestedListField {
			dst = ssz.WriteOffset(dst, offset)
			offset += len(o)
		}
	}
	for _, o := range c.NestedListField {
		if len(o) > 50 {
			return nil, ssz.ErrListTooBig
		}
		dst = append(dst, o...)
	}
	return dst, err
}

func (c *VariableTestContainer) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 116 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]   // c.LeadingField
	sszSlice8 := buf[60:116] // c.TrailingField

	sszVarOffset1 := ssz.ReadOffset(buf[32:36]) // c.FieldListUint64
	if sszVarOffset1 != 116 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset1 > size {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[36:40]) // c.FieldListContainer
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszVarOffset3 := ssz.ReadOffset(buf[40:44]) // c.FieldListBytes32
	if sszVarOffset3 > size || sszVarOffset3 < sszVarOffset2 {
		return ssz.ErrOffset
	}
	sszVarOffset4 := ssz.ReadOffset(buf[44:48]) // c.Nested
	if sszVarOffset4 > size || sszVarOffset4 < sszVarOffset3 {
		return ssz.ErrOffset
	}
	sszVarOffset5 := ssz.ReadOffset(buf[48:52]) // c.VariableContainerList
	if sszVarOffset5 > size || sszVarOffset5 < sszVarOffset4 {
		return ssz.ErrOffset
	}
	sszVarOffset6 := ssz.ReadOffset(buf[52:56]) // c.BitlistField
	if sszVarOffset6 > size || sszVarOffset6 < sszVarOffset5 {
		return ssz.ErrOffset
	}
	sszVarOffset7 := ssz.ReadOffset(buf[56:60]) // c.NestedListField
	if sszVarOffset7 > size || sszVarOffset7 < sszVarOffset6 {
		return ssz.ErrOffset
	}
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.FieldListUint64
	sszSlice2 := buf[sszVarOffset2:sszVarOffset3] // c.FieldListContainer
	sszSlice3 := buf[sszVarOffset3:sszVarOffset4] // c.FieldListBytes32
	sszSlice4 := buf[sszVarOffset4:sszVarOffset5] // c.Nested
	sszSlice5 := buf[sszVarOffset5:sszVarOffset6] // c.VariableContainerList
	sszSlice6 := buf[sszVarOffset6:sszVarOffset7] // c.BitlistField
	sszSlice7 := buf[sszVarOffset7:]              // c.NestedListField

	// Field 0: LeadingField
	c.LeadingField = make([]byte, 0, 32)
	c.LeadingField = append(c.LeadingField, sszSlice0...)

	// Field 1: FieldListUint64
	{
		if len(sszSlice1)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.FieldListUint64 length is %d, which is not a multiple of 8: %w", len(sszSlice1), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice1) / 8
		if numElem > 2048 {
			return fmt.Errorf("ssz-max exceeded: c.FieldListUint64 has %d elements, ssz-max is 2048: %w", numElem, ssz.ErrListTooBig)
		}
		c.FieldListUint64 = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice1[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.FieldListUint64[i] = tmp
		}
	}

	// Field 2: FieldListContainer
	{
		if len(sszSlice2)%40 != 0 {
			return fmt.Errorf("misaligned bytes: c.FieldListContainer length is %d, which is not a multiple of 40: %w", len(sszSlice2), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice2) / 40
		if numElem > 128 {
			return fmt.Errorf("ssz-max exceeded: c.FieldListContainer has %d elements, ssz-max is 128: %w", numElem, ssz.ErrListTooBig)
		}
		c.FieldListContainer = make([]*FixedNestedContainer, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *FixedNestedContainer
			tmp = new(FixedNestedContainer)
			tmpSlice := sszSlice2[i*40 : (1+i)*40]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("FieldListContainer: %w", err)
			}
			c.FieldListContainer[i] = tmp
		}
	}

	// Field 3: FieldListBytes32
	{
		if len(sszSlice3)%32 != 0 {
			return fmt.Errorf("misaligned bytes: c.FieldListBytes32 length is %d, which is not a multiple of 32: %w", len(sszSlice3), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice3) / 32
		if numElem > 100 {
			return fmt.Errorf("ssz-max exceeded: c.FieldListBytes32 has %d elements, ssz-max is 100: %w", numElem, ssz.ErrListTooBig)
		}
		c.FieldListBytes32 = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice3[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.FieldListBytes32[i] = tmp
		}
	}

	// Field 4: Nested
	c.Nested = new(VariableNestedContainer)
	if err = c.Nested.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("Nested: %w", err)
	}

	// Field 5: VariableContainerList
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice5) > 3 {
			startOffset := ssz.ReadOffset(sszSlice5[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.VariableContainerList")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.VariableContainerList, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			if listLen > 10 {
				return fmt.Errorf("ssz-max exceeded: c.VariableContainerList has %d elements, ssz-max is 10: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice5))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.VariableContainerList")
			}
			c.VariableContainerList = make([]*VariableOuterContainer, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp *VariableOuterContainer
				tmp = new(VariableOuterContainer)
				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice5[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.VariableContainerList", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.VariableContainerList", endOffset, startOffset)
				}
				tmpSlice = sszSlice5[startOffset:endOffset]
				if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
					return fmt.Errorf("VariableContainerList: %w", err)
				}
				c.VariableContainerList[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice5) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.VariableContainerList")
			}
			c.VariableContainerList = make([]*VariableOuterContainer, 0)
		}
	}

	// Field 6: BitlistField
	if err = ssz.ValidateBitlist(sszSlice6, 2048); err != nil {
		return fmt.Errorf("BitlistField: %w", err)
	}
	c.BitlistField = append([]byte{}, go_bitfield.Bitlist(sszSlice6)...)

	// Field 7: NestedListField
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice7) > 3 {
			startOffset := ssz.ReadOffset(sszSlice7[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.NestedListField")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.NestedListField, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			if listLen > 100 {
				return fmt.Errorf("ssz-max exceeded: c.NestedListField has %d elements, ssz-max is 100: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice7))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.NestedListField")
			}
			c.NestedListField = make([][]byte, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp []byte

				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice7[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.NestedListField", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.NestedListField", endOffset, startOffset)
				}
				tmpSlice = sszSlice7[startOffset:endOffset]
				tmp = append([]byte{}, tmpSlice...)
				c.NestedListField[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice7) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.NestedListField")
			}
			c.NestedListField = make([][]byte, 0)
		}
	}

	// Field 8: TrailingField
	c.TrailingField = make([]byte, 0, 56)
	c.TrailingField = append(c.TrailingField, sszSlice8...)
	return err
}

func (c *VariableTestContainer) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *VariableTestContainer) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: LeadingField
	if len(c.LeadingField) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LeadingField)
	// Field 1: FieldListUint64
	{
		if len(c.FieldListUint64) > 2048 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.FieldListUint64 {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		numItems := uint64(len(c.FieldListUint64))
		hh.MerkleizeWithMixin(subIndx, numItems, ssz.CalculateLimit(2048, numItems, 8))
	}
	// Field 2: FieldListContainer
	{
		if len(c.FieldListContainer) > 128 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.FieldListContainer {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("FieldListContainer: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.FieldListContainer)), 128)
	}
	// Field 3: FieldListBytes32
	{
		if len(c.FieldListBytes32) > 100 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.FieldListBytes32 {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.FieldListBytes32)), 100)
	}
	// Field 4: Nested
	if err := c.Nested.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Nested: %w", err)
	}
	// Field 5: VariableContainerList
	{
		if len(c.VariableContainerList) > 10 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.VariableContainerList {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("VariableContainerList: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.VariableContainerList)), 10)
	}
	// Field 6: BitlistField
	if len(c.BitlistField) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutBitlist(c.BitlistField, 2048)
	// Field 7: NestedListField
	{
		if len(c.NestedListField) > 100 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.NestedListField {

			{
				if len(o) > 50 {
					return ssz.ErrBytesLength
				}
				subIndx := hh.Index()
				hh.AppendBytes32(o)
				numItems := uint64(len(o))
				hh.MerkleizeWithMixin(subIndx, numItems, (50*1+31)/32)
			}

		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.NestedListField)), 100)
	}
	// Field 8: TrailingField
	if len(c.TrailingField) != 56 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.TrailingField)
	hh.Merkleize(indx)
	return nil
}
