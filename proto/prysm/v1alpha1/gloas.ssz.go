//go:build !minimal

package eth

import (
	binary "encoding/binary"
	"fmt"
	go_bitfield "github.com/OffchainLabs/go-bitfield"
	ssz "github.com/OffchainLabs/methodical-ssz/ssz"
	primitives "github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	v1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

func (c *AttestationGloas) SizeSSZ() int {
	size := 236
	size += len(c.AggregationBits)
	return size
}

func (c *AttestationGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AttestationGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 236

	// Field 0: AggregationBits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.AggregationBits)

	// Field 1: Data
	if c.Data == nil {
		c.Data = new(AttestationData)
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
	if len(c.AggregationBits) > 131072 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.AggregationBits...)
	return dst, err
}

func (c *AttestationGloas) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 236 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:132]   // c.Data
	sszSlice2 := buf[132:228] // c.Signature
	sszSlice3 := buf[228:236] // c.CommitteeBits

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.AggregationBits
	if sszVarOffset0 != 236 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.AggregationBits

	// Field 0: AggregationBits
	if err = ssz.ValidateBitlist(sszSlice0, 131072); err != nil {
		return fmt.Errorf("AggregationBits: %w", err)
	}
	c.AggregationBits = append([]byte{}, go_bitfield.Bitlist(sszSlice0)...)

	// Field 1: Data
	c.Data = new(AttestationData)
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

func (c *AttestationGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AttestationGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AggregationBits
	if len(c.AggregationBits) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutBitlist(c.AggregationBits, 131072)
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
	hh.Merkleize(indx)
	return nil
}

func (c *AggregateAttestationAndProofGloas) SizeSSZ() int {
	size := 108
	if c.Aggregate == nil {
		c.Aggregate = new(AttestationGloas)
	}
	size += c.Aggregate.SizeSSZ()
	return size
}

func (c *AggregateAttestationAndProofGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AggregateAttestationAndProofGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 108

	// Field 0: AggregatorIndex
	if dst, err = c.AggregatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("AggregatorIndex: %w", err)
	}

	// Field 1: Aggregate
	if c.Aggregate == nil {
		c.Aggregate = new(AttestationGloas)
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

func (c *AggregateAttestationAndProofGloas) UnmarshalSSZ(buf []byte) error {
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
	c.Aggregate = new(AttestationGloas)
	if err = c.Aggregate.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Aggregate: %w", err)
	}

	// Field 2: SelectionProof
	c.SelectionProof = make([]byte, 0, 96)
	c.SelectionProof = append(c.SelectionProof, sszSlice2...)
	return err
}

func (c *AggregateAttestationAndProofGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AggregateAttestationAndProofGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *SignedAggregateAttestationAndProofGloas) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(AggregateAttestationAndProofGloas)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedAggregateAttestationAndProofGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedAggregateAttestationAndProofGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(AggregateAttestationAndProofGloas)
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

func (c *SignedAggregateAttestationAndProofGloas) UnmarshalSSZ(buf []byte) error {
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
	c.Message = new(AggregateAttestationAndProofGloas)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedAggregateAttestationAndProofGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedAggregateAttestationAndProofGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *AttesterSlashingGloas) SizeSSZ() int {
	size := 8
	if c.Attestation_1 == nil {
		c.Attestation_1 = new(IndexedAttestationGloas)
	}
	size += c.Attestation_1.SizeSSZ()
	if c.Attestation_2 == nil {
		c.Attestation_2 = new(IndexedAttestationGloas)
	}
	size += c.Attestation_2.SizeSSZ()
	return size
}

func (c *AttesterSlashingGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AttesterSlashingGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 8

	// Field 0: Attestation_1
	if c.Attestation_1 == nil {
		c.Attestation_1 = new(IndexedAttestationGloas)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Attestation_1.SizeSSZ()

	// Field 1: Attestation_2
	if c.Attestation_2 == nil {
		c.Attestation_2 = new(IndexedAttestationGloas)
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

func (c *AttesterSlashingGloas) UnmarshalSSZ(buf []byte) error {
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
	c.Attestation_1 = new(IndexedAttestationGloas)
	if err = c.Attestation_1.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Attestation_1: %w", err)
	}

	// Field 1: Attestation_2
	c.Attestation_2 = new(IndexedAttestationGloas)
	if err = c.Attestation_2.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Attestation_2: %w", err)
	}
	return err
}

func (c *AttesterSlashingGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AttesterSlashingGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *BeaconBlockBodyGloas) SizeSSZ() int {
	size := 396
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
	size += len(c.PayloadAttestations) * 202
	if c.ParentExecutionRequests == nil {
		c.ParentExecutionRequests = new(v1.ExecutionRequestsGloas)
	}
	size += c.ParentExecutionRequests.SizeSSZ()
	return size
}

func (c *BeaconBlockBodyGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockBodyGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 396

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
	offset += len(c.PayloadAttestations) * 202

	// Field 12: ParentExecutionRequests
	if c.ParentExecutionRequests == nil {
		c.ParentExecutionRequests = new(v1.ExecutionRequestsGloas)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.ParentExecutionRequests.SizeSSZ()

	// Field 3: ProposerSlashings
	if len(c.ProposerSlashings) > 16 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.ProposerSlashings {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("ProposerSlashings: %w", err)
		}
	}

	// Field 4: AttesterSlashings
	if len(c.AttesterSlashings) > 1 {
		return nil, ssz.ErrListTooBig
	}
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
	if len(c.Attestations) > 8 {
		return nil, ssz.ErrListTooBig
	}
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
	if len(c.Deposits) > 16 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Deposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Deposits: %w", err)
		}
	}

	// Field 7: VoluntaryExits
	if len(c.VoluntaryExits) > 16 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.VoluntaryExits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("VoluntaryExits: %w", err)
		}
	}

	// Field 9: BlsToExecutionChanges
	if len(c.BlsToExecutionChanges) > 16 {
		return nil, ssz.ErrListTooBig
	}
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
	if len(c.PayloadAttestations) > 4 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.PayloadAttestations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PayloadAttestations: %w", err)
		}
	}

	// Field 12: ParentExecutionRequests
	if dst, err = c.ParentExecutionRequests.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ParentExecutionRequests: %w", err)
	}
	return dst, err
}

func (c *BeaconBlockBodyGloas) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 396 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:96]    // c.RandaoReveal
	sszSlice1 := buf[96:168]  // c.Eth1Data
	sszSlice2 := buf[168:200] // c.Graffiti
	sszSlice8 := buf[220:380] // c.SyncAggregate

	sszVarOffset3 := ssz.ReadOffset(buf[200:204]) // c.ProposerSlashings
	if sszVarOffset3 != 396 {
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
	sszVarOffset9 := ssz.ReadOffset(buf[380:384]) // c.BlsToExecutionChanges
	if sszVarOffset9 > size || sszVarOffset9 < sszVarOffset7 {
		return ssz.ErrOffset
	}
	sszVarOffset10 := ssz.ReadOffset(buf[384:388]) // c.SignedExecutionPayloadBid
	if sszVarOffset10 > size || sszVarOffset10 < sszVarOffset9 {
		return ssz.ErrOffset
	}
	sszVarOffset11 := ssz.ReadOffset(buf[388:392]) // c.PayloadAttestations
	if sszVarOffset11 > size || sszVarOffset11 < sszVarOffset10 {
		return ssz.ErrOffset
	}
	sszVarOffset12 := ssz.ReadOffset(buf[392:396]) // c.ParentExecutionRequests
	if sszVarOffset12 > size || sszVarOffset12 < sszVarOffset11 {
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
	sszSlice12 := buf[sszVarOffset12:]               // c.ParentExecutionRequests

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
			c.Attestations = make([]*AttestationGloas, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp *AttestationGloas
				tmp = new(AttestationGloas)
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
			c.Attestations = make([]*AttestationGloas, 0)
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
		if len(sszSlice11)%202 != 0 {
			return fmt.Errorf("misaligned bytes: c.PayloadAttestations length is %d, which is not a multiple of 202: %w", len(sszSlice11), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice11) / 202
		if numElem > 4 {
			return fmt.Errorf("ssz-max exceeded: c.PayloadAttestations has %d elements, ssz-max is 4: %w", numElem, ssz.ErrListTooBig)
		}
		c.PayloadAttestations = make([]*PayloadAttestation, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PayloadAttestation
			tmp = new(PayloadAttestation)
			tmpSlice := sszSlice11[i*202 : (1+i)*202]
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
	return err
}

func (c *BeaconBlockBodyGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockBodyGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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
		if len(c.ProposerSlashings) > 16 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.ProposerSlashings {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("ProposerSlashings: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.ProposerSlashings)), 16)
	}
	// Field 4: AttesterSlashings
	{
		if len(c.AttesterSlashings) > 1 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.AttesterSlashings {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("AttesterSlashings: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.AttesterSlashings)), 1)
	}
	// Field 5: Attestations
	{
		if len(c.Attestations) > 8 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Attestations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Attestations: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Attestations)), 8)
	}
	// Field 6: Deposits
	{
		if len(c.Deposits) > 16 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Deposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Deposits)), 16)
	}
	// Field 7: VoluntaryExits
	{
		if len(c.VoluntaryExits) > 16 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.VoluntaryExits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("VoluntaryExits: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.VoluntaryExits)), 16)
	}
	// Field 8: SyncAggregate
	if err := c.SyncAggregate.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}
	// Field 9: BlsToExecutionChanges
	{
		if len(c.BlsToExecutionChanges) > 16 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.BlsToExecutionChanges {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BlsToExecutionChanges: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.BlsToExecutionChanges)), 16)
	}
	// Field 10: SignedExecutionPayloadBid
	if err := c.SignedExecutionPayloadBid.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}
	// Field 11: PayloadAttestations
	{
		if len(c.PayloadAttestations) > 4 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.PayloadAttestations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PayloadAttestations: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.PayloadAttestations)), 4)
	}
	// Field 12: ParentExecutionRequests
	if err := c.ParentExecutionRequests.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ParentExecutionRequests: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *BeaconBlockContentsGloas) SizeSSZ() int {
	size := 16
	if c.Block == nil {
		c.Block = new(BeaconBlockGloas)
	}
	size += c.Block.SizeSSZ()
	if c.ExecutionPayloadEnvelope == nil {
		c.ExecutionPayloadEnvelope = new(ExecutionPayloadEnvelope)
	}
	size += c.ExecutionPayloadEnvelope.SizeSSZ()
	size += len(c.KzgProofs) * 48
	size += len(c.Blobs) * 131072
	return size
}

