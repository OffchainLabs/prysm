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

func (c *AggregateAttestationAndProofPQ) SizeSSZ() int {
	size := 16
	if c.Aggregate == nil {
		c.Aggregate = new(AttestationPQ)
	}
	size += c.Aggregate.SizeSSZ()
	size += len(c.Proof)
	return size
}

func (c *AggregateAttestationAndProofPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AggregateAttestationAndProofPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 16

	// Field 0: AggregatorIndex
	if dst, err = c.AggregatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("AggregatorIndex: %w", err)
	}

	// Field 1: Aggregate
	if c.Aggregate == nil {
		c.Aggregate = new(AttestationPQ)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Aggregate.SizeSSZ()

	// Field 2: Proof
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Proof)

	// Field 1: Aggregate
	if dst, err = c.Aggregate.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Aggregate: %w", err)
	}

	// Field 2: Proof
	if len(c.Proof) > 1048576 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.Proof...)
	return dst, err
}

func (c *AggregateAttestationAndProofPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 16 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8] // c.AggregatorIndex

	sszVarOffset1 := ssz.ReadOffset(buf[8:12]) // c.Aggregate
	if sszVarOffset1 != 16 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset1 > size {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[12:16]) // c.Proof
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.Aggregate
	sszSlice2 := buf[sszVarOffset2:]              // c.Proof

	// Field 0: AggregatorIndex
	if err = c.AggregatorIndex.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("AggregatorIndex: %w", err)
	}

	// Field 1: Aggregate
	c.Aggregate = new(AttestationPQ)
	if err = c.Aggregate.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Aggregate: %w", err)
	}

	// Field 2: Proof
	c.Proof = append([]byte{}, sszSlice2...)
	return err
}

func (c *AggregateAttestationAndProofPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AggregateAttestationAndProofPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AggregatorIndex
	if err := c.AggregatorIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("AggregatorIndex: %w", err)
	}
	// Field 1: Aggregate
	if err := c.Aggregate.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Aggregate: %w", err)
	}
	// Field 2: Proof

	{
		if len(c.Proof) > 1048576 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.Proof)
		numItems := uint64(len(c.Proof))
		hh.MerkleizeWithMixin(subIndx, numItems, (1048576*1+31)/32)
	}

	hh.Merkleize(indx)
	return nil
}

func (c *AttestationPQ) SizeSSZ() int {
	size := 134
	size += len(c.AggregationBits)
	return size
}

func (c *AttestationPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AttestationPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 134

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

	// Field 2: CommitteeBits
	if len([]byte(c.CommitteeBits)) != 2 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.CommitteeBits)...)

	// Field 0: AggregationBits
	dst = append(dst, c.AggregationBits...)
	return dst, err
}

func (c *AttestationPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 134 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:132]   // c.Data
	sszSlice2 := buf[132:134] // c.CommitteeBits

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.AggregationBits
	if sszVarOffset0 != 134 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.AggregationBits

	// Field 0: AggregationBits
	if err = ssz.ValidateBitlist(sszSlice0, 32768); err != nil {
		return fmt.Errorf("AggregationBits: %w", err)
	}
	c.AggregationBits = append([]byte{}, go_bitfield.Bitlist(sszSlice0)...)

	// Field 1: Data
	c.Data = new(AttestationData)
	if err = c.Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 2: CommitteeBits
	c.CommitteeBits = make([]byte, 0, 2)
	c.CommitteeBits = append(c.CommitteeBits, go_bitfield.Bitvector16(sszSlice2)...)
	return err
}

func (c *AttestationPQ) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *AttestationPQ) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsAttestationPQ = []byte{0b00001011}

func (c *AttestationPQ) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AttestationPQ) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Position 0: AggregationBits
	if len(c.AggregationBits) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutProgressiveBitlist(c.AggregationBits)
	// Position 1: Data
	if err := c.Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Data: %w", err)
	}
	// Position 2: inactive (zero chunk)
	hh.PutBool(false)
	// Position 3: CommitteeBits
	if len([]byte(c.CommitteeBits)) != 2 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.CommitteeBits))
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsAttestationPQ)
	return nil
}

func (c *AttesterSlashingPQ) SizeSSZ() int {
	size := 8
	if c.Attestation_1 == nil {
		c.Attestation_1 = new(IndexedAttestationPQ)
	}
	size += c.Attestation_1.SizeSSZ()
	if c.Attestation_2 == nil {
		c.Attestation_2 = new(IndexedAttestationPQ)
	}
	size += c.Attestation_2.SizeSSZ()
	return size
}

func (c *AttesterSlashingPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AttesterSlashingPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 8

	// Field 0: Attestation_1
	if c.Attestation_1 == nil {
		c.Attestation_1 = new(IndexedAttestationPQ)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Attestation_1.SizeSSZ()

	// Field 1: Attestation_2
	if c.Attestation_2 == nil {
		c.Attestation_2 = new(IndexedAttestationPQ)
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

func (c *AttesterSlashingPQ) UnmarshalSSZ(buf []byte) error {
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
	c.Attestation_1 = new(IndexedAttestationPQ)
	if err = c.Attestation_1.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Attestation_1: %w", err)
	}

	// Field 1: Attestation_2
	c.Attestation_2 = new(IndexedAttestationPQ)
	if err = c.Attestation_2.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Attestation_2: %w", err)
	}
	return err
}

func (c *AttesterSlashingPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AttesterSlashingPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *BeaconBlockBodyPQ) SizeSSZ() int {
	size := 1156
	size += len(c.ProposerSlashings) * 5032
	for _, o := range c.AttesterSlashings {
		size += 4
		size += o.SizeSSZ()
	}
	for _, o := range c.Attestations {
		size += 4
		size += o.SizeSSZ()
	}
	size += len(c.Deposits) * 1240
	if c.SignedExecutionPayloadBid == nil {
		c.SignedExecutionPayloadBid = new(SignedExecutionPayloadBidPQ)
	}
	size += c.SignedExecutionPayloadBid.SizeSSZ()
	if c.ParentExecutionRequests == nil {
		c.ParentExecutionRequests = new(v1.ExecutionRequestsPQ)
	}
	size += c.ParentExecutionRequests.SizeSSZ()
	size += len(c.Proof)
	return size
}

func (c *BeaconBlockBodyPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockBodyPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 1156

	// Field 0: RandaoReveal
	if len(c.RandaoReveal) != 1024 {
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
	offset += len(c.ProposerSlashings) * 5032

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

	// Field 7: SignedExecutionPayloadBid
	if c.SignedExecutionPayloadBid == nil {
		c.SignedExecutionPayloadBid = new(SignedExecutionPayloadBidPQ)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.SignedExecutionPayloadBid.SizeSSZ()

	// Field 8: ParentExecutionRequests
	if c.ParentExecutionRequests == nil {
		c.ParentExecutionRequests = new(v1.ExecutionRequestsPQ)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.ParentExecutionRequests.SizeSSZ()

	// Field 9: Proof
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Proof)

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

	// Field 7: SignedExecutionPayloadBid
	if dst, err = c.SignedExecutionPayloadBid.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}

	// Field 8: ParentExecutionRequests
	if dst, err = c.ParentExecutionRequests.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ParentExecutionRequests: %w", err)
	}

	// Field 9: Proof
	if len(c.Proof) > 1048576 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.Proof...)
	return dst, err
}

