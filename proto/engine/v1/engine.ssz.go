//go:build !minimal

package enginev1

import (
	binary "encoding/binary"
	"fmt"
	ssz "github.com/OffchainLabs/methodical-ssz/ssz"
)

func (c *BlobsBundle) SizeSSZ() int {
	size := 12
	size += len(c.KzgCommitments) * 48
	size += len(c.Proofs) * 48
	size += len(c.Blobs) * 131072
	return size
}

func (c *BlobsBundle) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BlobsBundle) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 12

	// Field 0: KzgCommitments
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.KzgCommitments) * 48

	// Field 1: Proofs
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Proofs) * 48

	// Field 2: Blobs
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Blobs) * 131072

	// Field 0: KzgCommitments
	if len(c.KzgCommitments) > 4096 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.KzgCommitments {
		if len(o) != 48 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 1: Proofs
	if len(c.Proofs) > 4096 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Proofs {
		if len(o) != 48 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 2: Blobs
	if len(c.Blobs) > 4096 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Blobs {
		if len(o) != 131072 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}
	return dst, err
}

func (c *BlobsBundle) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 12 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.KzgCommitments
	if sszVarOffset0 != 12 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.Proofs
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[8:12]) // c.Blobs
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.KzgCommitments
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.Proofs
	sszSlice2 := buf[sszVarOffset2:]              // c.Blobs

	// Field 0: KzgCommitments
	{
		if len(sszSlice0)%48 != 0 {
			return fmt.Errorf("misaligned bytes: c.KzgCommitments length is %d, which is not a multiple of 48: %w", len(sszSlice0), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice0) / 48
		if numElem > 4096 {
			return fmt.Errorf("ssz-max exceeded: c.KzgCommitments has %d elements, ssz-max is 4096: %w", numElem, ssz.ErrListTooBig)
		}
		c.KzgCommitments = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice0[i*48 : (1+i)*48]
			tmp = make([]byte, 0, 48)
			tmp = append(tmp, tmpSlice...)
			c.KzgCommitments[i] = tmp
		}
	}

	// Field 1: Proofs
	{
		if len(sszSlice1)%48 != 0 {
			return fmt.Errorf("misaligned bytes: c.Proofs length is %d, which is not a multiple of 48: %w", len(sszSlice1), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice1) / 48
		if numElem > 4096 {
			return fmt.Errorf("ssz-max exceeded: c.Proofs has %d elements, ssz-max is 4096: %w", numElem, ssz.ErrListTooBig)
		}
		c.Proofs = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice1[i*48 : (1+i)*48]
			tmp = make([]byte, 0, 48)
			tmp = append(tmp, tmpSlice...)
			c.Proofs[i] = tmp
		}
	}

	// Field 2: Blobs
	{
		if len(sszSlice2)%131072 != 0 {
			return fmt.Errorf("misaligned bytes: c.Blobs length is %d, which is not a multiple of 131072: %w", len(sszSlice2), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice2) / 131072
		if numElem > 4096 {
			return fmt.Errorf("ssz-max exceeded: c.Blobs has %d elements, ssz-max is 4096: %w", numElem, ssz.ErrListTooBig)
		}
		c.Blobs = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice2[i*131072 : (1+i)*131072]
			tmp = make([]byte, 0, 131072)
			tmp = append(tmp, tmpSlice...)
			c.Blobs[i] = tmp
		}
	}
	return err
}

func (c *BlobsBundle) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BlobsBundle) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: KzgCommitments
	{
		if len(c.KzgCommitments) > 4096 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.KzgCommitments {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.KzgCommitments)), 4096)
	}
	// Field 1: Proofs
	{
		if len(c.Proofs) > 4096 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Proofs {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Proofs)), 4096)
	}
	// Field 2: Blobs
	{
		if len(c.Blobs) > 4096 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Blobs {
			if len(o) != 131072 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Blobs)), 4096)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *BlobsBundleV2) SizeSSZ() int {
	size := 12
	size += len(c.KzgCommitments) * 48
	size += len(c.Proofs) * 48
	size += len(c.Blobs) * 131072
	return size
}

func (c *BlobsBundleV2) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BlobsBundleV2) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 12

	// Field 0: KzgCommitments
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.KzgCommitments) * 48

	// Field 1: Proofs
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Proofs) * 48

	// Field 2: Blobs
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Blobs) * 131072

	// Field 0: KzgCommitments
	if len(c.KzgCommitments) > 4096 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.KzgCommitments {
		if len(o) != 48 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 1: Proofs
	if len(c.Proofs) > 33554432 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Proofs {
		if len(o) != 48 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 2: Blobs
	if len(c.Blobs) > 4096 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Blobs {
		if len(o) != 131072 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}
	return dst, err
}

func (c *BlobsBundleV2) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 12 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.KzgCommitments
	if sszVarOffset0 != 12 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.Proofs
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[8:12]) // c.Blobs
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.KzgCommitments
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.Proofs
	sszSlice2 := buf[sszVarOffset2:]              // c.Blobs

	// Field 0: KzgCommitments
	{
		if len(sszSlice0)%48 != 0 {
			return fmt.Errorf("misaligned bytes: c.KzgCommitments length is %d, which is not a multiple of 48: %w", len(sszSlice0), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice0) / 48
		if numElem > 4096 {
			return fmt.Errorf("ssz-max exceeded: c.KzgCommitments has %d elements, ssz-max is 4096: %w", numElem, ssz.ErrListTooBig)
		}
		c.KzgCommitments = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice0[i*48 : (1+i)*48]
			tmp = make([]byte, 0, 48)
			tmp = append(tmp, tmpSlice...)
			c.KzgCommitments[i] = tmp
		}
	}

	// Field 1: Proofs
	{
		if len(sszSlice1)%48 != 0 {
			return fmt.Errorf("misaligned bytes: c.Proofs length is %d, which is not a multiple of 48: %w", len(sszSlice1), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice1) / 48
		if numElem > 33554432 {
			return fmt.Errorf("ssz-max exceeded: c.Proofs has %d elements, ssz-max is 33554432: %w", numElem, ssz.ErrListTooBig)
		}
		c.Proofs = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice1[i*48 : (1+i)*48]
			tmp = make([]byte, 0, 48)
			tmp = append(tmp, tmpSlice...)
			c.Proofs[i] = tmp
		}
	}

	// Field 2: Blobs
	{
		if len(sszSlice2)%131072 != 0 {
			return fmt.Errorf("misaligned bytes: c.Blobs length is %d, which is not a multiple of 131072: %w", len(sszSlice2), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice2) / 131072
		if numElem > 4096 {
			return fmt.Errorf("ssz-max exceeded: c.Blobs has %d elements, ssz-max is 4096: %w", numElem, ssz.ErrListTooBig)
		}
		c.Blobs = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice2[i*131072 : (1+i)*131072]
			tmp = make([]byte, 0, 131072)
			tmp = append(tmp, tmpSlice...)
			c.Blobs[i] = tmp
		}
	}
	return err
}

func (c *BlobsBundleV2) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BlobsBundleV2) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: KzgCommitments
	{
		if len(c.KzgCommitments) > 4096 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.KzgCommitments {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.KzgCommitments)), 4096)
	}
	// Field 1: Proofs
	{
		if len(c.Proofs) > 33554432 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Proofs {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Proofs)), 33554432)
	}
	// Field 2: Blobs
	{
		if len(c.Blobs) > 4096 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Blobs {
			if len(o) != 131072 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Blobs)), 4096)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *BuilderDepositRequest) SizeSSZ() int {
	size := 184

	return size
}

func (c *BuilderDepositRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BuilderDepositRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey...)

	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalCredentials...)

	// Field 2: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	// Field 3: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	return dst, err
}

func (c *BuilderDepositRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 184 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:48]   // c.Pubkey
	sszSlice1 := buf[48:80]  // c.WithdrawalCredentials
	sszSlice2 := buf[80:88]  // c.Amount
	sszSlice3 := buf[88:184] // c.Signature

	// Field 0: Pubkey
	c.Pubkey = make([]byte, 0, 48)
	c.Pubkey = append(c.Pubkey, sszSlice0...)

	// Field 1: WithdrawalCredentials
	c.WithdrawalCredentials = make([]byte, 0, 32)
	c.WithdrawalCredentials = append(c.WithdrawalCredentials, sszSlice1...)

	// Field 2: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice3...)
	return err
}

func (c *BuilderDepositRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BuilderDepositRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey)
	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalCredentials)
	// Field 2: Amount
	hh.PutUint64(c.Amount)
	// Field 3: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *BuilderExitRequest) SizeSSZ() int {
	size := 68

	return size
}

func (c *BuilderExitRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BuilderExitRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SourceAddress...)

	// Field 1: Pubkey
	if len(c.Pubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey...)

	return dst, err
}

func (c *BuilderExitRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 68 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:20]  // c.SourceAddress
	sszSlice1 := buf[20:68] // c.Pubkey

	// Field 0: SourceAddress
	c.SourceAddress = make([]byte, 0, 20)
	c.SourceAddress = append(c.SourceAddress, sszSlice0...)

	// Field 1: Pubkey
	c.Pubkey = make([]byte, 0, 48)
	c.Pubkey = append(c.Pubkey, sszSlice1...)
	return err
}

func (c *BuilderExitRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BuilderExitRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SourceAddress)
	// Field 1: Pubkey
	if len(c.Pubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey)
	hh.Merkleize(indx)
	return nil
}

func (c *ConsolidationRequest) SizeSSZ() int {
	size := 116

	return size
}

func (c *ConsolidationRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ConsolidationRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SourceAddress...)

	// Field 1: SourcePubkey
	if len(c.SourcePubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SourcePubkey...)

	// Field 2: TargetPubkey
	if len(c.TargetPubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.TargetPubkey...)

	return dst, err
}

