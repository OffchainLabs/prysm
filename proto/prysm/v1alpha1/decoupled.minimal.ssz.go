//go:build minimal

package eth

import (
	binary "encoding/binary"
	"fmt"
	go_bitfield "github.com/OffchainLabs/go-bitfield"
	ssz "github.com/OffchainLabs/methodical-ssz/ssz"
	primitives "github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	v1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
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
	size := 238
	size += len(c.AggregationBits)
	return size
}

func (c *AttestationDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AttestationDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 238

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
	if len([]byte(c.CommitteeBits)) != 1 {
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
	if size < 238 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:141]   // c.Data
	sszSlice2 := buf[141:237] // c.Signature
	sszSlice3 := buf[237:238] // c.CommitteeBits

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.AggregationBits
	if sszVarOffset0 != 238 {
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
	c.CommitteeBits = make([]byte, 0, 1)
	c.CommitteeBits = append(c.CommitteeBits, go_bitfield.Bitvector4(sszSlice3)...)
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
	if len([]byte(c.CommitteeBits)) != 1 {
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

func (c *AvailableCommittee) SizeSSZ() int {
	size := 4096

	return size
}

func (c *AvailableCommittee) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AvailableCommittee) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: ValidatorIndices
	if len(c.ValidatorIndices) != 512 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.ValidatorIndices {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("ValidatorIndices: %w", err)
		}
	}

	return dst, err
}

func (c *AvailableCommittee) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 4096 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:4096] // c.ValidatorIndices

	// Field 0: ValidatorIndices
	{
		c.ValidatorIndices = make([]primitives.ValidatorIndex, 512)
		for i := 0; i < 512; i++ {
			var tmp primitives.ValidatorIndex

			tmpSlice := sszSlice0[i*8 : (1+i)*8]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("ValidatorIndices: %w", err)
			}
			c.ValidatorIndices[i] = tmp
		}
	}
	return err
}

func (c *AvailableCommittee) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AvailableCommittee) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ValidatorIndices
	{
		if len(c.ValidatorIndices) != 512 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.ValidatorIndices {
			hh.AppendUint64(uint64(o))
		}
		hh.Merkleize(subIndx)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *BuilderPendingPaymentDecoupled) SizeSSZ() int {
	size := 172

	return size
}

func (c *BuilderPendingPaymentDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BuilderPendingPaymentDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Weight
	if dst, err = c.Weight.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Weight: %w", err)
	}

	// Field 1: AvailableParticipation
	if len([]byte(c.AvailableParticipation)) != 64 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.AvailableParticipation)...)

	// Field 2: TimelyHeadParticipation
	if len([]byte(c.TimelyHeadParticipation)) != 64 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.TimelyHeadParticipation)...)

	// Field 3: Withdrawal
	if c.Withdrawal == nil {
		c.Withdrawal = new(BuilderPendingWithdrawal)
	}
	if dst, err = c.Withdrawal.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Withdrawal: %w", err)
	}

	return dst, err
}

func (c *BuilderPendingPaymentDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 172 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]     // c.Weight
	sszSlice1 := buf[8:72]    // c.AvailableParticipation
	sszSlice2 := buf[72:136]  // c.TimelyHeadParticipation
	sszSlice3 := buf[136:172] // c.Withdrawal

	// Field 0: Weight
	if err = c.Weight.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Weight: %w", err)
	}

	// Field 1: AvailableParticipation
	c.AvailableParticipation = make([]byte, 0, 64)
	c.AvailableParticipation = append(c.AvailableParticipation, go_bitfield.Bitvector512(sszSlice1)...)

	// Field 2: TimelyHeadParticipation
	c.TimelyHeadParticipation = make([]byte, 0, 64)
	c.TimelyHeadParticipation = append(c.TimelyHeadParticipation, go_bitfield.Bitvector512(sszSlice2)...)

	// Field 3: Withdrawal
	c.Withdrawal = new(BuilderPendingWithdrawal)
	if err = c.Withdrawal.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("Withdrawal: %w", err)
	}
	return err
}

func (c *BuilderPendingPaymentDecoupled) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BuilderPendingPaymentDecoupled) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Weight
	if err := c.Weight.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Weight: %w", err)
	}
	// Field 1: AvailableParticipation
	if len([]byte(c.AvailableParticipation)) != 64 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.AvailableParticipation))
	// Field 2: TimelyHeadParticipation
	if len([]byte(c.TimelyHeadParticipation)) != 64 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.TimelyHeadParticipation))
	// Field 3: Withdrawal
	if err := c.Withdrawal.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Withdrawal: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *BeaconStateDecoupled) SizeSSZ() int {
	size := 114665
	size += len(c.HistoricalRoots) * 32
	size += len(c.Eth1DataVotes) * 72
	size += len(c.Validators) * 121
	size += len(c.Balances) * 8
	size += len(c.PreviousRoundParticipation)
	size += len(c.CurrentRoundParticipation)
	size += len(c.InactivityScores) * 8
	if c.LatestExecutionPayloadBid == nil {
		c.LatestExecutionPayloadBid = new(ExecutionPayloadBid)
	}
	size += c.LatestExecutionPayloadBid.SizeSSZ()
	size += len(c.HistoricalSummaries) * 64
	size += len(c.PendingDeposits) * 192
	size += len(c.PendingPartialWithdrawals) * 24
	size += len(c.PendingConsolidations) * 16
	size += len(c.Builders) * 93
	size += len(c.BuilderPendingWithdrawals) * 36
	size += len(c.PayloadExpectedWithdrawals) * 44
	size += len(c.TargetParticipation)
	size += len(c.Progress)
	size += len(c.FinalityParticipation)
	return size
}

