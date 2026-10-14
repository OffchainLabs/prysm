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
	size := 81

	return size
}

func (c *AttestationDataDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AttestationDataDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Height
	if dst, err = c.Height.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Height: %w", err)
	}

	// Field 1: Target
	if len(c.Target) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Target...)

	// Field 2: Timeout
	if c.Timeout {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}

	// Field 3: FinalityHeight
	if dst, err = c.FinalityHeight.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("FinalityHeight: %w", err)
	}

	// Field 4: FinalityTarget
	if len(c.FinalityTarget) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FinalityTarget...)

	return dst, err
}

func (c *AttestationDataDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 81 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.Height
	sszSlice1 := buf[8:40]  // c.Target
	sszSlice2 := buf[40:41] // c.Timeout
	sszSlice3 := buf[41:49] // c.FinalityHeight
	sszSlice4 := buf[49:81] // c.FinalityTarget

	// Field 0: Height
	if err = c.Height.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Height: %w", err)
	}

	// Field 1: Target
	c.Target = make([]byte, 0, 32)
	c.Target = append(c.Target, sszSlice1...)

	// Field 2: Timeout
	if sszSlice2[0] > 1 {
		return ssz.ErrInvalidSerialization
	}
	if sszSlice2[0] == 1 {
		c.Timeout = true
	} else {
		c.Timeout = false
	}

	// Field 3: FinalityHeight
	if err = c.FinalityHeight.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("FinalityHeight: %w", err)
	}

	// Field 4: FinalityTarget
	c.FinalityTarget = make([]byte, 0, 32)
	c.FinalityTarget = append(c.FinalityTarget, sszSlice4...)
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
	// Field 0: Height
	if err := c.Height.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Height: %w", err)
	}
	// Field 1: Target
	if len(c.Target) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Target)
	// Field 2: Timeout
	hh.PutBool(c.Timeout)
	// Field 3: FinalityHeight
	if err := c.FinalityHeight.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("FinalityHeight: %w", err)
	}
	// Field 4: FinalityTarget
	if len(c.FinalityTarget) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FinalityTarget)
	hh.Merkleize(indx)
	return nil
}

func (c *IndexedAttestationDecoupled) SizeSSZ() int {
	size := 181
	size += len(c.AttestingIndices) * 8
	return size
}

func (c *IndexedAttestationDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *IndexedAttestationDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 181

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
	if size < 181 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:85]   // c.Data
	sszSlice2 := buf[85:181] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.AttestingIndices
	if sszVarOffset0 != 181 {
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
		if numElem > 8192 {
			return fmt.Errorf("ssz-max exceeded: c.AttestingIndices has %d elements, ssz-max is 8192: %w", numElem, ssz.ErrListTooBig)
		}
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

func (c *PendingFinality) SizeSSZ() int {
	size := 52
	size += len(c.Participation)
	return size
}

func (c *PendingFinality) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *PendingFinality) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 52

	// Field 0: Height
	if dst, err = c.Height.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Height: %w", err)
	}

	// Field 1: Target
	if c.Target == nil {
		c.Target = new(CheckpointDecoupled)
	}
	if dst, err = c.Target.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Target: %w", err)
	}

	// Field 2: Participation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Participation)

	// Field 2: Participation
	dst = append(dst, c.Participation...)
	return dst, err
}

func (c *PendingFinality) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 52 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.Height
	sszSlice1 := buf[8:48] // c.Target

	sszVarOffset2 := ssz.ReadOffset(buf[48:52]) // c.Participation
	if sszVarOffset2 != 52 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset2 > size {
		return ssz.ErrOffset
	}
	sszSlice2 := buf[sszVarOffset2:] // c.Participation

	// Field 0: Height
	if err = c.Height.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Height: %w", err)
	}

	// Field 1: Target
	c.Target = new(CheckpointDecoupled)
	if err = c.Target.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Target: %w", err)
	}

	// Field 2: Participation
	if err = ssz.ValidateBitlist(sszSlice2, 1099511627776); err != nil {
		return fmt.Errorf("Participation: %w", err)
	}
	c.Participation = append([]byte{}, go_bitfield.Bitlist(sszSlice2)...)
	return err
}

func (c *PendingFinality) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *PendingFinality) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Height
	if err := c.Height.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Height: %w", err)
	}
	// Field 1: Target
	if err := c.Target.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Target: %w", err)
	}
	// Field 2: Participation
	if len(c.Participation) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutProgressiveBitlist(c.Participation)
	hh.Merkleize(indx)
	return nil
}

func (c *BeaconStateDecoupled) SizeSSZ() int {
	size := 14561
	size += len(c.HistoricalRoots) * 32
	size += len(c.Eth1DataVotes) * 72
	size += len(c.Validators) * 121
	size += len(c.Balances) * 8
	size += len(c.PreviousEpochParticipation)
	size += len(c.CurrentEpochParticipation)
	size += len(c.InactivityScores) * 8
	size += len(c.HistoricalSummaries) * 64
	size += len(c.PendingDeposits) * 192
	size += len(c.PendingPartialWithdrawals) * 24
	size += len(c.PendingConsolidations) * 16
	size += len(c.Builders) * 93
	size += len(c.BuilderPendingWithdrawals) * 36
	if c.LatestExecutionPayloadBid == nil {
		c.LatestExecutionPayloadBid = new(ExecutionPayloadBid)
	}
	size += c.LatestExecutionPayloadBid.SizeSSZ()
	size += len(c.PayloadExpectedWithdrawals) * 44
	size += len(c.TargetParticipation)
	size += len(c.Progress)
	for _, o := range c.PendingFinality {
		size += 4
		size += o.SizeSSZ()
	}
	return size
}

