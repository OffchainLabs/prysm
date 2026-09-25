//go:build !minimal

package eth

import (
	binary "encoding/binary"
	"fmt"
	go_bitfield "github.com/OffchainLabs/go-bitfield"
	ssz "github.com/OffchainLabs/methodical-ssz/ssz"
)

func (c *AvailableAttestationData) SizeSSZ() int {
	size := 41

	return size
}

func (c *AvailableAttestationData) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AvailableAttestationData) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 1: PayloadPresent
	if c.PayloadPresent {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}

	// Field 2: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BeaconBlockRoot...)

	return dst, err
}

func (c *AvailableAttestationData) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 41 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.Slot
	sszSlice1 := buf[8:9]  // c.PayloadPresent
	sszSlice2 := buf[9:41] // c.BeaconBlockRoot

	// Field 0: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 1: PayloadPresent
	if sszSlice1[0] > 1 {
		return ssz.ErrInvalidSerialization
	}
	if sszSlice1[0] == 1 {
		c.PayloadPresent = true
	} else {
		c.PayloadPresent = false
	}

	// Field 2: BeaconBlockRoot
	c.BeaconBlockRoot = make([]byte, 0, 32)
	c.BeaconBlockRoot = append(c.BeaconBlockRoot, sszSlice2...)
	return err
}

func (c *AvailableAttestationData) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AvailableAttestationData) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 1: PayloadPresent
	hh.PutBool(c.PayloadPresent)
	// Field 2: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BeaconBlockRoot)
	hh.Merkleize(indx)
	return nil
}

func (c *AvailableAttestation) SizeSSZ() int {
	size := 201

	return size
}

func (c *AvailableAttestation) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AvailableAttestation) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: AggregationBits
	if len([]byte(c.AggregationBits)) != 64 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.AggregationBits)...)

	// Field 1: Data
	if c.Data == nil {
		c.Data = new(AvailableAttestationData)
	}
	if dst, err = c.Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	return dst, err
}

func (c *AvailableAttestation) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 201 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:64]    // c.AggregationBits
	sszSlice1 := buf[64:105]  // c.Data
	sszSlice2 := buf[105:201] // c.Signature

	// Field 0: AggregationBits
	c.AggregationBits = make([]byte, 0, 64)
	c.AggregationBits = append(c.AggregationBits, go_bitfield.Bitvector512(sszSlice0)...)

	// Field 1: Data
	c.Data = new(AvailableAttestationData)
	if err = c.Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice2...)
	return err
}

func (c *AvailableAttestation) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AvailableAttestation) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AggregationBits
	if len([]byte(c.AggregationBits)) != 64 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.AggregationBits))
	// Field 1: Data
	if err := c.Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Data: %w", err)
	}
	// Field 2: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *CheckpointDecoupled) SizeSSZ() int {
	size := 40

	return size
}

func (c *CheckpointDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *CheckpointDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 1: Root
	if len(c.Root) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Root...)

	return dst, err
}

func (c *CheckpointDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 40 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.Slot
	sszSlice1 := buf[8:40] // c.Root

	// Field 0: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 1: Root
	c.Root = make([]byte, 0, 32)
	c.Root = append(c.Root, sszSlice1...)
	return err
}

func (c *CheckpointDecoupled) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *CheckpointDecoupled) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 1: Root
	if len(c.Root) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Root)
	hh.Merkleize(indx)
	return nil
}

func (c *AttestationDataDecoupled) SizeSSZ() int {
	size := 137

	return size
}

func (c *AttestationDataDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AttestationDataDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 1: SafeBlockRoot
	if len(c.SafeBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SafeBlockRoot...)

	// Field 2: Target
	if c.Target == nil {
		c.Target = new(CheckpointDecoupled)
	}
	if dst, err = c.Target.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Target: %w", err)
	}

	// Field 3: Height
	if dst, err = c.Height.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Height: %w", err)
	}

	// Field 4: Timeout
	if c.Timeout {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}

	// Field 5: FinalityTarget
	if c.FinalityTarget == nil {
		c.FinalityTarget = new(CheckpointDecoupled)
	}
	if dst, err = c.FinalityTarget.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("FinalityTarget: %w", err)
	}

	// Field 6: FinalityHeight
	if dst, err = c.FinalityHeight.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("FinalityHeight: %w", err)
	}

	return dst, err
}