func (c *BeaconBlockBodyPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 1156 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:1024]    // c.RandaoReveal
	sszSlice1 := buf[1024:1096] // c.Eth1Data
	sszSlice2 := buf[1096:1128] // c.Graffiti

	sszVarOffset3 := ssz.ReadOffset(buf[1128:1132]) // c.ProposerSlashings
	if sszVarOffset3 != 1156 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset3 > size {
		return ssz.ErrOffset
	}
	sszVarOffset4 := ssz.ReadOffset(buf[1132:1136]) // c.AttesterSlashings
	if sszVarOffset4 > size || sszVarOffset4 < sszVarOffset3 {
		return ssz.ErrOffset
	}
	sszVarOffset5 := ssz.ReadOffset(buf[1136:1140]) // c.Attestations
	if sszVarOffset5 > size || sszVarOffset5 < sszVarOffset4 {
		return ssz.ErrOffset
	}
	sszVarOffset6 := ssz.ReadOffset(buf[1140:1144]) // c.Deposits
	if sszVarOffset6 > size || sszVarOffset6 < sszVarOffset5 {
		return ssz.ErrOffset
	}
	sszVarOffset7 := ssz.ReadOffset(buf[1144:1148]) // c.SignedExecutionPayloadBid
	if sszVarOffset7 > size || sszVarOffset7 < sszVarOffset6 {
		return ssz.ErrOffset
	}
	sszVarOffset8 := ssz.ReadOffset(buf[1148:1152]) // c.ParentExecutionRequests
	if sszVarOffset8 > size || sszVarOffset8 < sszVarOffset7 {
		return ssz.ErrOffset
	}
	sszVarOffset9 := ssz.ReadOffset(buf[1152:1156]) // c.Proof
	if sszVarOffset9 > size || sszVarOffset9 < sszVarOffset8 {
		return ssz.ErrOffset
	}
	sszSlice3 := buf[sszVarOffset3:sszVarOffset4] // c.ProposerSlashings
	sszSlice4 := buf[sszVarOffset4:sszVarOffset5] // c.AttesterSlashings
	sszSlice5 := buf[sszVarOffset5:sszVarOffset6] // c.Attestations
	sszSlice6 := buf[sszVarOffset6:sszVarOffset7] // c.Deposits
	sszSlice7 := buf[sszVarOffset7:sszVarOffset8] // c.SignedExecutionPayloadBid
	sszSlice8 := buf[sszVarOffset8:sszVarOffset9] // c.ParentExecutionRequests
	sszSlice9 := buf[sszVarOffset9:]              // c.Proof

	// Field 0: RandaoReveal
	c.RandaoReveal = make([]byte, 0, 1024)
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
		if len(sszSlice3)%5032 != 0 {
			return fmt.Errorf("misaligned bytes: c.ProposerSlashings length is %d, which is not a multiple of 5032: %w", len(sszSlice3), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice3) / 5032
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.ProposerSlashings has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.ProposerSlashings = make([]*ProposerSlashingPQ, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ProposerSlashingPQ
			tmp = new(ProposerSlashingPQ)
			tmpSlice := sszSlice3[i*5032 : (1+i)*5032]
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
			c.AttesterSlashings = make([]*AttesterSlashingPQ, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp *AttesterSlashingPQ
				tmp = new(AttesterSlashingPQ)
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
			c.AttesterSlashings = make([]*AttesterSlashingPQ, 0)
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
			if listLen > 16 {
				return fmt.Errorf("ssz-max exceeded: c.Attestations has %d elements, ssz-max is 16: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice5))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Attestations")
			}
			c.Attestations = make([]*AttestationPQ, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp *AttestationPQ
				tmp = new(AttestationPQ)
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
			c.Attestations = make([]*AttestationPQ, 0)
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

	// Field 7: SignedExecutionPayloadBid
	c.SignedExecutionPayloadBid = new(SignedExecutionPayloadBidPQ)
	if err = c.SignedExecutionPayloadBid.UnmarshalSSZ(sszSlice7); err != nil {
		return fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}

	// Field 8: ParentExecutionRequests
	c.ParentExecutionRequests = new(v1.ExecutionRequestsPQ)
	if err = c.ParentExecutionRequests.UnmarshalSSZ(sszSlice8); err != nil {
		return fmt.Errorf("ParentExecutionRequests: %w", err)
	}

	// Field 9: Proof
	c.Proof = append([]byte{}, sszSlice9...)
	return err
}

func (c *BeaconBlockBodyPQ) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *BeaconBlockBodyPQ) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsBeaconBlockBodyPQ = []byte{0b01111111, 0b00110100}

func (c *BeaconBlockBodyPQ) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockBodyPQ) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Position 0: RandaoReveal
	if len(c.RandaoReveal) != 1024 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.RandaoReveal)
	// Position 1: Eth1Data
	if err := c.Eth1Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Eth1Data: %w", err)
	}
	// Position 2: Graffiti
	if len(c.Graffiti) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Graffiti)
	// Position 3: ProposerSlashings
	{
		subIndx := hh.Index()
		for _, o := range c.ProposerSlashings {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("ProposerSlashings: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.ProposerSlashings)))
	}
	// Position 4: AttesterSlashings
	{
		subIndx := hh.Index()
		for _, o := range c.AttesterSlashings {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("AttesterSlashings: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.AttesterSlashings)))
	}
	// Position 5: Attestations
	{
		subIndx := hh.Index()
		for _, o := range c.Attestations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Attestations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Attestations)))
	}
	// Position 6: Deposits
	{
		subIndx := hh.Index()
		for _, o := range c.Deposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Deposits)))
	}
	// Position 7: inactive (zero chunk)
	hh.PutBool(false)
	// Position 8: inactive (zero chunk)
	hh.PutBool(false)
	// Position 9: inactive (zero chunk)
	hh.PutBool(false)
	// Position 10: SignedExecutionPayloadBid
	if err := c.SignedExecutionPayloadBid.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}
	// Position 11: inactive (zero chunk)
	hh.PutBool(false)
	// Position 12: ParentExecutionRequests
	if err := c.ParentExecutionRequests.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ParentExecutionRequests: %w", err)
	}
	// Position 13: Proof

	{
		if len(c.Proof) > 1048576 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.Proof)
		numItems := uint64(len(c.Proof))
		hh.MerkleizeWithMixin(subIndx, numItems, (1048576*1+31)/32)
	}

	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsBeaconBlockBodyPQ)
	return nil
}

func (c *BeaconBlockPQ) SizeSSZ() int {
	size := 84
	if c.Body == nil {
		c.Body = new(BeaconBlockBodyPQ)
	}
	size += c.Body.SizeSSZ()
	return size
}

func (c *BeaconBlockPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
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
		c.Body = new(BeaconBlockBodyPQ)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Body.SizeSSZ()

	// Field 4: Body
	if dst, err = c.Body.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Body: %w", err)
	}
	return dst, err
}

func (c *BeaconBlockPQ) UnmarshalSSZ(buf []byte) error {
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
	c.Body = new(BeaconBlockBodyPQ)
	if err = c.Body.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("Body: %w", err)
	}
	return err
}