func (c *BeaconStateDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconStateDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 14561

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

	// Field 15: PreviousEpochParticipation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PreviousEpochParticipation)

	// Field 16: CurrentEpochParticipation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.CurrentEpochParticipation)

	// Field 17: JustificationBits
	if len([]byte(c.JustificationBits)) != 1 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.JustificationBits)...)

	// Field 18: PreviousJustifiedCheckpoint
	if c.PreviousJustifiedCheckpoint == nil {
		c.PreviousJustifiedCheckpoint = new(Checkpoint)
	}
	if dst, err = c.PreviousJustifiedCheckpoint.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("PreviousJustifiedCheckpoint: %w", err)
	}

	// Field 19: CurrentJustifiedCheckpoint
	if c.CurrentJustifiedCheckpoint == nil {
		c.CurrentJustifiedCheckpoint = new(Checkpoint)
	}
	if dst, err = c.CurrentJustifiedCheckpoint.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("CurrentJustifiedCheckpoint: %w", err)
	}

	// Field 20: FinalizedCheckpoint
	if c.FinalizedCheckpoint == nil {
		c.FinalizedCheckpoint = new(Checkpoint)
	}
	if dst, err = c.FinalizedCheckpoint.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("FinalizedCheckpoint: %w", err)
	}

	// Field 21: InactivityScores
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.InactivityScores) * 8

	// Field 22: CurrentSyncCommittee
	if c.CurrentSyncCommittee == nil {
		c.CurrentSyncCommittee = new(SyncCommittee)
	}
	if dst, err = c.CurrentSyncCommittee.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("CurrentSyncCommittee: %w", err)
	}

	// Field 23: NextSyncCommittee
	if c.NextSyncCommittee == nil {
		c.NextSyncCommittee = new(SyncCommittee)
	}
	if dst, err = c.NextSyncCommittee.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextSyncCommittee: %w", err)
	}

	// Field 24: LatestBlockHash
	if len(c.LatestBlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LatestBlockHash...)

	// Field 25: NextWithdrawalIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.NextWithdrawalIndex)

	// Field 26: NextWithdrawalValidatorIndex
	if dst, err = c.NextWithdrawalValidatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextWithdrawalValidatorIndex: %w", err)
	}

	// Field 27: HistoricalSummaries
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.HistoricalSummaries) * 64

	// Field 28: DepositRequestsStartIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.DepositRequestsStartIndex)

	// Field 29: DepositBalanceToConsume
	if dst, err = c.DepositBalanceToConsume.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("DepositBalanceToConsume: %w", err)
	}

	// Field 30: ExitBalanceToConsume
	if dst, err = c.ExitBalanceToConsume.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ExitBalanceToConsume: %w", err)
	}

	// Field 31: EarliestExitEpoch
	if dst, err = c.EarliestExitEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("EarliestExitEpoch: %w", err)
	}

	// Field 32: ConsolidationBalanceToConsume
	if dst, err = c.ConsolidationBalanceToConsume.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ConsolidationBalanceToConsume: %w", err)
	}

	// Field 33: EarliestConsolidationEpoch
	if dst, err = c.EarliestConsolidationEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("EarliestConsolidationEpoch: %w", err)
	}

	// Field 34: PendingDeposits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingDeposits) * 192

	// Field 35: PendingPartialWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingPartialWithdrawals) * 24

	// Field 36: PendingConsolidations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingConsolidations) * 16

	// Field 37: ProposerLookahead
	if len(c.ProposerLookahead) != 16 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.ProposerLookahead {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("ProposerLookahead: %w", err)
		}
	}

	// Field 38: Builders
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Builders) * 93

	// Field 39: NextWithdrawalBuilderIndex
	if dst, err = c.NextWithdrawalBuilderIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextWithdrawalBuilderIndex: %w", err)
	}

	// Field 40: ExecutionPayloadAvailability
	if len(c.ExecutionPayloadAvailability) != 8 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ExecutionPayloadAvailability...)

	// Field 41: BuilderPendingPayments
	if len(c.BuilderPendingPayments) != 16 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.BuilderPendingPayments {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderPendingPayments: %w", err)
		}
	}

	// Field 42: BuilderPendingWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BuilderPendingWithdrawals) * 36

	// Field 43: LatestExecutionPayloadBid
	if c.LatestExecutionPayloadBid == nil {
		c.LatestExecutionPayloadBid = new(ExecutionPayloadBid)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.LatestExecutionPayloadBid.SizeSSZ()

	// Field 44: PayloadExpectedWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PayloadExpectedWithdrawals) * 44

	// Field 45: PtcWindow
	if len(c.PtcWindow) != 24 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.PtcWindow {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PtcWindow: %w", err)
		}
	}

	// Field 46: Height
	if dst, err = c.Height.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Height: %w", err)
	}

	// Field 47: Entry
	if c.Entry == nil {
		c.Entry = new(CheckpointDecoupled)
	}
	if dst, err = c.Entry.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Entry: %w", err)
	}

	// Field 48: TargetParticipation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.TargetParticipation)

	// Field 49: Progress
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Progress)

	// Field 50: JustifiedHeight
	if dst, err = c.JustifiedHeight.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("JustifiedHeight: %w", err)
	}

	// Field 51: Justified
	if c.Justified == nil {
		c.Justified = new(CheckpointDecoupled)
	}
	if dst, err = c.Justified.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Justified: %w", err)
	}

	// Field 52: FinalizedHeight
	if dst, err = c.FinalizedHeight.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("FinalizedHeight: %w", err)
	}

	// Field 53: Finalized
	if c.Finalized == nil {
		c.Finalized = new(CheckpointDecoupled)
	}
	if dst, err = c.Finalized.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Finalized: %w", err)
	}

	// Field 54: PendingFinality
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.PendingFinality {
		offset += 4
		offset += o.SizeSSZ()
	}

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

	// Field 15: PreviousEpochParticipation
	dst = append(dst, c.PreviousEpochParticipation...)

	// Field 16: CurrentEpochParticipation
	dst = append(dst, c.CurrentEpochParticipation...)

	// Field 21: InactivityScores
	for _, o := range c.InactivityScores {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 27: HistoricalSummaries
	if len(c.HistoricalSummaries) > 16777216 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.HistoricalSummaries {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("HistoricalSummaries: %w", err)
		}
	}

	// Field 34: PendingDeposits
	for _, o := range c.PendingDeposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingDeposits: %w", err)
		}
	}

	// Field 35: PendingPartialWithdrawals
	for _, o := range c.PendingPartialWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingPartialWithdrawals: %w", err)
		}
	}

	// Field 36: PendingConsolidations
	for _, o := range c.PendingConsolidations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingConsolidations: %w", err)
		}
	}

	// Field 38: Builders
	for _, o := range c.Builders {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Builders: %w", err)
		}
	}

	// Field 42: BuilderPendingWithdrawals
	for _, o := range c.BuilderPendingWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderPendingWithdrawals: %w", err)
		}
	}

	// Field 43: LatestExecutionPayloadBid
	if dst, err = c.LatestExecutionPayloadBid.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}

	// Field 44: PayloadExpectedWithdrawals
	for _, o := range c.PayloadExpectedWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
		}
	}

	// Field 48: TargetParticipation
	dst = append(dst, c.TargetParticipation...)

	// Field 49: Progress
	dst = append(dst, c.Progress...)

	// Field 54: PendingFinality
	if len(c.PendingFinality) > 8 {
		return nil, ssz.ErrListTooBig
	}
	{
		offset = 4 * len(c.PendingFinality)
		for _, o := range c.PendingFinality {
			dst = ssz.WriteOffset(dst, offset)
			offset += o.SizeSSZ()
		}
	}
	for _, o := range c.PendingFinality {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingFinality: %w", err)
		}
	}
	return dst, err
}