func (c *BeaconBlockContentsGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockContentsGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 16

	// Field 0: Block
	if c.Block == nil {
		c.Block = new(BeaconBlockGloas)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Block.SizeSSZ()

	// Field 1: ExecutionPayloadEnvelope
	if c.ExecutionPayloadEnvelope == nil {
		c.ExecutionPayloadEnvelope = new(ExecutionPayloadEnvelope)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.ExecutionPayloadEnvelope.SizeSSZ()

	// Field 2: KzgProofs
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.KzgProofs) * 48

	// Field 3: Blobs
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Blobs) * 131072

	// Field 0: Block
	if dst, err = c.Block.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Block: %w", err)
	}

	// Field 1: ExecutionPayloadEnvelope
	if dst, err = c.ExecutionPayloadEnvelope.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ExecutionPayloadEnvelope: %w", err)
	}

	// Field 2: KzgProofs
	if len(c.KzgProofs) > 33554432 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.KzgProofs {
		if len(o) != 48 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 3: Blobs
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

func (c *BeaconBlockContentsGloas) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 16 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Block
	if sszVarOffset0 != 16 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.ExecutionPayloadEnvelope
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[8:12]) // c.KzgProofs
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszVarOffset3 := ssz.ReadOffset(buf[12:16]) // c.Blobs
	if sszVarOffset3 > size || sszVarOffset3 < sszVarOffset2 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.Block
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.ExecutionPayloadEnvelope
	sszSlice2 := buf[sszVarOffset2:sszVarOffset3] // c.KzgProofs
	sszSlice3 := buf[sszVarOffset3:]              // c.Blobs

	// Field 0: Block
	c.Block = new(BeaconBlockGloas)
	if err = c.Block.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Block: %w", err)
	}

	// Field 1: ExecutionPayloadEnvelope
	c.ExecutionPayloadEnvelope = new(ExecutionPayloadEnvelope)
	if err = c.ExecutionPayloadEnvelope.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("ExecutionPayloadEnvelope: %w", err)
	}

	// Field 2: KzgProofs
	{
		if len(sszSlice2)%48 != 0 {
			return fmt.Errorf("misaligned bytes: c.KzgProofs length is %d, which is not a multiple of 48: %w", len(sszSlice2), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice2) / 48
		if numElem > 33554432 {
			return fmt.Errorf("ssz-max exceeded: c.KzgProofs has %d elements, ssz-max is 33554432: %w", numElem, ssz.ErrListTooBig)
		}
		c.KzgProofs = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice2[i*48 : (1+i)*48]
			tmp = make([]byte, 0, 48)
			tmp = append(tmp, tmpSlice...)
			c.KzgProofs[i] = tmp
		}
	}

	// Field 3: Blobs
	{
		if len(sszSlice3)%131072 != 0 {
			return fmt.Errorf("misaligned bytes: c.Blobs length is %d, which is not a multiple of 131072: %w", len(sszSlice3), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice3) / 131072
		if numElem > 4096 {
			return fmt.Errorf("ssz-max exceeded: c.Blobs has %d elements, ssz-max is 4096: %w", numElem, ssz.ErrListTooBig)
		}
		c.Blobs = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice3[i*131072 : (1+i)*131072]
			tmp = make([]byte, 0, 131072)
			tmp = append(tmp, tmpSlice...)
			c.Blobs[i] = tmp
		}
	}
	return err
}

func (c *BeaconBlockContentsGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockContentsGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Block
	if err := c.Block.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Block: %w", err)
	}
	// Field 1: ExecutionPayloadEnvelope
	if err := c.ExecutionPayloadEnvelope.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExecutionPayloadEnvelope: %w", err)
	}
	// Field 2: KzgProofs
	{
		if len(c.KzgProofs) > 33554432 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.KzgProofs {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.KzgProofs)), 33554432)
	}
	// Field 3: Blobs
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

func (c *BeaconBlockGloas) SizeSSZ() int {
	size := 84
	if c.Body == nil {
		c.Body = new(BeaconBlockBodyGloas)
	}
	size += c.Body.SizeSSZ()
	return size
}

func (c *BeaconBlockGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
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
		c.Body = new(BeaconBlockBodyGloas)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Body.SizeSSZ()

	// Field 4: Body
	if dst, err = c.Body.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Body: %w", err)
	}
	return dst, err
}

func (c *BeaconBlockGloas) UnmarshalSSZ(buf []byte) error {
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
	c.Body = new(BeaconBlockBodyGloas)
	if err = c.Body.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("Body: %w", err)
	}
	return err
}

func (c *BeaconBlockGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *BeaconStateGloas) SizeSSZ() int {
	size := 3134845
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
	return size
}

func (c *BeaconStateGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconStateGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 3134845

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
	if len(c.BlockRoots) != 8192 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.BlockRoots {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 6: StateRoots
	if len(c.StateRoots) != 8192 {
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
	if len(c.RandaoMixes) != 65536 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.RandaoMixes {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 14: Slashings
	if len(c.Slashings) != 8192 {
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
	if len(c.ProposerLookahead) != 64 {
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
	if len(c.ExecutionPayloadAvailability) != 1024 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ExecutionPayloadAvailability...)

	// Field 41: BuilderPendingPayments
	if len(c.BuilderPendingPayments) != 64 {
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
	if len(c.PtcWindow) != 96 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.PtcWindow {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PtcWindow: %w", err)
		}
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
	if len(c.Eth1DataVotes) > 2048 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Eth1DataVotes {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Eth1DataVotes: %w", err)
		}
	}

	// Field 11: Validators
	if len(c.Validators) > 1099511627776 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Validators {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Validators: %w", err)
		}
	}

	// Field 12: Balances
	if len(c.Balances) > 1099511627776 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Balances {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 15: PreviousEpochParticipation
	if len(c.PreviousEpochParticipation) > 1099511627776 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.PreviousEpochParticipation...)

	// Field 16: CurrentEpochParticipation
	if len(c.CurrentEpochParticipation) > 1099511627776 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.CurrentEpochParticipation...)

	// Field 21: InactivityScores
	if len(c.InactivityScores) > 1099511627776 {
		return nil, ssz.ErrListTooBig
	}
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
	if len(c.PendingDeposits) > 134217728 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.PendingDeposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingDeposits: %w", err)
		}
	}

	// Field 35: PendingPartialWithdrawals
	if len(c.PendingPartialWithdrawals) > 134217728 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.PendingPartialWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingPartialWithdrawals: %w", err)
		}
	}

	// Field 36: PendingConsolidations
	if len(c.PendingConsolidations) > 262144 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.PendingConsolidations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingConsolidations: %w", err)
		}
	}

	// Field 38: Builders
	if len(c.Builders) > 1099511627776 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Builders {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Builders: %w", err)
		}
	}

	// Field 42: BuilderPendingWithdrawals
	if len(c.BuilderPendingWithdrawals) > 1048576 {
		return nil, ssz.ErrListTooBig
	}
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
	if len(c.PayloadExpectedWithdrawals) > 16 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.PayloadExpectedWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
		}
	}
	return dst, err
}