func (c *BeaconBlockPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *BeaconStatePQ) SizeSSZ() int {
	size := 3085609
	size += len(c.HistoricalRoots) * 32
	size += len(c.Eth1DataVotes) * 72
	size += len(c.Validators) * 137
	size += len(c.Balances) * 8
	size += len(c.PreviousEpochParticipation)
	size += len(c.CurrentEpochParticipation)
	size += len(c.InactivityScores) * 8
	size += len(c.HistoricalSummaries) * 64
	size += len(c.PendingDeposits) * 192
	size += len(c.PendingPartialWithdrawals) * 24
	size += len(c.PendingConsolidations) * 16
	size += len(c.Builders) * 77
	size += len(c.BuilderPendingWithdrawals) * 36
	if c.LatestExecutionPayloadBid == nil {
		c.LatestExecutionPayloadBid = new(ExecutionPayloadBid)
	}
	size += c.LatestExecutionPayloadBid.SizeSSZ()
	size += len(c.PayloadExpectedWithdrawals) * 44
	size += len(c.ValidatorPreregistrations) * 104
	size += len(c.BuilderPreregistrations) * 72
	for _, o := range c.AggregationCommitteeWindow {
		size += 4
		size += o.SizeSSZ()
	}
	return size
}

func (c *BeaconStatePQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconStatePQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 3085609

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
	offset += len(c.Validators) * 137

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

	// Field 22: LatestBlockHash
	if len(c.LatestBlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LatestBlockHash...)

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
	if len(c.ProposerLookahead) != 64 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.ProposerLookahead {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("ProposerLookahead: %w", err)
		}
	}

	// Field 36: Builders
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Builders) * 77

	// Field 37: NextWithdrawalBuilderIndex
	if dst, err = c.NextWithdrawalBuilderIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextWithdrawalBuilderIndex: %w", err)
	}

	// Field 38: ExecutionPayloadAvailability
	if len(c.ExecutionPayloadAvailability) != 1024 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ExecutionPayloadAvailability...)

	// Field 39: BuilderPendingPayments
	if len(c.BuilderPendingPayments) != 64 {
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

	// Field 41: LatestExecutionPayloadBid
	if c.LatestExecutionPayloadBid == nil {
		c.LatestExecutionPayloadBid = new(ExecutionPayloadBid)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.LatestExecutionPayloadBid.SizeSSZ()

	// Field 42: PayloadExpectedWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PayloadExpectedWithdrawals) * 44

	// Field 43: PtcWindow
	if len(c.PtcWindow) != 96 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.PtcWindow {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PtcWindow: %w", err)
		}
	}

	// Field 44: ValidatorPreregistrations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ValidatorPreregistrations) * 104

	// Field 45: BuilderPreregistrations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BuilderPreregistrations) * 72

	// Field 46: AggregationCommitteeWindow
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.AggregationCommitteeWindow {
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
	if len(c.Eth1DataVotes) > 2048 {
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

	// Field 41: LatestExecutionPayloadBid
	if dst, err = c.LatestExecutionPayloadBid.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}

	// Field 42: PayloadExpectedWithdrawals
	for _, o := range c.PayloadExpectedWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
		}
	}

	// Field 44: ValidatorPreregistrations
	for _, o := range c.ValidatorPreregistrations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("ValidatorPreregistrations: %w", err)
		}
	}

	// Field 45: BuilderPreregistrations
	for _, o := range c.BuilderPreregistrations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderPreregistrations: %w", err)
		}
	}

	// Field 46: AggregationCommitteeWindow
	if len(c.AggregationCommitteeWindow) != 96 {
		return nil, ssz.ErrBytesLength
	}
	{
		offset = 4 * len(c.AggregationCommitteeWindow)
		for _, o := range c.AggregationCommitteeWindow {
			dst = ssz.WriteOffset(dst, offset)
			offset += o.SizeSSZ()
		}
	}
	for _, o := range c.AggregationCommitteeWindow {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("AggregationCommitteeWindow: %w", err)
		}
	}
	return dst, err
}