func (c *BeaconStateDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconStateDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 114665

	// Field 0: GenesisTime
	dst = binary.LittleEndian.AppendUint64(dst, c.GenesisTime)

	// Field 1: GenesisValidatorsRoot
	if len(c.GenesisValidatorsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.GenesisValidatorsRoot...)

	// Field 2: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 3: Fork
	if c.Fork == nil {
		c.Fork = new(Fork)
	}
	if dst, err = c.Fork.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Fork: %w", err)
	}

	// Field 4: LatestBlockHeader
	if c.LatestBlockHeader == nil {
		c.LatestBlockHeader = new(BeaconBlockHeader)
	}
	if dst, err = c.LatestBlockHeader.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("LatestBlockHeader: %w", err)
	}

	// Field 5: BlockRoots
	if len(c.BlockRoots) != 64 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.BlockRoots {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 6: StateRoots
	if len(c.StateRoots) != 64 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.StateRoots {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 7: HistoricalRoots
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.HistoricalRoots) * 32

	// Field 8: Eth1Data
	if c.Eth1Data == nil {
		c.Eth1Data = new(Eth1Data)
	}
	if dst, err = c.Eth1Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Eth1Data: %w", err)
	}

	// Field 9: Eth1DataVotes
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Eth1DataVotes) * 72

	// Field 10: Eth1DepositIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.Eth1DepositIndex)

	// Field 11: Validators
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Validators) * 121

	// Field 12: Balances
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Balances) * 8

	// Field 13: RandaoMixes
	if len(c.RandaoMixes) != 64 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.RandaoMixes {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 14: Slashings
	if len(c.Slashings) != 64 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.Slashings {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 15: PreviousRoundParticipation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PreviousRoundParticipation)

	// Field 16: CurrentRoundParticipation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.CurrentRoundParticipation)

	// Field 17: JustifiedCheckpoint
	if c.JustifiedCheckpoint == nil {
		c.JustifiedCheckpoint = new(CheckpointDecoupled)
	}
	if dst, err = c.JustifiedCheckpoint.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("JustifiedCheckpoint: %w", err)
	}

	// Field 18: FinalizedCheckpoint
	if c.FinalizedCheckpoint == nil {
		c.FinalizedCheckpoint = new(CheckpointDecoupled)
	}
	if dst, err = c.FinalizedCheckpoint.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("FinalizedCheckpoint: %w", err)
	}

	// Field 19: InactivityScores
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.InactivityScores) * 8

	// Field 20: CurrentSyncCommittee
	if c.CurrentSyncCommittee == nil {
		c.CurrentSyncCommittee = new(SyncCommittee)
	}
	if dst, err = c.CurrentSyncCommittee.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("CurrentSyncCommittee: %w", err)
	}

	// Field 21: NextSyncCommittee
	if c.NextSyncCommittee == nil {
		c.NextSyncCommittee = new(SyncCommittee)
	}
	if dst, err = c.NextSyncCommittee.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextSyncCommittee: %w", err)
	}

	// Field 22: LatestExecutionPayloadBid
	if c.LatestExecutionPayloadBid == nil {
		c.LatestExecutionPayloadBid = new(ExecutionPayloadBid)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.LatestExecutionPayloadBid.SizeSSZ()

	// Field 23: NextWithdrawalIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.NextWithdrawalIndex)

	// Field 24: NextWithdrawalValidatorIndex
	if dst, err = c.NextWithdrawalValidatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextWithdrawalValidatorIndex: %w", err)
	}

	// Field 25: HistoricalSummaries
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.HistoricalSummaries) * 64

	// Field 26: DepositRequestsStartIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.DepositRequestsStartIndex)

	// Field 27: DepositBalanceToConsume
	if dst, err = c.DepositBalanceToConsume.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("DepositBalanceToConsume: %w", err)
	}

	// Field 28: ExitBalanceToConsume
	if dst, err = c.ExitBalanceToConsume.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ExitBalanceToConsume: %w", err)
	}

	// Field 29: EarliestExitEpoch
	if dst, err = c.EarliestExitEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("EarliestExitEpoch: %w", err)
	}

	// Field 30: ConsolidationBalanceToConsume
	if dst, err = c.ConsolidationBalanceToConsume.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ConsolidationBalanceToConsume: %w", err)
	}

	// Field 31: EarliestConsolidationEpoch
	if dst, err = c.EarliestConsolidationEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("EarliestConsolidationEpoch: %w", err)
	}

	// Field 32: PendingDeposits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingDeposits) * 192

	// Field 33: PendingPartialWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingPartialWithdrawals) * 24

	// Field 34: PendingConsolidations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingConsolidations) * 16

	// Field 35: ProposerLookahead
	if len(c.ProposerLookahead) != 16 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.ProposerLookahead {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("ProposerLookahead: %w", err)
		}
	}

	// Field 36: Builders
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Builders) * 93

	// Field 37: NextWithdrawalBuilderIndex
	if dst, err = c.NextWithdrawalBuilderIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextWithdrawalBuilderIndex: %w", err)
	}

	// Field 38: ExecutionPayloadAvailability
	if len(c.ExecutionPayloadAvailability) != 8 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ExecutionPayloadAvailability...)

	// Field 39: BuilderPendingPayments
	if len(c.BuilderPendingPayments) != 16 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.BuilderPendingPayments {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderPendingPayments: %w", err)
		}
	}

	// Field 40: BuilderPendingWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BuilderPendingWithdrawals) * 36

	// Field 41: LatestBlockHash
	if len(c.LatestBlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LatestBlockHash...)

	// Field 42: PayloadExpectedWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PayloadExpectedWithdrawals) * 44

	// Field 43: PtcWindow
	if len(c.PtcWindow) != 24 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.PtcWindow {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PtcWindow: %w", err)
		}
	}

	// Field 44: AvailableCommitteeWindow
	if len(c.AvailableCommitteeWindow) != 24 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.AvailableCommitteeWindow {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("AvailableCommitteeWindow: %w", err)
		}
	}

	// Field 45: JustifiedHeight
	if dst, err = c.JustifiedHeight.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("JustifiedHeight: %w", err)
	}

	// Field 46: FinalizedHeight
	if dst, err = c.FinalizedHeight.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("FinalizedHeight: %w", err)
	}

	// Field 47: CurrentHeight
	if dst, err = c.CurrentHeight.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("CurrentHeight: %w", err)
	}

	// Field 48: CurrentHeightNonjustifiable
	if c.CurrentHeightNonjustifiable {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}

	// Field 49: CurrentHeightTarget
	if c.CurrentHeightTarget == nil {
		c.CurrentHeightTarget = new(CheckpointDecoupled)
	}
	if dst, err = c.CurrentHeightTarget.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("CurrentHeightTarget: %w", err)
	}

	// Field 50: TargetParticipation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.TargetParticipation)

	// Field 51: Progress
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Progress)

	// Field 52: FinalityParticipation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.FinalityParticipation)

	// Field 7: HistoricalRoots
	if len(c.HistoricalRoots) > 16777216 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.HistoricalRoots {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 9: Eth1DataVotes
	if len(c.Eth1DataVotes) > 32 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Eth1DataVotes {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Eth1DataVotes: %w", err)
		}
	}

	// Field 11: Validators
	for _, o := range c.Validators {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Validators: %w", err)
		}
	}

	// Field 12: Balances
	for _, o := range c.Balances {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 15: PreviousRoundParticipation
	dst = append(dst, c.PreviousRoundParticipation...)

	// Field 16: CurrentRoundParticipation
	dst = append(dst, c.CurrentRoundParticipation...)

	// Field 19: InactivityScores
	for _, o := range c.InactivityScores {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 22: LatestExecutionPayloadBid
	if dst, err = c.LatestExecutionPayloadBid.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}

	// Field 25: HistoricalSummaries
	if len(c.HistoricalSummaries) > 16777216 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.HistoricalSummaries {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("HistoricalSummaries: %w", err)
		}
	}

	// Field 32: PendingDeposits
	for _, o := range c.PendingDeposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingDeposits: %w", err)
		}
	}

	// Field 33: PendingPartialWithdrawals
	for _, o := range c.PendingPartialWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingPartialWithdrawals: %w", err)
		}
	}

	// Field 34: PendingConsolidations
	for _, o := range c.PendingConsolidations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingConsolidations: %w", err)
		}
	}

	// Field 36: Builders
	for _, o := range c.Builders {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Builders: %w", err)
		}
	}

	// Field 40: BuilderPendingWithdrawals
	for _, o := range c.BuilderPendingWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderPendingWithdrawals: %w", err)
		}
	}

	// Field 42: PayloadExpectedWithdrawals
	for _, o := range c.PayloadExpectedWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
		}
	}

	// Field 50: TargetParticipation
	dst = append(dst, c.TargetParticipation...)

	// Field 51: Progress
	dst = append(dst, c.Progress...)

	// Field 52: FinalityParticipation
	dst = append(dst, c.FinalityParticipation...)
	return dst, err
}