func (c *BeaconStateGloas) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 3134845 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]              // c.GenesisTime
	sszSlice1 := buf[8:40]             // c.GenesisValidatorsRoot
	sszSlice2 := buf[40:48]            // c.Slot
	sszSlice3 := buf[48:64]            // c.Fork
	sszSlice4 := buf[64:176]           // c.LatestBlockHeader
	sszSlice5 := buf[176:262320]       // c.BlockRoots
	sszSlice6 := buf[262320:524464]    // c.StateRoots
	sszSlice8 := buf[524468:524540]    // c.Eth1Data
	sszSlice10 := buf[524544:524552]   // c.Eth1DepositIndex
	sszSlice13 := buf[524560:2621712]  // c.RandaoMixes
	sszSlice14 := buf[2621712:2687248] // c.Slashings
	sszSlice17 := buf[2687256:2687257] // c.JustificationBits
	sszSlice18 := buf[2687257:2687297] // c.PreviousJustifiedCheckpoint
	sszSlice19 := buf[2687297:2687337] // c.CurrentJustifiedCheckpoint
	sszSlice20 := buf[2687337:2687377] // c.FinalizedCheckpoint
	sszSlice22 := buf[2687381:2712005] // c.CurrentSyncCommittee
	sszSlice23 := buf[2712005:2736629] // c.NextSyncCommittee
	sszSlice24 := buf[2736629:2736661] // c.LatestBlockHash
	sszSlice25 := buf[2736661:2736669] // c.NextWithdrawalIndex
	sszSlice26 := buf[2736669:2736677] // c.NextWithdrawalValidatorIndex
	sszSlice28 := buf[2736681:2736689] // c.DepositRequestsStartIndex
	sszSlice29 := buf[2736689:2736697] // c.DepositBalanceToConsume
	sszSlice30 := buf[2736697:2736705] // c.ExitBalanceToConsume
	sszSlice31 := buf[2736705:2736713] // c.EarliestExitEpoch
	sszSlice32 := buf[2736713:2736721] // c.ConsolidationBalanceToConsume
	sszSlice33 := buf[2736721:2736729] // c.EarliestConsolidationEpoch
	sszSlice37 := buf[2736741:2737253] // c.ProposerLookahead
	sszSlice39 := buf[2737257:2737265] // c.NextWithdrawalBuilderIndex
	sszSlice40 := buf[2737265:2738289] // c.ExecutionPayloadAvailability
	sszSlice41 := buf[2738289:2741617] // c.BuilderPendingPayments
	sszSlice45 := buf[2741629:3134845] // c.PtcWindow

	sszVarOffset7 := ssz.ReadOffset(buf[524464:524468]) // c.HistoricalRoots
	if sszVarOffset7 != 3134845 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset7 > size {
		return ssz.ErrOffset
	}
	sszVarOffset9 := ssz.ReadOffset(buf[524540:524544]) // c.Eth1DataVotes
	if sszVarOffset9 > size || sszVarOffset9 < sszVarOffset7 {
		return ssz.ErrOffset
	}
	sszVarOffset11 := ssz.ReadOffset(buf[524552:524556]) // c.Validators
	if sszVarOffset11 > size || sszVarOffset11 < sszVarOffset9 {
		return ssz.ErrOffset
	}
	sszVarOffset12 := ssz.ReadOffset(buf[524556:524560]) // c.Balances
	if sszVarOffset12 > size || sszVarOffset12 < sszVarOffset11 {
		return ssz.ErrOffset
	}
	sszVarOffset15 := ssz.ReadOffset(buf[2687248:2687252]) // c.PreviousEpochParticipation
	if sszVarOffset15 > size || sszVarOffset15 < sszVarOffset12 {
		return ssz.ErrOffset
	}
	sszVarOffset16 := ssz.ReadOffset(buf[2687252:2687256]) // c.CurrentEpochParticipation
	if sszVarOffset16 > size || sszVarOffset16 < sszVarOffset15 {
		return ssz.ErrOffset
	}
	sszVarOffset21 := ssz.ReadOffset(buf[2687377:2687381]) // c.InactivityScores
	if sszVarOffset21 > size || sszVarOffset21 < sszVarOffset16 {
		return ssz.ErrOffset
	}
	sszVarOffset27 := ssz.ReadOffset(buf[2736677:2736681]) // c.HistoricalSummaries
	if sszVarOffset27 > size || sszVarOffset27 < sszVarOffset21 {
		return ssz.ErrOffset
	}
	sszVarOffset34 := ssz.ReadOffset(buf[2736729:2736733]) // c.PendingDeposits
	if sszVarOffset34 > size || sszVarOffset34 < sszVarOffset27 {
		return ssz.ErrOffset
	}
	sszVarOffset35 := ssz.ReadOffset(buf[2736733:2736737]) // c.PendingPartialWithdrawals
	if sszVarOffset35 > size || sszVarOffset35 < sszVarOffset34 {
		return ssz.ErrOffset
	}
	sszVarOffset36 := ssz.ReadOffset(buf[2736737:2736741]) // c.PendingConsolidations
	if sszVarOffset36 > size || sszVarOffset36 < sszVarOffset35 {
		return ssz.ErrOffset
	}
	sszVarOffset38 := ssz.ReadOffset(buf[2737253:2737257]) // c.Builders
	if sszVarOffset38 > size || sszVarOffset38 < sszVarOffset36 {
		return ssz.ErrOffset
	}
	sszVarOffset42 := ssz.ReadOffset(buf[2741617:2741621]) // c.BuilderPendingWithdrawals
	if sszVarOffset42 > size || sszVarOffset42 < sszVarOffset38 {
		return ssz.ErrOffset
	}
	sszVarOffset43 := ssz.ReadOffset(buf[2741621:2741625]) // c.LatestExecutionPayloadBid
	if sszVarOffset43 > size || sszVarOffset43 < sszVarOffset42 {
		return ssz.ErrOffset
	}
	sszVarOffset44 := ssz.ReadOffset(buf[2741625:2741629]) // c.PayloadExpectedWithdrawals
	if sszVarOffset44 > size || sszVarOffset44 < sszVarOffset43 {
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
	sszSlice44 := buf[sszVarOffset44:]               // c.PayloadExpectedWithdrawals

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
		c.BlockRoots = make([][]byte, 8192)
		for i := 0; i < 8192; i++ {
			var tmp []byte

			tmpSlice := sszSlice5[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.BlockRoots[i] = tmp
		}
	}

	// Field 6: StateRoots
	{
		c.StateRoots = make([][]byte, 8192)
		for i := 0; i < 8192; i++ {
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
		if numElem > 2048 {
			return fmt.Errorf("ssz-max exceeded: c.Eth1DataVotes has %d elements, ssz-max is 2048: %w", numElem, ssz.ErrListTooBig)
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
		c.RandaoMixes = make([][]byte, 65536)
		for i := 0; i < 65536; i++ {
			var tmp []byte

			tmpSlice := sszSlice13[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.RandaoMixes[i] = tmp
		}
	}

	// Field 14: Slashings
	{
		c.Slashings = make([]uint64, 8192)
		for i := 0; i < 8192; i++ {
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
		if numElem > 134217728 {
			return fmt.Errorf("ssz-max exceeded: c.PendingPartialWithdrawals has %d elements, ssz-max is 134217728: %w", numElem, ssz.ErrListTooBig)
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
		if numElem > 262144 {
			return fmt.Errorf("ssz-max exceeded: c.PendingConsolidations has %d elements, ssz-max is 262144: %w", numElem, ssz.ErrListTooBig)
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
		c.ProposerLookahead = make([]primitives.ValidatorIndex, 64)
		for i := 0; i < 64; i++ {
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
	c.ExecutionPayloadAvailability = make([]byte, 0, 1024)
	c.ExecutionPayloadAvailability = append(c.ExecutionPayloadAvailability, sszSlice40...)

	// Field 41: BuilderPendingPayments
	{
		c.BuilderPendingPayments = make([]*BuilderPendingPayment, 64)
		for i := 0; i < 64; i++ {
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
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.PayloadExpectedWithdrawals has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
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
		c.PtcWindow = make([]*PTCs, 96)
		for i := 0; i < 96; i++ {
			var tmp *PTCs
			tmp = new(PTCs)
			tmpSlice := sszSlice45[i*4096 : (1+i)*4096]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PtcWindow: %w", err)
			}
			c.PtcWindow[i] = tmp
		}
	}
	return err
}

func (c *BeaconStateGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconStateGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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
		if len(c.BlockRoots) != 8192 {
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
		if len(c.StateRoots) != 8192 {
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
		if len(c.Eth1DataVotes) > 2048 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Eth1DataVotes {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Eth1DataVotes: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Eth1DataVotes)), 2048)
	}
	// Field 10: Eth1DepositIndex
	hh.PutUint64(c.Eth1DepositIndex)
	// Field 11: Validators
	{
		if len(c.Validators) > 1099511627776 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Validators {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Validators: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Validators)), 1099511627776)
	}
	// Field 12: Balances
	{
		if len(c.Balances) > 1099511627776 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Balances {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		numItems := uint64(len(c.Balances))
		hh.MerkleizeWithMixin(subIndx, numItems, ssz.CalculateLimit(1099511627776, numItems, 8))
	}
	// Field 13: RandaoMixes
	{
		if len(c.RandaoMixes) != 65536 {
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
		if len(c.Slashings) != 8192 {
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
		if len(c.PreviousEpochParticipation) > 1099511627776 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.PreviousEpochParticipation)
		numItems := uint64(len(c.PreviousEpochParticipation))
		hh.MerkleizeWithMixin(subIndx, numItems, (1099511627776*1+31)/32)
	}

	// Field 16: CurrentEpochParticipation

	{
		if len(c.CurrentEpochParticipation) > 1099511627776 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.CurrentEpochParticipation)
		numItems := uint64(len(c.CurrentEpochParticipation))
		hh.MerkleizeWithMixin(subIndx, numItems, (1099511627776*1+31)/32)
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
		if len(c.InactivityScores) > 1099511627776 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.InactivityScores {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		numItems := uint64(len(c.InactivityScores))
		hh.MerkleizeWithMixin(subIndx, numItems, ssz.CalculateLimit(1099511627776, numItems, 8))
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
		if len(c.PendingDeposits) > 134217728 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.PendingDeposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingDeposits: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.PendingDeposits)), 134217728)
	}
	// Field 35: PendingPartialWithdrawals
	{
		if len(c.PendingPartialWithdrawals) > 134217728 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.PendingPartialWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingPartialWithdrawals: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.PendingPartialWithdrawals)), 134217728)
	}
	// Field 36: PendingConsolidations
	{
		if len(c.PendingConsolidations) > 262144 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.PendingConsolidations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingConsolidations: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.PendingConsolidations)), 262144)
	}
	// Field 37: ProposerLookahead
	{
		if len(c.ProposerLookahead) != 64 {
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
		if len(c.Builders) > 1099511627776 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Builders {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Builders: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Builders)), 1099511627776)
	}
	// Field 39: NextWithdrawalBuilderIndex
	if err := c.NextWithdrawalBuilderIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextWithdrawalBuilderIndex: %w", err)
	}
	// Field 40: ExecutionPayloadAvailability
	if len(c.ExecutionPayloadAvailability) != 1024 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ExecutionPayloadAvailability)
	// Field 41: BuilderPendingPayments
	{
		if len(c.BuilderPendingPayments) != 64 {
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
		if len(c.BuilderPendingWithdrawals) > 1048576 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.BuilderPendingWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderPendingWithdrawals: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.BuilderPendingWithdrawals)), 1048576)
	}
	// Field 43: LatestExecutionPayloadBid
	if err := c.LatestExecutionPayloadBid.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}
	// Field 44: PayloadExpectedWithdrawals
	{
		if len(c.PayloadExpectedWithdrawals) > 16 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.PayloadExpectedWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.PayloadExpectedWithdrawals)), 16)
	}
	// Field 45: PtcWindow
	{
		if len(c.PtcWindow) != 96 {
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
	hh.Merkleize(indx)
	return nil
}

func (c *BlindedExecutionPayloadEnvelope) SizeSSZ() int {
	size := 148
	if c.ExecutionRequests == nil {
		c.ExecutionRequests = new(v1.ExecutionRequestsGloas)
	}
	size += c.ExecutionRequests.SizeSSZ()
	return size
}

func (c *BlindedExecutionPayloadEnvelope) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BlindedExecutionPayloadEnvelope) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 148

	// Field 0: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash...)

	// Field 1: ExecutionRequests
	if c.ExecutionRequests == nil {
		c.ExecutionRequests = new(v1.ExecutionRequestsGloas)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.ExecutionRequests.SizeSSZ()

	// Field 2: BuilderIndex
	if dst, err = c.BuilderIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("BuilderIndex: %w", err)
	}

	// Field 3: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BeaconBlockRoot...)

	// Field 4: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 5: ParentBlockHash
	if len(c.ParentBlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentBlockHash...)

	// Field 6: ParentBeaconBlockRoot
	if len(c.ParentBeaconBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentBeaconBlockRoot...)

	// Field 1: ExecutionRequests
	if dst, err = c.ExecutionRequests.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ExecutionRequests: %w", err)
	}
	return dst, err
}