func (c *BeaconStateDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 14561 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]          // c.GenesisTime
	sszSlice1 := buf[8:40]         // c.GenesisValidatorsRoot
	sszSlice2 := buf[40:48]        // c.Slot
	sszSlice3 := buf[48:64]        // c.Fork
	sszSlice4 := buf[64:176]       // c.LatestBlockHeader
	sszSlice5 := buf[176:2224]     // c.BlockRoots
	sszSlice6 := buf[2224:4272]    // c.StateRoots
	sszSlice8 := buf[4276:4348]    // c.Eth1Data
	sszSlice10 := buf[4352:4360]   // c.Eth1DepositIndex
	sszSlice13 := buf[4368:6416]   // c.RandaoMixes
	sszSlice14 := buf[6416:6928]   // c.Slashings
	sszSlice17 := buf[6936:6937]   // c.JustificationBits
	sszSlice18 := buf[6937:6977]   // c.PreviousJustifiedCheckpoint
	sszSlice19 := buf[6977:7017]   // c.CurrentJustifiedCheckpoint
	sszSlice20 := buf[7017:7057]   // c.FinalizedCheckpoint
	sszSlice22 := buf[7061:8645]   // c.CurrentSyncCommittee
	sszSlice23 := buf[8645:10229]  // c.NextSyncCommittee
	sszSlice24 := buf[10229:10261] // c.LatestBlockHash
	sszSlice25 := buf[10261:10269] // c.NextWithdrawalIndex
	sszSlice26 := buf[10269:10277] // c.NextWithdrawalValidatorIndex
	sszSlice28 := buf[10281:10289] // c.DepositRequestsStartIndex
	sszSlice29 := buf[10289:10297] // c.DepositBalanceToConsume
	sszSlice30 := buf[10297:10305] // c.ExitBalanceToConsume
	sszSlice31 := buf[10305:10313] // c.EarliestExitEpoch
	sszSlice32 := buf[10313:10321] // c.ConsolidationBalanceToConsume
	sszSlice33 := buf[10321:10329] // c.EarliestConsolidationEpoch
	sszSlice37 := buf[10341:10469] // c.ProposerLookahead
	sszSlice39 := buf[10473:10481] // c.NextWithdrawalBuilderIndex
	sszSlice40 := buf[10481:10489] // c.ExecutionPayloadAvailability
	sszSlice41 := buf[10489:11321] // c.BuilderPendingPayments
	sszSlice45 := buf[11333:14405] // c.PtcWindow
	sszSlice46 := buf[14405:14413] // c.Height
	sszSlice47 := buf[14413:14453] // c.Entry
	sszSlice50 := buf[14461:14469] // c.JustifiedHeight
	sszSlice51 := buf[14469:14509] // c.Justified
	sszSlice52 := buf[14509:14517] // c.FinalizedHeight
	sszSlice53 := buf[14517:14557] // c.Finalized

	sszVarOffset7 := ssz.ReadOffset(buf[4272:4276]) // c.HistoricalRoots
	if sszVarOffset7 != 14561 {
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
	sszVarOffset15 := ssz.ReadOffset(buf[6928:6932]) // c.PreviousEpochParticipation
	if sszVarOffset15 > size || sszVarOffset15 < sszVarOffset12 {
		return ssz.ErrOffset
	}
	sszVarOffset16 := ssz.ReadOffset(buf[6932:6936]) // c.CurrentEpochParticipation
	if sszVarOffset16 > size || sszVarOffset16 < sszVarOffset15 {
		return ssz.ErrOffset
	}
	sszVarOffset21 := ssz.ReadOffset(buf[7057:7061]) // c.InactivityScores
	if sszVarOffset21 > size || sszVarOffset21 < sszVarOffset16 {
		return ssz.ErrOffset
	}
	sszVarOffset27 := ssz.ReadOffset(buf[10277:10281]) // c.HistoricalSummaries
	if sszVarOffset27 > size || sszVarOffset27 < sszVarOffset21 {
		return ssz.ErrOffset
	}
	sszVarOffset34 := ssz.ReadOffset(buf[10329:10333]) // c.PendingDeposits
	if sszVarOffset34 > size || sszVarOffset34 < sszVarOffset27 {
		return ssz.ErrOffset
	}
	sszVarOffset35 := ssz.ReadOffset(buf[10333:10337]) // c.PendingPartialWithdrawals
	if sszVarOffset35 > size || sszVarOffset35 < sszVarOffset34 {
		return ssz.ErrOffset
	}
	sszVarOffset36 := ssz.ReadOffset(buf[10337:10341]) // c.PendingConsolidations
	if sszVarOffset36 > size || sszVarOffset36 < sszVarOffset35 {
		return ssz.ErrOffset
	}
	sszVarOffset38 := ssz.ReadOffset(buf[10469:10473]) // c.Builders
	if sszVarOffset38 > size || sszVarOffset38 < sszVarOffset36 {
		return ssz.ErrOffset
	}
	sszVarOffset42 := ssz.ReadOffset(buf[11321:11325]) // c.BuilderPendingWithdrawals
	if sszVarOffset42 > size || sszVarOffset42 < sszVarOffset38 {
		return ssz.ErrOffset
	}
	sszVarOffset43 := ssz.ReadOffset(buf[11325:11329]) // c.LatestExecutionPayloadBid
	if sszVarOffset43 > size || sszVarOffset43 < sszVarOffset42 {
		return ssz.ErrOffset
	}
	sszVarOffset44 := ssz.ReadOffset(buf[11329:11333]) // c.PayloadExpectedWithdrawals
	if sszVarOffset44 > size || sszVarOffset44 < sszVarOffset43 {
		return ssz.ErrOffset
	}
	sszVarOffset48 := ssz.ReadOffset(buf[14453:14457]) // c.TargetParticipation
	if sszVarOffset48 > size || sszVarOffset48 < sszVarOffset44 {
		return ssz.ErrOffset
	}
	sszVarOffset49 := ssz.ReadOffset(buf[14457:14461]) // c.Progress
	if sszVarOffset49 > size || sszVarOffset49 < sszVarOffset48 {
		return ssz.ErrOffset
	}
	sszVarOffset54 := ssz.ReadOffset(buf[14557:14561]) // c.PendingFinality
	if sszVarOffset54 > size || sszVarOffset54 < sszVarOffset49 {
		return ssz.ErrOffset
	}
	sszSlice7 := buf[sszVarOffset7:sszVarOffset9]    // c.HistoricalRoots
	sszSlice9 := buf[sszVarOffset9:sszVarOffset11]   // c.Eth1DataVotes
	sszSlice11 := buf[sszVarOffset11:sszVarOffset12] // c.Validators
	sszSlice12 := buf[sszVarOffset12:sszVarOffset15] // c.Balances
	sszSlice15 := buf[sszVarOffset15:sszVarOffset16] // c.PreviousEpochParticipation
	sszSlice16 := buf[sszVarOffset16:sszVarOffset21] // c.CurrentEpochParticipation
	sszSlice21 := buf[sszVarOffset21:sszVarOffset27] // c.InactivityScores
	sszSlice27 := buf[sszVarOffset27:sszVarOffset34] // c.HistoricalSummaries
	sszSlice34 := buf[sszVarOffset34:sszVarOffset35] // c.PendingDeposits
	sszSlice35 := buf[sszVarOffset35:sszVarOffset36] // c.PendingPartialWithdrawals
	sszSlice36 := buf[sszVarOffset36:sszVarOffset38] // c.PendingConsolidations
	sszSlice38 := buf[sszVarOffset38:sszVarOffset42] // c.Builders
	sszSlice42 := buf[sszVarOffset42:sszVarOffset43] // c.BuilderPendingWithdrawals
	sszSlice43 := buf[sszVarOffset43:sszVarOffset44] // c.LatestExecutionPayloadBid
	sszSlice44 := buf[sszVarOffset44:sszVarOffset48] // c.PayloadExpectedWithdrawals
	sszSlice48 := buf[sszVarOffset48:sszVarOffset49] // c.TargetParticipation
	sszSlice49 := buf[sszVarOffset49:sszVarOffset54] // c.Progress
	sszSlice54 := buf[sszVarOffset54:]               // c.PendingFinality

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
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.Validators has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
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
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.Balances has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
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

	// Field 15: PreviousEpochParticipation
	c.PreviousEpochParticipation = append([]byte{}, sszSlice15...)

	// Field 16: CurrentEpochParticipation
	c.CurrentEpochParticipation = append([]byte{}, sszSlice16...)

	// Field 17: JustificationBits
	c.JustificationBits = make([]byte, 0, 1)
	c.JustificationBits = append(c.JustificationBits, go_bitfield.Bitvector4(sszSlice17)...)

	// Field 18: PreviousJustifiedCheckpoint
	c.PreviousJustifiedCheckpoint = new(Checkpoint)
	if err = c.PreviousJustifiedCheckpoint.UnmarshalSSZ(sszSlice18); err != nil {
		return fmt.Errorf("PreviousJustifiedCheckpoint: %w", err)
	}

	// Field 19: CurrentJustifiedCheckpoint
	c.CurrentJustifiedCheckpoint = new(Checkpoint)
	if err = c.CurrentJustifiedCheckpoint.UnmarshalSSZ(sszSlice19); err != nil {
		return fmt.Errorf("CurrentJustifiedCheckpoint: %w", err)
	}

	// Field 20: FinalizedCheckpoint
	c.FinalizedCheckpoint = new(Checkpoint)
	if err = c.FinalizedCheckpoint.UnmarshalSSZ(sszSlice20); err != nil {
		return fmt.Errorf("FinalizedCheckpoint: %w", err)
	}

	// Field 21: InactivityScores
	{
		if len(sszSlice21)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.InactivityScores length is %d, which is not a multiple of 8: %w", len(sszSlice21), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice21) / 8
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.InactivityScores has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
		c.InactivityScores = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice21[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.InactivityScores[i] = tmp
		}
	}

	// Field 22: CurrentSyncCommittee
	c.CurrentSyncCommittee = new(SyncCommittee)
	if err = c.CurrentSyncCommittee.UnmarshalSSZ(sszSlice22); err != nil {
		return fmt.Errorf("CurrentSyncCommittee: %w", err)
	}

	// Field 23: NextSyncCommittee
	c.NextSyncCommittee = new(SyncCommittee)
	if err = c.NextSyncCommittee.UnmarshalSSZ(sszSlice23); err != nil {
		return fmt.Errorf("NextSyncCommittee: %w", err)
	}

	// Field 24: LatestBlockHash
	c.LatestBlockHash = make([]byte, 0, 32)
	c.LatestBlockHash = append(c.LatestBlockHash, sszSlice24...)

	// Field 25: NextWithdrawalIndex
	c.NextWithdrawalIndex = binary.LittleEndian.Uint64(sszSlice25)

	// Field 26: NextWithdrawalValidatorIndex
	if err = c.NextWithdrawalValidatorIndex.UnmarshalSSZ(sszSlice26); err != nil {
		return fmt.Errorf("NextWithdrawalValidatorIndex: %w", err)
	}

	// Field 27: HistoricalSummaries
	{
		if len(sszSlice27)%64 != 0 {
			return fmt.Errorf("misaligned bytes: c.HistoricalSummaries length is %d, which is not a multiple of 64: %w", len(sszSlice27), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice27) / 64
		if numElem > 16777216 {
			return fmt.Errorf("ssz-max exceeded: c.HistoricalSummaries has %d elements, ssz-max is 16777216: %w", numElem, ssz.ErrListTooBig)
		}
		c.HistoricalSummaries = make([]*HistoricalSummary, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *HistoricalSummary
			tmp = new(HistoricalSummary)
			tmpSlice := sszSlice27[i*64 : (1+i)*64]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("HistoricalSummaries: %w", err)
			}
			c.HistoricalSummaries[i] = tmp
		}
	}

	// Field 28: DepositRequestsStartIndex
	c.DepositRequestsStartIndex = binary.LittleEndian.Uint64(sszSlice28)

	// Field 29: DepositBalanceToConsume
	if err = c.DepositBalanceToConsume.UnmarshalSSZ(sszSlice29); err != nil {
		return fmt.Errorf("DepositBalanceToConsume: %w", err)
	}

	// Field 30: ExitBalanceToConsume
	if err = c.ExitBalanceToConsume.UnmarshalSSZ(sszSlice30); err != nil {
		return fmt.Errorf("ExitBalanceToConsume: %w", err)
	}

	// Field 31: EarliestExitEpoch
	if err = c.EarliestExitEpoch.UnmarshalSSZ(sszSlice31); err != nil {
		return fmt.Errorf("EarliestExitEpoch: %w", err)
	}

	// Field 32: ConsolidationBalanceToConsume
	if err = c.ConsolidationBalanceToConsume.UnmarshalSSZ(sszSlice32); err != nil {
		return fmt.Errorf("ConsolidationBalanceToConsume: %w", err)
	}

	// Field 33: EarliestConsolidationEpoch
	if err = c.EarliestConsolidationEpoch.UnmarshalSSZ(sszSlice33); err != nil {
		return fmt.Errorf("EarliestConsolidationEpoch: %w", err)
	}

	// Field 34: PendingDeposits
	{
		if len(sszSlice34)%192 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingDeposits length is %d, which is not a multiple of 192: %w", len(sszSlice34), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice34) / 192
		if numElem > 134217728 {
			return fmt.Errorf("ssz-max exceeded: c.PendingDeposits has %d elements, ssz-max is 134217728: %w", numElem, ssz.ErrListTooBig)
		}
		c.PendingDeposits = make([]*PendingDeposit, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PendingDeposit
			tmp = new(PendingDeposit)
			tmpSlice := sszSlice34[i*192 : (1+i)*192]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingDeposits: %w", err)
			}
			c.PendingDeposits[i] = tmp
		}
	}

	// Field 35: PendingPartialWithdrawals
	{
		if len(sszSlice35)%24 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingPartialWithdrawals length is %d, which is not a multiple of 24: %w", len(sszSlice35), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice35) / 24
		if numElem > 64 {
			return fmt.Errorf("ssz-max exceeded: c.PendingPartialWithdrawals has %d elements, ssz-max is 64: %w", numElem, ssz.ErrListTooBig)
		}
		c.PendingPartialWithdrawals = make([]*PendingPartialWithdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PendingPartialWithdrawal
			tmp = new(PendingPartialWithdrawal)
			tmpSlice := sszSlice35[i*24 : (1+i)*24]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingPartialWithdrawals: %w", err)
			}
			c.PendingPartialWithdrawals[i] = tmp
		}
	}

	// Field 36: PendingConsolidations
	{
		if len(sszSlice36)%16 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingConsolidations length is %d, which is not a multiple of 16: %w", len(sszSlice36), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice36) / 16
		if numElem > 64 {
			return fmt.Errorf("ssz-max exceeded: c.PendingConsolidations has %d elements, ssz-max is 64: %w", numElem, ssz.ErrListTooBig)
		}
		c.PendingConsolidations = make([]*PendingConsolidation, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PendingConsolidation
			tmp = new(PendingConsolidation)
			tmpSlice := sszSlice36[i*16 : (1+i)*16]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingConsolidations: %w", err)
			}
			c.PendingConsolidations[i] = tmp
		}
	}

	// Field 37: ProposerLookahead
	{
		c.ProposerLookahead = make([]primitives.ValidatorIndex, 16)
		for i := 0; i < 16; i++ {
			var tmp primitives.ValidatorIndex

			tmpSlice := sszSlice37[i*8 : (1+i)*8]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("ProposerLookahead: %w", err)
			}
			c.ProposerLookahead[i] = tmp
		}
	}

	// Field 38: Builders
	{
		if len(sszSlice38)%93 != 0 {
			return fmt.Errorf("misaligned bytes: c.Builders length is %d, which is not a multiple of 93: %w", len(sszSlice38), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice38) / 93
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.Builders has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
		c.Builders = make([]*Builder, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Builder
			tmp = new(Builder)
			tmpSlice := sszSlice38[i*93 : (1+i)*93]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Builders: %w", err)
			}
			c.Builders[i] = tmp
		}
	}

	// Field 39: NextWithdrawalBuilderIndex
	if err = c.NextWithdrawalBuilderIndex.UnmarshalSSZ(sszSlice39); err != nil {
		return fmt.Errorf("NextWithdrawalBuilderIndex: %w", err)
	}

	// Field 40: ExecutionPayloadAvailability
	c.ExecutionPayloadAvailability = make([]byte, 0, 8)
	c.ExecutionPayloadAvailability = append(c.ExecutionPayloadAvailability, sszSlice40...)

	// Field 41: BuilderPendingPayments
	{
		c.BuilderPendingPayments = make([]*BuilderPendingPayment, 16)
		for i := 0; i < 16; i++ {
			var tmp *BuilderPendingPayment
			tmp = new(BuilderPendingPayment)
			tmpSlice := sszSlice41[i*52 : (1+i)*52]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderPendingPayments: %w", err)
			}
			c.BuilderPendingPayments[i] = tmp
		}
	}

	// Field 42: BuilderPendingWithdrawals
	{
		if len(sszSlice42)%36 != 0 {
			return fmt.Errorf("misaligned bytes: c.BuilderPendingWithdrawals length is %d, which is not a multiple of 36: %w", len(sszSlice42), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice42) / 36
		if numElem > 1048576 {
			return fmt.Errorf("ssz-max exceeded: c.BuilderPendingWithdrawals has %d elements, ssz-max is 1048576: %w", numElem, ssz.ErrListTooBig)
		}
		c.BuilderPendingWithdrawals = make([]*BuilderPendingWithdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *BuilderPendingWithdrawal
			tmp = new(BuilderPendingWithdrawal)
			tmpSlice := sszSlice42[i*36 : (1+i)*36]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderPendingWithdrawals: %w", err)
			}
			c.BuilderPendingWithdrawals[i] = tmp
		}
	}

	// Field 43: LatestExecutionPayloadBid
	c.LatestExecutionPayloadBid = new(ExecutionPayloadBid)
	if err = c.LatestExecutionPayloadBid.UnmarshalSSZ(sszSlice43); err != nil {
		return fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}

	// Field 44: PayloadExpectedWithdrawals
	{
		if len(sszSlice44)%44 != 0 {
			return fmt.Errorf("misaligned bytes: c.PayloadExpectedWithdrawals length is %d, which is not a multiple of 44: %w", len(sszSlice44), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice44) / 44
		if numElem > 4 {
			return fmt.Errorf("ssz-max exceeded: c.PayloadExpectedWithdrawals has %d elements, ssz-max is 4: %w", numElem, ssz.ErrListTooBig)
		}
		c.PayloadExpectedWithdrawals = make([]*v1.Withdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *v1.Withdrawal
			tmp = new(v1.Withdrawal)
			tmpSlice := sszSlice44[i*44 : (1+i)*44]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
			}
			c.PayloadExpectedWithdrawals[i] = tmp
		}
	}

	// Field 45: PtcWindow
	{
		c.PtcWindow = make([]*PTCs, 24)
		for i := 0; i < 24; i++ {
			var tmp *PTCs
			tmp = new(PTCs)
			tmpSlice := sszSlice45[i*128 : (1+i)*128]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PtcWindow: %w", err)
			}
			c.PtcWindow[i] = tmp
		}
	}

	// Field 46: Height
	if err = c.Height.UnmarshalSSZ(sszSlice46); err != nil {
		return fmt.Errorf("Height: %w", err)
	}

	// Field 47: Entry
	c.Entry = new(CheckpointDecoupled)
	if err = c.Entry.UnmarshalSSZ(sszSlice47); err != nil {
		return fmt.Errorf("Entry: %w", err)
	}

	// Field 48: TargetParticipation
	if err = ssz.ValidateBitlist(sszSlice48, 1099511627776); err != nil {
		return fmt.Errorf("TargetParticipation: %w", err)
	}
	c.TargetParticipation = append([]byte{}, go_bitfield.Bitlist(sszSlice48)...)

	// Field 49: Progress
	if err = ssz.ValidateBitlist(sszSlice49, 1099511627776); err != nil {
		return fmt.Errorf("Progress: %w", err)
	}
	c.Progress = append([]byte{}, go_bitfield.Bitlist(sszSlice49)...)

	// Field 50: JustifiedHeight
	if err = c.JustifiedHeight.UnmarshalSSZ(sszSlice50); err != nil {
		return fmt.Errorf("JustifiedHeight: %w", err)
	}

	// Field 51: Justified
	c.Justified = new(CheckpointDecoupled)
	if err = c.Justified.UnmarshalSSZ(sszSlice51); err != nil {
		return fmt.Errorf("Justified: %w", err)
	}

	// Field 52: FinalizedHeight
	if err = c.FinalizedHeight.UnmarshalSSZ(sszSlice52); err != nil {
		return fmt.Errorf("FinalizedHeight: %w", err)
	}

	// Field 53: Finalized
	c.Finalized = new(CheckpointDecoupled)
	if err = c.Finalized.UnmarshalSSZ(sszSlice53); err != nil {
		return fmt.Errorf("Finalized: %w", err)
	}

	// Field 54: PendingFinality
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice54) > 3 {
			startOffset := ssz.ReadOffset(sszSlice54[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.PendingFinality")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.PendingFinality, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			if listLen > 8 {
				return fmt.Errorf("ssz-max exceeded: c.PendingFinality has %d elements, ssz-max is 8: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice54))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.PendingFinality")
			}
			c.PendingFinality = make([]*PendingFinality, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp *PendingFinality
				tmp = new(PendingFinality)
				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice54[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.PendingFinality", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.PendingFinality", endOffset, startOffset)
				}
				tmpSlice = sszSlice54[startOffset:endOffset]
				if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
					return fmt.Errorf("PendingFinality: %w", err)
				}
				c.PendingFinality[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice54) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.PendingFinality")
			}
			c.PendingFinality = make([]*PendingFinality, 0)
		}
	}
	return err
}