func (c *ConsolidationRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 116 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:20]   // c.SourceAddress
	sszSlice1 := buf[20:68]  // c.SourcePubkey
	sszSlice2 := buf[68:116] // c.TargetPubkey

	// Field 0: SourceAddress
	c.SourceAddress = make([]byte, 0, 20)
	c.SourceAddress = append(c.SourceAddress, sszSlice0...)

	// Field 1: SourcePubkey
	c.SourcePubkey = make([]byte, 0, 48)
	c.SourcePubkey = append(c.SourcePubkey, sszSlice1...)

	// Field 2: TargetPubkey
	c.TargetPubkey = make([]byte, 0, 48)
	c.TargetPubkey = append(c.TargetPubkey, sszSlice2...)
	return err
}

func (c *ConsolidationRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ConsolidationRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SourceAddress)
	// Field 1: SourcePubkey
	if len(c.SourcePubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SourcePubkey)
	// Field 2: TargetPubkey
	if len(c.TargetPubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.TargetPubkey)
	hh.Merkleize(indx)
	return nil
}

func (c *DepositRequest) SizeSSZ() int {
	size := 192

	return size
}

func (c *DepositRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *DepositRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey...)

	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalCredentials...)

	// Field 2: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	// Field 3: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	// Field 4: Index
	dst = binary.LittleEndian.AppendUint64(dst, c.Index)

	return dst, err
}

func (c *DepositRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 192 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:48]    // c.Pubkey
	sszSlice1 := buf[48:80]   // c.WithdrawalCredentials
	sszSlice2 := buf[80:88]   // c.Amount
	sszSlice3 := buf[88:184]  // c.Signature
	sszSlice4 := buf[184:192] // c.Index

	// Field 0: Pubkey
	c.Pubkey = make([]byte, 0, 48)
	c.Pubkey = append(c.Pubkey, sszSlice0...)

	// Field 1: WithdrawalCredentials
	c.WithdrawalCredentials = make([]byte, 0, 32)
	c.WithdrawalCredentials = append(c.WithdrawalCredentials, sszSlice1...)

	// Field 2: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice3...)

	// Field 4: Index
	c.Index = binary.LittleEndian.Uint64(sszSlice4)
	return err
}

func (c *DepositRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *DepositRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey)
	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalCredentials)
	// Field 2: Amount
	hh.PutUint64(c.Amount)
	// Field 3: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	// Field 4: Index
	hh.PutUint64(c.Index)
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayload) SizeSSZ() int {
	size := 508
	size += len(c.ExtraData)
	for _, o := range c.Transactions {
		size += 4
		size += len(o)
	}
	return size
}

func (c *ExecutionPayload) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayload) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 508

	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentHash...)

	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient...)

	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateRoot...)

	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ReceiptsRoot...)

	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LogsBloom...)

	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PrevRandao...)

	// Field 6: BlockNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.BlockNumber)

	// Field 7: GasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.GasLimit)

	// Field 8: GasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.GasUsed)

	// Field 9: Timestamp
	dst = binary.LittleEndian.AppendUint64(dst, c.Timestamp)

	// Field 10: ExtraData
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ExtraData)

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BaseFeePerGas...)

	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash...)

	// Field 13: Transactions
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.Transactions {
		offset += 4
		offset += len(o)
	}

	// Field 10: ExtraData
	if len(c.ExtraData) > 32 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.ExtraData...)

	// Field 13: Transactions
	if len(c.Transactions) > 1048576 {
		return nil, ssz.ErrListTooBig
	}
	{
		offset = 4 * len(c.Transactions)
		for _, o := range c.Transactions {
			dst = ssz.WriteOffset(dst, offset)
			offset += len(o)
		}
	}
	for _, o := range c.Transactions {
		if len(o) > 1073741824 {
			return nil, ssz.ErrListTooBig
		}
		dst = append(dst, o...)
	}
	return dst, err
}

func (c *ExecutionPayload) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 508 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]     // c.ParentHash
	sszSlice1 := buf[32:52]    // c.FeeRecipient
	sszSlice2 := buf[52:84]    // c.StateRoot
	sszSlice3 := buf[84:116]   // c.ReceiptsRoot
	sszSlice4 := buf[116:372]  // c.LogsBloom
	sszSlice5 := buf[372:404]  // c.PrevRandao
	sszSlice6 := buf[404:412]  // c.BlockNumber
	sszSlice7 := buf[412:420]  // c.GasLimit
	sszSlice8 := buf[420:428]  // c.GasUsed
	sszSlice9 := buf[428:436]  // c.Timestamp
	sszSlice11 := buf[440:472] // c.BaseFeePerGas
	sszSlice12 := buf[472:504] // c.BlockHash

	sszVarOffset10 := ssz.ReadOffset(buf[436:440]) // c.ExtraData
	if sszVarOffset10 != 508 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset10 > size {
		return ssz.ErrOffset
	}
	sszVarOffset13 := ssz.ReadOffset(buf[504:508]) // c.Transactions
	if sszVarOffset13 > size || sszVarOffset13 < sszVarOffset10 {
		return ssz.ErrOffset
	}
	sszSlice10 := buf[sszVarOffset10:sszVarOffset13] // c.ExtraData
	sszSlice13 := buf[sszVarOffset13:]               // c.Transactions

	// Field 0: ParentHash
	c.ParentHash = make([]byte, 0, 32)
	c.ParentHash = append(c.ParentHash, sszSlice0...)

	// Field 1: FeeRecipient
	c.FeeRecipient = make([]byte, 0, 20)
	c.FeeRecipient = append(c.FeeRecipient, sszSlice1...)

	// Field 2: StateRoot
	c.StateRoot = make([]byte, 0, 32)
	c.StateRoot = append(c.StateRoot, sszSlice2...)

	// Field 3: ReceiptsRoot
	c.ReceiptsRoot = make([]byte, 0, 32)
	c.ReceiptsRoot = append(c.ReceiptsRoot, sszSlice3...)

	// Field 4: LogsBloom
	c.LogsBloom = make([]byte, 0, 256)
	c.LogsBloom = append(c.LogsBloom, sszSlice4...)

	// Field 5: PrevRandao
	c.PrevRandao = make([]byte, 0, 32)
	c.PrevRandao = append(c.PrevRandao, sszSlice5...)

	// Field 6: BlockNumber
	c.BlockNumber = binary.LittleEndian.Uint64(sszSlice6)

	// Field 7: GasLimit
	c.GasLimit = binary.LittleEndian.Uint64(sszSlice7)

	// Field 8: GasUsed
	c.GasUsed = binary.LittleEndian.Uint64(sszSlice8)

	// Field 9: Timestamp
	c.Timestamp = binary.LittleEndian.Uint64(sszSlice9)

	// Field 10: ExtraData
	c.ExtraData = append([]byte{}, sszSlice10...)

	// Field 11: BaseFeePerGas
	c.BaseFeePerGas = make([]byte, 0, 32)
	c.BaseFeePerGas = append(c.BaseFeePerGas, sszSlice11...)

	// Field 12: BlockHash
	c.BlockHash = make([]byte, 0, 32)
	c.BlockHash = append(c.BlockHash, sszSlice12...)

	// Field 13: Transactions
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice13) > 3 {
			startOffset := ssz.ReadOffset(sszSlice13[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.Transactions")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.Transactions, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			if listLen > 1048576 {
				return fmt.Errorf("ssz-max exceeded: c.Transactions has %d elements, ssz-max is 1048576: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice13))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Transactions")
			}
			c.Transactions = make([][]byte, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp []byte

				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice13[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.Transactions", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.Transactions", endOffset, startOffset)
				}
				tmpSlice = sszSlice13[startOffset:endOffset]
				tmp = append([]byte{}, tmpSlice...)
				c.Transactions[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice13) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Transactions")
			}
			c.Transactions = make([][]byte, 0)
		}
	}
	return err
}

func (c *ExecutionPayload) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayload) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentHash)
	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateRoot)
	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ReceiptsRoot)
	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LogsBloom)
	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PrevRandao)
	// Field 6: BlockNumber
	hh.PutUint64(c.BlockNumber)
	// Field 7: GasLimit
	hh.PutUint64(c.GasLimit)
	// Field 8: GasUsed
	hh.PutUint64(c.GasUsed)
	// Field 9: Timestamp
	hh.PutUint64(c.Timestamp)
	// Field 10: ExtraData

	{
		if len(c.ExtraData) > 32 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.ExtraData)
		numItems := uint64(len(c.ExtraData))
		hh.MerkleizeWithMixin(subIndx, numItems, (32*1+31)/32)
	}

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BaseFeePerGas)
	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash)
	// Field 13: Transactions
	{
		if len(c.Transactions) > 1048576 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Transactions {

			{
				if len(o) > 1073741824 {
					return ssz.ErrBytesLength
				}
				subIndx := hh.Index()
				hh.AppendBytes32(o)
				numItems := uint64(len(o))
				hh.MerkleizeWithMixin(subIndx, numItems, (1073741824*1+31)/32)
			}

		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Transactions)), 1048576)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadCapella) SizeSSZ() int {
	size := 512
	size += len(c.ExtraData)
	for _, o := range c.Transactions {
		size += 4
		size += len(o)
	}
	size += len(c.Withdrawals) * 44
	return size
}