func (c *BlindedExecutionPayloadEnvelope) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 148 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]    // c.BlockHash
	sszSlice2 := buf[36:44]   // c.BuilderIndex
	sszSlice3 := buf[44:76]   // c.BeaconBlockRoot
	sszSlice4 := buf[76:84]   // c.Slot
	sszSlice5 := buf[84:116]  // c.ParentBlockHash
	sszSlice6 := buf[116:148] // c.ParentBeaconBlockRoot

	sszVarOffset1 := ssz.ReadOffset(buf[32:36]) // c.ExecutionRequests
	if sszVarOffset1 != 148 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset1 > size {
		return ssz.ErrOffset
	}
	sszSlice1 := buf[sszVarOffset1:] // c.ExecutionRequests

	// Field 0: BlockHash
	c.BlockHash = make([]byte, 0, 32)
	c.BlockHash = append(c.BlockHash, sszSlice0...)

	// Field 1: ExecutionRequests
	c.ExecutionRequests = new(v1.ExecutionRequestsGloas)
	if err = c.ExecutionRequests.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("ExecutionRequests: %w", err)
	}

	// Field 2: BuilderIndex
	if err = c.BuilderIndex.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("BuilderIndex: %w", err)
	}

	// Field 3: BeaconBlockRoot
	c.BeaconBlockRoot = make([]byte, 0, 32)
	c.BeaconBlockRoot = append(c.BeaconBlockRoot, sszSlice3...)

	// Field 4: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 5: ParentBlockHash
	c.ParentBlockHash = make([]byte, 0, 32)
	c.ParentBlockHash = append(c.ParentBlockHash, sszSlice5...)

	// Field 6: ParentBeaconBlockRoot
	c.ParentBeaconBlockRoot = make([]byte, 0, 32)
	c.ParentBeaconBlockRoot = append(c.ParentBeaconBlockRoot, sszSlice6...)
	return err
}

func (c *BlindedExecutionPayloadEnvelope) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BlindedExecutionPayloadEnvelope) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash)
	// Field 1: ExecutionRequests
	if err := c.ExecutionRequests.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExecutionRequests: %w", err)
	}
	// Field 2: BuilderIndex
	if err := c.BuilderIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("BuilderIndex: %w", err)
	}
	// Field 3: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BeaconBlockRoot)
	// Field 4: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 5: ParentBlockHash
	if len(c.ParentBlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentBlockHash)
	// Field 6: ParentBeaconBlockRoot
	if len(c.ParentBeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentBeaconBlockRoot)
	hh.Merkleize(indx)
	return nil
}

func (c *Builder) SizeSSZ() int {
	size := 93

	return size
}

func (c *Builder) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *Builder) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey...)

	// Field 1: Version
	if len(c.Version) != 1 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Version...)

	// Field 2: ExecutionAddress
	if len(c.ExecutionAddress) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ExecutionAddress...)

	// Field 3: Balance
	if dst, err = c.Balance.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Balance: %w", err)
	}

	// Field 4: DepositEpoch
	if dst, err = c.DepositEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("DepositEpoch: %w", err)
	}

	// Field 5: WithdrawableEpoch
	if dst, err = c.WithdrawableEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("WithdrawableEpoch: %w", err)
	}

	return dst, err
}