func (c *BeaconStateDecoupled) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *BeaconStateDecoupled) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsBeaconStateDecoupled = []byte{0b11111111, 0b11111111, 0b11111111, 0b11111111, 0b11111111, 0b11111111, 0b01111111}

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
	// Field 15: PreviousEpochParticipation
	{
		subIndx := hh.Index()
		hh.AppendBytes32(c.PreviousEpochParticipation)
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PreviousEpochParticipation)))
	}
	// Field 16: CurrentEpochParticipation
	{
		subIndx := hh.Index()
		hh.AppendBytes32(c.CurrentEpochParticipation)
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.CurrentEpochParticipation)))
	}
	// Field 17: JustificationBits
	if len([]byte(c.JustificationBits)) != 1 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.JustificationBits))
	// Field 18: PreviousJustifiedCheckpoint
	if err := c.PreviousJustifiedCheckpoint.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("PreviousJustifiedCheckpoint: %w", err)
	}
	// Field 19: CurrentJustifiedCheckpoint
	if err := c.CurrentJustifiedCheckpoint.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("CurrentJustifiedCheckpoint: %w", err)
	}
	// Field 20: FinalizedCheckpoint
	if err := c.FinalizedCheckpoint.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("FinalizedCheckpoint: %w", err)
	}
	// Field 21: InactivityScores
	{
		subIndx := hh.Index()
		for _, o := range c.InactivityScores {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.InactivityScores)))
	}
	// Field 22: CurrentSyncCommittee
	if err := c.CurrentSyncCommittee.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("CurrentSyncCommittee: %w", err)
	}
	// Field 23: NextSyncCommittee
	if err := c.NextSyncCommittee.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextSyncCommittee: %w", err)
	}
	// Field 24: LatestBlockHash
	if len(c.LatestBlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LatestBlockHash)
	// Field 25: NextWithdrawalIndex
	hh.PutUint64(c.NextWithdrawalIndex)
	// Field 26: NextWithdrawalValidatorIndex
	if err := c.NextWithdrawalValidatorIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextWithdrawalValidatorIndex: %w", err)
	}
	// Field 27: HistoricalSummaries
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
	// Field 28: DepositRequestsStartIndex
	hh.PutUint64(c.DepositRequestsStartIndex)
	// Field 29: DepositBalanceToConsume
	if err := c.DepositBalanceToConsume.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("DepositBalanceToConsume: %w", err)
	}
	// Field 30: ExitBalanceToConsume
	if err := c.ExitBalanceToConsume.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExitBalanceToConsume: %w", err)
	}
	// Field 31: EarliestExitEpoch
	if err := c.EarliestExitEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("EarliestExitEpoch: %w", err)
	}
	// Field 32: ConsolidationBalanceToConsume
	if err := c.ConsolidationBalanceToConsume.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ConsolidationBalanceToConsume: %w", err)
	}
	// Field 33: EarliestConsolidationEpoch
	if err := c.EarliestConsolidationEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("EarliestConsolidationEpoch: %w", err)
	}
	// Field 34: PendingDeposits
	{
		subIndx := hh.Index()
		for _, o := range c.PendingDeposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingDeposits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingDeposits)))
	}
	// Field 35: PendingPartialWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.PendingPartialWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingPartialWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingPartialWithdrawals)))
	}
	// Field 36: PendingConsolidations
	{
		subIndx := hh.Index()
		for _, o := range c.PendingConsolidations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingConsolidations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingConsolidations)))
	}
	// Field 37: ProposerLookahead
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
	// Field 38: Builders
	{
		subIndx := hh.Index()
		for _, o := range c.Builders {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Builders: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Builders)))
	}
	// Field 39: NextWithdrawalBuilderIndex
	if err := c.NextWithdrawalBuilderIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextWithdrawalBuilderIndex: %w", err)
	}
	// Field 40: ExecutionPayloadAvailability
	if len(c.ExecutionPayloadAvailability) != 8 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ExecutionPayloadAvailability)
	// Field 41: BuilderPendingPayments
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
	// Field 42: BuilderPendingWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.BuilderPendingWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderPendingWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BuilderPendingWithdrawals)))
	}
	// Field 43: LatestExecutionPayloadBid
	if err := c.LatestExecutionPayloadBid.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}
	// Field 44: PayloadExpectedWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.PayloadExpectedWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PayloadExpectedWithdrawals)))
	}
	// Field 45: PtcWindow
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
	// Field 46: Height
	if err := c.Height.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Height: %w", err)
	}
	// Field 47: Entry
	if err := c.Entry.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Entry: %w", err)
	}
	// Field 48: TargetParticipation
	if len(c.TargetParticipation) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutProgressiveBitlist(c.TargetParticipation)
	// Field 49: Progress
	if len(c.Progress) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutProgressiveBitlist(c.Progress)
	// Field 50: JustifiedHeight
	if err := c.JustifiedHeight.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("JustifiedHeight: %w", err)
	}
	// Field 51: Justified
	if err := c.Justified.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Justified: %w", err)
	}
	// Field 52: FinalizedHeight
	if err := c.FinalizedHeight.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("FinalizedHeight: %w", err)
	}
	// Field 53: Finalized
	if err := c.Finalized.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Finalized: %w", err)
	}
	// Field 54: PendingFinality
	{
		if len(c.PendingFinality) > 8 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.PendingFinality {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingFinality: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.PendingFinality)), 8)
	}
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsBeaconStateDecoupled)
	return nil
}