func (c *AttestationDataDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 137 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]     // c.Slot
	sszSlice1 := buf[8:40]    // c.SafeBlockRoot
	sszSlice2 := buf[40:80]   // c.Target
	sszSlice3 := buf[80:88]   // c.Height
	sszSlice4 := buf[88:89]   // c.Timeout
	sszSlice5 := buf[89:129]  // c.FinalityTarget
	sszSlice6 := buf[129:137] // c.FinalityHeight

	// Field 0: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 1: SafeBlockRoot
	c.SafeBlockRoot = make([]byte, 0, 32)
	c.SafeBlockRoot = append(c.SafeBlockRoot, sszSlice1...)

	// Field 2: Target
	c.Target = new(CheckpointDecoupled)
	if err = c.Target.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("Target: %w", err)
	}

	// Field 3: Height
	if err = c.Height.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("Height: %w", err)
	}

	// Field 4: Timeout
	if sszSlice4[0] > 1 {
		return ssz.ErrInvalidSerialization
	}
	if sszSlice4[0] == 1 {
		c.Timeout = true
	} else {
		c.Timeout = false
	}

	// Field 5: FinalityTarget
	c.FinalityTarget = new(CheckpointDecoupled)
	if err = c.FinalityTarget.UnmarshalSSZ(sszSlice5); err != nil {
		return fmt.Errorf("FinalityTarget: %w", err)
	}

	// Field 6: FinalityHeight
	if err = c.FinalityHeight.UnmarshalSSZ(sszSlice6); err != nil {
		return fmt.Errorf("FinalityHeight: %w", err)
	}
	return err
}

func (c *AttestationDataDecoupled) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AttestationDataDecoupled) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 1: SafeBlockRoot
	if len(c.SafeBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SafeBlockRoot)
	// Field 2: Target
	if err := c.Target.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Target: %w", err)
	}
	// Field 3: Height
	if err := c.Height.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Height: %w", err)
	}
	// Field 4: Timeout
	hh.PutBool(c.Timeout)
	// Field 5: FinalityTarget
	if err := c.FinalityTarget.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("FinalityTarget: %w", err)
	}
	// Field 6: FinalityHeight
	if err := c.FinalityHeight.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("FinalityHeight: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *AttestationDecoupled) SizeSSZ() int {
	size := 245
	size += len(c.AggregationBits)
	return size
}

func (c *AttestationDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AttestationDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 245

	// Field 0: AggregationBits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.AggregationBits)

	// Field 1: Data
	if c.Data == nil {
		c.Data = new(AttestationDataDecoupled)
	}
	if dst, err = c.Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	// Field 3: CommitteeBits
	if len([]byte(c.CommitteeBits)) != 8 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.CommitteeBits)...)

	// Field 0: AggregationBits
	dst = append(dst, c.AggregationBits...)
	return dst, err
}

func (c *AttestationDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 245 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:141]   // c.Data
	sszSlice2 := buf[141:237] // c.Signature
	sszSlice3 := buf[237:245] // c.CommitteeBits

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.AggregationBits
	if sszVarOffset0 != 245 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.AggregationBits

	// Field 0: AggregationBits
	if err = ssz.ValidateProgressiveBitlist(sszSlice0); err != nil {
		return fmt.Errorf("AggregationBits: %w", err)
	}
	c.AggregationBits = append([]byte{}, go_bitfield.Bitlist(sszSlice0)...)

	// Field 1: Data
	c.Data = new(AttestationDataDecoupled)
	if err = c.Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice2...)

	// Field 3: CommitteeBits
	c.CommitteeBits = make([]byte, 0, 8)
	c.CommitteeBits = append(c.CommitteeBits, go_bitfield.Bitvector64(sszSlice3)...)
	return err
}

func (c *AttestationDecoupled) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *AttestationDecoupled) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsAttestationDecoupled = []byte{0b00001111}