func (c *BeaconStatePQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 3085609 {
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
	sszSlice22 := buf[2687381:2687413] // c.LatestBlockHash
	sszSlice23 := buf[2687413:2687421] // c.NextWithdrawalIndex
	sszSlice24 := buf[2687421:2687429] // c.NextWithdrawalValidatorIndex
	sszSlice26 := buf[2687433:2687441] // c.DepositRequestsStartIndex
	sszSlice27 := buf[2687441:2687449] // c.DepositBalanceToConsume
	sszSlice28 := buf[2687449:2687457] // c.ExitBalanceToConsume
	sszSlice29 := buf[2687457:2687465] // c.EarliestExitEpoch
	sszSlice30 := buf[2687465:2687473] // c.ConsolidationBalanceToConsume
	sszSlice31 := buf[2687473:2687481] // c.EarliestConsolidationEpoch
	sszSlice35 := buf[2687493:2688005] // c.ProposerLookahead
	sszSlice37 := buf[2688009:2688017] // c.NextWithdrawalBuilderIndex
	sszSlice38 := buf[2688017:2689041] // c.ExecutionPayloadAvailability
	sszSlice39 := buf[2689041:2692369] // c.BuilderPendingPayments
	sszSlice43 := buf[2692381:3085597] // c.PtcWindow

	sszVarOffset7 := ssz.ReadOffset(buf[524464:524468]) // c.HistoricalRoots
	if sszVarOffset7 != 3085609 {
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
	sszVarOffset25 := ssz.ReadOffset(buf[2687429:2687433]) // c.HistoricalSummaries
	if sszVarOffset25 > size || sszVarOffset25 < sszVarOffset21 {
		return ssz.ErrOffset
	}
	sszVarOffset32 := ssz.ReadOffset(buf[2687481:2687485]) // c.PendingDeposits
	if sszVarOffset32 > size || sszVarOffset32 < sszVarOffset25 {
		return ssz.ErrOffset
	}
	sszVarOffset33 := ssz.ReadOffset(buf[2687485:2687489]) // c.PendingPartialWithdrawals
	if sszVarOffset33 > size || sszVarOffset33 < sszVarOffset32 {
		return ssz.ErrOffset
	}
	sszVarOffset34 := ssz.ReadOffset(buf[2687489:2687493]) // c.PendingConsolidations
	if sszVarOffset34 > size || sszVarOffset34 < sszVarOffset33 {
		return ssz.ErrOffset
	}
	sszVarOffset36 := ssz.ReadOffset(buf[2688005:2688009]) // c.Builders
	if sszVarOffset36 > size || sszVarOffset36 < sszVarOffset34 {
		return ssz.ErrOffset
	}
	sszVarOffset40 := ssz.ReadOffset(buf[2692369:2692373]) // c.BuilderPendingWithdrawals
	if sszVarOffset40 > size || sszVarOffset40 < sszVarOffset36 {
		return ssz.ErrOffset
	}
	sszVarOffset41 := ssz.ReadOffset(buf[2692373:2692377]) // c.LatestExecutionPayloadBid
	if sszVarOffset41 > size || sszVarOffset41 < sszVarOffset40 {
		return ssz.ErrOffset
	}
	sszVarOffset42 := ssz.ReadOffset(buf[2692377:2692381]) // c.PayloadExpectedWithdrawals
	if sszVarOffset42 > size || sszVarOffset42 < sszVarOffset41 {
		return ssz.ErrOffset
	}
	sszVarOffset44 := ssz.ReadOffset(buf[3085597:3085601]) // c.ValidatorPreregistrations
	if sszVarOffset44 > size || sszVarOffset44 < sszVarOffset42 {
		return ssz.ErrOffset
	}
	sszVarOffset45 := ssz.ReadOffset(buf[3085601:3085605]) // c.BuilderPreregistrations
	if sszVarOffset45 > size || sszVarOffset45 < sszVarOffset44 {
		return ssz.ErrOffset
	}
	sszVarOffset46 := ssz.ReadOffset(buf[3085605:3085609]) // c.AggregationCommitteeWindow
	if sszVarOffset46 > size || sszVarOffset46 < sszVarOffset45 {
		return ssz.ErrOffset
	}
	sszSlice7 := buf[sszVarOffset7:sszVarOffset9]    // c.HistoricalRoots
	sszSlice9 := buf[sszVarOffset9:sszVarOffset11]   // c.Eth1DataVotes
	sszSlice11 := buf[sszVarOffset11:sszVarOffset12] // c.Validators
	sszSlice12 := buf[sszVarOffset12:sszVarOffset15] // c.Balances
	sszSlice15 := buf[sszVarOffset15:sszVarOffset16] // c.PreviousEpochParticipation
	sszSlice16 := buf[sszVarOffset16:sszVarOffset21] // c.CurrentEpochParticipation
	sszSlice21 := buf[sszVarOffset21:sszVarOffset25] // c.InactivityScores
	sszSlice25 := buf[sszVarOffset25:sszVarOffset32] // c.HistoricalSummaries
	sszSlice32 := buf[sszVarOffset32:sszVarOffset33] // c.PendingDeposits
	sszSlice33 := buf[sszVarOffset33:sszVarOffset34] // c.PendingPartialWithdrawals
	sszSlice34 := buf[sszVarOffset34:sszVarOffset36] // c.PendingConsolidations
	sszSlice36 := buf[sszVarOffset36:sszVarOffset40] // c.Builders
	sszSlice40 := buf[sszVarOffset40:sszVarOffset41] // c.BuilderPendingWithdrawals
	sszSlice41 := buf[sszVarOffset41:sszVarOffset42] // c.LatestExecutionPayloadBid
	sszSlice42 := buf[sszVarOffset42:sszVarOffset44] // c.PayloadExpectedWithdrawals
	sszSlice44 := buf[sszVarOffset44:sszVarOffset45] // c.ValidatorPreregistrations
	sszSlice45 := buf[sszVarOffset45:sszVarOffset46] // c.BuilderPreregistrations
	sszSlice46 := buf[sszVarOffset46:]               // c.AggregationCommitteeWindow

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
		if len(sszSlice11)%137 != 0 {
			return fmt.Errorf("misaligned bytes: c.Validators length is %d, which is not a multiple of 137: %w", len(sszSlice11), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice11) / 137
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.Validators has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
		c.Validators = make([]*ValidatorPQ, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ValidatorPQ
			tmp = new(ValidatorPQ)
			tmpSlice := sszSlice11[i*137 : (1+i)*137]
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

	// Field 22: LatestBlockHash
	c.LatestBlockHash = make([]byte, 0, 32)
	c.LatestBlockHash = append(c.LatestBlockHash, sszSlice22...)

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
		if numElem > 134217728 {
			return fmt.Errorf("ssz-max exceeded: c.PendingDeposits has %d elements, ssz-max is 134217728: %w", numElem, ssz.ErrListTooBig)
		}
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
		if numElem > 134217728 {
			return fmt.Errorf("ssz-max exceeded: c.PendingPartialWithdrawals has %d elements, ssz-max is 134217728: %w", numElem, ssz.ErrListTooBig)
		}
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
		if numElem > 262144 {
			return fmt.Errorf("ssz-max exceeded: c.PendingConsolidations has %d elements, ssz-max is 262144: %w", numElem, ssz.ErrListTooBig)
		}
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
		c.ProposerLookahead = make([]primitives.ValidatorIndex, 64)
		for i := 0; i < 64; i++ {
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
		if len(sszSlice36)%77 != 0 {
			return fmt.Errorf("misaligned bytes: c.Builders length is %d, which is not a multiple of 77: %w", len(sszSlice36), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice36) / 77
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.Builders has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
		c.Builders = make([]*BuilderPQ, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *BuilderPQ
			tmp = new(BuilderPQ)
			tmpSlice := sszSlice36[i*77 : (1+i)*77]
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
	c.ExecutionPayloadAvailability = make([]byte, 0, 1024)
	c.ExecutionPayloadAvailability = append(c.ExecutionPayloadAvailability, sszSlice38...)

	// Field 39: BuilderPendingPayments
	{
		c.BuilderPendingPayments = make([]*BuilderPendingPayment, 64)
		for i := 0; i < 64; i++ {
			var tmp *BuilderPendingPayment
			tmp = new(BuilderPendingPayment)
			tmpSlice := sszSlice39[i*52 : (1+i)*52]
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
		if numElem > 1048576 {
			return fmt.Errorf("ssz-max exceeded: c.BuilderPendingWithdrawals has %d elements, ssz-max is 1048576: %w", numElem, ssz.ErrListTooBig)
		}
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

	// Field 41: LatestExecutionPayloadBid
	c.LatestExecutionPayloadBid = new(ExecutionPayloadBid)
	if err = c.LatestExecutionPayloadBid.UnmarshalSSZ(sszSlice41); err != nil {
		return fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}

	// Field 42: PayloadExpectedWithdrawals
	{
		if len(sszSlice42)%44 != 0 {
			return fmt.Errorf("misaligned bytes: c.PayloadExpectedWithdrawals length is %d, which is not a multiple of 44: %w", len(sszSlice42), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice42) / 44
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.PayloadExpectedWithdrawals has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
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
		c.PtcWindow = make([]*PTCs, 96)
		for i := 0; i < 96; i++ {
			var tmp *PTCs
			tmp = new(PTCs)
			tmpSlice := sszSlice43[i*4096 : (1+i)*4096]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PtcWindow: %w", err)
			}
			c.PtcWindow[i] = tmp
		}
	}

	// Field 44: ValidatorPreregistrations
	{
		if len(sszSlice44)%104 != 0 {
			return fmt.Errorf("misaligned bytes: c.ValidatorPreregistrations length is %d, which is not a multiple of 104: %w", len(sszSlice44), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice44) / 104
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.ValidatorPreregistrations has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
		c.ValidatorPreregistrations = make([]*ValidatorPreregistration, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ValidatorPreregistration
			tmp = new(ValidatorPreregistration)
			tmpSlice := sszSlice44[i*104 : (1+i)*104]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("ValidatorPreregistrations: %w", err)
			}
			c.ValidatorPreregistrations[i] = tmp
		}
	}

	// Field 45: BuilderPreregistrations
	{
		if len(sszSlice45)%72 != 0 {
			return fmt.Errorf("misaligned bytes: c.BuilderPreregistrations length is %d, which is not a multiple of 72: %w", len(sszSlice45), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice45) / 72
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.BuilderPreregistrations has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
		c.BuilderPreregistrations = make([]*BuilderPreregistration, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *BuilderPreregistration
			tmp = new(BuilderPreregistration)
			tmpSlice := sszSlice45[i*72 : (1+i)*72]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderPreregistrations: %w", err)
			}
			c.BuilderPreregistrations[i] = tmp
		}
	}

	// Field 46: AggregationCommitteeWindow
	{
		if len(sszSlice46) < 4*96 {
			return fmt.Errorf("vector bytes too short to contain offsets when decoding c.AggregationCommitteeWindow: %w", ssz.ErrSize)
		}
		startOffset := ssz.ReadOffset(sszSlice46[0:4])
		if startOffset != 4*96 {
			return fmt.Errorf("invalid initial offset %d when decoding c.AggregationCommitteeWindow, expected %d: %w", startOffset, 4*96, ssz.ErrInvalidVariableOffset)
		}
		totalVarBytes := uint64(len(sszSlice46))
		c.AggregationCommitteeWindow = make([]*AggregationCommittees, 96)
		var tmpSlice []byte
		for i := uint64(0); i < 96; i++ {
			var tmp *AggregationCommittees
			tmp = new(AggregationCommittees)
			endOffset := totalVarBytes
			if i+1 != 96 {
				endOffset = ssz.ReadOffset(sszSlice46[(i+1)*4 : (i+2)*4])
				if totalVarBytes < endOffset {
					return fmt.Errorf("offset %d points past the end of buffer when decoding c.AggregationCommitteeWindow", endOffset)
				}
			}
			if endOffset < startOffset {
				return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.AggregationCommitteeWindow", endOffset, startOffset)
			}
			tmpSlice = sszSlice46[startOffset:endOffset]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("AggregationCommitteeWindow: %w", err)
			}
			c.AggregationCommitteeWindow[i] = tmp
			startOffset = endOffset
		}
	}
	return err
}

func (c *BeaconStatePQ) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *BeaconStatePQ) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsBeaconStatePQ = []byte{0b11111111, 0b11111111, 0b00111111, 0b11111111, 0b11111111, 0b00111111, 0b00000111}

func (c *BeaconStatePQ) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconStatePQ) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Position 0: GenesisTime
	hh.PutUint64(c.GenesisTime)
	// Position 1: GenesisValidatorsRoot
	if len(c.GenesisValidatorsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.GenesisValidatorsRoot)
	// Position 2: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Position 3: Fork
	if err := c.Fork.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Fork: %w", err)
	}
	// Position 4: LatestBlockHeader
	if err := c.LatestBlockHeader.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("LatestBlockHeader: %w", err)
	}
	// Position 5: BlockRoots
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
	// Position 6: StateRoots
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
	// Position 7: HistoricalRoots
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
	// Position 8: Eth1Data
	if err := c.Eth1Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Eth1Data: %w", err)
	}
	// Position 9: Eth1DataVotes
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
	// Position 10: Eth1DepositIndex
	hh.PutUint64(c.Eth1DepositIndex)
	// Position 11: Validators
	{
		subIndx := hh.Index()
		for _, o := range c.Validators {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Validators: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Validators)))
	}
	// Position 12: Balances
	{
		subIndx := hh.Index()
		for _, o := range c.Balances {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Balances)))
	}
	// Position 13: RandaoMixes
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
	// Position 14: Slashings
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
	// Position 15: PreviousEpochParticipation
	{
		subIndx := hh.Index()
		hh.AppendBytes32(c.PreviousEpochParticipation)
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PreviousEpochParticipation)))
	}
	// Position 16: CurrentEpochParticipation
	{
		subIndx := hh.Index()
		hh.AppendBytes32(c.CurrentEpochParticipation)
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.CurrentEpochParticipation)))
	}
	// Position 17: JustificationBits
	if len([]byte(c.JustificationBits)) != 1 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.JustificationBits))
	// Position 18: PreviousJustifiedCheckpoint
	if err := c.PreviousJustifiedCheckpoint.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("PreviousJustifiedCheckpoint: %w", err)
	}
	// Position 19: CurrentJustifiedCheckpoint
	if err := c.CurrentJustifiedCheckpoint.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("CurrentJustifiedCheckpoint: %w", err)
	}
	// Position 20: FinalizedCheckpoint
	if err := c.FinalizedCheckpoint.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("FinalizedCheckpoint: %w", err)
	}
	// Position 21: InactivityScores
	{
		subIndx := hh.Index()
		for _, o := range c.InactivityScores {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.InactivityScores)))
	}
	// Position 22: inactive (zero chunk)
	hh.PutBool(false)
	// Position 23: inactive (zero chunk)
	hh.PutBool(false)
	// Position 24: LatestBlockHash
	if len(c.LatestBlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LatestBlockHash)
	// Position 25: NextWithdrawalIndex
	hh.PutUint64(c.NextWithdrawalIndex)
	// Position 26: NextWithdrawalValidatorIndex
	if err := c.NextWithdrawalValidatorIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextWithdrawalValidatorIndex: %w", err)
	}
	// Position 27: HistoricalSummaries
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
	// Position 28: DepositRequestsStartIndex
	hh.PutUint64(c.DepositRequestsStartIndex)
	// Position 29: DepositBalanceToConsume
	if err := c.DepositBalanceToConsume.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("DepositBalanceToConsume: %w", err)
	}
	// Position 30: ExitBalanceToConsume
	if err := c.ExitBalanceToConsume.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExitBalanceToConsume: %w", err)
	}
	// Position 31: EarliestExitEpoch
	if err := c.EarliestExitEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("EarliestExitEpoch: %w", err)
	}
	// Position 32: ConsolidationBalanceToConsume
	if err := c.ConsolidationBalanceToConsume.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ConsolidationBalanceToConsume: %w", err)
	}
	// Position 33: EarliestConsolidationEpoch
	if err := c.EarliestConsolidationEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("EarliestConsolidationEpoch: %w", err)
	}
	// Position 34: PendingDeposits
	{
		subIndx := hh.Index()
		for _, o := range c.PendingDeposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingDeposits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingDeposits)))
	}
	// Position 35: PendingPartialWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.PendingPartialWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingPartialWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingPartialWithdrawals)))
	}
	// Position 36: PendingConsolidations
	{
		subIndx := hh.Index()
		for _, o := range c.PendingConsolidations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingConsolidations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingConsolidations)))
	}
	// Position 37: ProposerLookahead
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
	// Position 38: Builders
	{
		subIndx := hh.Index()
		for _, o := range c.Builders {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Builders: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Builders)))
	}
	// Position 39: NextWithdrawalBuilderIndex
	if err := c.NextWithdrawalBuilderIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextWithdrawalBuilderIndex: %w", err)
	}
	// Position 40: ExecutionPayloadAvailability
	if len(c.ExecutionPayloadAvailability) != 1024 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ExecutionPayloadAvailability)
	// Position 41: BuilderPendingPayments
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
	// Position 42: BuilderPendingWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.BuilderPendingWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderPendingWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BuilderPendingWithdrawals)))
	}
	// Position 43: LatestExecutionPayloadBid
	if err := c.LatestExecutionPayloadBid.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}
	// Position 44: PayloadExpectedWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.PayloadExpectedWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PayloadExpectedWithdrawals)))
	}
	// Position 45: PtcWindow
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
	// Position 46: inactive (zero chunk)
	hh.PutBool(false)
	// Position 47: inactive (zero chunk)
	hh.PutBool(false)
	// Position 48: ValidatorPreregistrations
	{
		subIndx := hh.Index()
		for _, o := range c.ValidatorPreregistrations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("ValidatorPreregistrations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.ValidatorPreregistrations)))
	}
	// Position 49: BuilderPreregistrations
	{
		subIndx := hh.Index()
		for _, o := range c.BuilderPreregistrations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderPreregistrations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BuilderPreregistrations)))
	}
	// Position 50: AggregationCommitteeWindow
	{
		if len(c.AggregationCommitteeWindow) != 96 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.AggregationCommitteeWindow {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("AggregationCommitteeWindow: %w", err)
			}
		}
		hh.Merkleize(subIndx)
	}
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsBeaconStatePQ)
	return nil
}