func (c *BeaconBlockBodyDecoupled) SizeSSZ() int {
	size := 344
	size += len(c.ProposerSlashings) * 416
	for _, o := range c.AttesterSlashings {
		size += 4
		size += o.SizeSSZ()
	}
	for _, o := range c.Attestations {
		size += 4
		size += o.SizeSSZ()
	}
	size += len(c.Deposits) * 1240
	size += len(c.VoluntaryExits) * 112
	size += len(c.BlsToExecutionChanges) * 172
	if c.SignedExecutionPayloadBid == nil {
		c.SignedExecutionPayloadBid = new(SignedExecutionPayloadBid)
	}
	size += c.SignedExecutionPayloadBid.SizeSSZ()
	size += len(c.PayloadAttestations) * 140
	if c.ParentExecutionRequests == nil {
		c.ParentExecutionRequests = new(v1.ExecutionRequestsGloas)
	}
	size += c.ParentExecutionRequests.SizeSSZ()
	size += len(c.AvailableAttestations) * 201
	size += len(c.SupportVotes)
	return size
}

func (c *BeaconBlockBodyDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockBodyDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 344

	// Field 0: RandaoReveal
	if len(c.RandaoReveal) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.RandaoReveal...)

	// Field 1: Eth1Data
	if c.Eth1Data == nil {
		c.Eth1Data = new(Eth1Data)
	}
	if dst, err = c.Eth1Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Eth1Data: %w", err)
	}

	// Field 2: Graffiti
	if len(c.Graffiti) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Graffiti...)

	// Field 3: ProposerSlashings
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ProposerSlashings) * 416

	// Field 4: AttesterSlashings
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.AttesterSlashings {
		offset += 4
		offset += o.SizeSSZ()
	}

	// Field 5: Attestations
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.Attestations {
		offset += 4
		offset += o.SizeSSZ()
	}

	// Field 6: Deposits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Deposits) * 1240

	// Field 7: VoluntaryExits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.VoluntaryExits) * 112

	// Field 8: SyncAggregate
	if c.SyncAggregate == nil {
		c.SyncAggregate = new(SyncAggregate)
	}
	if dst, err = c.SyncAggregate.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 9: BlsToExecutionChanges
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BlsToExecutionChanges) * 172

	// Field 10: SignedExecutionPayloadBid
	if c.SignedExecutionPayloadBid == nil {
		c.SignedExecutionPayloadBid = new(SignedExecutionPayloadBid)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.SignedExecutionPayloadBid.SizeSSZ()

	// Field 11: PayloadAttestations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PayloadAttestations) * 140

	// Field 12: ParentExecutionRequests
	if c.ParentExecutionRequests == nil {
		c.ParentExecutionRequests = new(v1.ExecutionRequestsGloas)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.ParentExecutionRequests.SizeSSZ()

	// Field 13: AvailableAttestations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.AvailableAttestations) * 201

	// Field 14: SupportVotes
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.SupportVotes)

	// Field 3: ProposerSlashings
	for _, o := range c.ProposerSlashings {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("ProposerSlashings: %w", err)
		}
	}

	// Field 4: AttesterSlashings
	{
		offset = 4 * len(c.AttesterSlashings)
		for _, o := range c.AttesterSlashings {
			dst = ssz.WriteOffset(dst, offset)
			offset += o.SizeSSZ()
		}
	}
	for _, o := range c.AttesterSlashings {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("AttesterSlashings: %w", err)
		}
	}

	// Field 5: Attestations
	{
		offset = 4 * len(c.Attestations)
		for _, o := range c.Attestations {
			dst = ssz.WriteOffset(dst, offset)
			offset += o.SizeSSZ()
		}
	}
	for _, o := range c.Attestations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Attestations: %w", err)
		}
	}

	// Field 6: Deposits
	for _, o := range c.Deposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Deposits: %w", err)
		}
	}

	// Field 7: VoluntaryExits
	for _, o := range c.VoluntaryExits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("VoluntaryExits: %w", err)
		}
	}

	// Field 9: BlsToExecutionChanges
	for _, o := range c.BlsToExecutionChanges {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BlsToExecutionChanges: %w", err)
		}
	}

	// Field 10: SignedExecutionPayloadBid
	if dst, err = c.SignedExecutionPayloadBid.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}

	// Field 11: PayloadAttestations
	for _, o := range c.PayloadAttestations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PayloadAttestations: %w", err)
		}
	}

	// Field 12: ParentExecutionRequests
	if dst, err = c.ParentExecutionRequests.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ParentExecutionRequests: %w", err)
	}

	// Field 13: AvailableAttestations
	for _, o := range c.AvailableAttestations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("AvailableAttestations: %w", err)
		}
	}

	// Field 14: SupportVotes
	dst = append(dst, c.SupportVotes...)
	return dst, err
}