func (c *ExecutionPayloadCapella) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadCapella) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 512

	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentHash...)

	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient...)

	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateRoot...)

	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ReceiptsRoot...)

	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LogsBloom...)

	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PrevRandao...)

	// Field 6: BlockNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.BlockNumber)

	// Field 7: GasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.GasLimit)

	// Field 8: GasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.GasUsed)

	// Field 9: Timestamp
	dst = binary.LittleEndian.AppendUint64(dst, c.Timestamp)

	// Field 10: ExtraData
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ExtraData)

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BaseFeePerGas...)

	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash...)

	// Field 13: Transactions
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.Transactions {
		offset += 4
		offset += len(o)
	}

	// Field 14: Withdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Withdrawals) * 44

	// Field 10: ExtraData
	if len(c.ExtraData) > 32 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.ExtraData...)

	// Field 13: Transactions
	if len(c.Transactions) > 1048576 {
		return nil, ssz.ErrListTooBig
	}
	{
		offset = 4 * len(c.Transactions)
		for _, o := range c.Transactions {
			dst = ssz.WriteOffset(dst, offset)
			offset += len(o)
		}
	}
	for _, o := range c.Transactions {
		if len(o) > 1073741824 {
			return nil, ssz.ErrListTooBig
		}
		dst = append(dst, o...)
	}

	// Field 14: Withdrawals
	if len(c.Withdrawals) > 16 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Withdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Withdrawals: %w", err)
		}
	}
	return dst, err
}

func (c *ExecutionPayloadCapella) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 512 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]     // c.ParentHash
	sszSlice1 := buf[32:52]    // c.FeeRecipient
	sszSlice2 := buf[52:84]    // c.StateRoot
	sszSlice3 := buf[84:116]   // c.ReceiptsRoot
	sszSlice4 := buf[116:372]  // c.LogsBloom
	sszSlice5 := buf[372:404]  // c.PrevRandao
	sszSlice6 := buf[404:412]  // c.BlockNumber
	sszSlice7 := buf[412:420]  // c.GasLimit
	sszSlice8 := buf[420:428]  // c.GasUsed
	sszSlice9 := buf[428:436]  // c.Timestamp
	sszSlice11 := buf[440:472] // c.BaseFeePerGas
	sszSlice12 := buf[472:504] // c.BlockHash

	sszVarOffset10 := ssz.ReadOffset(buf[436:440]) // c.ExtraData
	if sszVarOffset10 != 512 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset10 > size {
		return ssz.ErrOffset
	}
	sszVarOffset13 := ssz.ReadOffset(buf[504:508]) // c.Transactions
	if sszVarOffset13 > size || sszVarOffset13 < sszVarOffset10 {
		return ssz.ErrOffset
	}
	sszVarOffset14 := ssz.ReadOffset(buf[508:512]) // c.Withdrawals
	if sszVarOffset14 > size || sszVarOffset14 < sszVarOffset13 {
		return ssz.ErrOffset
	}
	sszSlice10 := buf[sszVarOffset10:sszVarOffset13] // c.ExtraData
	sszSlice13 := buf[sszVarOffset13:sszVarOffset14] // c.Transactions
	sszSlice14 := buf[sszVarOffset14:]               // c.Withdrawals

	// Field 0: ParentHash
	c.ParentHash = make([]byte, 0, 32)
	c.ParentHash = append(c.ParentHash, sszSlice0...)

	// Field 1: FeeRecipient
	c.FeeRecipient = make([]byte, 0, 20)
	c.FeeRecipient = append(c.FeeRecipient, sszSlice1...)

	// Field 2: StateRoot
	c.StateRoot = make([]byte, 0, 32)
	c.StateRoot = append(c.StateRoot, sszSlice2...)

	// Field 3: ReceiptsRoot
	c.ReceiptsRoot = make([]byte, 0, 32)
	c.ReceiptsRoot = append(c.ReceiptsRoot, sszSlice3...)

	// Field 4: LogsBloom
	c.LogsBloom = make([]byte, 0, 256)
	c.LogsBloom = append(c.LogsBloom, sszSlice4...)

	// Field 5: PrevRandao
	c.PrevRandao = make([]byte, 0, 32)
	c.PrevRandao = append(c.PrevRandao, sszSlice5...)

	// Field 6: BlockNumber
	c.BlockNumber = binary.LittleEndian.Uint64(sszSlice6)

	// Field 7: GasLimit
	c.GasLimit = binary.LittleEndian.Uint64(sszSlice7)

	// Field 8: GasUsed
	c.GasUsed = binary.LittleEndian.Uint64(sszSlice8)

	// Field 9: Timestamp
	c.Timestamp = binary.LittleEndian.Uint64(sszSlice9)

	// Field 10: ExtraData
	c.ExtraData = append([]byte{}, sszSlice10...)

	// Field 11: BaseFeePerGas
	c.BaseFeePerGas = make([]byte, 0, 32)
	c.BaseFeePerGas = append(c.BaseFeePerGas, sszSlice11...)

	// Field 12: BlockHash
	c.BlockHash = make([]byte, 0, 32)
	c.BlockHash = append(c.BlockHash, sszSlice12...)

	// Field 13: Transactions
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice13) > 3 {
			startOffset := ssz.ReadOffset(sszSlice13[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.Transactions")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.Transactions, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			if listLen > 1048576 {
				return fmt.Errorf("ssz-max exceeded: c.Transactions has %d elements, ssz-max is 1048576: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice13))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Transactions")
			}
			c.Transactions = make([][]byte, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp []byte

				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice13[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.Transactions", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.Transactions", endOffset, startOffset)
				}
				tmpSlice = sszSlice13[startOffset:endOffset]
				tmp = append([]byte{}, tmpSlice...)
				c.Transactions[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice13) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Transactions")
			}
			c.Transactions = make([][]byte, 0)
		}
	}

	// Field 14: Withdrawals
	{
		if len(sszSlice14)%44 != 0 {
			return fmt.Errorf("misaligned bytes: c.Withdrawals length is %d, which is not a multiple of 44: %w", len(sszSlice14), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice14) / 44
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.Withdrawals has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.Withdrawals = make([]*Withdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Withdrawal
			tmp = new(Withdrawal)
			tmpSlice := sszSlice14[i*44 : (1+i)*44]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
			c.Withdrawals[i] = tmp
		}
	}
	return err
}

func (c *ExecutionPayloadCapella) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadCapella) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentHash)
	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateRoot)
	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ReceiptsRoot)
	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LogsBloom)
	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PrevRandao)
	// Field 6: BlockNumber
	hh.PutUint64(c.BlockNumber)
	// Field 7: GasLimit
	hh.PutUint64(c.GasLimit)
	// Field 8: GasUsed
	hh.PutUint64(c.GasUsed)
	// Field 9: Timestamp
	hh.PutUint64(c.Timestamp)
	// Field 10: ExtraData

	{
		if len(c.ExtraData) > 32 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.ExtraData)
		numItems := uint64(len(c.ExtraData))
		hh.MerkleizeWithMixin(subIndx, numItems, (32*1+31)/32)
	}

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BaseFeePerGas)
	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash)
	// Field 13: Transactions
	{
		if len(c.Transactions) > 1048576 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Transactions {

			{
				if len(o) > 1073741824 {
					return ssz.ErrBytesLength
				}
				subIndx := hh.Index()
				hh.AppendBytes32(o)
				numItems := uint64(len(o))
				hh.MerkleizeWithMixin(subIndx, numItems, (1073741824*1+31)/32)
			}

		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Transactions)), 1048576)
	}
	// Field 14: Withdrawals
	{
		if len(c.Withdrawals) > 16 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Withdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Withdrawals)), 16)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadDeneb) SizeSSZ() int {
	size := 528
	size += len(c.ExtraData)
	for _, o := range c.Transactions {
		size += 4
		size += len(o)
	}
	size += len(c.Withdrawals) * 44
	return size
}

func (c *ExecutionPayloadDeneb) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadDeneb) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 528

	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentHash...)

	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient...)

	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateRoot...)

	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ReceiptsRoot...)

	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LogsBloom...)

	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PrevRandao...)

	// Field 6: BlockNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.BlockNumber)

	// Field 7: GasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.GasLimit)

	// Field 8: GasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.GasUsed)

	// Field 9: Timestamp
	dst = binary.LittleEndian.AppendUint64(dst, c.Timestamp)

	// Field 10: ExtraData
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ExtraData)

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BaseFeePerGas...)

	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash...)

	// Field 13: Transactions
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.Transactions {
		offset += 4
		offset += len(o)
	}

	// Field 14: Withdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Withdrawals) * 44

	// Field 15: BlobGasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.BlobGasUsed)

	// Field 16: ExcessBlobGas
	dst = binary.LittleEndian.AppendUint64(dst, c.ExcessBlobGas)

	// Field 10: ExtraData
	if len(c.ExtraData) > 32 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.ExtraData...)

	// Field 13: Transactions
	if len(c.Transactions) > 1048576 {
		return nil, ssz.ErrListTooBig
	}
	{
		offset = 4 * len(c.Transactions)
		for _, o := range c.Transactions {
			dst = ssz.WriteOffset(dst, offset)
			offset += len(o)
		}
	}
	for _, o := range c.Transactions {
		if len(o) > 1073741824 {
			return nil, ssz.ErrListTooBig
		}
		dst = append(dst, o...)
	}

	// Field 14: Withdrawals
	if len(c.Withdrawals) > 16 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Withdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Withdrawals: %w", err)
		}
	}
	return dst, err
}