func (c *Builder) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 93 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:48]  // c.Pubkey
	sszSlice1 := buf[48:49] // c.Version
	sszSlice2 := buf[49:69] // c.ExecutionAddress
	sszSlice3 := buf[69:77] // c.Balance
	sszSlice4 := buf[77:85] // c.DepositEpoch
	sszSlice5 := buf[85:93] // c.WithdrawableEpoch

	// Field 0: Pubkey
	c.Pubkey = make([]byte, 0, 48)
	c.Pubkey = append(c.Pubkey, sszSlice0...)

	// Field 1: Version
	c.Version = make([]byte, 0, 1)
	c.Version = append(c.Version, sszSlice1...)

	// Field 2: ExecutionAddress
	c.ExecutionAddress = make([]byte, 0, 20)
	c.ExecutionAddress = append(c.ExecutionAddress, sszSlice2...)

	// Field 3: Balance
	if err = c.Balance.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("Balance: %w", err)
	}

	// Field 4: DepositEpoch
	if err = c.DepositEpoch.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("DepositEpoch: %w", err)
	}

	// Field 5: WithdrawableEpoch
	if err = c.WithdrawableEpoch.UnmarshalSSZ(sszSlice5); err != nil {
		return fmt.Errorf("WithdrawableEpoch: %w", err)
	}
	return err
}

func (c *Builder) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *Builder) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey)
	// Field 1: Version
	if len(c.Version) != 1 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Version)
	// Field 2: ExecutionAddress
	if len(c.ExecutionAddress) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ExecutionAddress)
	// Field 3: Balance
	if err := c.Balance.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Balance: %w", err)
	}
	// Field 4: DepositEpoch
	if err := c.DepositEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("DepositEpoch: %w", err)
	}
	// Field 5: WithdrawableEpoch
	if err := c.WithdrawableEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("WithdrawableEpoch: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *BuilderPendingPayment) SizeSSZ() int {
	size := 52

	return size
}

func (c *BuilderPendingPayment) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BuilderPendingPayment) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Weight
	if dst, err = c.Weight.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Weight: %w", err)
	}

	// Field 1: Withdrawal
	if c.Withdrawal == nil {
		c.Withdrawal = new(BuilderPendingWithdrawal)
	}
	if dst, err = c.Withdrawal.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Withdrawal: %w", err)
	}

	// Field 2: ProposerIndex
	if dst, err = c.ProposerIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ProposerIndex: %w", err)
	}

	return dst, err
}

func (c *BuilderPendingPayment) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 52 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.Weight
	sszSlice1 := buf[8:44]  // c.Withdrawal
	sszSlice2 := buf[44:52] // c.ProposerIndex

	// Field 0: Weight
	if err = c.Weight.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Weight: %w", err)
	}

	// Field 1: Withdrawal
	c.Withdrawal = new(BuilderPendingWithdrawal)
	if err = c.Withdrawal.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Withdrawal: %w", err)
	}

	// Field 2: ProposerIndex
	if err = c.ProposerIndex.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("ProposerIndex: %w", err)
	}
	return err
}

func (c *BuilderPendingPayment) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BuilderPendingPayment) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Weight
	if err := c.Weight.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Weight: %w", err)
	}
	// Field 1: Withdrawal
	if err := c.Withdrawal.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Withdrawal: %w", err)
	}
	// Field 2: ProposerIndex
	if err := c.ProposerIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ProposerIndex: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *BuilderPendingWithdrawal) SizeSSZ() int {
	size := 36

	return size
}

func (c *BuilderPendingWithdrawal) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BuilderPendingWithdrawal) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient...)

	// Field 1: Amount
	if dst, err = c.Amount.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Amount: %w", err)
	}

	// Field 2: BuilderIndex
	if dst, err = c.BuilderIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("BuilderIndex: %w", err)
	}

	return dst, err
}

func (c *BuilderPendingWithdrawal) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 36 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:20]  // c.FeeRecipient
	sszSlice1 := buf[20:28] // c.Amount
	sszSlice2 := buf[28:36] // c.BuilderIndex

	// Field 0: FeeRecipient
	c.FeeRecipient = make([]byte, 0, 20)
	c.FeeRecipient = append(c.FeeRecipient, sszSlice0...)

	// Field 1: Amount
	if err = c.Amount.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Amount: %w", err)
	}

	// Field 2: BuilderIndex
	if err = c.BuilderIndex.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("BuilderIndex: %w", err)
	}
	return err
}

func (c *BuilderPendingWithdrawal) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BuilderPendingWithdrawal) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Field 1: Amount
	if err := c.Amount.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Amount: %w", err)
	}
	// Field 2: BuilderIndex
	if err := c.BuilderIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("BuilderIndex: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *BuilderPreferencesRequestV1) SizeSSZ() int {
	size := 12
	if c.Auth == nil {
		c.Auth = new(SignedRequestAuthV1)
	}
	size += c.Auth.SizeSSZ()
	return size
}

func (c *BuilderPreferencesRequestV1) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BuilderPreferencesRequestV1) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 12

	// Field 0: Preferences
	if c.Preferences == nil {
		c.Preferences = new(BuilderPreferencesV1)
	}
	if dst, err = c.Preferences.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Preferences: %w", err)
	}

	// Field 1: Auth
	if c.Auth == nil {
		c.Auth = new(SignedRequestAuthV1)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Auth.SizeSSZ()

	// Field 1: Auth
	if dst, err = c.Auth.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Auth: %w", err)
	}
	return dst, err
}

func (c *BuilderPreferencesRequestV1) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 12 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8] // c.Preferences

	sszVarOffset1 := ssz.ReadOffset(buf[8:12]) // c.Auth
	if sszVarOffset1 != 12 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset1 > size {
		return ssz.ErrOffset
	}
	sszSlice1 := buf[sszVarOffset1:] // c.Auth

	// Field 0: Preferences
	c.Preferences = new(BuilderPreferencesV1)
	if err = c.Preferences.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Preferences: %w", err)
	}

	// Field 1: Auth
	c.Auth = new(SignedRequestAuthV1)
	if err = c.Auth.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Auth: %w", err)
	}
	return err
}

func (c *BuilderPreferencesRequestV1) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BuilderPreferencesRequestV1) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Preferences
	if err := c.Preferences.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Preferences: %w", err)
	}
	// Field 1: Auth
	if err := c.Auth.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Auth: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *BuilderPreferencesV1) SizeSSZ() int {
	size := 8

	return size
}

func (c *BuilderPreferencesV1) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BuilderPreferencesV1) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: MaxExecutionPayment
	if dst, err = c.MaxExecutionPayment.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("MaxExecutionPayment: %w", err)
	}

	return dst, err
}

func (c *BuilderPreferencesV1) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 8 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8] // c.MaxExecutionPayment

	// Field 0: MaxExecutionPayment
	if err = c.MaxExecutionPayment.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("MaxExecutionPayment: %w", err)
	}
	return err
}

func (c *BuilderPreferencesV1) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BuilderPreferencesV1) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: MaxExecutionPayment
	if err := c.MaxExecutionPayment.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("MaxExecutionPayment: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *DataColumnSidecarGloas) SizeSSZ() int {
	size := 56
	size += len(c.Column) * 2048
	size += len(c.KzgProofs) * 48
	return size
}

func (c *DataColumnSidecarGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *DataColumnSidecarGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 56

	// Field 0: Index
	dst = binary.LittleEndian.AppendUint64(dst, c.Index)

	// Field 1: Column
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Column) * 2048

	// Field 2: KzgProofs
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.KzgProofs) * 48

	// Field 3: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 4: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BeaconBlockRoot...)

	// Field 1: Column
	if len(c.Column) > 4096 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.Column {
		if len(o) != 2048 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 2: KzgProofs
	if len(c.KzgProofs) > 4096 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.KzgProofs {
		if len(o) != 48 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}
	return dst, err
}