func (c *BeaconBlockBodyDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 344 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:96]    // c.RandaoReveal
	sszSlice1 := buf[96:168]  // c.Eth1Data
	sszSlice2 := buf[168:200] // c.Graffiti
	sszSlice8 := buf[220:320] // c.SyncAggregate

	sszVarOffset3 := ssz.ReadOffset(buf[200:204]) // c.ProposerSlashings
	if sszVarOffset3 != 344 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset3 > size {
		return ssz.ErrOffset
	}
	sszVarOffset4 := ssz.ReadOffset(buf[204:208]) // c.AttesterSlashings
	if sszVarOffset4 > size || sszVarOffset4 < sszVarOffset3 {
		return ssz.ErrOffset
	}
	sszVarOffset5 := ssz.ReadOffset(buf[208:212]) // c.Attestations
	if sszVarOffset5 > size || sszVarOffset5 < sszVarOffset4 {
		return ssz.ErrOffset
	}
	sszVarOffset6 := ssz.ReadOffset(buf[212:216]) // c.Deposits
	if sszVarOffset6 > size || sszVarOffset6 < sszVarOffset5 {
		return ssz.ErrOffset
	}
	sszVarOffset7 := ssz.ReadOffset(buf[216:220]) // c.VoluntaryExits
	if sszVarOffset7 > size || sszVarOffset7 < sszVarOffset6 {
		return ssz.ErrOffset
	}
	sszVarOffset9 := ssz.ReadOffset(buf[320:324]) // c.BlsToExecutionChanges
	if sszVarOffset9 > size || sszVarOffset9 < sszVarOffset7 {
		return ssz.ErrOffset
	}
	sszVarOffset10 := ssz.ReadOffset(buf[324:328]) // c.SignedExecutionPayloadBid
	if sszVarOffset10 > size || sszVarOffset10 < sszVarOffset9 {
		return ssz.ErrOffset
	}
	sszVarOffset11 := ssz.ReadOffset(buf[328:332]) // c.PayloadAttestations
	if sszVarOffset11 > size || sszVarOffset11 < sszVarOffset10 {
		return ssz.ErrOffset
	}
	sszVarOffset12 := ssz.ReadOffset(buf[332:336]) // c.ParentExecutionRequests
	if sszVarOffset12 > size || sszVarOffset12 < sszVarOffset11 {
		return ssz.ErrOffset
	}
	sszVarOffset13 := ssz.ReadOffset(buf[336:340]) // c.AvailableAttestations
	if sszVarOffset13 > size || sszVarOffset13 < sszVarOffset12 {
		return ssz.ErrOffset
	}
	sszVarOffset14 := ssz.ReadOffset(buf[340:344]) // c.SupportVotes
	if sszVarOffset14 > size || sszVarOffset14 < sszVarOffset13 {
		return ssz.ErrOffset
	}
	sszSlice3 := buf[sszVarOffset3:sszVarOffset4]    // c.ProposerSlashings
	sszSlice4 := buf[sszVarOffset4:sszVarOffset5]    // c.AttesterSlashings
	sszSlice5 := buf[sszVarOffset5:sszVarOffset6]    // c.Attestations
	sszSlice6 := buf[sszVarOffset6:sszVarOffset7]    // c.Deposits
	sszSlice7 := buf[sszVarOffset7:sszVarOffset9]    // c.VoluntaryExits
	sszSlice9 := buf[sszVarOffset9:sszVarOffset10]   // c.BlsToExecutionChanges
	sszSlice10 := buf[sszVarOffset10:sszVarOffset11] // c.SignedExecutionPayloadBid
	sszSlice11 := buf[sszVarOffset11:sszVarOffset12] // c.PayloadAttestations
	sszSlice12 := buf[sszVarOffset12:sszVarOffset13] // c.ParentExecutionRequests
	sszSlice13 := buf[sszVarOffset13:sszVarOffset14] // c.AvailableAttestations
	sszSlice14 := buf[sszVarOffset14:]               // c.SupportVotes

	// Field 0: RandaoReveal
	c.RandaoReveal = make([]byte, 0, 96)
	c.RandaoReveal = append(c.RandaoReveal, sszSlice0...)

	// Field 1: Eth1Data
	c.Eth1Data = new(Eth1Data)
	if err = c.Eth1Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Eth1Data: %w", err)
	}

	// Field 2: Graffiti
	c.Graffiti = make([]byte, 0, 32)
	c.Graffiti = append(c.Graffiti, sszSlice2...)

	// Field 3: ProposerSlashings
	{
		if len(sszSlice3)%416 != 0 {
			return fmt.Errorf("misaligned bytes: c.ProposerSlashings length is %d, which is not a multiple of 416: %w", len(sszSlice3), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice3) / 416
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.ProposerSlashings has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.ProposerSlashings = make([]*ProposerSlashing, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ProposerSlashing
			tmp = new(ProposerSlashing)
			tmpSlice := sszSlice3[i*416 : (1+i)*416]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("ProposerSlashings: %w", err)
			}
			c.ProposerSlashings[i] = tmp
		}
	}

	// Field 4: AttesterSlashings
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice4) > 3 {
			startOffset := ssz.ReadOffset(sszSlice4[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.AttesterSlashings")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.AttesterSlashings, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			if listLen > 1 {
				return fmt.Errorf("ssz-max exceeded: c.AttesterSlashings has %d elements, ssz-max is 1: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice4))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.AttesterSlashings")
			}
			c.AttesterSlashings = make([]*AttesterSlashingGloas, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp *AttesterSlashingGloas
				tmp = new(AttesterSlashingGloas)
				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice4[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.AttesterSlashings", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.AttesterSlashings", endOffset, startOffset)
				}
				tmpSlice = sszSlice4[startOffset:endOffset]
				if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
					return fmt.Errorf("AttesterSlashings: %w", err)
				}
				c.AttesterSlashings[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice4) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.AttesterSlashings")
			}
			c.AttesterSlashings = make([]*AttesterSlashingGloas, 0)
		}
	}

	// Field 5: Attestations
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice5) > 3 {
			startOffset := ssz.ReadOffset(sszSlice5[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.Attestations")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.Attestations, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			if listLen > 8 {
				return fmt.Errorf("ssz-max exceeded: c.Attestations has %d elements, ssz-max is 8: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice5))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Attestations")
			}
			c.Attestations = make([]*IndexedAttestationDecoupled, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp *IndexedAttestationDecoupled
				tmp = new(IndexedAttestationDecoupled)
				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice5[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.Attestations", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.Attestations", endOffset, startOffset)
				}
				tmpSlice = sszSlice5[startOffset:endOffset]
				if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
					return fmt.Errorf("Attestations: %w", err)
				}
				c.Attestations[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice5) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Attestations")
			}
			c.Attestations = make([]*IndexedAttestationDecoupled, 0)
		}
	}

	// Field 6: Deposits
	{
		if len(sszSlice6)%1240 != 0 {
			return fmt.Errorf("misaligned bytes: c.Deposits length is %d, which is not a multiple of 1240: %w", len(sszSlice6), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice6) / 1240
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.Deposits has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.Deposits = make([]*Deposit, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Deposit
			tmp = new(Deposit)
			tmpSlice := sszSlice6[i*1240 : (1+i)*1240]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
			c.Deposits[i] = tmp
		}
	}

	// Field 7: VoluntaryExits
	{
		if len(sszSlice7)%112 != 0 {
			return fmt.Errorf("misaligned bytes: c.VoluntaryExits length is %d, which is not a multiple of 112: %w", len(sszSlice7), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice7) / 112
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.VoluntaryExits has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.VoluntaryExits = make([]*SignedVoluntaryExit, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *SignedVoluntaryExit
			tmp = new(SignedVoluntaryExit)
			tmpSlice := sszSlice7[i*112 : (1+i)*112]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("VoluntaryExits: %w", err)
			}
			c.VoluntaryExits[i] = tmp
		}
	}

	// Field 8: SyncAggregate
	c.SyncAggregate = new(SyncAggregate)
	if err = c.SyncAggregate.UnmarshalSSZ(sszSlice8); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 9: BlsToExecutionChanges
	{
		if len(sszSlice9)%172 != 0 {
			return fmt.Errorf("misaligned bytes: c.BlsToExecutionChanges length is %d, which is not a multiple of 172: %w", len(sszSlice9), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice9) / 172
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.BlsToExecutionChanges has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.BlsToExecutionChanges = make([]*SignedBLSToExecutionChange, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *SignedBLSToExecutionChange
			tmp = new(SignedBLSToExecutionChange)
			tmpSlice := sszSlice9[i*172 : (1+i)*172]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BlsToExecutionChanges: %w", err)
			}
			c.BlsToExecutionChanges[i] = tmp
		}
	}

	// Field 10: SignedExecutionPayloadBid
	c.SignedExecutionPayloadBid = new(SignedExecutionPayloadBid)
	if err = c.SignedExecutionPayloadBid.UnmarshalSSZ(sszSlice10); err != nil {
		return fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}

	// Field 11: PayloadAttestations
	{
		if len(sszSlice11)%140 != 0 {
			return fmt.Errorf("misaligned bytes: c.PayloadAttestations length is %d, which is not a multiple of 140: %w", len(sszSlice11), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice11) / 140
		if numElem > 4 {
			return fmt.Errorf("ssz-max exceeded: c.PayloadAttestations has %d elements, ssz-max is 4: %w", numElem, ssz.ErrListTooBig)
		}
		c.PayloadAttestations = make([]*PayloadAttestation, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PayloadAttestation
			tmp = new(PayloadAttestation)
			tmpSlice := sszSlice11[i*140 : (1+i)*140]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PayloadAttestations: %w", err)
			}
			c.PayloadAttestations[i] = tmp
		}
	}

	// Field 12: ParentExecutionRequests
	c.ParentExecutionRequests = new(v1.ExecutionRequestsGloas)
	if err = c.ParentExecutionRequests.UnmarshalSSZ(sszSlice12); err != nil {
		return fmt.Errorf("ParentExecutionRequests: %w", err)
	}

	// Field 13: AvailableAttestations
	{
		if len(sszSlice13)%201 != 0 {
			return fmt.Errorf("misaligned bytes: c.AvailableAttestations length is %d, which is not a multiple of 201: %w", len(sszSlice13), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice13) / 201
		if numElem > 8 {
			return fmt.Errorf("ssz-max exceeded: c.AvailableAttestations has %d elements, ssz-max is 8: %w", numElem, ssz.ErrListTooBig)
		}
		c.AvailableAttestations = make([]*AvailableAttestation, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *AvailableAttestation
			tmp = new(AvailableAttestation)
			tmpSlice := sszSlice13[i*201 : (1+i)*201]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("AvailableAttestations: %w", err)
			}
			c.AvailableAttestations[i] = tmp
		}
	}

	// Field 14: SupportVotes
	if err = ssz.ValidateBitlist(sszSlice14, 8); err != nil {
		return fmt.Errorf("SupportVotes: %w", err)
	}
	c.SupportVotes = append([]byte{}, go_bitfield.Bitlist(sszSlice14)...)
	return err
}