func (c *ExecutionPayloadDeneb) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 528 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]     // c.ParentHash
	sszSlice1 := buf[32:52]    // c.FeeRecipient
	sszSlice2 := buf[52:84]    // c.StateRoot
	sszSlice3 := buf[84:116]   // c.ReceiptsRoot
	sszSlice4 := buf[116:372]  // c.LogsBloom
	sszSlice5 := buf[372:404]  // c.PrevRandao
	sszSlice6 := buf[404:412]  // c.BlockNumber
	sszSlice7 := buf[412:420]  // c.GasLimit
	sszSlice8 := buf[420:428]  // c.GasUsed
	sszSlice9 := buf[428:436]  // c.Timestamp
	sszSlice11 := buf[440:472] // c.BaseFeePerGas
	sszSlice12 := buf[472:504] // c.BlockHash
	sszSlice15 := buf[512:520] // c.BlobGasUsed
	sszSlice16 := buf[520:528] // c.ExcessBlobGas

	sszVarOffset10 := ssz.ReadOffset(buf[436:440]) // c.ExtraData
	if sszVarOffset10 != 528 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset10 > size {
		return ssz.ErrOffset
	}
	sszVarOffset13 := ssz.ReadOffset(buf[504:508]) // c.Transactions
	if sszVarOffset13 > size || sszVarOffset13 < sszVarOffset10 {
		return ssz.ErrOffset
	}
	sszVarOffset14 := ssz.ReadOffset(buf[508:512]) // c.Withdrawals
	if sszVarOffset14 > size || sszVarOffset14 < sszVarOffset13 {
		return ssz.ErrOffset
	}
	sszSlice10 := buf[sszVarOffset10:sszVarOffset13] // c.ExtraData
	sszSlice13 := buf[sszVarOffset13:sszVarOffset14] // c.Transactions
	sszSlice14 := buf[sszVarOffset14:]               // c.Withdrawals

	// Field 0: ParentHash
	c.ParentHash = make([]byte, 0, 32)
	c.ParentHash = append(c.ParentHash, sszSlice0...)

	// Field 1: FeeRecipient
	c.FeeRecipient = make([]byte, 0, 20)
	c.FeeRecipient = append(c.FeeRecipient, sszSlice1...)

	// Field 2: StateRoot
	c.StateRoot = make([]byte, 0, 32)
	c.StateRoot = append(c.StateRoot, sszSlice2...)

	// Field 3: ReceiptsRoot
	c.ReceiptsRoot = make([]byte, 0, 32)
	c.ReceiptsRoot = append(c.ReceiptsRoot, sszSlice3...)

	// Field 4: LogsBloom
	c.LogsBloom = make([]byte, 0, 256)
	c.LogsBloom = append(c.LogsBloom, sszSlice4...)

	// Field 5: PrevRandao
	c.PrevRandao = make([]byte, 0, 32)
	c.PrevRandao = append(c.PrevRandao, sszSlice5...)

	// Field 6: BlockNumber
	c.BlockNumber = binary.LittleEndian.Uint64(sszSlice6)

	// Field 7: GasLimit
	c.GasLimit = binary.LittleEndian.Uint64(sszSlice7)

	// Field 8: GasUsed
	c.GasUsed = binary.LittleEndian.Uint64(sszSlice8)

	// Field 9: Timestamp
	c.Timestamp = binary.LittleEndian.Uint64(sszSlice9)

	// Field 10: ExtraData
	c.ExtraData = append([]byte{}, sszSlice10...)

	// Field 11: BaseFeePerGas
	c.BaseFeePerGas = make([]byte, 0, 32)
	c.BaseFeePerGas = append(c.BaseFeePerGas, sszSlice11...)

	// Field 12: BlockHash
	c.BlockHash = make([]byte, 0, 32)
	c.BlockHash = append(c.BlockHash, sszSlice12...)

	// Field 13: Transactions
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice13) > 3 {
			startOffset := ssz.ReadOffset(sszSlice13[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.Transactions")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.Transactions, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			if listLen > 1048576 {
				return fmt.Errorf("ssz-max exceeded: c.Transactions has %d elements, ssz-max is 1048576: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice13))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Transactions")
			}
			c.Transactions = make([][]byte, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp []byte

				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice13[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.Transactions", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.Transactions", endOffset, startOffset)
				}
				tmpSlice = sszSlice13[startOffset:endOffset]
				tmp = append([]byte{}, tmpSlice...)
				c.Transactions[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice13) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Transactions")
			}
			c.Transactions = make([][]byte, 0)
		}
	}

	// Field 14: Withdrawals
	{
		if len(sszSlice14)%44 != 0 {
			return fmt.Errorf("misaligned bytes: c.Withdrawals length is %d, which is not a multiple of 44: %w", len(sszSlice14), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice14) / 44
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.Withdrawals has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.Withdrawals = make([]*Withdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Withdrawal
			tmp = new(Withdrawal)
			tmpSlice := sszSlice14[i*44 : (1+i)*44]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
			c.Withdrawals[i] = tmp
		}
	}

	// Field 15: BlobGasUsed
	c.BlobGasUsed = binary.LittleEndian.Uint64(sszSlice15)

	// Field 16: ExcessBlobGas
	c.ExcessBlobGas = binary.LittleEndian.Uint64(sszSlice16)
	return err
}

func (c *ExecutionPayloadDeneb) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadDeneb) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentHash)
	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateRoot)
	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ReceiptsRoot)
	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LogsBloom)
	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PrevRandao)
	// Field 6: BlockNumber
	hh.PutUint64(c.BlockNumber)
	// Field 7: GasLimit
	hh.PutUint64(c.GasLimit)
	// Field 8: GasUsed
	hh.PutUint64(c.GasUsed)
	// Field 9: Timestamp
	hh.PutUint64(c.Timestamp)
	// Field 10: ExtraData

	{
		if len(c.ExtraData) > 32 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.ExtraData)
		numItems := uint64(len(c.ExtraData))
		hh.MerkleizeWithMixin(subIndx, numItems, (32*1+31)/32)
	}

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BaseFeePerGas)
	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash)
	// Field 13: Transactions
	{
		if len(c.Transactions) > 1048576 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Transactions {

			{
				if len(o) > 1073741824 {
					return ssz.ErrBytesLength
				}
				subIndx := hh.Index()
				hh.AppendBytes32(o)
				numItems := uint64(len(o))
				hh.MerkleizeWithMixin(subIndx, numItems, (1073741824*1+31)/32)
			}

		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Transactions)), 1048576)
	}
	// Field 14: Withdrawals
	{
		if len(c.Withdrawals) > 16 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Withdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Withdrawals)), 16)
	}
	// Field 15: BlobGasUsed
	hh.PutUint64(c.BlobGasUsed)
	// Field 16: ExcessBlobGas
	hh.PutUint64(c.ExcessBlobGas)
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadGloas) SizeSSZ() int {
	size := 540
	size += len(c.ExtraData)
	for _, o := range c.Transactions {
		size += 4
		size += len(o)
	}
	size += len(c.Withdrawals) * 44
	size += len(c.BlockAccessList)
	return size
}

func (c *ExecutionPayloadGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 540

	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentHash...)

	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient...)

	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateRoot...)

	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ReceiptsRoot...)

	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LogsBloom...)

	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PrevRandao...)

	// Field 6: BlockNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.BlockNumber)

	// Field 7: GasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.GasLimit)

	// Field 8: GasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.GasUsed)

	// Field 9: Timestamp
	dst = binary.LittleEndian.AppendUint64(dst, c.Timestamp)

	// Field 10: ExtraData
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ExtraData)

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BaseFeePerGas...)

	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash...)

	// Field 13: Transactions
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.Transactions {
		offset += 4
		offset += len(o)
	}

	// Field 14: Withdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Withdrawals) * 44

	// Field 15: BlobGasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.BlobGasUsed)

	// Field 16: ExcessBlobGas
	dst = binary.LittleEndian.AppendUint64(dst, c.ExcessBlobGas)

	// Field 17: BlockAccessList
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BlockAccessList)

	// Field 18: SlotNumber
	if dst, err = c.SlotNumber.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SlotNumber: %w", err)
	}

	// Field 10: ExtraData
	if len(c.ExtraData) > 32 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.ExtraData...)

	// Field 13: Transactions
	if len(c.Transactions) > 1048576 {
		return nil, ssz.ErrListTooBig
	}
	{
		offset = 4 * len(c.Transactions)
		for _, o := range c.Transactions {
			dst = ssz.WriteOffset(dst, offset)
			offset += len(o)
		}
	}
	for _, o := range c.Transactions {
		if len(o) > 1073741824 {
			return nil, ssz.ErrListTooBig
		}
		dst = append(dst, o...)
	}

	// Field 14: Withdrawals
	if len(c.Withdrawals) > 16 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Withdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Withdrawals: %w", err)
		}
	}

	// Field 17: BlockAccessList
	if len(c.BlockAccessList) > 1073741824 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.BlockAccessList...)
	return dst, err
}