func (c *BeaconStateDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 114665 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]            // c.GenesisTime
	sszSlice1 := buf[8:40]           // c.GenesisValidatorsRoot
	sszSlice2 := buf[40:48]          // c.Slot
	sszSlice3 := buf[48:64]          // c.Fork
	sszSlice4 := buf[64:176]         // c.LatestBlockHeader
	sszSlice5 := buf[176:2224]       // c.BlockRoots
	sszSlice6 := buf[2224:4272]      // c.StateRoots
	sszSlice8 := buf[4276:4348]      // c.Eth1Data
	sszSlice10 := buf[4352:4360]     // c.Eth1DepositIndex
	sszSlice13 := buf[4368:6416]     // c.RandaoMixes
	sszSlice14 := buf[6416:6928]     // c.Slashings
	sszSlice17 := buf[6936:6976]     // c.JustifiedCheckpoint
	sszSlice18 := buf[6976:7016]     // c.FinalizedCheckpoint
	sszSlice20 := buf[7020:8604]     // c.CurrentSyncCommittee
	sszSlice21 := buf[8604:10188]    // c.NextSyncCommittee
	sszSlice23 := buf[10192:10200]   // c.NextWithdrawalIndex
	sszSlice24 := buf[10200:10208]   // c.NextWithdrawalValidatorIndex
	sszSlice26 := buf[10212:10220]   // c.DepositRequestsStartIndex
	sszSlice27 := buf[10220:10228]   // c.DepositBalanceToConsume
	sszSlice28 := buf[10228:10236]   // c.ExitBalanceToConsume
	sszSlice29 := buf[10236:10244]   // c.EarliestExitEpoch
	sszSlice30 := buf[10244:10252]   // c.ConsolidationBalanceToConsume
	sszSlice31 := buf[10252:10260]   // c.EarliestConsolidationEpoch
	sszSlice35 := buf[10272:10400]   // c.ProposerLookahead
	sszSlice37 := buf[10404:10412]   // c.NextWithdrawalBuilderIndex
	sszSlice38 := buf[10412:10420]   // c.ExecutionPayloadAvailability
	sszSlice39 := buf[10420:13172]   // c.BuilderPendingPayments
	sszSlice41 := buf[13176:13208]   // c.LatestBlockHash
	sszSlice43 := buf[13212:16284]   // c.PtcWindow
	sszSlice44 := buf[16284:114588]  // c.AvailableCommitteeWindow
	sszSlice45 := buf[114588:114596] // c.JustifiedHeight
	sszSlice46 := buf[114596:114604] // c.FinalizedHeight
	sszSlice47 := buf[114604:114612] // c.CurrentHeight
	sszSlice48 := buf[114612:114613] // c.CurrentHeightNonjustifiable
	sszSlice49 := buf[114613:114653] // c.CurrentHeightTarget

	sszVarOffset7 := ssz.ReadOffset(buf[4272:4276]) // c.HistoricalRoots
	if sszVarOffset7 != 114665 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset7 > size {
		return ssz.ErrOffset
	}
	sszVarOffset9 := ssz.ReadOffset(buf[4348:4352]) // c.Eth1DataVotes
	if sszVarOffset9 > size || sszVarOffset9 < sszVarOffset7 {
		return ssz.ErrOffset
	}
	sszVarOffset11 := ssz.ReadOffset(buf[4360:4364]) // c.Validators
	if sszVarOffset11 > size || sszVarOffset11 < sszVarOffset9 {
		return ssz.ErrOffset
	}
	sszVarOffset12 := ssz.ReadOffset(buf[4364:4368]) // c.Balances
	if sszVarOffset12 > size || sszVarOffset12 < sszVarOffset11 {
		return ssz.ErrOffset
	}
	sszVarOffset15 := ssz.ReadOffset(buf[6928:6932]) // c.PreviousRoundParticipation
	if sszVarOffset15 > size || sszVarOffset15 < sszVarOffset12 {
		return ssz.ErrOffset
	}
	sszVarOffset16 := ssz.ReadOffset(buf[6932:6936]) // c.CurrentRoundParticipation
	if sszVarOffset16 > size || sszVarOffset16 < sszVarOffset15 {
		return ssz.ErrOffset
	}
	sszVarOffset19 := ssz.ReadOffset(buf[7016:7020]) // c.InactivityScores
	if sszVarOffset19 > size || sszVarOffset19 < sszVarOffset16 {
		return ssz.ErrOffset
	}
	sszVarOffset22 := ssz.ReadOffset(buf[10188:10192]) // c.LatestExecutionPayloadBid
	if sszVarOffset22 > size || sszVarOffset22 < sszVarOffset19 {
		return ssz.ErrOffset
	}
	sszVarOffset25 := ssz.ReadOffset(buf[10208:10212]) // c.HistoricalSummaries
	if sszVarOffset25 > size || sszVarOffset25 < sszVarOffset22 {
		return ssz.ErrOffset
	}
	sszVarOffset32 := ssz.ReadOffset(buf[10260:10264]) // c.PendingDeposits
	if sszVarOffset32 > size || sszVarOffset32 < sszVarOffset25 {
		return ssz.ErrOffset
	}
	sszVarOffset33 := ssz.ReadOffset(buf[10264:10268]) // c.PendingPartialWithdrawals
	if sszVarOffset33 > size || sszVarOffset33 < sszVarOffset32 {
		return ssz.ErrOffset
	}
	sszVarOffset34 := ssz.ReadOffset(buf[10268:10272]) // c.PendingConsolidations
	if sszVarOffset34 > size || sszVarOffset34 < sszVarOffset33 {
		return ssz.ErrOffset
	}
	sszVarOffset36 := ssz.ReadOffset(buf[10400:10404]) // c.Builders
	if sszVarOffset36 > size || sszVarOffset36 < sszVarOffset34 {
		return ssz.ErrOffset
	}
	sszVarOffset40 := ssz.ReadOffset(buf[13172:13176]) // c.BuilderPendingWithdrawals
	if sszVarOffset40 > size || sszVarOffset40 < sszVarOffset36 {
		return ssz.ErrOffset
	}
	sszVarOffset42 := ssz.ReadOffset(buf[13208:13212]) // c.PayloadExpectedWithdrawals
	if sszVarOffset42 > size || sszVarOffset42 < sszVarOffset40 {
		return ssz.ErrOffset
	}
	sszVarOffset50 := ssz.ReadOffset(buf[114653:114657]) // c.TargetParticipation
	if sszVarOffset50 > size || sszVarOffset50 < sszVarOffset42 {
		return ssz.ErrOffset
	}
	sszVarOffset51 := ssz.ReadOffset(buf[114657:114661]) // c.Progress
	if sszVarOffset51 > size || sszVarOffset51 < sszVarOffset50 {
		return ssz.ErrOffset
	}
	sszVarOffset52 := ssz.ReadOffset(buf[114661:114665]) // c.FinalityParticipation
	if sszVarOffset52 > size || sszVarOffset52 < sszVarOffset51 {
		return ssz.ErrOffset
	}
	sszSlice7 := buf[sszVarOffset7:sszVarOffset9]    // c.HistoricalRoots
	sszSlice9 := buf[sszVarOffset9:sszVarOffset11]   // c.Eth1DataVotes
	sszSlice11 := buf[sszVarOffset11:sszVarOffset12] // c.Validators
	sszSlice12 := buf[sszVarOffset12:sszVarOffset15] // c.Balances
	sszSlice15 := buf[sszVarOffset15:sszVarOffset16] // c.PreviousRoundParticipation
	sszSlice16 := buf[sszVarOffset16:sszVarOffset19] // c.CurrentRoundParticipation
	sszSlice19 := buf[sszVarOffset19:sszVarOffset22] // c.InactivityScores
	sszSlice22 := buf[sszVarOffset22:sszVarOffset25] // c.LatestExecutionPayloadBid
	sszSlice25 := buf[sszVarOffset25:sszVarOffset32] // c.HistoricalSummaries
	sszSlice32 := buf[sszVarOffset32:sszVarOffset33] // c.PendingDeposits
	sszSlice33 := buf[sszVarOffset33:sszVarOffset34] // c.PendingPartialWithdrawals
	sszSlice34 := buf[sszVarOffset34:sszVarOffset36] // c.PendingConsolidations
	sszSlice36 := buf[sszVarOffset36:sszVarOffset40] // c.Builders
	sszSlice40 := buf[sszVarOffset40:sszVarOffset42] // c.BuilderPendingWithdrawals
	sszSlice42 := buf[sszVarOffset42:sszVarOffset50] // c.PayloadExpectedWithdrawals
	sszSlice50 := buf[sszVarOffset50:sszVarOffset51] // c.TargetParticipation
	sszSlice51 := buf[sszVarOffset51:sszVarOffset52] // c.Progress
	sszSlice52 := buf[sszVarOffset52:]               // c.FinalityParticipation

	// Field 0: GenesisTime
	c.GenesisTime = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: GenesisValidatorsRoot
	c.GenesisValidatorsRoot = make([]byte, 0, 32)
	c.GenesisValidatorsRoot = append(c.GenesisValidatorsRoot, sszSlice1...)

	// Field 2: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 3: Fork
	c.Fork = new(Fork)
	if err = c.Fork.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("Fork: %w", err)
	}

	// Field 4: LatestBlockHeader
	c.LatestBlockHeader = new(BeaconBlockHeader)
	if err = c.LatestBlockHeader.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("LatestBlockHeader: %w", err)
	}

	// Field 5: BlockRoots
	{
		c.BlockRoots = make([][]byte, 64)
		for i := 0; i < 64; i++ {
			var tmp []byte

			tmpSlice := sszSlice5[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.BlockRoots[i] = tmp
		}
	}

	// Field 6: StateRoots
	{
		c.StateRoots = make([][]byte, 64)
		for i := 0; i < 64; i++ {
			var tmp []byte

			tmpSlice := sszSlice6[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.StateRoots[i] = tmp
		}
	}

	// Field 7: HistoricalRoots
	{
		if len(sszSlice7)%32 != 0 {
			return fmt.Errorf("misaligned bytes: c.HistoricalRoots length is %d, which is not a multiple of 32: %w", len(sszSlice7), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice7) / 32
		if numElem > 16777216 {
			return fmt.Errorf("ssz-max exceeded: c.HistoricalRoots has %d elements, ssz-max is 16777216: %w", numElem, ssz.ErrListTooBig)
		}
		c.HistoricalRoots = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice7[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.HistoricalRoots[i] = tmp
		}
	}

	// Field 8: Eth1Data
	c.Eth1Data = new(Eth1Data)
	if err = c.Eth1Data.UnmarshalSSZ(sszSlice8); err != nil {
		return fmt.Errorf("Eth1Data: %w", err)
	}

	// Field 9: Eth1DataVotes
	{
		if len(sszSlice9)%72 != 0 {
			return fmt.Errorf("misaligned bytes: c.Eth1DataVotes length is %d, which is not a multiple of 72: %w", len(sszSlice9), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice9) / 72
		if numElem > 32 {
			return fmt.Errorf("ssz-max exceeded: c.Eth1DataVotes has %d elements, ssz-max is 32: %w", numElem, ssz.ErrListTooBig)
		}
		c.Eth1DataVotes = make([]*Eth1Data, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Eth1Data
			tmp = new(Eth1Data)
			tmpSlice := sszSlice9[i*72 : (1+i)*72]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Eth1DataVotes: %w", err)
			}
			c.Eth1DataVotes[i] = tmp
		}
	}

	// Field 10: Eth1DepositIndex
	c.Eth1DepositIndex = binary.LittleEndian.Uint64(sszSlice10)

	// Field 11: Validators
	{
		if len(sszSlice11)%121 != 0 {
			return fmt.Errorf("misaligned bytes: c.Validators length is %d, which is not a multiple of 121: %w", len(sszSlice11), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice11) / 121
		c.Validators = make([]*Validator, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Validator
			tmp = new(Validator)
			tmpSlice := sszSlice11[i*121 : (1+i)*121]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Validators: %w", err)
			}
			c.Validators[i] = tmp
		}
	}

	// Field 12: Balances
	{
		if len(sszSlice12)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.Balances length is %d, which is not a multiple of 8: %w", len(sszSlice12), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice12) / 8
		c.Balances = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice12[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.Balances[i] = tmp
		}
	}

	// Field 13: RandaoMixes
	{
		c.RandaoMixes = make([][]byte, 64)
		for i := 0; i < 64; i++ {
			var tmp []byte

			tmpSlice := sszSlice13[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.RandaoMixes[i] = tmp
		}
	}

	// Field 14: Slashings
	{
		c.Slashings = make([]uint64, 64)
		for i := 0; i < 64; i++ {
			var tmp uint64

			tmpSlice := sszSlice14[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.Slashings[i] = tmp
		}
	}

	// Field 15: PreviousRoundParticipation
	c.PreviousRoundParticipation = append([]byte{}, sszSlice15...)

	// Field 16: CurrentRoundParticipation
	c.CurrentRoundParticipation = append([]byte{}, sszSlice16...)

	// Field 17: JustifiedCheckpoint
	c.JustifiedCheckpoint = new(CheckpointDecoupled)
	if err = c.JustifiedCheckpoint.UnmarshalSSZ(sszSlice17); err != nil {
		return fmt.Errorf("JustifiedCheckpoint: %w", err)
	}

	// Field 18: FinalizedCheckpoint
	c.FinalizedCheckpoint = new(CheckpointDecoupled)
	if err = c.FinalizedCheckpoint.UnmarshalSSZ(sszSlice18); err != nil {
		return fmt.Errorf("FinalizedCheckpoint: %w", err)
	}

	// Field 19: InactivityScores
	{
		if len(sszSlice19)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.InactivityScores length is %d, which is not a multiple of 8: %w", len(sszSlice19), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice19) / 8
		c.InactivityScores = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice19[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.InactivityScores[i] = tmp
		}
	}

	// Field 20: CurrentSyncCommittee
	c.CurrentSyncCommittee = new(SyncCommittee)
	if err = c.CurrentSyncCommittee.UnmarshalSSZ(sszSlice20); err != nil {
		return fmt.Errorf("CurrentSyncCommittee: %w", err)
	}

	// Field 21: NextSyncCommittee
	c.NextSyncCommittee = new(SyncCommittee)
	if err = c.NextSyncCommittee.UnmarshalSSZ(sszSlice21); err != nil {
		return fmt.Errorf("NextSyncCommittee: %w", err)
	}

	// Field 22: LatestExecutionPayloadBid
	c.LatestExecutionPayloadBid = new(ExecutionPayloadBid)
	if err = c.LatestExecutionPayloadBid.UnmarshalSSZ(sszSlice22); err != nil {
		return fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}

	// Field 23: NextWithdrawalIndex
	c.NextWithdrawalIndex = binary.LittleEndian.Uint64(sszSlice23)

	// Field 24: NextWithdrawalValidatorIndex
	if err = c.NextWithdrawalValidatorIndex.UnmarshalSSZ(sszSlice24); err != nil {
		return fmt.Errorf("NextWithdrawalValidatorIndex: %w", err)
	}

	// Field 25: HistoricalSummaries
	{
		if len(sszSlice25)%64 != 0 {
			return fmt.Errorf("misaligned bytes: c.HistoricalSummaries length is %d, which is not a multiple of 64: %w", len(sszSlice25), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice25) / 64
		if numElem > 16777216 {
			return fmt.Errorf("ssz-max exceeded: c.HistoricalSummaries has %d elements, ssz-max is 16777216: %w", numElem, ssz.ErrListTooBig)
		}
		c.HistoricalSummaries = make([]*HistoricalSummary, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *HistoricalSummary
			tmp = new(HistoricalSummary)
			tmpSlice := sszSlice25[i*64 : (1+i)*64]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("HistoricalSummaries: %w", err)
			}
			c.HistoricalSummaries[i] = tmp
		}
	}

	// Field 26: DepositRequestsStartIndex
	c.DepositRequestsStartIndex = binary.LittleEndian.Uint64(sszSlice26)

	// Field 27: DepositBalanceToConsume
	if err = c.DepositBalanceToConsume.UnmarshalSSZ(sszSlice27); err != nil {
		return fmt.Errorf("DepositBalanceToConsume: %w", err)
	}

	// Field 28: ExitBalanceToConsume
	if err = c.ExitBalanceToConsume.UnmarshalSSZ(sszSlice28); err != nil {
		return fmt.Errorf("ExitBalanceToConsume: %w", err)
	}

	// Field 29: EarliestExitEpoch
	if err = c.EarliestExitEpoch.UnmarshalSSZ(sszSlice29); err != nil {
		return fmt.Errorf("EarliestExitEpoch: %w", err)
	}

	// Field 30: ConsolidationBalanceToConsume
	if err = c.ConsolidationBalanceToConsume.UnmarshalSSZ(sszSlice30); err != nil {
		return fmt.Errorf("ConsolidationBalanceToConsume: %w", err)
	}

	// Field 31: EarliestConsolidationEpoch
	if err = c.EarliestConsolidationEpoch.UnmarshalSSZ(sszSlice31); err != nil {
		return fmt.Errorf("EarliestConsolidationEpoch: %w", err)
	}

	// Field 32: PendingDeposits
	{
		if len(sszSlice32)%192 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingDeposits length is %d, which is not a multiple of 192: %w", len(sszSlice32), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice32) / 192
		c.PendingDeposits = make([]*PendingDeposit, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PendingDeposit
			tmp = new(PendingDeposit)
			tmpSlice := sszSlice32[i*192 : (1+i)*192]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingDeposits: %w", err)
			}
			c.PendingDeposits[i] = tmp
		}
	}

	// Field 33: PendingPartialWithdrawals
	{
		if len(sszSlice33)%24 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingPartialWithdrawals length is %d, which is not a multiple of 24: %w", len(sszSlice33), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice33) / 24
		c.PendingPartialWithdrawals = make([]*PendingPartialWithdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PendingPartialWithdrawal
			tmp = new(PendingPartialWithdrawal)
			tmpSlice := sszSlice33[i*24 : (1+i)*24]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingPartialWithdrawals: %w", err)
			}
			c.PendingPartialWithdrawals[i] = tmp
		}
	}

	// Field 34: PendingConsolidations
	{
		if len(sszSlice34)%16 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingConsolidations length is %d, which is not a multiple of 16: %w", len(sszSlice34), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice34) / 16
		c.PendingConsolidations = make([]*PendingConsolidation, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PendingConsolidation
			tmp = new(PendingConsolidation)
			tmpSlice := sszSlice34[i*16 : (1+i)*16]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingConsolidations: %w", err)
			}
			c.PendingConsolidations[i] = tmp
		}
	}

	// Field 35: ProposerLookahead
	{
		c.ProposerLookahead = make([]primitives.ValidatorIndex, 16)
		for i := 0; i < 16; i++ {
			var tmp primitives.ValidatorIndex

			tmpSlice := sszSlice35[i*8 : (1+i)*8]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("ProposerLookahead: %w", err)
			}
			c.ProposerLookahead[i] = tmp
		}
	}

	// Field 36: Builders
	{
		if len(sszSlice36)%93 != 0 {
			return fmt.Errorf("misaligned bytes: c.Builders length is %d, which is not a multiple of 93: %w", len(sszSlice36), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice36) / 93
		c.Builders = make([]*Builder, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Builder
			tmp = new(Builder)
			tmpSlice := sszSlice36[i*93 : (1+i)*93]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Builders: %w", err)
			}
			c.Builders[i] = tmp
		}
	}

	// Field 37: NextWithdrawalBuilderIndex
	if err = c.NextWithdrawalBuilderIndex.UnmarshalSSZ(sszSlice37); err != nil {
		return fmt.Errorf("NextWithdrawalBuilderIndex: %w", err)
	}

	// Field 38: ExecutionPayloadAvailability
	c.ExecutionPayloadAvailability = make([]byte, 0, 8)
	c.ExecutionPayloadAvailability = append(c.ExecutionPayloadAvailability, sszSlice38...)

	// Field 39: BuilderPendingPayments
	{
		c.BuilderPendingPayments = make([]*BuilderPendingPaymentDecoupled, 16)
		for i := 0; i < 16; i++ {
			var tmp *BuilderPendingPaymentDecoupled
			tmp = new(BuilderPendingPaymentDecoupled)
			tmpSlice := sszSlice39[i*172 : (1+i)*172]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderPendingPayments: %w", err)
			}
			c.BuilderPendingPayments[i] = tmp
		}
	}

	// Field 40: BuilderPendingWithdrawals
	{
		if len(sszSlice40)%36 != 0 {
			return fmt.Errorf("misaligned bytes: c.BuilderPendingWithdrawals length is %d, which is not a multiple of 36: %w", len(sszSlice40), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice40) / 36
		c.BuilderPendingWithdrawals = make([]*BuilderPendingWithdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *BuilderPendingWithdrawal
			tmp = new(BuilderPendingWithdrawal)
			tmpSlice := sszSlice40[i*36 : (1+i)*36]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderPendingWithdrawals: %w", err)
			}
			c.BuilderPendingWithdrawals[i] = tmp
		}
	}

	// Field 41: LatestBlockHash
	c.LatestBlockHash = make([]byte, 0, 32)
	c.LatestBlockHash = append(c.LatestBlockHash, sszSlice41...)

	// Field 42: PayloadExpectedWithdrawals
	{
		if len(sszSlice42)%44 != 0 {
			return fmt.Errorf("misaligned bytes: c.PayloadExpectedWithdrawals length is %d, which is not a multiple of 44: %w", len(sszSlice42), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice42) / 44
		c.PayloadExpectedWithdrawals = make([]*v1.Withdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *v1.Withdrawal
			tmp = new(v1.Withdrawal)
			tmpSlice := sszSlice42[i*44 : (1+i)*44]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
			}
			c.PayloadExpectedWithdrawals[i] = tmp
		}
	}

	// Field 43: PtcWindow
	{
		c.PtcWindow = make([]*PTCs, 24)
		for i := 0; i < 24; i++ {
			var tmp *PTCs
			tmp = new(PTCs)
			tmpSlice := sszSlice43[i*128 : (1+i)*128]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PtcWindow: %w", err)
			}
			c.PtcWindow[i] = tmp
		}
	}

	// Field 44: AvailableCommitteeWindow
	{
		c.AvailableCommitteeWindow = make([]*AvailableCommittee, 24)
		for i := 0; i < 24; i++ {
			var tmp *AvailableCommittee
			tmp = new(AvailableCommittee)
			tmpSlice := sszSlice44[i*4096 : (1+i)*4096]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("AvailableCommitteeWindow: %w", err)
			}
			c.AvailableCommitteeWindow[i] = tmp
		}
	}

	// Field 45: JustifiedHeight
	if err = c.JustifiedHeight.UnmarshalSSZ(sszSlice45); err != nil {
		return fmt.Errorf("JustifiedHeight: %w", err)
	}

	// Field 46: FinalizedHeight
	if err = c.FinalizedHeight.UnmarshalSSZ(sszSlice46); err != nil {
		return fmt.Errorf("FinalizedHeight: %w", err)
	}

	// Field 47: CurrentHeight
	if err = c.CurrentHeight.UnmarshalSSZ(sszSlice47); err != nil {
		return fmt.Errorf("CurrentHeight: %w", err)
	}

	// Field 48: CurrentHeightNonjustifiable
	if sszSlice48[0] > 1 {
		return ssz.ErrInvalidSerialization
	}
	if sszSlice48[0] == 1 {
		c.CurrentHeightNonjustifiable = true
	} else {
		c.CurrentHeightNonjustifiable = false
	}

	// Field 49: CurrentHeightTarget
	c.CurrentHeightTarget = new(CheckpointDecoupled)
	if err = c.CurrentHeightTarget.UnmarshalSSZ(sszSlice49); err != nil {
		return fmt.Errorf("CurrentHeightTarget: %w", err)
	}

	// Field 50: TargetParticipation
	if err = ssz.ValidateProgressiveBitlist(sszSlice50); err != nil {
		return fmt.Errorf("TargetParticipation: %w", err)
	}
	c.TargetParticipation = append([]byte{}, go_bitfield.Bitlist(sszSlice50)...)

	// Field 51: Progress
	if err = ssz.ValidateProgressiveBitlist(sszSlice51); err != nil {
		return fmt.Errorf("Progress: %w", err)
	}
	c.Progress = append([]byte{}, go_bitfield.Bitlist(sszSlice51)...)

	// Field 52: FinalityParticipation
	if err = ssz.ValidateProgressiveBitlist(sszSlice52); err != nil {
		return fmt.Errorf("FinalityParticipation: %w", err)
	}
	c.FinalityParticipation = append([]byte{}, go_bitfield.Bitlist(sszSlice52)...)
	return err
}