func (c *BlindedExecutionPayloadEnvelopePQ) SizeSSZ() int {
	size := 148
	if c.ExecutionRequests == nil {
		c.ExecutionRequests = new(v1.ExecutionRequestsPQ)
	}
	size += c.ExecutionRequests.SizeSSZ()
	return size
}

func (c *BlindedExecutionPayloadEnvelopePQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BlindedExecutionPayloadEnvelopePQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 148

	// Field 0: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash...)

	// Field 1: ExecutionRequests
	if c.ExecutionRequests == nil {
		c.ExecutionRequests = new(v1.ExecutionRequestsPQ)
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

func (c *BlindedExecutionPayloadEnvelopePQ) UnmarshalSSZ(buf []byte) error {
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
	c.ExecutionRequests = new(v1.ExecutionRequestsPQ)
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

func (c *BlindedExecutionPayloadEnvelopePQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BlindedExecutionPayloadEnvelopePQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *BuilderPQ) SizeSSZ() int {
	size := 77

	return size
}

func (c *BuilderPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BuilderPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Pubkey
	if len(c.Pubkey) != 32 {
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

func (c *BuilderPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 77 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]  // c.Pubkey
	sszSlice1 := buf[32:33] // c.Version
	sszSlice2 := buf[33:53] // c.ExecutionAddress
	sszSlice3 := buf[53:61] // c.Balance
	sszSlice4 := buf[61:69] // c.DepositEpoch
	sszSlice5 := buf[69:77] // c.WithdrawableEpoch

	// Field 0: Pubkey
	c.Pubkey = make([]byte, 0, 32)
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

func (c *BuilderPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BuilderPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Pubkey
	if len(c.Pubkey) != 32 {
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

func (c *BuilderPreregistration) SizeSSZ() int {
	size := 72

	return size
}

func (c *BuilderPreregistration) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BuilderPreregistration) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Pubkey
	if len(c.Pubkey) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey...)

	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalCredentials...)

	// Field 2: ExpirySlot
	if dst, err = c.ExpirySlot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ExpirySlot: %w", err)
	}

	return dst, err
}

func (c *BuilderPreregistration) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 72 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]  // c.Pubkey
	sszSlice1 := buf[32:64] // c.WithdrawalCredentials
	sszSlice2 := buf[64:72] // c.ExpirySlot

	// Field 0: Pubkey
	c.Pubkey = make([]byte, 0, 32)
	c.Pubkey = append(c.Pubkey, sszSlice0...)

	// Field 1: WithdrawalCredentials
	c.WithdrawalCredentials = make([]byte, 0, 32)
	c.WithdrawalCredentials = append(c.WithdrawalCredentials, sszSlice1...)

	// Field 2: ExpirySlot
	if err = c.ExpirySlot.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("ExpirySlot: %w", err)
	}
	return err
}

