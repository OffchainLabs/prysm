//go:build !minimal

package eth

import (
	binary "encoding/binary"
	"fmt"
	go_bitfield "github.com/OffchainLabs/go-bitfield"
	ssz "github.com/OffchainLabs/methodical-ssz/ssz"
	v1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

func (c *BeaconBlocksByRangeRequest) SizeSSZ() int {
	size := 24

	return size
}

func (c *BeaconBlocksByRangeRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlocksByRangeRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: StartSlot
	if dst, err = c.StartSlot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("StartSlot: %w", err)
	}

	// Field 1: Count
	dst = binary.LittleEndian.AppendUint64(dst, c.Count)

	// Field 2: Step
	dst = binary.LittleEndian.AppendUint64(dst, c.Step)

	return dst, err
}

func (c *BeaconBlocksByRangeRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 24 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.StartSlot
	sszSlice1 := buf[8:16]  // c.Count
	sszSlice2 := buf[16:24] // c.Step

	// Field 0: StartSlot
	if err = c.StartSlot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("StartSlot: %w", err)
	}

	// Field 1: Count
	c.Count = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: Step
	c.Step = binary.LittleEndian.Uint64(sszSlice2)
	return err
}

func (c *BeaconBlocksByRangeRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlocksByRangeRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: StartSlot
	if err := c.StartSlot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("StartSlot: %w", err)
	}
	// Field 1: Count
	hh.PutUint64(c.Count)
	// Field 2: Step
	hh.PutUint64(c.Step)
	hh.Merkleize(indx)
	return nil
}

func (c *BlobSidecarsByRangeRequest) SizeSSZ() int {
	size := 16

	return size
}

func (c *BlobSidecarsByRangeRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BlobSidecarsByRangeRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: StartSlot
	if dst, err = c.StartSlot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("StartSlot: %w", err)
	}

	// Field 1: Count
	dst = binary.LittleEndian.AppendUint64(dst, c.Count)

	return dst, err
}

func (c *BlobSidecarsByRangeRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 16 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.StartSlot
	sszSlice1 := buf[8:16] // c.Count

	// Field 0: StartSlot
	if err = c.StartSlot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("StartSlot: %w", err)
	}

	// Field 1: Count
	c.Count = binary.LittleEndian.Uint64(sszSlice1)
	return err
}

func (c *BlobSidecarsByRangeRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BlobSidecarsByRangeRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: StartSlot
	if err := c.StartSlot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("StartSlot: %w", err)
	}
	// Field 1: Count
	hh.PutUint64(c.Count)
	hh.Merkleize(indx)
	return nil
}

func (c *BuilderBid) SizeSSZ() int {
	size := 84
	if c.Header == nil {
		c.Header = new(v1.ExecutionPayloadHeader)
	}
	size += c.Header.SizeSSZ()
	return size
}

func (c *BuilderBid) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BuilderBid) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 84

	// Field 0: Header
	if c.Header == nil {
		c.Header = new(v1.ExecutionPayloadHeader)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Header.SizeSSZ()

	// Field 1: Value
	if len(c.Value) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Value...)

	// Field 2: Pubkey
	if len(c.Pubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey...)

	// Field 0: Header
	if dst, err = c.Header.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Header: %w", err)
	}
	return dst, err
}

func (c *BuilderBid) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 84 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:36]  // c.Value
	sszSlice2 := buf[36:84] // c.Pubkey

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Header
	if sszVarOffset0 != 84 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Header

	// Field 0: Header
	c.Header = new(v1.ExecutionPayloadHeader)
	if err = c.Header.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Header: %w", err)
	}

	// Field 1: Value
	c.Value = make([]byte, 0, 32)
	c.Value = append(c.Value, sszSlice1...)

	// Field 2: Pubkey
	c.Pubkey = make([]byte, 0, 48)
	c.Pubkey = append(c.Pubkey, sszSlice2...)
	return err
}

func (c *BuilderBid) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BuilderBid) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Header
	if err := c.Header.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Header: %w", err)
	}
	// Field 1: Value
	if len(c.Value) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Value)
	// Field 2: Pubkey
	if len(c.Pubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey)
	hh.Merkleize(indx)
	return nil
}

func (c *DataColumnSidecarsByRangeRequest) SizeSSZ() int {
	size := 20
	size += len(c.Columns) * 8
	return size
}

func (c *DataColumnSidecarsByRangeRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *DataColumnSidecarsByRangeRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 20

	// Field 0: StartSlot
	if dst, err = c.StartSlot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("StartSlot: %w", err)
	}

	// Field 1: Count
	dst = binary.LittleEndian.AppendUint64(dst, c.Count)

	// Field 2: Columns
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Columns) * 8

	// Field 2: Columns
	if len(c.Columns) > 128 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Columns {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}
	return dst, err
}

func (c *DataColumnSidecarsByRangeRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 20 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.StartSlot
	sszSlice1 := buf[8:16] // c.Count

	sszVarOffset2 := ssz.ReadOffset(buf[16:20]) // c.Columns
	if sszVarOffset2 != 20 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset2 > size {
		return ssz.ErrOffset
	}
	sszSlice2 := buf[sszVarOffset2:] // c.Columns

	// Field 0: StartSlot
	if err = c.StartSlot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("StartSlot: %w", err)
	}

	// Field 1: Count
	c.Count = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: Columns
	{
		if len(sszSlice2)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.Columns length is %d, which is not a multiple of 8: %w", len(sszSlice2), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice2) / 8
		if numElem > 128 {
			return fmt.Errorf("ssz-max exceeded: c.Columns has %d elements, ssz-max is 128: %w", numElem, ssz.ErrListTooBig)
		}
		c.Columns = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice2[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.Columns[i] = tmp
		}
	}
	return err
}

func (c *DataColumnSidecarsByRangeRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *DataColumnSidecarsByRangeRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: StartSlot
	if err := c.StartSlot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("StartSlot: %w", err)
	}
	// Field 1: Count
	hh.PutUint64(c.Count)
	// Field 2: Columns
	{
		if len(c.Columns) > 128 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Columns {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		numItems := uint64(len(c.Columns))
		hh.MerkleizeWithMixin(subIndx, numItems, ssz.CalculateLimit(128, numItems, 8))
	}
	hh.Merkleize(indx)
	return nil
}

func (c *DepositSnapshot) SizeSSZ() int {
	size := 84
	size += len(c.Finalized) * 32
	return size
}

func (c *DepositSnapshot) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *DepositSnapshot) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 84

	// Field 0: Finalized
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Finalized) * 32

	// Field 1: DepositRoot
	if len(c.DepositRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.DepositRoot...)

	// Field 2: DepositCount
	dst = binary.LittleEndian.AppendUint64(dst, c.DepositCount)

	// Field 3: ExecutionHash
	if len(c.ExecutionHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ExecutionHash...)

	// Field 4: ExecutionDepth
	dst = binary.LittleEndian.AppendUint64(dst, c.ExecutionDepth)

	// Field 0: Finalized
	if len(c.Finalized) > 32 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Finalized {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}
	return dst, err
}

func (c *DepositSnapshot) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 84 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:36]  // c.DepositRoot
	sszSlice2 := buf[36:44] // c.DepositCount
	sszSlice3 := buf[44:76] // c.ExecutionHash
	sszSlice4 := buf[76:84] // c.ExecutionDepth

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Finalized
	if sszVarOffset0 != 84 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Finalized

	// Field 0: Finalized
	{
		if len(sszSlice0)%32 != 0 {
			return fmt.Errorf("misaligned bytes: c.Finalized length is %d, which is not a multiple of 32: %w", len(sszSlice0), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice0) / 32
		if numElem > 32 {
			return fmt.Errorf("ssz-max exceeded: c.Finalized has %d elements, ssz-max is 32: %w", numElem, ssz.ErrListTooBig)
		}
		c.Finalized = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice0[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.Finalized[i] = tmp
		}
	}

	// Field 1: DepositRoot
	c.DepositRoot = make([]byte, 0, 32)
	c.DepositRoot = append(c.DepositRoot, sszSlice1...)

	// Field 2: DepositCount
	c.DepositCount = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: ExecutionHash
	c.ExecutionHash = make([]byte, 0, 32)
	c.ExecutionHash = append(c.ExecutionHash, sszSlice3...)

	// Field 4: ExecutionDepth
	c.ExecutionDepth = binary.LittleEndian.Uint64(sszSlice4)
	return err
}

func (c *DepositSnapshot) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *DepositSnapshot) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Finalized
	{
		if len(c.Finalized) > 32 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Finalized {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Finalized)), 32)
	}
	// Field 1: DepositRoot
	if len(c.DepositRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.DepositRoot)
	// Field 2: DepositCount
	hh.PutUint64(c.DepositCount)
	// Field 3: ExecutionHash
	if len(c.ExecutionHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ExecutionHash)
	// Field 4: ExecutionDepth
	hh.PutUint64(c.ExecutionDepth)
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadEnvelopesByRangeRequest) SizeSSZ() int {
	size := 16

	return size
}