func (c *DataColumnSidecarGloas) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 56 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.Index
	sszSlice3 := buf[16:24] // c.Slot
	sszSlice4 := buf[24:56] // c.BeaconBlockRoot

	sszVarOffset1 := ssz.ReadOffset(buf[8:12]) // c.Column
	if sszVarOffset1 != 56 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset1 > size {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[12:16]) // c.KzgProofs
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.Column
	sszSlice2 := buf[sszVarOffset2:]              // c.KzgProofs

	// Field 0: Index
	c.Index = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: Column
	{
		if len(sszSlice1)%2048 != 0 {
			return fmt.Errorf("misaligned bytes: c.Column length is %d, which is not a multiple of 2048: %w", len(sszSlice1), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice1) / 2048
		if numElem > 4096 {
			return fmt.Errorf("ssz-max exceeded: c.Column has %d elements, ssz-max is 4096: %w", numElem, ssz.ErrListTooBig)
		}
		c.Column = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice1[i*2048 : (1+i)*2048]
			tmp = make([]byte, 0, 2048)
			tmp = append(tmp, tmpSlice...)
			c.Column[i] = tmp
		}
	}

	// Field 2: KzgProofs
	{
		if len(sszSlice2)%48 != 0 {
			return fmt.Errorf("misaligned bytes: c.KzgProofs length is %d, which is not a multiple of 48: %w", len(sszSlice2), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice2) / 48
		if numElem > 4096 {
			return fmt.Errorf("ssz-max exceeded: c.KzgProofs has %d elements, ssz-max is 4096: %w", numElem, ssz.ErrListTooBig)
		}
		c.KzgProofs = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice2[i*48 : (1+i)*48]
			tmp = make([]byte, 0, 48)
			tmp = append(tmp, tmpSlice...)
			c.KzgProofs[i] = tmp
		}
	}

	// Field 3: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 4: BeaconBlockRoot
	c.BeaconBlockRoot = make([]byte, 0, 32)
	c.BeaconBlockRoot = append(c.BeaconBlockRoot, sszSlice4...)
	return err
}

func (c *DataColumnSidecarGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *DataColumnSidecarGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Index
	hh.PutUint64(c.Index)
	// Field 1: Column
	{
		if len(c.Column) > 4096 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Column {
			if len(o) != 2048 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Column)), 4096)
	}
	// Field 2: KzgProofs
	{
		if len(c.KzgProofs) > 4096 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.KzgProofs {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.KzgProofs)), 4096)
	}
	// Field 3: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 4: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BeaconBlockRoot)
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadBid) SizeSSZ() int {
	size := 224
	size += len(c.BlobKzgCommitments) * 48
	return size
}

func (c *ExecutionPayloadBid) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadBid) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 224

	// Field 0: ParentBlockHash
	if len(c.ParentBlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentBlockHash...)

	// Field 1: ParentBlockRoot
	if len(c.ParentBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentBlockRoot...)

	// Field 2: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash...)

	// Field 3: PrevRandao
	if len(c.PrevRandao) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PrevRandao...)

	// Field 4: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient...)

	// Field 5: GasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.GasLimit)

	// Field 6: BuilderIndex
	if dst, err = c.BuilderIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("BuilderIndex: %w", err)
	}

	// Field 7: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 8: Value
	if dst, err = c.Value.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Value: %w", err)
	}

	// Field 9: ExecutionPayment
	if dst, err = c.ExecutionPayment.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ExecutionPayment: %w", err)
	}

	// Field 10: BlobKzgCommitments
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BlobKzgCommitments) * 48

	// Field 11: ExecutionRequestsRoot
	if len(c.ExecutionRequestsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ExecutionRequestsRoot...)

	// Field 10: BlobKzgCommitments
	if len(c.BlobKzgCommitments) > 4096 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.BlobKzgCommitments {
		if len(o) != 48 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}
	return dst, err
}

func (c *ExecutionPayloadBid) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 224 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]     // c.ParentBlockHash
	sszSlice1 := buf[32:64]    // c.ParentBlockRoot
	sszSlice2 := buf[64:96]    // c.BlockHash
	sszSlice3 := buf[96:128]   // c.PrevRandao
	sszSlice4 := buf[128:148]  // c.FeeRecipient
	sszSlice5 := buf[148:156]  // c.GasLimit
	sszSlice6 := buf[156:164]  // c.BuilderIndex
	sszSlice7 := buf[164:172]  // c.Slot
	sszSlice8 := buf[172:180]  // c.Value
	sszSlice9 := buf[180:188]  // c.ExecutionPayment
	sszSlice11 := buf[192:224] // c.ExecutionRequestsRoot

	sszVarOffset10 := ssz.ReadOffset(buf[188:192]) // c.BlobKzgCommitments
	if sszVarOffset10 != 224 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset10 > size {
		return ssz.ErrOffset
	}
	sszSlice10 := buf[sszVarOffset10:] // c.BlobKzgCommitments

	// Field 0: ParentBlockHash
	c.ParentBlockHash = make([]byte, 0, 32)
	c.ParentBlockHash = append(c.ParentBlockHash, sszSlice0...)

	// Field 1: ParentBlockRoot
	c.ParentBlockRoot = make([]byte, 0, 32)
	c.ParentBlockRoot = append(c.ParentBlockRoot, sszSlice1...)

	// Field 2: BlockHash
	c.BlockHash = make([]byte, 0, 32)
	c.BlockHash = append(c.BlockHash, sszSlice2...)

	// Field 3: PrevRandao
	c.PrevRandao = make([]byte, 0, 32)
	c.PrevRandao = append(c.PrevRandao, sszSlice3...)

	// Field 4: FeeRecipient
	c.FeeRecipient = make([]byte, 0, 20)
	c.FeeRecipient = append(c.FeeRecipient, sszSlice4...)

	// Field 5: GasLimit
	c.GasLimit = binary.LittleEndian.Uint64(sszSlice5)

	// Field 6: BuilderIndex
	if err = c.BuilderIndex.UnmarshalSSZ(sszSlice6); err != nil {
		return fmt.Errorf("BuilderIndex: %w", err)
	}

	// Field 7: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice7); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 8: Value
	if err = c.Value.UnmarshalSSZ(sszSlice8); err != nil {
		return fmt.Errorf("Value: %w", err)
	}

	// Field 9: ExecutionPayment
	if err = c.ExecutionPayment.UnmarshalSSZ(sszSlice9); err != nil {
		return fmt.Errorf("ExecutionPayment: %w", err)
	}

	// Field 10: BlobKzgCommitments
	{
		if len(sszSlice10)%48 != 0 {
			return fmt.Errorf("misaligned bytes: c.BlobKzgCommitments length is %d, which is not a multiple of 48: %w", len(sszSlice10), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice10) / 48
		if numElem > 4096 {
			return fmt.Errorf("ssz-max exceeded: c.BlobKzgCommitments has %d elements, ssz-max is 4096: %w", numElem, ssz.ErrListTooBig)
		}
		c.BlobKzgCommitments = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice10[i*48 : (1+i)*48]
			tmp = make([]byte, 0, 48)
			tmp = append(tmp, tmpSlice...)
			c.BlobKzgCommitments[i] = tmp
		}
	}

	// Field 11: ExecutionRequestsRoot
	c.ExecutionRequestsRoot = make([]byte, 0, 32)
	c.ExecutionRequestsRoot = append(c.ExecutionRequestsRoot, sszSlice11...)
	return err
}

func (c *ExecutionPayloadBid) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadBid) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ParentBlockHash
	if len(c.ParentBlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentBlockHash)
	// Field 1: ParentBlockRoot
	if len(c.ParentBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentBlockRoot)
	// Field 2: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash)
	// Field 3: PrevRandao
	if len(c.PrevRandao) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PrevRandao)
	// Field 4: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Field 5: GasLimit
	hh.PutUint64(c.GasLimit)
	// Field 6: BuilderIndex
	if err := c.BuilderIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("BuilderIndex: %w", err)
	}
	// Field 7: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 8: Value
	if err := c.Value.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Value: %w", err)
	}
	// Field 9: ExecutionPayment
	if err := c.ExecutionPayment.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExecutionPayment: %w", err)
	}
	// Field 10: BlobKzgCommitments
	{
		if len(c.BlobKzgCommitments) > 4096 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.BlobKzgCommitments {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.BlobKzgCommitments)), 4096)
	}
	// Field 11: ExecutionRequestsRoot
	if len(c.ExecutionRequestsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ExecutionRequestsRoot)
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadEnvelope) SizeSSZ() int {
	size := 80
	if c.Payload == nil {
		c.Payload = new(v1.ExecutionPayloadGloas)
	}
	size += c.Payload.SizeSSZ()
	if c.ExecutionRequests == nil {
		c.ExecutionRequests = new(v1.ExecutionRequestsGloas)
	}
	size += c.ExecutionRequests.SizeSSZ()
	return size
}