func (c *BuilderPreregistration) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BuilderPreregistration) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Pubkey
	if len(c.Pubkey) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey)
	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalCredentials)
	// Field 2: ExpirySlot
	if err := c.ExpirySlot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExpirySlot: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *ExecutionPayloadEnvelopePQ) SizeSSZ() int {
	size := 80
	if c.Payload == nil {
		c.Payload = new(v1.ExecutionPayloadGloas)
	}
	size += c.Payload.SizeSSZ()
	if c.ExecutionRequests == nil {
		c.ExecutionRequests = new(v1.ExecutionRequestsPQ)
	}
	size += c.ExecutionRequests.SizeSSZ()
	return size
}

func (c *ExecutionPayloadEnvelopePQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadEnvelopePQ) MarshalSSZTo(dst []byte) ([]byte, error) {
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
		c.ExecutionRequests = new(v1.ExecutionRequestsPQ)
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

func (c *ExecutionPayloadEnvelopePQ) UnmarshalSSZ(buf []byte) error {
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
	c.ExecutionRequests = new(v1.ExecutionRequestsPQ)
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

func (c *ExecutionPayloadEnvelopePQ) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *ExecutionPayloadEnvelopePQ) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsExecutionPayloadEnvelopePQ = []byte{0b00011111}

func (c *ExecutionPayloadEnvelopePQ) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadEnvelopePQ) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Position 0: Payload
	if err := c.Payload.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Payload: %w", err)
	}
	// Position 1: ExecutionRequests
	if err := c.ExecutionRequests.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExecutionRequests: %w", err)
	}
	// Position 2: BuilderIndex
	if err := c.BuilderIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("BuilderIndex: %w", err)
	}
	// Position 3: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BeaconBlockRoot)
	// Position 4: ParentBeaconBlockRoot
	if len(c.ParentBeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentBeaconBlockRoot)
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsExecutionPayloadEnvelopePQ)
	return nil
}

func (c *IndexedAttestationPQ) SizeSSZ() int {
	size := 136
	size += len(c.AttestingIndices) * 8
	size += len(c.Proof)
	return size
}

func (c *IndexedAttestationPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *IndexedAttestationPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 136

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

	// Field 2: Proof
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Proof)

	// Field 0: AttestingIndices
	for _, o := range c.AttestingIndices {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 2: Proof
	if len(c.Proof) > 1048576 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.Proof...)
	return dst, err
}

func (c *IndexedAttestationPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 136 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:132] // c.Data

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.AttestingIndices
	if sszVarOffset0 != 136 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[132:136]) // c.Proof
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset2] // c.AttestingIndices
	sszSlice2 := buf[sszVarOffset2:]              // c.Proof

	// Field 0: AttestingIndices
	{
		if len(sszSlice0)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.AttestingIndices length is %d, which is not a multiple of 8: %w", len(sszSlice0), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice0) / 8
		if numElem > 32768 {
			return fmt.Errorf("ssz-max exceeded: c.AttestingIndices has %d elements, ssz-max is 32768: %w", numElem, ssz.ErrListTooBig)
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

	// Field 2: Proof
	c.Proof = append([]byte{}, sszSlice2...)
	return err
}

func (c *IndexedAttestationPQ) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *IndexedAttestationPQ) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsIndexedAttestationPQ = []byte{0b00000111}

func (c *IndexedAttestationPQ) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *IndexedAttestationPQ) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Position 0: AttestingIndices
	{
		subIndx := hh.Index()
		for _, o := range c.AttestingIndices {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.AttestingIndices)))
	}
	// Position 1: Data
	if err := c.Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Data: %w", err)
	}
	// Position 2: Proof

	{
		if len(c.Proof) > 1048576 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.Proof)
		numItems := uint64(len(c.Proof))
		hh.MerkleizeWithMixin(subIndx, numItems, (1048576*1+31)/32)
	}

	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsIndexedAttestationPQ)
	return nil
}