func (c *ExecutionPayloadGloas) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 540 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]     // c.ParentHash
	sszSlice1 := buf[32:52]    // c.FeeRecipient
	sszSlice2 := buf[52:84]    // c.StateRoot
	sszSlice3 := buf[84:116]   // c.ReceiptsRoot
	sszSlice4 := buf[116:372]  // c.LogsBloom
	sszSlice5 := buf[372:404]  // c.PrevRandao
	sszSlice6 := buf[404:412]  // c.BlockNumber
	sszSlice7 := buf[412:420]  // c.GasLimit
	sszSlice8 := buf[420:428]  // c.GasUsed
	sszSlice9 := buf[428:436]  // c.Timestamp
	sszSlice11 := buf[440:472] // c.BaseFeePerGas
	sszSlice12 := buf[472:504] // c.BlockHash
	sszSlice15 := buf[512:520] // c.BlobGasUsed
	sszSlice16 := buf[520:528] // c.ExcessBlobGas
	sszSlice18 := buf[532:540] // c.SlotNumber

	sszVarOffset10 := ssz.ReadOffset(buf[436:440]) // c.ExtraData
	if sszVarOffset10 != 540 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset10 > size {
		return ssz.ErrOffset
	}
	sszVarOffset13 := ssz.ReadOffset(buf[504:508]) // c.Transactions
	if sszVarOffset13 > size || sszVarOffset13 < sszVarOffset10 {
		return ssz.ErrOffset
	}
	sszVarOffset14 := ssz.ReadOffset(buf[508:512]) // c.Withdrawals
	if sszVarOffset14 > size || sszVarOffset14 < sszVarOffset13 {
		return ssz.ErrOffset
	}
	sszVarOffset17 := ssz.ReadOffset(buf[528:532]) // c.BlockAccessList
	if sszVarOffset17 > size || sszVarOffset17 < sszVarOffset14 {
		return ssz.ErrOffset
	}
	sszSlice10 := buf[sszVarOffset10:sszVarOffset13] // c.ExtraData
	sszSlice13 := buf[sszVarOffset13:sszVarOffset14] // c.Transactions
	sszSlice14 := buf[sszVarOffset14:sszVarOffset17] // c.Withdrawals
	sszSlice17 := buf[sszVarOffset17:]               // c.BlockAccessList

	// Field 0: ParentHash
	c.ParentHash = make([]byte, 0, 32)
	c.ParentHash = append(c.ParentHash, sszSlice0...)

	// Field 1: FeeRecipient
	c.FeeRecipient = make([]byte, 0, 20)
	c.FeeRecipient = append(c.FeeRecipient, sszSlice1...)

	// Field 2: StateRoot
	c.StateRoot = make([]byte, 0, 32)
	c.StateRoot = append(c.StateRoot, sszSlice2...)

	// Field 3: ReceiptsRoot
	c.ReceiptsRoot = make([]byte, 0, 32)
	c.ReceiptsRoot = append(c.ReceiptsRoot, sszSlice3...)

	// Field 4: LogsBloom
	c.LogsBloom = make([]byte, 0, 256)
	c.LogsBloom = append(c.LogsBloom, sszSlice4...)

	// Field 5: PrevRandao
	c.PrevRandao = make([]byte, 0, 32)
	c.PrevRandao = append(c.PrevRandao, sszSlice5...)

	// Field 6: BlockNumber
	c.BlockNumber = binary.LittleEndian.Uint64(sszSlice6)

	// Field 7: GasLimit
	c.GasLimit = binary.LittleEndian.Uint64(sszSlice7)

	// Field 8: GasUsed
	c.GasUsed = binary.LittleEndian.Uint64(sszSlice8)

	// Field 9: Timestamp
	c.Timestamp = binary.LittleEndian.Uint64(sszSlice9)

	// Field 10: ExtraData
	c.ExtraData = append([]byte{}, sszSlice10...)

	// Field 11: BaseFeePerGas
	c.BaseFeePerGas = make([]byte, 0, 32)
	c.BaseFeePerGas = append(c.BaseFeePerGas, sszSlice11...)

	// Field 12: BlockHash
	c.BlockHash = make([]byte, 0, 32)
	c.BlockHash = append(c.BlockHash, sszSlice12...)

	// Field 13: Transactions
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice13) > 3 {
			startOffset := ssz.ReadOffset(sszSlice13[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.Transactions")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.Transactions, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			if listLen > 1048576 {
				return fmt.Errorf("ssz-max exceeded: c.Transactions has %d elements, ssz-max is 1048576: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice13))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Transactions")
			}
			c.Transactions = make([][]byte, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp []byte

				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice13[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.Transactions", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.Transactions", endOffset, startOffset)
				}
				tmpSlice = sszSlice13[startOffset:endOffset]
				tmp = append([]byte{}, tmpSlice...)
				c.Transactions[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice13) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Transactions")
			}
			c.Transactions = make([][]byte, 0)
		}
	}

	// Field 14: Withdrawals
	{
		if len(sszSlice14)%44 != 0 {
			return fmt.Errorf("misaligned bytes: c.Withdrawals length is %d, which is not a multiple of 44: %w", len(sszSlice14), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice14) / 44
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.Withdrawals has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.Withdrawals = make([]*Withdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Withdrawal
			tmp = new(Withdrawal)
			tmpSlice := sszSlice14[i*44 : (1+i)*44]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
			c.Withdrawals[i] = tmp
		}
	}

	// Field 15: BlobGasUsed
	c.BlobGasUsed = binary.LittleEndian.Uint64(sszSlice15)

	// Field 16: ExcessBlobGas
	c.ExcessBlobGas = binary.LittleEndian.Uint64(sszSlice16)

	// Field 17: BlockAccessList
	c.BlockAccessList = append([]byte{}, sszSlice17...)

	// Field 18: SlotNumber
	if err = c.SlotNumber.UnmarshalSSZ(sszSlice18); err != nil {
		return fmt.Errorf("SlotNumber: %w", err)
	}
	return err
}

func (c *ExecutionPayloadGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentHash)
	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateRoot)
	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ReceiptsRoot)
	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LogsBloom)
	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PrevRandao)
	// Field 6: BlockNumber
	hh.PutUint64(c.BlockNumber)
	// Field 7: GasLimit
	hh.PutUint64(c.GasLimit)
	// Field 8: GasUsed
	hh.PutUint64(c.GasUsed)
	// Field 9: Timestamp
	hh.PutUint64(c.Timestamp)
	// Field 10: ExtraData

	{
		if len(c.ExtraData) > 32 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.ExtraData)
		numItems := uint64(len(c.ExtraData))
		hh.MerkleizeWithMixin(subIndx, numItems, (32*1+31)/32)
	}

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BaseFeePerGas)
	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash)
	// Field 13: Transactions
	{
		if len(c.Transactions) > 1048576 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Transactions {

			{
				if len(o) > 1073741824 {
					return ssz.ErrBytesLength
				}
				subIndx := hh.Index()
				hh.AppendBytes32(o)
				numItems := uint64(len(o))
				hh.MerkleizeWithMixin(subIndx, numItems, (1073741824*1+31)/32)
			}

		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Transactions)), 1048576)
	}
	// Field 14: Withdrawals
	{
		if len(c.Withdrawals) > 16 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Withdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Withdrawals)), 16)
	}
	// Field 15: BlobGasUsed
	hh.PutUint64(c.BlobGasUsed)
	// Field 16: ExcessBlobGas
	hh.PutUint64(c.ExcessBlobGas)
	// Field 17: BlockAccessList

	{
		if len(c.BlockAccessList) > 1073741824 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.BlockAccessList)
		numItems := uint64(len(c.BlockAccessList))
		hh.MerkleizeWithMixin(subIndx, numItems, (1073741824*1+31)/32)
	}

	// Field 18: SlotNumber
	if err := c.SlotNumber.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SlotNumber: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadDenebAndBlobsBundle) SizeSSZ() int {
	size := 8
	if c.Payload == nil {
		c.Payload = new(ExecutionPayloadDeneb)
	}
	size += c.Payload.SizeSSZ()
	if c.BlobsBundle == nil {
		c.BlobsBundle = new(BlobsBundle)
	}
	size += c.BlobsBundle.SizeSSZ()
	return size
}

func (c *ExecutionPayloadDenebAndBlobsBundle) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadDenebAndBlobsBundle) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 8

	// Field 0: Payload
	if c.Payload == nil {
		c.Payload = new(ExecutionPayloadDeneb)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Payload.SizeSSZ()

	// Field 1: BlobsBundle
	if c.BlobsBundle == nil {
		c.BlobsBundle = new(BlobsBundle)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.BlobsBundle.SizeSSZ()

	// Field 0: Payload
	if dst, err = c.Payload.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Payload: %w", err)
	}

	// Field 1: BlobsBundle
	if dst, err = c.BlobsBundle.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("BlobsBundle: %w", err)
	}
	return dst, err
}

func (c *ExecutionPayloadDenebAndBlobsBundle) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 8 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Payload
	if sszVarOffset0 != 8 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.BlobsBundle
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.Payload
	sszSlice1 := buf[sszVarOffset1:]              // c.BlobsBundle

	// Field 0: Payload
	c.Payload = new(ExecutionPayloadDeneb)
	if err = c.Payload.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Payload: %w", err)
	}

	// Field 1: BlobsBundle
	c.BlobsBundle = new(BlobsBundle)
	if err = c.BlobsBundle.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("BlobsBundle: %w", err)
	}
	return err
}

func (c *ExecutionPayloadDenebAndBlobsBundle) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadDenebAndBlobsBundle) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Payload
	if err := c.Payload.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Payload: %w", err)
	}
	// Field 1: BlobsBundle
	if err := c.BlobsBundle.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("BlobsBundle: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadDenebAndBlobsBundleV2) SizeSSZ() int {
	size := 8
	if c.Payload == nil {
		c.Payload = new(ExecutionPayloadDeneb)
	}
	size += c.Payload.SizeSSZ()
	if c.BlobsBundle == nil {
		c.BlobsBundle = new(BlobsBundleV2)
	}
	size += c.BlobsBundle.SizeSSZ()
	return size
}

func (c *ExecutionPayloadDenebAndBlobsBundleV2) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadDenebAndBlobsBundleV2) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 8

	// Field 0: Payload
	if c.Payload == nil {
		c.Payload = new(ExecutionPayloadDeneb)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Payload.SizeSSZ()

	// Field 1: BlobsBundle
	if c.BlobsBundle == nil {
		c.BlobsBundle = new(BlobsBundleV2)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.BlobsBundle.SizeSSZ()

	// Field 0: Payload
	if dst, err = c.Payload.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Payload: %w", err)
	}

	// Field 1: BlobsBundle
	if dst, err = c.BlobsBundle.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("BlobsBundle: %w", err)
	}
	return dst, err
}

func (c *ExecutionPayloadDenebAndBlobsBundleV2) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 8 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Payload
	if sszVarOffset0 != 8 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.BlobsBundle
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.Payload
	sszSlice1 := buf[sszVarOffset1:]              // c.BlobsBundle

	// Field 0: Payload
	c.Payload = new(ExecutionPayloadDeneb)
	if err = c.Payload.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Payload: %w", err)
	}

	// Field 1: BlobsBundle
	c.BlobsBundle = new(BlobsBundleV2)
	if err = c.BlobsBundle.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("BlobsBundle: %w", err)
	}
	return err
}