func (c *ExecutionPayloadEnvelopesByRangeRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadEnvelopesByRangeRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: StartSlot
	if dst, err = c.StartSlot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("StartSlot: %w", err)
	}

	// Field 1: Count
	dst = binary.LittleEndian.AppendUint64(dst, c.Count)

	return dst, err
}

func (c *ExecutionPayloadEnvelopesByRangeRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 16 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.StartSlot
	sszSlice1 := buf[8:16] // c.Count

	// Field 0: StartSlot
	if err = c.StartSlot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("StartSlot: %w", err)
	}

	// Field 1: Count
	c.Count = binary.LittleEndian.Uint64(sszSlice1)
	return err
}

func (c *ExecutionPayloadEnvelopesByRangeRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadEnvelopesByRangeRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: StartSlot
	if err := c.StartSlot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("StartSlot: %w", err)
	}
	// Field 1: Count
	hh.PutUint64(c.Count)
	hh.Merkleize(indx)
	return nil
}

func (c *MetaDataV0) SizeSSZ() int {
	size := 16

	return size
}

func (c *MetaDataV0) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *MetaDataV0) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SeqNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.SeqNumber)

	// Field 1: Attnets
	if len([]byte(c.Attnets)) != 8 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.Attnets)...)

	return dst, err
}

func (c *MetaDataV0) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 16 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.SeqNumber
	sszSlice1 := buf[8:16] // c.Attnets

	// Field 0: SeqNumber
	c.SeqNumber = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: Attnets
	c.Attnets = make([]byte, 0, 8)
	c.Attnets = append(c.Attnets, go_bitfield.Bitvector64(sszSlice1)...)
	return err
}

func (c *MetaDataV0) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *MetaDataV0) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SeqNumber
	hh.PutUint64(c.SeqNumber)
	// Field 1: Attnets
	if len([]byte(c.Attnets)) != 8 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.Attnets))
	hh.Merkleize(indx)
	return nil
}

func (c *MetaDataV1) SizeSSZ() int {
	size := 17

	return size
}

func (c *MetaDataV1) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *MetaDataV1) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SeqNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.SeqNumber)

	// Field 1: Attnets
	if len([]byte(c.Attnets)) != 8 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.Attnets)...)

	// Field 2: Syncnets
	if len([]byte(c.Syncnets)) != 1 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.Syncnets)...)

	return dst, err
}

func (c *MetaDataV1) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 17 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.SeqNumber
	sszSlice1 := buf[8:16]  // c.Attnets
	sszSlice2 := buf[16:17] // c.Syncnets

	// Field 0: SeqNumber
	c.SeqNumber = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: Attnets
	c.Attnets = make([]byte, 0, 8)
	c.Attnets = append(c.Attnets, go_bitfield.Bitvector64(sszSlice1)...)

	// Field 2: Syncnets
	c.Syncnets = make([]byte, 0, 1)
	c.Syncnets = append(c.Syncnets, go_bitfield.Bitvector4(sszSlice2)...)
	return err
}

func (c *MetaDataV1) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *MetaDataV1) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SeqNumber
	hh.PutUint64(c.SeqNumber)
	// Field 1: Attnets
	if len([]byte(c.Attnets)) != 8 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.Attnets))
	// Field 2: Syncnets
	if len([]byte(c.Syncnets)) != 1 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.Syncnets))
	hh.Merkleize(indx)
	return nil
}

func (c *MetaDataV2) SizeSSZ() int {
	size := 25

	return size
}

func (c *MetaDataV2) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *MetaDataV2) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SeqNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.SeqNumber)

	// Field 1: Attnets
	if len([]byte(c.Attnets)) != 8 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.Attnets)...)

	// Field 2: Syncnets
	if len([]byte(c.Syncnets)) != 1 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.Syncnets)...)

	// Field 3: CustodyGroupCount
	dst = binary.LittleEndian.AppendUint64(dst, c.CustodyGroupCount)

	return dst, err
}

func (c *MetaDataV2) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 25 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.SeqNumber
	sszSlice1 := buf[8:16]  // c.Attnets
	sszSlice2 := buf[16:17] // c.Syncnets
	sszSlice3 := buf[17:25] // c.CustodyGroupCount

	// Field 0: SeqNumber
	c.SeqNumber = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: Attnets
	c.Attnets = make([]byte, 0, 8)
	c.Attnets = append(c.Attnets, go_bitfield.Bitvector64(sszSlice1)...)

	// Field 2: Syncnets
	c.Syncnets = make([]byte, 0, 1)
	c.Syncnets = append(c.Syncnets, go_bitfield.Bitvector4(sszSlice2)...)

	// Field 3: CustodyGroupCount
	c.CustodyGroupCount = binary.LittleEndian.Uint64(sszSlice3)
	return err
}