func (c *PayloadAttestationMessagePQ) SizeSSZ() int {
	size := 2454

	return size
}

func (c *PayloadAttestationMessagePQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *PayloadAttestationMessagePQ) MarshalSSZTo(dst []byte) ([]byte, error) {
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
	if len(c.Signature) != 2404 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	return dst, err
}

func (c *PayloadAttestationMessagePQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 2454 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]     // c.ValidatorIndex
	sszSlice1 := buf[8:50]    // c.Data
	sszSlice2 := buf[50:2454] // c.Signature

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
	c.Signature = make([]byte, 0, 2404)
	c.Signature = append(c.Signature, sszSlice2...)
	return err
}

func (c *PayloadAttestationMessagePQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *PayloadAttestationMessagePQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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
	if len(c.Signature) != 2404 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *ProposerSlashingPQ) SizeSSZ() int {
	size := 5032

	return size
}

func (c *ProposerSlashingPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ProposerSlashingPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Header_1
	if c.Header_1 == nil {
		c.Header_1 = new(SignedBeaconBlockHeaderPQ)
	}
	if dst, err = c.Header_1.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Header_1: %w", err)
	}

	// Field 1: Header_2
	if c.Header_2 == nil {
		c.Header_2 = new(SignedBeaconBlockHeaderPQ)
	}
	if dst, err = c.Header_2.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Header_2: %w", err)
	}

	return dst, err
}

func (c *ProposerSlashingPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 5032 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:2516]    // c.Header_1
	sszSlice1 := buf[2516:5032] // c.Header_2

	// Field 0: Header_1
	c.Header_1 = new(SignedBeaconBlockHeaderPQ)
	if err = c.Header_1.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Header_1: %w", err)
	}

	// Field 1: Header_2
	c.Header_2 = new(SignedBeaconBlockHeaderPQ)
	if err = c.Header_2.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Header_2: %w", err)
	}
	return err
}

func (c *ProposerSlashingPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ProposerSlashingPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Header_1
	if err := c.Header_1.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Header_1: %w", err)
	}
	// Field 1: Header_2
	if err := c.Header_2.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Header_2: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *SignedAggregateAttestationAndProofPQ) SizeSSZ() int {
	size := 2408
	if c.Message == nil {
		c.Message = new(AggregateAttestationAndProofPQ)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedAggregateAttestationAndProofPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedAggregateAttestationAndProofPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 2408

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(AggregateAttestationAndProofPQ)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Message.SizeSSZ()

	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	// Field 0: Message
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}
	return dst, err
}

func (c *SignedAggregateAttestationAndProofPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 2408 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:2408] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Message
	if sszVarOffset0 != 2408 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Message

	// Field 0: Message
	c.Message = new(AggregateAttestationAndProofPQ)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 2404)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedAggregateAttestationAndProofPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedAggregateAttestationAndProofPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *SignedBeaconBlockHeaderPQ) SizeSSZ() int {
	size := 2516

	return size
}

func (c *SignedBeaconBlockHeaderPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedBeaconBlockHeaderPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Header
	if c.Header == nil {
		c.Header = new(BeaconBlockHeader)
	}
	if dst, err = c.Header.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Header: %w", err)
	}

	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	return dst, err
}

func (c *SignedBeaconBlockHeaderPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 2516 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:112]    // c.Header
	sszSlice1 := buf[112:2516] // c.Signature

	// Field 0: Header
	c.Header = new(BeaconBlockHeader)
	if err = c.Header.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Header: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 2404)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedBeaconBlockHeaderPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedBeaconBlockHeaderPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Header
	if err := c.Header.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Header: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *SignedBeaconBlockPQ) SizeSSZ() int {
	size := 2408
	if c.Block == nil {
		c.Block = new(BeaconBlockPQ)
	}
	size += c.Block.SizeSSZ()
	return size
}

func (c *SignedBeaconBlockPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedBeaconBlockPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 2408

	// Field 0: Block
	if c.Block == nil {
		c.Block = new(BeaconBlockPQ)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Block.SizeSSZ()

	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	// Field 0: Block
	if dst, err = c.Block.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Block: %w", err)
	}
	return dst, err
}

func (c *SignedBeaconBlockPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 2408 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:2408] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Block
	if sszVarOffset0 != 2408 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Block

	// Field 0: Block
	c.Block = new(BeaconBlockPQ)
	if err = c.Block.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Block: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 2404)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedBeaconBlockPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedBeaconBlockPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Block
	if err := c.Block.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Block: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *SignedBlindedExecutionPayloadEnvelopePQ) SizeSSZ() int {
	size := 2408
	if c.Message == nil {
		c.Message = new(BlindedExecutionPayloadEnvelopePQ)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedBlindedExecutionPayloadEnvelopePQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedBlindedExecutionPayloadEnvelopePQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 2408

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(BlindedExecutionPayloadEnvelopePQ)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Message.SizeSSZ()

	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	// Field 0: Message
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}
	return dst, err
}

func (c *SignedBlindedExecutionPayloadEnvelopePQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 2408 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:2408] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Message
	if sszVarOffset0 != 2408 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Message

	// Field 0: Message
	c.Message = new(BlindedExecutionPayloadEnvelopePQ)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 2404)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedBlindedExecutionPayloadEnvelopePQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedBlindedExecutionPayloadEnvelopePQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *SignedExecutionPayloadBidPQ) SizeSSZ() int {
	size := 2408
	if c.Message == nil {
		c.Message = new(ExecutionPayloadBid)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedExecutionPayloadBidPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedExecutionPayloadBidPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 2408

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(ExecutionPayloadBid)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Message.SizeSSZ()

	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	// Field 0: Message
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}
	return dst, err
}

func (c *SignedExecutionPayloadBidPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 2408 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:2408] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Message
	if sszVarOffset0 != 2408 {
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
	c.Signature = make([]byte, 0, 2404)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedExecutionPayloadBidPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedExecutionPayloadBidPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *SignedExecutionPayloadEnvelopePQ) SizeSSZ() int {
	size := 2408
	if c.Message == nil {
		c.Message = new(ExecutionPayloadEnvelopePQ)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedExecutionPayloadEnvelopePQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedExecutionPayloadEnvelopePQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 2408

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(ExecutionPayloadEnvelopePQ)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Message.SizeSSZ()

	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	// Field 0: Message
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}
	return dst, err
}

func (c *SignedExecutionPayloadEnvelopePQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 2408 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:2408] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Message
	if sszVarOffset0 != 2408 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Message

	// Field 0: Message
	c.Message = new(ExecutionPayloadEnvelopePQ)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 2404)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedExecutionPayloadEnvelopePQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedExecutionPayloadEnvelopePQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *SignedProposerPreferencesPQ) SizeSSZ() int {
	size := 2480

	return size
}

func (c *SignedProposerPreferencesPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedProposerPreferencesPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(ProposerPreferences)
	}
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	return dst, err
}

func (c *SignedProposerPreferencesPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 2480 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:76]    // c.Message
	sszSlice1 := buf[76:2480] // c.Signature

	// Field 0: Message
	c.Message = new(ProposerPreferences)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 2404)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedProposerPreferencesPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedProposerPreferencesPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 2404 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *SingleAttestationPQ) SizeSSZ() int {
	size := 2548

	return size
}