func (c *BeaconBlockBodyDecoupled) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *BeaconBlockBodyDecoupled) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsBeaconBlockBodyDecoupled = []byte{0b11111111, 0b01111111}

func (c *BeaconBlockBodyDecoupled) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockBodyDecoupled) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: RandaoReveal
	if len(c.RandaoReveal) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.RandaoReveal)
	// Field 1: Eth1Data
	if err := c.Eth1Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Eth1Data: %w", err)
	}
	// Field 2: Graffiti
	if len(c.Graffiti) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Graffiti)
	// Field 3: ProposerSlashings
	{
		subIndx := hh.Index()
		for _, o := range c.ProposerSlashings {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("ProposerSlashings: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.ProposerSlashings)))
	}
	// Field 4: AttesterSlashings
	{
		subIndx := hh.Index()
		for _, o := range c.AttesterSlashings {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("AttesterSlashings: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.AttesterSlashings)))
	}
	// Field 5: Attestations
	{
		subIndx := hh.Index()
		for _, o := range c.Attestations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Attestations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Attestations)))
	}
	// Field 6: Deposits
	{
		subIndx := hh.Index()
		for _, o := range c.Deposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Deposits)))
	}
	// Field 7: VoluntaryExits
	{
		subIndx := hh.Index()
		for _, o := range c.VoluntaryExits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("VoluntaryExits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.VoluntaryExits)))
	}
	// Field 8: SyncAggregate
	if err := c.SyncAggregate.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}
	// Field 9: BlsToExecutionChanges
	{
		subIndx := hh.Index()
		for _, o := range c.BlsToExecutionChanges {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BlsToExecutionChanges: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BlsToExecutionChanges)))
	}
	// Field 10: SignedExecutionPayloadBid
	if err := c.SignedExecutionPayloadBid.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}
	// Field 11: PayloadAttestations
	{
		subIndx := hh.Index()
		for _, o := range c.PayloadAttestations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PayloadAttestations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PayloadAttestations)))
	}
	// Field 12: ParentExecutionRequests
	if err := c.ParentExecutionRequests.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ParentExecutionRequests: %w", err)
	}
	// Field 13: AvailableAttestations
	{
		subIndx := hh.Index()
		for _, o := range c.AvailableAttestations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("AvailableAttestations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.AvailableAttestations)))
	}
	// Field 14: SupportVotes
	if len(c.SupportVotes) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutProgressiveBitlist(c.SupportVotes)
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsBeaconBlockBodyDecoupled)
	return nil
}