func (c *ExecutionPayloadDenebAndBlobsBundleV2) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadDenebAndBlobsBundleV2) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Payload
	if err := c.Payload.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Payload: %w", err)
	}
	// Field 1: BlobsBundle
	if err := c.BlobsBundle.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("BlobsBundle: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadHeader) SizeSSZ() int {
	size := 536
	size += len(c.ExtraData)
	return size
}

func (c *ExecutionPayloadHeader) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadHeader) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 536

	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentHash...)

	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient...)

	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateRoot...)

	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ReceiptsRoot...)

	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LogsBloom...)

	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PrevRandao...)

	// Field 6: BlockNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.BlockNumber)

	// Field 7: GasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.GasLimit)

	// Field 8: GasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.GasUsed)

	// Field 9: Timestamp
	dst = binary.LittleEndian.AppendUint64(dst, c.Timestamp)

	// Field 10: ExtraData
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ExtraData)

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BaseFeePerGas...)

	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash...)

	// Field 13: TransactionsRoot
	if len(c.TransactionsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.TransactionsRoot...)

	// Field 10: ExtraData
	if len(c.ExtraData) > 32 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.ExtraData...)
	return dst, err
}

func (c *ExecutionPayloadHeader) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 536 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]     // c.ParentHash
	sszSlice1 := buf[32:52]    // c.FeeRecipient
	sszSlice2 := buf[52:84]    // c.StateRoot
	sszSlice3 := buf[84:116]   // c.ReceiptsRoot
	sszSlice4 := buf[116:372]  // c.LogsBloom
	sszSlice5 := buf[372:404]  // c.PrevRandao
	sszSlice6 := buf[404:412]  // c.BlockNumber
	sszSlice7 := buf[412:420]  // c.GasLimit
	sszSlice8 := buf[420:428]  // c.GasUsed
	sszSlice9 := buf[428:436]  // c.Timestamp
	sszSlice11 := buf[440:472] // c.BaseFeePerGas
	sszSlice12 := buf[472:504] // c.BlockHash
	sszSlice13 := buf[504:536] // c.TransactionsRoot

	sszVarOffset10 := ssz.ReadOffset(buf[436:440]) // c.ExtraData
	if sszVarOffset10 != 536 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset10 > size {
		return ssz.ErrOffset
	}
	sszSlice10 := buf[sszVarOffset10:] // c.ExtraData

	// Field 0: ParentHash
	c.ParentHash = make([]byte, 0, 32)
	c.ParentHash = append(c.ParentHash, sszSlice0...)

	// Field 1: FeeRecipient
	c.FeeRecipient = make([]byte, 0, 20)
	c.FeeRecipient = append(c.FeeRecipient, sszSlice1...)

	// Field 2: StateRoot
	c.StateRoot = make([]byte, 0, 32)
	c.StateRoot = append(c.StateRoot, sszSlice2...)

	// Field 3: ReceiptsRoot
	c.ReceiptsRoot = make([]byte, 0, 32)
	c.ReceiptsRoot = append(c.ReceiptsRoot, sszSlice3...)

	// Field 4: LogsBloom
	c.LogsBloom = make([]byte, 0, 256)
	c.LogsBloom = append(c.LogsBloom, sszSlice4...)

	// Field 5: PrevRandao
	c.PrevRandao = make([]byte, 0, 32)
	c.PrevRandao = append(c.PrevRandao, sszSlice5...)

	// Field 6: BlockNumber
	c.BlockNumber = binary.LittleEndian.Uint64(sszSlice6)

	// Field 7: GasLimit
	c.GasLimit = binary.LittleEndian.Uint64(sszSlice7)

	// Field 8: GasUsed
	c.GasUsed = binary.LittleEndian.Uint64(sszSlice8)

	// Field 9: Timestamp
	c.Timestamp = binary.LittleEndian.Uint64(sszSlice9)

	// Field 10: ExtraData
	c.ExtraData = append([]byte{}, sszSlice10...)

	// Field 11: BaseFeePerGas
	c.BaseFeePerGas = make([]byte, 0, 32)
	c.BaseFeePerGas = append(c.BaseFeePerGas, sszSlice11...)

	// Field 12: BlockHash
	c.BlockHash = make([]byte, 0, 32)
	c.BlockHash = append(c.BlockHash, sszSlice12...)

	// Field 13: TransactionsRoot
	c.TransactionsRoot = make([]byte, 0, 32)
	c.TransactionsRoot = append(c.TransactionsRoot, sszSlice13...)
	return err
}

func (c *ExecutionPayloadHeader) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadHeader) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentHash)
	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateRoot)
	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ReceiptsRoot)
	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LogsBloom)
	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PrevRandao)
	// Field 6: BlockNumber
	hh.PutUint64(c.BlockNumber)
	// Field 7: GasLimit
	hh.PutUint64(c.GasLimit)
	// Field 8: GasUsed
	hh.PutUint64(c.GasUsed)
	// Field 9: Timestamp
	hh.PutUint64(c.Timestamp)
	// Field 10: ExtraData

	{
		if len(c.ExtraData) > 32 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.ExtraData)
		numItems := uint64(len(c.ExtraData))
		hh.MerkleizeWithMixin(subIndx, numItems, (32*1+31)/32)
	}

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BaseFeePerGas)
	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash)
	// Field 13: TransactionsRoot
	if len(c.TransactionsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.TransactionsRoot)
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadHeaderCapella) SizeSSZ() int {
	size := 568
	size += len(c.ExtraData)
	return size
}

func (c *ExecutionPayloadHeaderCapella) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadHeaderCapella) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 568

	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentHash...)

	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient...)

	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateRoot...)

	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ReceiptsRoot...)

	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LogsBloom...)

	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PrevRandao...)

	// Field 6: BlockNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.BlockNumber)

	// Field 7: GasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.GasLimit)

	// Field 8: GasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.GasUsed)

	// Field 9: Timestamp
	dst = binary.LittleEndian.AppendUint64(dst, c.Timestamp)

	// Field 10: ExtraData
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ExtraData)

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BaseFeePerGas...)

	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash...)

	// Field 13: TransactionsRoot
	if len(c.TransactionsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.TransactionsRoot...)

	// Field 14: WithdrawalsRoot
	if len(c.WithdrawalsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalsRoot...)

	// Field 10: ExtraData
	if len(c.ExtraData) > 32 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.ExtraData...)
	return dst, err
}

func (c *ExecutionPayloadHeaderCapella) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 568 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]     // c.ParentHash
	sszSlice1 := buf[32:52]    // c.FeeRecipient
	sszSlice2 := buf[52:84]    // c.StateRoot
	sszSlice3 := buf[84:116]   // c.ReceiptsRoot
	sszSlice4 := buf[116:372]  // c.LogsBloom
	sszSlice5 := buf[372:404]  // c.PrevRandao
	sszSlice6 := buf[404:412]  // c.BlockNumber
	sszSlice7 := buf[412:420]  // c.GasLimit
	sszSlice8 := buf[420:428]  // c.GasUsed
	sszSlice9 := buf[428:436]  // c.Timestamp
	sszSlice11 := buf[440:472] // c.BaseFeePerGas
	sszSlice12 := buf[472:504] // c.BlockHash
	sszSlice13 := buf[504:536] // c.TransactionsRoot
	sszSlice14 := buf[536:568] // c.WithdrawalsRoot

	sszVarOffset10 := ssz.ReadOffset(buf[436:440]) // c.ExtraData
	if sszVarOffset10 != 568 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset10 > size {
		return ssz.ErrOffset
	}
	sszSlice10 := buf[sszVarOffset10:] // c.ExtraData

	// Field 0: ParentHash
	c.ParentHash = make([]byte, 0, 32)
	c.ParentHash = append(c.ParentHash, sszSlice0...)

	// Field 1: FeeRecipient
	c.FeeRecipient = make([]byte, 0, 20)
	c.FeeRecipient = append(c.FeeRecipient, sszSlice1...)

	// Field 2: StateRoot
	c.StateRoot = make([]byte, 0, 32)
	c.StateRoot = append(c.StateRoot, sszSlice2...)

	// Field 3: ReceiptsRoot
	c.ReceiptsRoot = make([]byte, 0, 32)
	c.ReceiptsRoot = append(c.ReceiptsRoot, sszSlice3...)

	// Field 4: LogsBloom
	c.LogsBloom = make([]byte, 0, 256)
	c.LogsBloom = append(c.LogsBloom, sszSlice4...)

	// Field 5: PrevRandao
	c.PrevRandao = make([]byte, 0, 32)
	c.PrevRandao = append(c.PrevRandao, sszSlice5...)

	// Field 6: BlockNumber
	c.BlockNumber = binary.LittleEndian.Uint64(sszSlice6)

	// Field 7: GasLimit
	c.GasLimit = binary.LittleEndian.Uint64(sszSlice7)

	// Field 8: GasUsed
	c.GasUsed = binary.LittleEndian.Uint64(sszSlice8)

	// Field 9: Timestamp
	c.Timestamp = binary.LittleEndian.Uint64(sszSlice9)

	// Field 10: ExtraData
	c.ExtraData = append([]byte{}, sszSlice10...)

	// Field 11: BaseFeePerGas
	c.BaseFeePerGas = make([]byte, 0, 32)
	c.BaseFeePerGas = append(c.BaseFeePerGas, sszSlice11...)

	// Field 12: BlockHash
	c.BlockHash = make([]byte, 0, 32)
	c.BlockHash = append(c.BlockHash, sszSlice12...)

	// Field 13: TransactionsRoot
	c.TransactionsRoot = make([]byte, 0, 32)
	c.TransactionsRoot = append(c.TransactionsRoot, sszSlice13...)

	// Field 14: WithdrawalsRoot
	c.WithdrawalsRoot = make([]byte, 0, 32)
	c.WithdrawalsRoot = append(c.WithdrawalsRoot, sszSlice14...)
	return err
}