func (c *SingleAttestationPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SingleAttestationPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
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
		c.Data = new(AttestationData)
	}
	if dst, err = c.Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Data: %w", err)
	}

	// Field 3: Signature
	if len(c.Signature) != 2404 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	return dst, err
}

func (c *SingleAttestationPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 2548 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]      // c.CommitteeId
	sszSlice1 := buf[8:16]     // c.AttesterIndex
	sszSlice2 := buf[16:144]   // c.Data
	sszSlice3 := buf[144:2548] // c.Signature

	// Field 0: CommitteeId
	if err = c.CommitteeId.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("CommitteeId: %w", err)
	}

	// Field 1: AttesterIndex
	if err = c.AttesterIndex.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("AttesterIndex: %w", err)
	}

	// Field 2: Data
	c.Data = new(AttestationData)
	if err = c.Data.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 3: Signature
	c.Signature = make([]byte, 0, 2404)
	c.Signature = append(c.Signature, sszSlice3...)
	return err
}

func (c *SingleAttestationPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SingleAttestationPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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
	if len(c.Signature) != 2404 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *ValidatorPQ) SizeSSZ() int {
	size := 137

	return size
}

func (c *ValidatorPQ) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ValidatorPQ) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: PublicKey
	if len(c.PublicKey) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PublicKey...)

	// Field 1: VrfRoot
	if len(c.VrfRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.VrfRoot...)

	// Field 2: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalCredentials...)

	// Field 3: EffectiveBalance
	dst = binary.LittleEndian.AppendUint64(dst, c.EffectiveBalance)

	// Field 4: Slashed
	if c.Slashed {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}

	// Field 5: ActivationEligibilityEpoch
	if dst, err = c.ActivationEligibilityEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ActivationEligibilityEpoch: %w", err)
	}

	// Field 6: ActivationEpoch
	if dst, err = c.ActivationEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ActivationEpoch: %w", err)
	}

	// Field 7: ExitEpoch
	if dst, err = c.ExitEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ExitEpoch: %w", err)
	}

	// Field 8: WithdrawableEpoch
	if dst, err = c.WithdrawableEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("WithdrawableEpoch: %w", err)
	}

	return dst, err
}

func (c *ValidatorPQ) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 137 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]    // c.PublicKey
	sszSlice1 := buf[32:64]   // c.VrfRoot
	sszSlice2 := buf[64:96]   // c.WithdrawalCredentials
	sszSlice3 := buf[96:104]  // c.EffectiveBalance
	sszSlice4 := buf[104:105] // c.Slashed
	sszSlice5 := buf[105:113] // c.ActivationEligibilityEpoch
	sszSlice6 := buf[113:121] // c.ActivationEpoch
	sszSlice7 := buf[121:129] // c.ExitEpoch
	sszSlice8 := buf[129:137] // c.WithdrawableEpoch

	// Field 0: PublicKey
	c.PublicKey = make([]byte, 0, 32)
	c.PublicKey = append(c.PublicKey, sszSlice0...)

	// Field 1: VrfRoot
	c.VrfRoot = make([]byte, 0, 32)
	c.VrfRoot = append(c.VrfRoot, sszSlice1...)

	// Field 2: WithdrawalCredentials
	c.WithdrawalCredentials = make([]byte, 0, 32)
	c.WithdrawalCredentials = append(c.WithdrawalCredentials, sszSlice2...)

	// Field 3: EffectiveBalance
	c.EffectiveBalance = binary.LittleEndian.Uint64(sszSlice3)

	// Field 4: Slashed
	if sszSlice4[0] > 1 {
		return ssz.ErrInvalidSerialization
	}
	if sszSlice4[0] == 1 {
		c.Slashed = true
	} else {
		c.Slashed = false
	}

	// Field 5: ActivationEligibilityEpoch
	if err = c.ActivationEligibilityEpoch.UnmarshalSSZ(sszSlice5); err != nil {
		return fmt.Errorf("ActivationEligibilityEpoch: %w", err)
	}

	// Field 6: ActivationEpoch
	if err = c.ActivationEpoch.UnmarshalSSZ(sszSlice6); err != nil {
		return fmt.Errorf("ActivationEpoch: %w", err)
	}

	// Field 7: ExitEpoch
	if err = c.ExitEpoch.UnmarshalSSZ(sszSlice7); err != nil {
		return fmt.Errorf("ExitEpoch: %w", err)
	}

	// Field 8: WithdrawableEpoch
	if err = c.WithdrawableEpoch.UnmarshalSSZ(sszSlice8); err != nil {
		return fmt.Errorf("WithdrawableEpoch: %w", err)
	}
	return err
}

func (c *ValidatorPQ) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ValidatorPQ) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: PublicKey
	if len(c.PublicKey) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PublicKey)
	// Field 1: VrfRoot
	if len(c.VrfRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.VrfRoot)
	// Field 2: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalCredentials)
	// Field 3: EffectiveBalance
	hh.PutUint64(c.EffectiveBalance)
	// Field 4: Slashed
	hh.PutBool(c.Slashed)
	// Field 5: ActivationEligibilityEpoch
	if err := c.ActivationEligibilityEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ActivationEligibilityEpoch: %w", err)
	}
	// Field 6: ActivationEpoch
	if err := c.ActivationEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ActivationEpoch: %w", err)
	}
	// Field 7: ExitEpoch
	if err := c.ExitEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExitEpoch: %w", err)
	}
	// Field 8: WithdrawableEpoch
	if err := c.WithdrawableEpoch.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("WithdrawableEpoch: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *ValidatorPreregistration) SizeSSZ() int {
	size := 104

	return size
}

func (c *ValidatorPreregistration) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ValidatorPreregistration) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Pubkey
	if len(c.Pubkey) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey...)

	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalCredentials...)

	// Field 2: VrfRoot
	if len(c.VrfRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.VrfRoot...)

	// Field 3: ExpirySlot
	if dst, err = c.ExpirySlot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ExpirySlot: %w", err)
	}

	return dst, err
}

func (c *ValidatorPreregistration) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 104 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]   // c.Pubkey
	sszSlice1 := buf[32:64]  // c.WithdrawalCredentials
	sszSlice2 := buf[64:96]  // c.VrfRoot
	sszSlice3 := buf[96:104] // c.ExpirySlot

	// Field 0: Pubkey
	c.Pubkey = make([]byte, 0, 32)
	c.Pubkey = append(c.Pubkey, sszSlice0...)

	// Field 1: WithdrawalCredentials
	c.WithdrawalCredentials = make([]byte, 0, 32)
	c.WithdrawalCredentials = append(c.WithdrawalCredentials, sszSlice1...)

	// Field 2: VrfRoot
	c.VrfRoot = make([]byte, 0, 32)
	c.VrfRoot = append(c.VrfRoot, sszSlice2...)

	// Field 3: ExpirySlot
	if err = c.ExpirySlot.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("ExpirySlot: %w", err)
	}
	return err
}

func (c *ValidatorPreregistration) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ValidatorPreregistration) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Pubkey
	if len(c.Pubkey) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey)
	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalCredentials)
	// Field 2: VrfRoot
	if len(c.VrfRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.VrfRoot)
	// Field 3: ExpirySlot
	if err := c.ExpirySlot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExpirySlot: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}