func (c *BeaconBlockDecoupled) SizeSSZ() int {
	size := 84
	if c.Body == nil {
		c.Body = new(BeaconBlockBodyDecoupled)
	}
	size += c.Body.SizeSSZ()
	return size
}

func (c *BeaconBlockDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 84

	// Field 0: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 1: ProposerIndex
	if dst, err = c.ProposerIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ProposerIndex: %w", err)
	}

	// Field 2: ParentRoot
	if len(c.ParentRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentRoot...)

	// Field 3: StateRoot
	if len(c.StateRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateRoot...)

	// Field 4: Body
	if c.Body == nil {
		c.Body = new(BeaconBlockBodyDecoupled)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Body.SizeSSZ()

	// Field 4: Body
	if dst, err = c.Body.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Body: %w", err)
	}
	return dst, err
}

func (c *BeaconBlockDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 84 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.Slot
	sszSlice1 := buf[8:16]  // c.ProposerIndex
	sszSlice2 := buf[16:48] // c.ParentRoot
	sszSlice3 := buf[48:80] // c.StateRoot

	sszVarOffset4 := ssz.ReadOffset(buf[80:84]) // c.Body
	if sszVarOffset4 != 84 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset4 > size {
		return ssz.ErrOffset
	}
	sszSlice4 := buf[sszVarOffset4:] // c.Body

	// Field 0: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 1: ProposerIndex
	if err = c.ProposerIndex.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("ProposerIndex: %w", err)
	}

	// Field 2: ParentRoot
	c.ParentRoot = make([]byte, 0, 32)
	c.ParentRoot = append(c.ParentRoot, sszSlice2...)

	// Field 3: StateRoot
	c.StateRoot = make([]byte, 0, 32)
	c.StateRoot = append(c.StateRoot, sszSlice3...)

	// Field 4: Body
	c.Body = new(BeaconBlockBodyDecoupled)
	if err = c.Body.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("Body: %w", err)
	}
	return err
}

func (c *BeaconBlockDecoupled) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockDecoupled) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 1: ProposerIndex
	if err := c.ProposerIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ProposerIndex: %w", err)
	}
	// Field 2: ParentRoot
	if len(c.ParentRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentRoot)
	// Field 3: StateRoot
	if len(c.StateRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateRoot)
	// Field 4: Body
	if err := c.Body.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Body: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *SignedBeaconBlockDecoupled) SizeSSZ() int {
	size := 100
	if c.Block == nil {
		c.Block = new(BeaconBlockDecoupled)
	}
	size += c.Block.SizeSSZ()
	return size
}

func (c *SignedBeaconBlockDecoupled) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedBeaconBlockDecoupled) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Block
	if c.Block == nil {
		c.Block = new(BeaconBlockDecoupled)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Block.SizeSSZ()

	// Field 1: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	// Field 0: Block
	if dst, err = c.Block.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Block: %w", err)
	}
	return dst, err
}

func (c *SignedBeaconBlockDecoupled) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 100 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:100] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Block
	if sszVarOffset0 != 100 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Block

	// Field 0: Block
	c.Block = new(BeaconBlockDecoupled)
	if err = c.Block.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Block: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedBeaconBlockDecoupled) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedBeaconBlockDecoupled) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Block
	if err := c.Block.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Block: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}