func (c *ExecutionPayloadEnvelope) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadEnvelope) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 80

	// Field 0: Payload
	if c.Payload == nil {
		c.Payload = new(v1.ExecutionPayloadGloas)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Payload.SizeSSZ()

	// Field 1: ExecutionRequests
	if c.ExecutionRequests == nil {
		c.ExecutionRequests = new(v1.ExecutionRequestsGloas)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.ExecutionRequests.SizeSSZ()

	// Field 2: BuilderIndex
	if dst, err = c.BuilderIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("BuilderIndex: %w", err)
	}

	// Field 3: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BeaconBlockRoot...)

	// Field 4: ParentBeaconBlockRoot
	if len(c.ParentBeaconBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentBeaconBlockRoot...)

	// Field 0: Payload
	if dst, err = c.Payload.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Payload: %w", err)
	}

	// Field 1: ExecutionRequests
	if dst, err = c.ExecutionRequests.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ExecutionRequests: %w", err)
	}
	return dst, err
}

func (c *ExecutionPayloadEnvelope) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 80 {
		return ssz.ErrSize
	}

	sszSlice2 := buf[8:16]  // c.BuilderIndex
	sszSlice3 := buf[16:48] // c.BeaconBlockRoot
	sszSlice4 := buf[48:80] // c.ParentBeaconBlockRoot

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Payload
	if sszVarOffset0 != 80 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.ExecutionRequests
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.Payload
	sszSlice1 := buf[sszVarOffset1:]              // c.ExecutionRequests

	// Field 0: Payload
	c.Payload = new(v1.ExecutionPayloadGloas)
	if err = c.Payload.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Payload: %w", err)
	}

	// Field 1: ExecutionRequests
	c.ExecutionRequests = new(v1.ExecutionRequestsGloas)
	if err = c.ExecutionRequests.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("ExecutionRequests: %w", err)
	}

	// Field 2: BuilderIndex
	if err = c.BuilderIndex.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("BuilderIndex: %w", err)
	}

	// Field 3: BeaconBlockRoot
	c.BeaconBlockRoot = make([]byte, 0, 32)
	c.BeaconBlockRoot = append(c.BeaconBlockRoot, sszSlice3...)

	// Field 4: ParentBeaconBlockRoot
	c.ParentBeaconBlockRoot = make([]byte, 0, 32)
	c.ParentBeaconBlockRoot = append(c.ParentBeaconBlockRoot, sszSlice4...)
	return err
}

func (c *ExecutionPayloadEnvelope) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadEnvelope) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Payload
	if err := c.Payload.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Payload: %w", err)
	}
	// Field 1: ExecutionRequests
	if err := c.ExecutionRequests.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExecutionRequests: %w", err)
	}
	// Field 2: BuilderIndex
	if err := c.BuilderIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("BuilderIndex: %w", err)
	}
	// Field 3: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BeaconBlockRoot)
	// Field 4: ParentBeaconBlockRoot
	if len(c.ParentBeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentBeaconBlockRoot)
	hh.Merkleize(indx)
	return nil
}

func (c *IndexedAttestationGloas) SizeSSZ() int {
	size := 228
	size += len(c.AttestingIndices) * 8
	return size
}

func (c *IndexedAttestationGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *IndexedAttestationGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 228

	// Field 0: AttestingIndices
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.AttestingIndices) * 8

	// Field 1: Data
	if c.Data == nil {
		c.Data = new(AttestationData)
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
	if len(c.AttestingIndices) > 131072 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.AttestingIndices {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}
	return dst, err
}

func (c *IndexedAttestationGloas) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 228 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:132]   // c.Data
	sszSlice2 := buf[132:228] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.AttestingIndices
	if sszVarOffset0 != 228 {
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
		if numElem > 131072 {
			return fmt.Errorf("ssz-max exceeded: c.AttestingIndices has %d elements, ssz-max is 131072: %w", numElem, ssz.ErrListTooBig)
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
	c.Data = new(AttestationData)
	if err = c.Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice2...)
	return err
}

func (c *IndexedAttestationGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *IndexedAttestationGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AttestingIndices
	{
		if len(c.AttestingIndices) > 131072 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.AttestingIndices {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		numItems := uint64(len(c.AttestingIndices))
		hh.MerkleizeWithMixin(subIndx, numItems, ssz.CalculateLimit(131072, numItems, 8))
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
	hh.Merkleize(indx)
	return nil
}

func (c *PayloadAttestation) SizeSSZ() int {
	size := 202

	return size
}

func (c *PayloadAttestation) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *PayloadAttestation) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: AggregationBits
	if len([]byte(c.AggregationBits)) != 64 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.AggregationBits)...)

	// Field 1: Data
	if c.Data == nil {
		c.Data = new(PayloadAttestationData)
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

func (c *PayloadAttestation) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 202 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:64]    // c.AggregationBits
	sszSlice1 := buf[64:106]  // c.Data
	sszSlice2 := buf[106:202] // c.Signature

	// Field 0: AggregationBits
	c.AggregationBits = make([]byte, 0, 64)
	c.AggregationBits = append(c.AggregationBits, go_bitfield.Bitvector512(sszSlice0)...)

	// Field 1: Data
	c.Data = new(PayloadAttestationData)
	if err = c.Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice2...)
	return err
}

func (c *PayloadAttestation) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *PayloadAttestation) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *PayloadAttestationData) SizeSSZ() int {
	size := 42

	return size
}

func (c *PayloadAttestationData) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *PayloadAttestationData) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BeaconBlockRoot...)

	// Field 1: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 2: PayloadPresent
	if c.PayloadPresent {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}

	// Field 3: BlobDataAvailable
	if c.BlobDataAvailable {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}

	return dst, err
}

func (c *PayloadAttestationData) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 42 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]  // c.BeaconBlockRoot
	sszSlice1 := buf[32:40] // c.Slot
	sszSlice2 := buf[40:41] // c.PayloadPresent
	sszSlice3 := buf[41:42] // c.BlobDataAvailable

	// Field 0: BeaconBlockRoot
	c.BeaconBlockRoot = make([]byte, 0, 32)
	c.BeaconBlockRoot = append(c.BeaconBlockRoot, sszSlice0...)

	// Field 1: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 2: PayloadPresent
	if sszSlice2[0] > 1 {
		return ssz.ErrInvalidSerialization
	}
	if sszSlice2[0] == 1 {
		c.PayloadPresent = true
	} else {
		c.PayloadPresent = false
	}

	// Field 3: BlobDataAvailable
	if sszSlice3[0] > 1 {
		return ssz.ErrInvalidSerialization
	}
	if sszSlice3[0] == 1 {
		c.BlobDataAvailable = true
	} else {
		c.BlobDataAvailable = false
	}
	return err
}

func (c *PayloadAttestationData) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *PayloadAttestationData) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BeaconBlockRoot)
	// Field 1: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 2: PayloadPresent
	hh.PutBool(c.PayloadPresent)
	// Field 3: BlobDataAvailable
	hh.PutBool(c.BlobDataAvailable)
	hh.Merkleize(indx)
	return nil
}

func (c *PayloadAttestationMessage) SizeSSZ() int {
	size := 146

	return size
}

func (c *PayloadAttestationMessage) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *PayloadAttestationMessage) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: ValidatorIndex
	if dst, err = c.ValidatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ValidatorIndex: %w", err)
	}

	// Field 1: Data
	if c.Data == nil {
		c.Data = new(PayloadAttestationData)
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

func (c *PayloadAttestationMessage) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 146 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]    // c.ValidatorIndex
	sszSlice1 := buf[8:50]   // c.Data
	sszSlice2 := buf[50:146] // c.Signature

	// Field 0: ValidatorIndex
	if err = c.ValidatorIndex.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("ValidatorIndex: %w", err)
	}

	// Field 1: Data
	c.Data = new(PayloadAttestationData)
	if err = c.Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice2...)
	return err
}

func (c *PayloadAttestationMessage) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *PayloadAttestationMessage) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ValidatorIndex
	if err := c.ValidatorIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ValidatorIndex: %w", err)
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
	hh.Merkleize(indx)
	return nil
}

func (c *ProposerPreferences) SizeSSZ() int {
	size := 76

	return size
}

func (c *ProposerPreferences) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ProposerPreferences) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: DependentRoot
	if len(c.DependentRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.DependentRoot...)

	// Field 1: ProposalSlot
	if dst, err = c.ProposalSlot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ProposalSlot: %w", err)
	}

	// Field 2: ValidatorIndex
	if dst, err = c.ValidatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ValidatorIndex: %w", err)
	}

	// Field 3: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient...)

	// Field 4: TargetGasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.TargetGasLimit)

	return dst, err
}

