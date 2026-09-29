//go:build !minimal

package ssz_query

import (
	binary "encoding/binary"
	"fmt"
	ssz "github.com/OffchainLabs/methodical-ssz/ssz"
)

func (c *SSZQueryProof) SizeSSZ() int {
	size := 44
	size += len(c.Proofs) * 32
	return size
}

func (c *SSZQueryProof) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SSZQueryProof) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 44

	// Field 0: Leaf
	if len(c.Leaf) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Leaf...)

	// Field 1: Gindex
	dst = binary.LittleEndian.AppendUint64(dst, c.Gindex)

	// Field 2: Proofs
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Proofs) * 32

	// Field 2: Proofs
	if len(c.Proofs) > 64 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Proofs {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}
	return dst, err
}

func (c *SSZQueryProof) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 44 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]  // c.Leaf
	sszSlice1 := buf[32:40] // c.Gindex

	sszVarOffset2 := ssz.ReadOffset(buf[40:44]) // c.Proofs
	if sszVarOffset2 != 44 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset2 > size {
		return ssz.ErrOffset
	}
	sszSlice2 := buf[sszVarOffset2:] // c.Proofs

	// Field 0: Leaf
	c.Leaf = make([]byte, 0, 32)
	c.Leaf = append(c.Leaf, sszSlice0...)

	// Field 1: Gindex
	c.Gindex = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: Proofs
	{
		if len(sszSlice2)%32 != 0 {
			return fmt.Errorf("misaligned bytes: c.Proofs length is %d, which is not a multiple of 32: %w", len(sszSlice2), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice2) / 32
		if numElem > 64 {
			return fmt.Errorf("ssz-max exceeded: c.Proofs has %d elements, ssz-max is 64: %w", numElem, ssz.ErrListTooBig)
		}
		c.Proofs = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice2[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.Proofs[i] = tmp
		}
	}
	return err
}

func (c *SSZQueryProof) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SSZQueryProof) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Leaf
	if len(c.Leaf) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Leaf)
	// Field 1: Gindex
	hh.PutUint64(c.Gindex)
	// Field 2: Proofs
	{
		if len(c.Proofs) > 64 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Proofs {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Proofs)), 64)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *SSZQueryResponse) SizeSSZ() int {
	size := 36
	size += len(c.Result)
	return size
}

func (c *SSZQueryResponse) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SSZQueryResponse) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 36

	// Field 0: Root
	if len(c.Root) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Root...)

	// Field 1: Result
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Result)

	// Field 1: Result
	if len(c.Result) > 1073741824 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.Result...)
	return dst, err
}

func (c *SSZQueryResponse) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 36 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32] // c.Root

	sszVarOffset1 := ssz.ReadOffset(buf[32:36]) // c.Result
	if sszVarOffset1 != 36 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset1 > size {
		return ssz.ErrOffset
	}
	sszSlice1 := buf[sszVarOffset1:] // c.Result

	// Field 0: Root
	c.Root = make([]byte, 0, 32)
	c.Root = append(c.Root, sszSlice0...)

	// Field 1: Result
	c.Result = append([]byte{}, sszSlice1...)
	return err
}

func (c *SSZQueryResponse) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SSZQueryResponse) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Root
	if len(c.Root) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Root)
	// Field 1: Result

	{
		if len(c.Result) > 1073741824 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.Result)
		numItems := uint64(len(c.Result))
		hh.MerkleizeWithMixin(subIndx, numItems, (1073741824*1+31)/32)
	}

	hh.Merkleize(indx)
	return nil
}

func (c *SSZQueryResponseWithProof) SizeSSZ() int {
	size := 40
	size += len(c.Result)
	if c.Proof == nil {
		c.Proof = new(SSZQueryProof)
	}
	size += c.Proof.SizeSSZ()
	return size
}

func (c *SSZQueryResponseWithProof) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SSZQueryResponseWithProof) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 40

	// Field 0: Root
	if len(c.Root) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Root...)

	// Field 1: Result
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Result)

	// Field 2: Proof
	if c.Proof == nil {
		c.Proof = new(SSZQueryProof)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Proof.SizeSSZ()

	// Field 1: Result
	if len(c.Result) > 1073741824 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.Result...)

	// Field 2: Proof
	if dst, err = c.Proof.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Proof: %w", err)
	}
	return dst, err
}

func (c *SSZQueryResponseWithProof) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 40 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32] // c.Root

	sszVarOffset1 := ssz.ReadOffset(buf[32:36]) // c.Result
	if sszVarOffset1 != 40 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset1 > size {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[36:40]) // c.Proof
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.Result
	sszSlice2 := buf[sszVarOffset2:]              // c.Proof

	// Field 0: Root
	c.Root = make([]byte, 0, 32)
	c.Root = append(c.Root, sszSlice0...)

	// Field 1: Result
	c.Result = append([]byte{}, sszSlice1...)

	// Field 2: Proof
	c.Proof = new(SSZQueryProof)
	if err = c.Proof.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("Proof: %w", err)
	}
	return err
}

func (c *SSZQueryResponseWithProof) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SSZQueryResponseWithProof) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Root
	if len(c.Root) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Root)
	// Field 1: Result

	{
		if len(c.Result) > 1073741824 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.Result)
		numItems := uint64(len(c.Result))
		hh.MerkleizeWithMixin(subIndx, numItems, (1073741824*1+31)/32)
	}

	// Field 2: Proof
	if err := c.Proof.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Proof: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}