func (c *AttestationDecoupled) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AttestationDecoupled) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AggregationBits
	if len(c.AggregationBits) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutProgressiveBitlist(c.AggregationBits)
	// Field 1: Data
	if err := c.Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Data: %w", err)
	}
	// Field 2: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	// Field 3: CommitteeBits
	if len([]byte(c.CommitteeBits)) != 8 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.CommitteeBits))
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsAttestationDecoupled)
	return nil
}

func (c *IndexedAttestationDecoupled) SizeSSZ() int {
	size := 237
	size += len(c.AttestingIndices) * 8
	return size
}

func (c *IndexedAttestationDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *IndexedAttestationDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 237

	// Field 0: AttestingIndices
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.AttestingIndices) * 8

	// Field 1: Data
	if c.Data == nil {
		c.Data = new(AttestationDataDecoupled)
	}
	if dst, err = c.Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	// Field 0: AttestingIndices
	for _, o := range c.AttestingIndices {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}
	return dst, err
}

func (c *IndexedAttestationDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 237 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:141]   // c.Data
	sszSlice2 := buf[141:237] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.AttestingIndices
	if sszVarOffset0 != 237 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.AttestingIndices

	// Field 0: AttestingIndices
	{
		if len(sszSlice0)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.AttestingIndices length is %d, which is not a multiple of 8: %w", len(sszSlice0), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice0) / 8
		c.AttestingIndices = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice0[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.AttestingIndices[i] = tmp
		}
	}

	// Field 1: Data
	c.Data = new(AttestationDataDecoupled)
	if err = c.Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice2...)
	return err
}

func (c *IndexedAttestationDecoupled) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *IndexedAttestationDecoupled) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsIndexedAttestationDecoupled = []byte{0b00000111}

func (c *IndexedAttestationDecoupled) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *IndexedAttestationDecoupled) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AttestingIndices
	{
		subIndx := hh.Index()
		for _, o := range c.AttestingIndices {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.AttestingIndices)))
	}
	// Field 1: Data
	if err := c.Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Data: %w", err)
	}
	// Field 2: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsIndexedAttestationDecoupled)
	return nil
}

func (c *AttesterSlashingDecoupled) SizeSSZ() int {
	size := 8
	if c.Attestation_1 == nil {
		c.Attestation_1 = new(IndexedAttestationDecoupled)
	}
	size += c.Attestation_1.SizeSSZ()
	if c.Attestation_2 == nil {
		c.Attestation_2 = new(IndexedAttestationDecoupled)
	}
	size += c.Attestation_2.SizeSSZ()
	return size
}

func (c *AttesterSlashingDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AttesterSlashingDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 8

	// Field 0: Attestation_1
	if c.Attestation_1 == nil {
		c.Attestation_1 = new(IndexedAttestationDecoupled)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Attestation_1.SizeSSZ()

	// Field 1: Attestation_2
	if c.Attestation_2 == nil {
		c.Attestation_2 = new(IndexedAttestationDecoupled)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Attestation_2.SizeSSZ()

	// Field 0: Attestation_1
	if dst, err = c.Attestation_1.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Attestation_1: %w", err)
	}

	// Field 1: Attestation_2
	if dst, err = c.Attestation_2.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Attestation_2: %w", err)
	}
	return dst, err
}

func (c *AttesterSlashingDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 8 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Attestation_1
	if sszVarOffset0 != 8 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.Attestation_2
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.Attestation_1
	sszSlice1 := buf[sszVarOffset1:]              // c.Attestation_2

	// Field 0: Attestation_1
	c.Attestation_1 = new(IndexedAttestationDecoupled)
	if err = c.Attestation_1.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Attestation_1: %w", err)
	}

	// Field 1: Attestation_2
	c.Attestation_2 = new(IndexedAttestationDecoupled)
	if err = c.Attestation_2.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Attestation_2: %w", err)
	}
	return err
}

func (c *AttesterSlashingDecoupled) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AttesterSlashingDecoupled) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Attestation_1
	if err := c.Attestation_1.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Attestation_1: %w", err)
	}
	// Field 1: Attestation_2
	if err := c.Attestation_2.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Attestation_2: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *AggregateAttestationAndProofDecoupled) SizeSSZ() int {
	size := 108
	if c.Aggregate == nil {
		c.Aggregate = new(AttestationDecoupled)
	}
	size += c.Aggregate.SizeSSZ()
	return size
}

func (c *AggregateAttestationAndProofDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AggregateAttestationAndProofDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 108

	// Field 0: AggregatorIndex
	if dst, err = c.AggregatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("AggregatorIndex: %w", err)
	}

	// Field 1: Aggregate
	if c.Aggregate == nil {
		c.Aggregate = new(AttestationDecoupled)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Aggregate.SizeSSZ()

	// Field 2: SelectionProof
	if len(c.SelectionProof) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SelectionProof...)

	// Field 1: Aggregate
	if dst, err = c.Aggregate.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Aggregate: %w", err)
	}
	return dst, err
}

func (c *AggregateAttestationAndProofDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 108 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]    // c.AggregatorIndex
	sszSlice2 := buf[12:108] // c.SelectionProof

	sszVarOffset1 := ssz.ReadOffset(buf[8:12]) // c.Aggregate
	if sszVarOffset1 != 108 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset1 > size {
		return ssz.ErrOffset
	}
	sszSlice1 := buf[sszVarOffset1:] // c.Aggregate

	// Field 0: AggregatorIndex
	if err = c.AggregatorIndex.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("AggregatorIndex: %w", err)
	}

	// Field 1: Aggregate
	c.Aggregate = new(AttestationDecoupled)
	if err = c.Aggregate.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Aggregate: %w", err)
	}

	// Field 2: SelectionProof
	c.SelectionProof = make([]byte, 0, 96)
	c.SelectionProof = append(c.SelectionProof, sszSlice2...)
	return err
}

func (c *AggregateAttestationAndProofDecoupled) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AggregateAttestationAndProofDecoupled) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AggregatorIndex
	if err := c.AggregatorIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("AggregatorIndex: %w", err)
	}
	// Field 1: Aggregate
	if err := c.Aggregate.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Aggregate: %w", err)
	}
	// Field 2: SelectionProof
	if len(c.SelectionProof) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SelectionProof)
	hh.Merkleize(indx)
	return nil
}

func (c *SignedAggregateAttestationAndProofDecoupled) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(AggregateAttestationAndProofDecoupled)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedAggregateAttestationAndProofDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedAggregateAttestationAndProofDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(AggregateAttestationAndProofDecoupled)
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

func (c *SignedAggregateAttestationAndProofDecoupled) UnmarshalSSZ(buf []byte) error {
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
	c.Message = new(AggregateAttestationAndProofDecoupled)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedAggregateAttestationAndProofDecoupled) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedAggregateAttestationAndProofDecoupled) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *SingleAttestationDecoupled) SizeSSZ() int {
	size := 249

	return size
}

func (c *SingleAttestationDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SingleAttestationDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: CommitteeId
	if dst, err = c.CommitteeId.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("CommitteeId: %w", err)
	}

	// Field 1: AttesterIndex
	if dst, err = c.AttesterIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("AttesterIndex: %w", err)
	}

	// Field 2: Data
	if c.Data == nil {
		c.Data = new(AttestationDataDecoupled)
	}
	if dst, err = c.Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Data: %w", err)
	}

	// Field 3: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	return dst, err
}

func (c *SingleAttestationDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 249 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]     // c.CommitteeId
	sszSlice1 := buf[8:16]    // c.AttesterIndex
	sszSlice2 := buf[16:153]  // c.Data
	sszSlice3 := buf[153:249] // c.Signature

	// Field 0: CommitteeId
	if err = c.CommitteeId.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("CommitteeId: %w", err)
	}

	// Field 1: AttesterIndex
	if err = c.AttesterIndex.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("AttesterIndex: %w", err)
	}

	// Field 2: Data
	c.Data = new(AttestationDataDecoupled)
	if err = c.Data.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 3: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice3...)
	return err
}

func (c *SingleAttestationDecoupled) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SingleAttestationDecoupled) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: CommitteeId
	if err := c.CommitteeId.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("CommitteeId: %w", err)
	}
	// Field 1: AttesterIndex
	if err := c.AttesterIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("AttesterIndex: %w", err)
	}
	// Field 2: Data
	if err := c.Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Data: %w", err)
	}
	// Field 3: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}