func (c *ProposerPreferences) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 76 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]  // c.DependentRoot
	sszSlice1 := buf[32:40] // c.ProposalSlot
	sszSlice2 := buf[40:48] // c.ValidatorIndex
	sszSlice3 := buf[48:68] // c.FeeRecipient
	sszSlice4 := buf[68:76] // c.TargetGasLimit

	// Field 0: DependentRoot
	c.DependentRoot = make([]byte, 0, 32)
	c.DependentRoot = append(c.DependentRoot, sszSlice0...)

	// Field 1: ProposalSlot
	if err = c.ProposalSlot.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("ProposalSlot: %w", err)
	}

	// Field 2: ValidatorIndex
	if err = c.ValidatorIndex.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("ValidatorIndex: %w", err)
	}

	// Field 3: FeeRecipient
	c.FeeRecipient = make([]byte, 0, 20)
	c.FeeRecipient = append(c.FeeRecipient, sszSlice3...)

	// Field 4: TargetGasLimit
	c.TargetGasLimit = binary.LittleEndian.Uint64(sszSlice4)
	return err
}

func (c *ProposerPreferences) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ProposerPreferences) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: DependentRoot
	if len(c.DependentRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.DependentRoot)
	// Field 1: ProposalSlot
	if err := c.ProposalSlot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ProposalSlot: %w", err)
	}
	// Field 2: ValidatorIndex
	if err := c.ValidatorIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ValidatorIndex: %w", err)
	}
	// Field 3: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Field 4: TargetGasLimit
	hh.PutUint64(c.TargetGasLimit)
	hh.Merkleize(indx)
	return nil
}

func (c *PTCs) SizeSSZ() int {
	size := 4096

	return size
}

func (c *PTCs) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *PTCs) MarshalSSZTo(dst []byte) ([]byte, error) {
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

func (c *PTCs) UnmarshalSSZ(buf []byte) error {
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

func (c *PTCs) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *PTCs) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *RequestAuthV1) SizeSSZ() int {
	size := 12
	size += len(c.Data)
	return size
}

func (c *RequestAuthV1) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *RequestAuthV1) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 12

	// Field 0: Data
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Data)

	// Field 1: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 0: Data
	if len(c.Data) > 4096 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.Data...)
	return dst, err
}

func (c *RequestAuthV1) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 12 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:12] // c.Slot

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Data
	if sszVarOffset0 != 12 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Data

	// Field 0: Data
	c.Data = append([]byte{}, sszSlice0...)

	// Field 1: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	return err
}

func (c *RequestAuthV1) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *RequestAuthV1) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Data

	{
		if len(c.Data) > 4096 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.Data)
		numItems := uint64(len(c.Data))
		hh.MerkleizeWithMixin(subIndx, numItems, (4096*1+31)/32)
	}

	// Field 1: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *SignedBeaconBlockGloas) SizeSSZ() int {
	size := 100
	if c.Block == nil {
		c.Block = new(BeaconBlockGloas)
	}
	size += c.Block.SizeSSZ()
	return size
}

func (c *SignedBeaconBlockGloas) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedBeaconBlockGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Block
	if c.Block == nil {
		c.Block = new(BeaconBlockGloas)
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

func (c *SignedBeaconBlockGloas) UnmarshalSSZ(buf []byte) error {
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
	c.Block = new(BeaconBlockGloas)
	if err = c.Block.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Block: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedBeaconBlockGloas) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedBeaconBlockGloas) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *SignedBlindedExecutionPayloadEnvelope) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(BlindedExecutionPayloadEnvelope)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedBlindedExecutionPayloadEnvelope) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedBlindedExecutionPayloadEnvelope) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(BlindedExecutionPayloadEnvelope)
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

func (c *SignedBlindedExecutionPayloadEnvelope) UnmarshalSSZ(buf []byte) error {
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
	c.Message = new(BlindedExecutionPayloadEnvelope)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedBlindedExecutionPayloadEnvelope) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedBlindedExecutionPayloadEnvelope) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *SignedExecutionPayloadBid) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(ExecutionPayloadBid)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedExecutionPayloadBid) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedExecutionPayloadBid) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(ExecutionPayloadBid)
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

func (c *SignedExecutionPayloadBid) UnmarshalSSZ(buf []byte) error {
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
	c.Message = new(ExecutionPayloadBid)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedExecutionPayloadBid) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedExecutionPayloadBid) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *SignedExecutionPayloadEnvelope) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(ExecutionPayloadEnvelope)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedExecutionPayloadEnvelope) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedExecutionPayloadEnvelope) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(ExecutionPayloadEnvelope)
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

func (c *SignedExecutionPayloadEnvelope) UnmarshalSSZ(buf []byte) error {
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
	c.Message = new(ExecutionPayloadEnvelope)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedExecutionPayloadEnvelope) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedExecutionPayloadEnvelope) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *SignedExecutionPayloadEnvelopeContents) SizeSSZ() int {
	size := 12
	if c.SignedExecutionPayloadEnvelope == nil {
		c.SignedExecutionPayloadEnvelope = new(SignedExecutionPayloadEnvelope)
	}
	size += c.SignedExecutionPayloadEnvelope.SizeSSZ()
	size += len(c.KzgProofs) * 48
	size += len(c.Blobs) * 131072
	return size
}

func (c *SignedExecutionPayloadEnvelopeContents) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedExecutionPayloadEnvelopeContents) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 12

	// Field 0: SignedExecutionPayloadEnvelope
	if c.SignedExecutionPayloadEnvelope == nil {
		c.SignedExecutionPayloadEnvelope = new(SignedExecutionPayloadEnvelope)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.SignedExecutionPayloadEnvelope.SizeSSZ()

	// Field 1: KzgProofs
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.KzgProofs) * 48

	// Field 2: Blobs
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Blobs) * 131072

	// Field 0: SignedExecutionPayloadEnvelope
	if dst, err = c.SignedExecutionPayloadEnvelope.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SignedExecutionPayloadEnvelope: %w", err)
	}

	// Field 1: KzgProofs
	if len(c.KzgProofs) > 33554432 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.KzgProofs {
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

func (c *SignedExecutionPayloadEnvelopeContents) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 12 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.SignedExecutionPayloadEnvelope
	if sszVarOffset0 != 12 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.KzgProofs
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[8:12]) // c.Blobs
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.SignedExecutionPayloadEnvelope
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.KzgProofs
	sszSlice2 := buf[sszVarOffset2:]              // c.Blobs

	// Field 0: SignedExecutionPayloadEnvelope
	c.SignedExecutionPayloadEnvelope = new(SignedExecutionPayloadEnvelope)
	if err = c.SignedExecutionPayloadEnvelope.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("SignedExecutionPayloadEnvelope: %w", err)
	}

	// Field 1: KzgProofs
	{
		if len(sszSlice1)%48 != 0 {
			return fmt.Errorf("misaligned bytes: c.KzgProofs length is %d, which is not a multiple of 48: %w", len(sszSlice1), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice1) / 48
		if numElem > 33554432 {
			return fmt.Errorf("ssz-max exceeded: c.KzgProofs has %d elements, ssz-max is 33554432: %w", numElem, ssz.ErrListTooBig)
		}
		c.KzgProofs = make([][]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp []byte

			tmpSlice := sszSlice1[i*48 : (1+i)*48]
			tmp = make([]byte, 0, 48)
			tmp = append(tmp, tmpSlice...)
			c.KzgProofs[i] = tmp
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

func (c *SignedExecutionPayloadEnvelopeContents) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedExecutionPayloadEnvelopeContents) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SignedExecutionPayloadEnvelope
	if err := c.SignedExecutionPayloadEnvelope.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SignedExecutionPayloadEnvelope: %w", err)
	}
	// Field 1: KzgProofs
	{
		if len(c.KzgProofs) > 33554432 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.KzgProofs {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.KzgProofs)), 33554432)
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

func (c *SignedProposerPreferences) SizeSSZ() int {
	size := 172

	return size
}

func (c *SignedProposerPreferences) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedProposerPreferences) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(ProposerPreferences)
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

func (c *SignedProposerPreferences) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 172 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:76]   // c.Message
	sszSlice1 := buf[76:172] // c.Signature

	// Field 0: Message
	c.Message = new(ProposerPreferences)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedProposerPreferences) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedProposerPreferences) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *SignedRequestAuthV1) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(RequestAuthV1)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedRequestAuthV1) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedRequestAuthV1) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(RequestAuthV1)
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

func (c *SignedRequestAuthV1) UnmarshalSSZ(buf []byte) error {
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
	c.Message = new(RequestAuthV1)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedRequestAuthV1) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedRequestAuthV1) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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