func (c *MetaDataV2) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *MetaDataV2) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SeqNumber
	hh.PutUint64(c.SeqNumber)
	// Field 1: Attnets
	if len([]byte(c.Attnets)) != 8 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.Attnets))
	// Field 2: Syncnets
	if len([]byte(c.Syncnets)) != 1 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.Syncnets))
	// Field 3: CustodyGroupCount
	hh.PutUint64(c.CustodyGroupCount)
	hh.Merkleize(indx)
	return nil
}

func (c *SignedBuilderBid) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(BuilderBid)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedBuilderBid) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedBuilderBid) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(BuilderBid)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Message.SizeSSZ()

	// Field 1: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	// Field 0: Message
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}
	return dst, err
}

func (c *SignedBuilderBid) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 100 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:100] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Message
	if sszVarOffset0 != 100 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Message

	// Field 0: Message
	c.Message = new(BuilderBid)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedBuilderBid) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedBuilderBid) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *SignedValidatorRegistrationV1) SizeSSZ() int {
	size := 180

	return size
}

func (c *SignedValidatorRegistrationV1) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedValidatorRegistrationV1) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(ValidatorRegistrationV1)
	}
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	return dst, err
}

func (c *SignedValidatorRegistrationV1) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 180 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:84]   // c.Message
	sszSlice1 := buf[84:180] // c.Signature

	// Field 0: Message
	c.Message = new(ValidatorRegistrationV1)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedValidatorRegistrationV1) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedValidatorRegistrationV1) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *ValidatorRegistrationV1) SizeSSZ() int {
	size := 84

	return size
}

func (c *ValidatorRegistrationV1) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ValidatorRegistrationV1) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient...)

	// Field 1: GasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.GasLimit)

	// Field 2: Timestamp
	dst = binary.LittleEndian.AppendUint64(dst, c.Timestamp)

	// Field 3: Pubkey
	if len(c.Pubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey...)

	return dst, err
}

func (c *ValidatorRegistrationV1) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 84 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:20]  // c.FeeRecipient
	sszSlice1 := buf[20:28] // c.GasLimit
	sszSlice2 := buf[28:36] // c.Timestamp
	sszSlice3 := buf[36:84] // c.Pubkey

	// Field 0: FeeRecipient
	c.FeeRecipient = make([]byte, 0, 20)
	c.FeeRecipient = append(c.FeeRecipient, sszSlice0...)

	// Field 1: GasLimit
	c.GasLimit = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: Timestamp
	c.Timestamp = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: Pubkey
	c.Pubkey = make([]byte, 0, 48)
	c.Pubkey = append(c.Pubkey, sszSlice3...)
	return err
}

func (c *ValidatorRegistrationV1) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ValidatorRegistrationV1) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Field 1: GasLimit
	hh.PutUint64(c.GasLimit)
	// Field 2: Timestamp
	hh.PutUint64(c.Timestamp)
	// Field 3: Pubkey
	if len(c.Pubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey)
	hh.Merkleize(indx)
	return nil
}

func (c *ValidatorIdentity) SizeSSZ() int {
	size := 64

	return size
}

func (c *ValidatorIdentity) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ValidatorIdentity) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Index
	if dst, err = c.Index.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Index: %w", err)
	}

	// Field 1: Pubkey
	if len(c.Pubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey...)

	// Field 2: ActivationEpoch
	if dst, err = c.ActivationEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ActivationEpoch: %w", err)
	}

	return dst, err
}

func (c *ValidatorIdentity) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 64 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.Index
	sszSlice1 := buf[8:56]  // c.Pubkey
	sszSlice2 := buf[56:64] // c.ActivationEpoch

	// Field 0: Index
	if err = c.Index.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Index: %w", err)
	}

	// Field 1: Pubkey
	c.Pubkey = make([]byte, 0, 48)
	c.Pubkey = append(c.Pubkey, sszSlice1...)

	// Field 2: ActivationEpoch
	if err = c.ActivationEpoch.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("ActivationEpoch: %w", err)
	}
	return err
}

func (c *ValidatorIdentity) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ValidatorIdentity) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Index
	if err := c.Index.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Index: %w", err)
	}
	// Field 1: Pubkey
	if len(c.Pubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey)
	// Field 2: ActivationEpoch
	if err := c.ActivationEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ActivationEpoch: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}