func (c *ExecutionPayloadHeaderCapella) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadHeaderCapella) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentHash)
	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateRoot)
	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ReceiptsRoot)
	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LogsBloom)
	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PrevRandao)
	// Field 6: BlockNumber
	hh.PutUint64(c.BlockNumber)
	// Field 7: GasLimit
	hh.PutUint64(c.GasLimit)
	// Field 8: GasUsed
	hh.PutUint64(c.GasUsed)
	// Field 9: Timestamp
	hh.PutUint64(c.Timestamp)
	// Field 10: ExtraData

	{
		if len(c.ExtraData) > 32 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.ExtraData)
		numItems := uint64(len(c.ExtraData))
		hh.MerkleizeWithMixin(subIndx, numItems, (32*1+31)/32)
	}

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BaseFeePerGas)
	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash)
	// Field 13: TransactionsRoot
	if len(c.TransactionsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.TransactionsRoot)
	// Field 14: WithdrawalsRoot
	if len(c.WithdrawalsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalsRoot)
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadHeaderDeneb) SizeSSZ() int {
	size := 584
	size += len(c.ExtraData)
	return size
}

func (c *ExecutionPayloadHeaderDeneb) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadHeaderDeneb) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 584

	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentHash...)

	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient...)

	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateRoot...)

	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ReceiptsRoot...)

	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LogsBloom...)

	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PrevRandao...)

	// Field 6: BlockNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.BlockNumber)

	// Field 7: GasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.GasLimit)

	// Field 8: GasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.GasUsed)

	// Field 9: Timestamp
	dst = binary.LittleEndian.AppendUint64(dst, c.Timestamp)

	// Field 10: ExtraData
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ExtraData)

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BaseFeePerGas...)

	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash...)

	// Field 13: TransactionsRoot
	if len(c.TransactionsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.TransactionsRoot...)

	// Field 14: WithdrawalsRoot
	if len(c.WithdrawalsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalsRoot...)

	// Field 15: BlobGasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.BlobGasUsed)

	// Field 16: ExcessBlobGas
	dst = binary.LittleEndian.AppendUint64(dst, c.ExcessBlobGas)

	// Field 10: ExtraData
	if len(c.ExtraData) > 32 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.ExtraData...)
	return dst, err
}

func (c *ExecutionPayloadHeaderDeneb) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 584 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]     // c.ParentHash
	sszSlice1 := buf[32:52]    // c.FeeRecipient
	sszSlice2 := buf[52:84]    // c.StateRoot
	sszSlice3 := buf[84:116]   // c.ReceiptsRoot
	sszSlice4 := buf[116:372]  // c.LogsBloom
	sszSlice5 := buf[372:404]  // c.PrevRandao
	sszSlice6 := buf[404:412]  // c.BlockNumber
	sszSlice7 := buf[412:420]  // c.GasLimit
	sszSlice8 := buf[420:428]  // c.GasUsed
	sszSlice9 := buf[428:436]  // c.Timestamp
	sszSlice11 := buf[440:472] // c.BaseFeePerGas
	sszSlice12 := buf[472:504] // c.BlockHash
	sszSlice13 := buf[504:536] // c.TransactionsRoot
	sszSlice14 := buf[536:568] // c.WithdrawalsRoot
	sszSlice15 := buf[568:576] // c.BlobGasUsed
	sszSlice16 := buf[576:584] // c.ExcessBlobGas

	sszVarOffset10 := ssz.ReadOffset(buf[436:440]) // c.ExtraData
	if sszVarOffset10 != 584 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset10 > size {
		return ssz.ErrOffset
	}
	sszSlice10 := buf[sszVarOffset10:] // c.ExtraData

	// Field 0: ParentHash
	c.ParentHash = make([]byte, 0, 32)
	c.ParentHash = append(c.ParentHash, sszSlice0...)

	// Field 1: FeeRecipient
	c.FeeRecipient = make([]byte, 0, 20)
	c.FeeRecipient = append(c.FeeRecipient, sszSlice1...)

	// Field 2: StateRoot
	c.StateRoot = make([]byte, 0, 32)
	c.StateRoot = append(c.StateRoot, sszSlice2...)

	// Field 3: ReceiptsRoot
	c.ReceiptsRoot = make([]byte, 0, 32)
	c.ReceiptsRoot = append(c.ReceiptsRoot, sszSlice3...)

	// Field 4: LogsBloom
	c.LogsBloom = make([]byte, 0, 256)
	c.LogsBloom = append(c.LogsBloom, sszSlice4...)

	// Field 5: PrevRandao
	c.PrevRandao = make([]byte, 0, 32)
	c.PrevRandao = append(c.PrevRandao, sszSlice5...)

	// Field 6: BlockNumber
	c.BlockNumber = binary.LittleEndian.Uint64(sszSlice6)

	// Field 7: GasLimit
	c.GasLimit = binary.LittleEndian.Uint64(sszSlice7)

	// Field 8: GasUsed
	c.GasUsed = binary.LittleEndian.Uint64(sszSlice8)

	// Field 9: Timestamp
	c.Timestamp = binary.LittleEndian.Uint64(sszSlice9)

	// Field 10: ExtraData
	c.ExtraData = append([]byte{}, sszSlice10...)

	// Field 11: BaseFeePerGas
	c.BaseFeePerGas = make([]byte, 0, 32)
	c.BaseFeePerGas = append(c.BaseFeePerGas, sszSlice11...)

	// Field 12: BlockHash
	c.BlockHash = make([]byte, 0, 32)
	c.BlockHash = append(c.BlockHash, sszSlice12...)

	// Field 13: TransactionsRoot
	c.TransactionsRoot = make([]byte, 0, 32)
	c.TransactionsRoot = append(c.TransactionsRoot, sszSlice13...)

	// Field 14: WithdrawalsRoot
	c.WithdrawalsRoot = make([]byte, 0, 32)
	c.WithdrawalsRoot = append(c.WithdrawalsRoot, sszSlice14...)

	// Field 15: BlobGasUsed
	c.BlobGasUsed = binary.LittleEndian.Uint64(sszSlice15)

	// Field 16: ExcessBlobGas
	c.ExcessBlobGas = binary.LittleEndian.Uint64(sszSlice16)
	return err
}

func (c *ExecutionPayloadHeaderDeneb) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadHeaderDeneb) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentHash)
	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateRoot)
	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ReceiptsRoot)
	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LogsBloom)
	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PrevRandao)
	// Field 6: BlockNumber
	hh.PutUint64(c.BlockNumber)
	// Field 7: GasLimit
	hh.PutUint64(c.GasLimit)
	// Field 8: GasUsed
	hh.PutUint64(c.GasUsed)
	// Field 9: Timestamp
	hh.PutUint64(c.Timestamp)
	// Field 10: ExtraData

	{
		if len(c.ExtraData) > 32 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.ExtraData)
		numItems := uint64(len(c.ExtraData))
		hh.MerkleizeWithMixin(subIndx, numItems, (32*1+31)/32)
	}

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BaseFeePerGas)
	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash)
	// Field 13: TransactionsRoot
	if len(c.TransactionsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.TransactionsRoot)
	// Field 14: WithdrawalsRoot
	if len(c.WithdrawalsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalsRoot)
	// Field 15: BlobGasUsed
	hh.PutUint64(c.BlobGasUsed)
	// Field 16: ExcessBlobGas
	hh.PutUint64(c.ExcessBlobGas)
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionRequests) SizeSSZ() int {
	size := 12
	size += len(c.Deposits) * 192
	size += len(c.Withdrawals) * 76
	size += len(c.Consolidations) * 116
	return size
}

func (c *ExecutionRequests) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionRequests) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 12

	// Field 0: Deposits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Deposits) * 192

	// Field 1: Withdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Withdrawals) * 76

	// Field 2: Consolidations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Consolidations) * 116

	// Field 0: Deposits
	if len(c.Deposits) > 8192 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Deposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Deposits: %w", err)
		}
	}

	// Field 1: Withdrawals
	if len(c.Withdrawals) > 16 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Withdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Withdrawals: %w", err)
		}
	}

	// Field 2: Consolidations
	if len(c.Consolidations) > 2 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Consolidations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Consolidations: %w", err)
		}
	}
	return dst, err
}

func (c *ExecutionRequests) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 12 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Deposits
	if sszVarOffset0 != 12 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.Withdrawals
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[8:12]) // c.Consolidations
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.Deposits
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.Withdrawals
	sszSlice2 := buf[sszVarOffset2:]              // c.Consolidations

	// Field 0: Deposits
	{
		if len(sszSlice0)%192 != 0 {
			return fmt.Errorf("misaligned bytes: c.Deposits length is %d, which is not a multiple of 192: %w", len(sszSlice0), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice0) / 192
		if numElem > 8192 {
			return fmt.Errorf("ssz-max exceeded: c.Deposits has %d elements, ssz-max is 8192: %w", numElem, ssz.ErrListTooBig)
		}
		c.Deposits = make([]*DepositRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *DepositRequest
			tmp = new(DepositRequest)
			tmpSlice := sszSlice0[i*192 : (1+i)*192]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
			c.Deposits[i] = tmp
		}
	}

	// Field 1: Withdrawals
	{
		if len(sszSlice1)%76 != 0 {
			return fmt.Errorf("misaligned bytes: c.Withdrawals length is %d, which is not a multiple of 76: %w", len(sszSlice1), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice1) / 76
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.Withdrawals has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.Withdrawals = make([]*WithdrawalRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *WithdrawalRequest
			tmp = new(WithdrawalRequest)
			tmpSlice := sszSlice1[i*76 : (1+i)*76]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
			c.Withdrawals[i] = tmp
		}
	}

	// Field 2: Consolidations
	{
		if len(sszSlice2)%116 != 0 {
			return fmt.Errorf("misaligned bytes: c.Consolidations length is %d, which is not a multiple of 116: %w", len(sszSlice2), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice2) / 116
		if numElem > 2 {
			return fmt.Errorf("ssz-max exceeded: c.Consolidations has %d elements, ssz-max is 2: %w", numElem, ssz.ErrListTooBig)
		}
		c.Consolidations = make([]*ConsolidationRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ConsolidationRequest
			tmp = new(ConsolidationRequest)
			tmpSlice := sszSlice2[i*116 : (1+i)*116]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Consolidations: %w", err)
			}
			c.Consolidations[i] = tmp
		}
	}
	return err
}

func (c *ExecutionRequests) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionRequests) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Deposits
	{
		if len(c.Deposits) > 8192 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Deposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Deposits)), 8192)
	}
	// Field 1: Withdrawals
	{
		if len(c.Withdrawals) > 16 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Withdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Withdrawals)), 16)
	}
	// Field 2: Consolidations
	{
		if len(c.Consolidations) > 2 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Consolidations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Consolidations: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Consolidations)), 2)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionRequestsGloas) SizeSSZ() int {
	size := 20
	size += len(c.Deposits) * 192
	size += len(c.Withdrawals) * 76
	size += len(c.Consolidations) * 116
	size += len(c.BuilderDeposits) * 184
	size += len(c.BuilderExits) * 68
	return size
}