func (c *BeaconStateDecoupled) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *BeaconStateDecoupled) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsBeaconStateDecoupled = []byte{0b11111111, 0b11111111, 0b11111111, 0b11111111, 0b11111111, 0b11111111, 0b00011111}

func (c *BeaconStateDecoupled) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconStateDecoupled) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: GenesisTime
	hh.PutUint64(c.GenesisTime)
	// Field 1: GenesisValidatorsRoot
	if len(c.GenesisValidatorsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.GenesisValidatorsRoot)
	// Field 2: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 3: Fork
	if err := c.Fork.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Fork: %w", err)
	}
	// Field 4: LatestBlockHeader
	if err := c.LatestBlockHeader.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("LatestBlockHeader: %w", err)
	}
	// Field 5: BlockRoots
	{
		if len(c.BlockRoots) != 64 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.BlockRoots {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 6: StateRoots
	{
		if len(c.StateRoots) != 64 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.StateRoots {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 7: HistoricalRoots
	{
		if len(c.HistoricalRoots) > 16777216 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.HistoricalRoots {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.HistoricalRoots)), 16777216)
	}
	// Field 8: Eth1Data
	if err := c.Eth1Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Eth1Data: %w", err)
	}
	// Field 9: Eth1DataVotes
	{
		if len(c.Eth1DataVotes) > 32 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Eth1DataVotes {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Eth1DataVotes: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Eth1DataVotes)), 32)
	}
	// Field 10: Eth1DepositIndex
	hh.PutUint64(c.Eth1DepositIndex)
	// Field 11: Validators
	{
		subIndx := hh.Index()
		for _, o := range c.Validators {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Validators: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Validators)))
	}
	// Field 12: Balances
	{
		subIndx := hh.Index()
		for _, o := range c.Balances {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Balances)))
	}
	// Field 13: RandaoMixes
	{
		if len(c.RandaoMixes) != 64 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.RandaoMixes {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 14: Slashings
	{
		if len(c.Slashings) != 64 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.Slashings {
			hh.AppendUint64(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 15: PreviousRoundParticipation
	{
		subIndx := hh.Index()
		hh.AppendBytes32(c.PreviousRoundParticipation)
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PreviousRoundParticipation)))
	}
	// Field 16: CurrentRoundParticipation
	{
		subIndx := hh.Index()
		hh.AppendBytes32(c.CurrentRoundParticipation)
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.CurrentRoundParticipation)))
	}
	// Field 17: JustifiedCheckpoint
	if err := c.JustifiedCheckpoint.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("JustifiedCheckpoint: %w", err)
	}
	// Field 18: FinalizedCheckpoint
	if err := c.FinalizedCheckpoint.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("FinalizedCheckpoint: %w", err)
	}
	// Field 19: InactivityScores
	{
		subIndx := hh.Index()
		for _, o := range c.InactivityScores {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.InactivityScores)))
	}
	// Field 20: CurrentSyncCommittee
	if err := c.CurrentSyncCommittee.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("CurrentSyncCommittee: %w", err)
	}
	// Field 21: NextSyncCommittee
	if err := c.NextSyncCommittee.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextSyncCommittee: %w", err)
	}
	// Field 22: LatestExecutionPayloadBid
	if err := c.LatestExecutionPayloadBid.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}
	// Field 23: NextWithdrawalIndex
	hh.PutUint64(c.NextWithdrawalIndex)
	// Field 24: NextWithdrawalValidatorIndex
	if err := c.NextWithdrawalValidatorIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextWithdrawalValidatorIndex: %w", err)
	}
	// Field 25: HistoricalSummaries
	{
		if len(c.HistoricalSummaries) > 16777216 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.HistoricalSummaries {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("HistoricalSummaries: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.HistoricalSummaries)), 16777216)
	}
	// Field 26: DepositRequestsStartIndex
	hh.PutUint64(c.DepositRequestsStartIndex)
	// Field 27: DepositBalanceToConsume
	if err := c.DepositBalanceToConsume.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("DepositBalanceToConsume: %w", err)
	}
	// Field 28: ExitBalanceToConsume
	if err := c.ExitBalanceToConsume.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExitBalanceToConsume: %w", err)
	}
	// Field 29: EarliestExitEpoch
	if err := c.EarliestExitEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("EarliestExitEpoch: %w", err)
	}
	// Field 30: ConsolidationBalanceToConsume
	if err := c.ConsolidationBalanceToConsume.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ConsolidationBalanceToConsume: %w", err)
	}
	// Field 31: EarliestConsolidationEpoch
	if err := c.EarliestConsolidationEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("EarliestConsolidationEpoch: %w", err)
	}
	// Field 32: PendingDeposits
	{
		subIndx := hh.Index()
		for _, o := range c.PendingDeposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingDeposits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingDeposits)))
	}
	// Field 33: PendingPartialWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.PendingPartialWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingPartialWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingPartialWithdrawals)))
	}
	// Field 34: PendingConsolidations
	{
		subIndx := hh.Index()
		for _, o := range c.PendingConsolidations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingConsolidations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingConsolidations)))
	}
	// Field 35: ProposerLookahead
	{
		if len(c.ProposerLookahead) != 16 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.ProposerLookahead {
			hh.AppendUint64(uint64(o))
		}
		hh.Merkleize(subIndx)
	}
	// Field 36: Builders
	{
		subIndx := hh.Index()
		for _, o := range c.Builders {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Builders: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Builders)))
	}
	// Field 37: NextWithdrawalBuilderIndex
	if err := c.NextWithdrawalBuilderIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextWithdrawalBuilderIndex: %w", err)
	}
	// Field 38: ExecutionPayloadAvailability
	if len(c.ExecutionPayloadAvailability) != 8 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ExecutionPayloadAvailability)
	// Field 39: BuilderPendingPayments
	{
		if len(c.BuilderPendingPayments) != 16 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.BuilderPendingPayments {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderPendingPayments: %w", err)
			}
		}
		hh.Merkleize(subIndx)
	}
	// Field 40: BuilderPendingWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.BuilderPendingWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderPendingWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BuilderPendingWithdrawals)))
	}
	// Field 41: LatestBlockHash
	if len(c.LatestBlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LatestBlockHash)
	// Field 42: PayloadExpectedWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.PayloadExpectedWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PayloadExpectedWithdrawals)))
	}
	// Field 43: PtcWindow
	{
		if len(c.PtcWindow) != 24 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.PtcWindow {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PtcWindow: %w", err)
			}
		}
		hh.Merkleize(subIndx)
	}
	// Field 44: AvailableCommitteeWindow
	{
		if len(c.AvailableCommitteeWindow) != 24 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.AvailableCommitteeWindow {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("AvailableCommitteeWindow: %w", err)
			}
		}
		hh.Merkleize(subIndx)
	}
	// Field 45: JustifiedHeight
	if err := c.JustifiedHeight.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("JustifiedHeight: %w", err)
	}
	// Field 46: FinalizedHeight
	if err := c.FinalizedHeight.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("FinalizedHeight: %w", err)
	}
	// Field 47: CurrentHeight
	if err := c.CurrentHeight.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("CurrentHeight: %w", err)
	}
	// Field 48: CurrentHeightNonjustifiable
	hh.PutBool(c.CurrentHeightNonjustifiable)
	// Field 49: CurrentHeightTarget
	if err := c.CurrentHeightTarget.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("CurrentHeightTarget: %w", err)
	}
	// Field 50: TargetParticipation
	if len(c.TargetParticipation) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutProgressiveBitlist(c.TargetParticipation)
	// Field 51: Progress
	if len(c.Progress) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutProgressiveBitlist(c.Progress)
	// Field 52: FinalityParticipation
	if len(c.FinalityParticipation) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutProgressiveBitlist(c.FinalityParticipation)
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsBeaconStateDecoupled)
	return nil
}