func (c *ExecutionRequestsGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionRequestsGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 20

	// Field 0: Deposits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Deposits) * 192

	// Field 1: Withdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Withdrawals) * 76

	// Field 2: Consolidations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Consolidations) * 116

	// Field 3: BuilderDeposits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BuilderDeposits) * 184

	// Field 4: BuilderExits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BuilderExits) * 68

	// Field 0: Deposits
	if len(c.Deposits) > 8192 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Deposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Deposits: %w", err)
		}
	}

	// Field 1: Withdrawals
	if len(c.Withdrawals) > 16 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Withdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Withdrawals: %w", err)
		}
	}

	// Field 2: Consolidations
	if len(c.Consolidations) > 2 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Consolidations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Consolidations: %w", err)
		}
	}

	// Field 3: BuilderDeposits
	if len(c.BuilderDeposits) > 256 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.BuilderDeposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderDeposits: %w", err)
		}
	}

	// Field 4: BuilderExits
	if len(c.BuilderExits) > 16 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.BuilderExits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderExits: %w", err)
		}
	}
	return dst, err
}

func (c *ExecutionRequestsGloas) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 20 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Deposits
	if sszVarOffset0 != 20 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.Withdrawals
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[8:12]) // c.Consolidations
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszVarOffset3 := ssz.ReadOffset(buf[12:16]) // c.BuilderDeposits
	if sszVarOffset3 > size || sszVarOffset3 < sszVarOffset2 {
		return ssz.ErrOffset
	}
	sszVarOffset4 := ssz.ReadOffset(buf[16:20]) // c.BuilderExits
	if sszVarOffset4 > size || sszVarOffset4 < sszVarOffset3 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.Deposits
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.Withdrawals
	sszSlice2 := buf[sszVarOffset2:sszVarOffset3] // c.Consolidations
	sszSlice3 := buf[sszVarOffset3:sszVarOffset4] // c.BuilderDeposits
	sszSlice4 := buf[sszVarOffset4:]              // c.BuilderExits

	// Field 0: Deposits
	{
		if len(sszSlice0)%192 != 0 {
			return fmt.Errorf("misaligned bytes: c.Deposits length is %d, which is not a multiple of 192: %w", len(sszSlice0), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice0) / 192
		if numElem > 8192 {
			return fmt.Errorf("ssz-max exceeded: c.Deposits has %d elements, ssz-max is 8192: %w", numElem, ssz.ErrListTooBig)
		}
		c.Deposits = make([]*DepositRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *DepositRequest
			tmp = new(DepositRequest)
			tmpSlice := sszSlice0[i*192 : (1+i)*192]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
			c.Deposits[i] = tmp
		}
	}

	// Field 1: Withdrawals
	{
		if len(sszSlice1)%76 != 0 {
			return fmt.Errorf("misaligned bytes: c.Withdrawals length is %d, which is not a multiple of 76: %w", len(sszSlice1), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice1) / 76
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.Withdrawals has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.Withdrawals = make([]*WithdrawalRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *WithdrawalRequest
			tmp = new(WithdrawalRequest)
			tmpSlice := sszSlice1[i*76 : (1+i)*76]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
			c.Withdrawals[i] = tmp
		}
	}

	// Field 2: Consolidations
	{
		if len(sszSlice2)%116 != 0 {
			return fmt.Errorf("misaligned bytes: c.Consolidations length is %d, which is not a multiple of 116: %w", len(sszSlice2), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice2) / 116
		if numElem > 2 {
			return fmt.Errorf("ssz-max exceeded: c.Consolidations has %d elements, ssz-max is 2: %w", numElem, ssz.ErrListTooBig)
		}
		c.Consolidations = make([]*ConsolidationRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ConsolidationRequest
			tmp = new(ConsolidationRequest)
			tmpSlice := sszSlice2[i*116 : (1+i)*116]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Consolidations: %w", err)
			}
			c.Consolidations[i] = tmp
		}
	}

	// Field 3: BuilderDeposits
	{
		if len(sszSlice3)%184 != 0 {
			return fmt.Errorf("misaligned bytes: c.BuilderDeposits length is %d, which is not a multiple of 184: %w", len(sszSlice3), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice3) / 184
		if numElem > 256 {
			return fmt.Errorf("ssz-max exceeded: c.BuilderDeposits has %d elements, ssz-max is 256: %w", numElem, ssz.ErrListTooBig)
		}
		c.BuilderDeposits = make([]*BuilderDepositRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *BuilderDepositRequest
			tmp = new(BuilderDepositRequest)
			tmpSlice := sszSlice3[i*184 : (1+i)*184]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderDeposits: %w", err)
			}
			c.BuilderDeposits[i] = tmp
		}
	}

	// Field 4: BuilderExits
	{
		if len(sszSlice4)%68 != 0 {
			return fmt.Errorf("misaligned bytes: c.BuilderExits length is %d, which is not a multiple of 68: %w", len(sszSlice4), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice4) / 68
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.BuilderExits has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.BuilderExits = make([]*BuilderExitRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *BuilderExitRequest
			tmp = new(BuilderExitRequest)
			tmpSlice := sszSlice4[i*68 : (1+i)*68]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderExits: %w", err)
			}
			c.BuilderExits[i] = tmp
		}
	}
	return err
}

func (c *ExecutionRequestsGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionRequestsGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Deposits
	{
		if len(c.Deposits) > 8192 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Deposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Deposits)), 8192)
	}
	// Field 1: Withdrawals
	{
		if len(c.Withdrawals) > 16 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Withdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Withdrawals)), 16)
	}
	// Field 2: Consolidations
	{
		if len(c.Consolidations) > 2 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Consolidations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Consolidations: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Consolidations)), 2)
	}
	// Field 3: BuilderDeposits
	{
		if len(c.BuilderDeposits) > 256 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.BuilderDeposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderDeposits: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.BuilderDeposits)), 256)
	}
	// Field 4: BuilderExits
	{
		if len(c.BuilderExits) > 16 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.BuilderExits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderExits: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.BuilderExits)), 16)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *Withdrawal) SizeSSZ() int {
	size := 44

	return size
}

func (c *Withdrawal) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *Withdrawal) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Index
	dst = binary.LittleEndian.AppendUint64(dst, c.Index)

	// Field 1: ValidatorIndex
	if dst, err = c.ValidatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ValidatorIndex: %w", err)
	}

	// Field 2: Address
	if len(c.Address) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Address...)

	// Field 3: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	return dst, err
}

func (c *Withdrawal) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 44 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.Index
	sszSlice1 := buf[8:16]  // c.ValidatorIndex
	sszSlice2 := buf[16:36] // c.Address
	sszSlice3 := buf[36:44] // c.Amount

	// Field 0: Index
	c.Index = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: ValidatorIndex
	if err = c.ValidatorIndex.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("ValidatorIndex: %w", err)
	}

	// Field 2: Address
	c.Address = make([]byte, 0, 20)
	c.Address = append(c.Address, sszSlice2...)

	// Field 3: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice3)
	return err
}

func (c *Withdrawal) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *Withdrawal) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Index
	hh.PutUint64(c.Index)
	// Field 1: ValidatorIndex
	if err := c.ValidatorIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ValidatorIndex: %w", err)
	}
	// Field 2: Address
	if len(c.Address) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Address)
	// Field 3: Amount
	hh.PutUint64(c.Amount)
	hh.Merkleize(indx)
	return nil
}

func (c *WithdrawalRequest) SizeSSZ() int {
	size := 76

	return size
}

func (c *WithdrawalRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *WithdrawalRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SourceAddress...)

	// Field 1: ValidatorPubkey
	if len(c.ValidatorPubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ValidatorPubkey...)

	// Field 2: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	return dst, err
}

func (c *WithdrawalRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 76 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:20]  // c.SourceAddress
	sszSlice1 := buf[20:68] // c.ValidatorPubkey
	sszSlice2 := buf[68:76] // c.Amount

	// Field 0: SourceAddress
	c.SourceAddress = make([]byte, 0, 20)
	c.SourceAddress = append(c.SourceAddress, sszSlice0...)

	// Field 1: ValidatorPubkey
	c.ValidatorPubkey = make([]byte, 0, 48)
	c.ValidatorPubkey = append(c.ValidatorPubkey, sszSlice1...)

	// Field 2: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice2)
	return err
}

func (c *WithdrawalRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *WithdrawalRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SourceAddress)
	// Field 1: ValidatorPubkey
	if len(c.ValidatorPubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ValidatorPubkey)
	// Field 2: Amount
	hh.PutUint64(c.Amount)
	hh.Merkleize(indx)
	return nil
}
