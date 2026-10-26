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

func (c *BeaconBlockBodyHeze) SizeSSZ() int {
	size := 320
	size += len(c.ProposerSlashings) * 416
	for _, o := range c.AttesterSlashings {
		size += 4
		size += o.SizeSSZ()
	}
	for _, o := range c.Attestations {
		size += 4
		size += o.SizeSSZ()
	}
	size += len(c.VoluntaryExits) * 112
	size += len(c.BlsToExecutionChanges) * 172
	if c.SignedExecutionPayloadBid == nil {
		c.SignedExecutionPayloadBid = new(SignedExecutionPayloadBidHeze)
	}
	size += c.SignedExecutionPayloadBid.SizeSSZ()
	size += len(c.PayloadAttestations) * 202
	if c.ParentExecutionRequests == nil {
		c.ParentExecutionRequests = new(v1.ExecutionRequestsGloas)
	}
	size += c.ParentExecutionRequests.SizeSSZ()
	return size
}

func (c *BeaconBlockBodyHeze) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockBodyHeze) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 320

	// Field 0: RandaoReveal
	if len(c.RandaoReveal) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.RandaoReveal...)

	// Field 1: Graffiti
	if len(c.Graffiti) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Graffiti...)

	// Field 2: ProposerSlashings
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ProposerSlashings) * 416

	// Field 3: AttesterSlashings
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.AttesterSlashings {
		offset += 4
		offset += o.SizeSSZ()
	}

	// Field 4: Attestations
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.Attestations {
		offset += 4
		offset += o.SizeSSZ()
	}

	// Field 5: VoluntaryExits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.VoluntaryExits) * 112

	// Field 6: SyncAggregate
	if c.SyncAggregate == nil {
		c.SyncAggregate = new(SyncAggregate)
	}
	if dst, err = c.SyncAggregate.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 7: BlsToExecutionChanges
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BlsToExecutionChanges) * 172

	// Field 8: SignedExecutionPayloadBid
	if c.SignedExecutionPayloadBid == nil {
		c.SignedExecutionPayloadBid = new(SignedExecutionPayloadBidHeze)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.SignedExecutionPayloadBid.SizeSSZ()

	// Field 9: PayloadAttestations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PayloadAttestations) * 202

	// Field 10: ParentExecutionRequests
	if c.ParentExecutionRequests == nil {
		c.ParentExecutionRequests = new(v1.ExecutionRequestsGloas)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.ParentExecutionRequests.SizeSSZ()

	// Field 2: ProposerSlashings
	for _, o := range c.ProposerSlashings {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("ProposerSlashings: %w", err)
		}
	}

	// Field 3: AttesterSlashings
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

	// Field 4: Attestations
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

	// Field 5: VoluntaryExits
	for _, o := range c.VoluntaryExits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("VoluntaryExits: %w", err)
		}
	}

	// Field 7: BlsToExecutionChanges
	for _, o := range c.BlsToExecutionChanges {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BlsToExecutionChanges: %w", err)
		}
	}

	// Field 8: SignedExecutionPayloadBid
	if dst, err = c.SignedExecutionPayloadBid.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}

	// Field 9: PayloadAttestations
	for _, o := range c.PayloadAttestations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PayloadAttestations: %w", err)
		}
	}

	// Field 10: ParentExecutionRequests
	if dst, err = c.ParentExecutionRequests.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ParentExecutionRequests: %w", err)
	}
	return dst, err
}

func (c *BeaconBlockBodyHeze) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 320 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:96]    // c.RandaoReveal
	sszSlice1 := buf[96:128]  // c.Graffiti
	sszSlice6 := buf[144:304] // c.SyncAggregate

	sszVarOffset2 := ssz.ReadOffset(buf[128:132]) // c.ProposerSlashings
	if sszVarOffset2 != 320 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset2 > size {
		return ssz.ErrOffset
	}
	sszVarOffset3 := ssz.ReadOffset(buf[132:136]) // c.AttesterSlashings
	if sszVarOffset3 > size || sszVarOffset3 < sszVarOffset2 {
		return ssz.ErrOffset
	}
	sszVarOffset4 := ssz.ReadOffset(buf[136:140]) // c.Attestations
	if sszVarOffset4 > size || sszVarOffset4 < sszVarOffset3 {
		return ssz.ErrOffset
	}
	sszVarOffset5 := ssz.ReadOffset(buf[140:144]) // c.VoluntaryExits
	if sszVarOffset5 > size || sszVarOffset5 < sszVarOffset4 {
		return ssz.ErrOffset
	}
	sszVarOffset7 := ssz.ReadOffset(buf[304:308]) // c.BlsToExecutionChanges
	if sszVarOffset7 > size || sszVarOffset7 < sszVarOffset5 {
		return ssz.ErrOffset
	}
	sszVarOffset8 := ssz.ReadOffset(buf[308:312]) // c.SignedExecutionPayloadBid
	if sszVarOffset8 > size || sszVarOffset8 < sszVarOffset7 {
		return ssz.ErrOffset
	}
	sszVarOffset9 := ssz.ReadOffset(buf[312:316]) // c.PayloadAttestations
	if sszVarOffset9 > size || sszVarOffset9 < sszVarOffset8 {
		return ssz.ErrOffset
	}
	sszVarOffset10 := ssz.ReadOffset(buf[316:320]) // c.ParentExecutionRequests
	if sszVarOffset10 > size || sszVarOffset10 < sszVarOffset9 {
		return ssz.ErrOffset
	}
	sszSlice2 := buf[sszVarOffset2:sszVarOffset3]  // c.ProposerSlashings
	sszSlice3 := buf[sszVarOffset3:sszVarOffset4]  // c.AttesterSlashings
	sszSlice4 := buf[sszVarOffset4:sszVarOffset5]  // c.Attestations
	sszSlice5 := buf[sszVarOffset5:sszVarOffset7]  // c.VoluntaryExits
	sszSlice7 := buf[sszVarOffset7:sszVarOffset8]  // c.BlsToExecutionChanges
	sszSlice8 := buf[sszVarOffset8:sszVarOffset9]  // c.SignedExecutionPayloadBid
	sszSlice9 := buf[sszVarOffset9:sszVarOffset10] // c.PayloadAttestations
	sszSlice10 := buf[sszVarOffset10:]             // c.ParentExecutionRequests

	// Field 0: RandaoReveal
	c.RandaoReveal = make([]byte, 0, 96)
	c.RandaoReveal = append(c.RandaoReveal, sszSlice0...)

	// Field 1: Graffiti
	c.Graffiti = make([]byte, 0, 32)
	c.Graffiti = append(c.Graffiti, sszSlice1...)

	// Field 2: ProposerSlashings
	{
		if len(sszSlice2)%416 != 0 {
			return fmt.Errorf("misaligned bytes: c.ProposerSlashings length is %d, which is not a multiple of 416: %w", len(sszSlice2), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice2) / 416
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.ProposerSlashings has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.ProposerSlashings = make([]*ProposerSlashing, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ProposerSlashing
			tmp = new(ProposerSlashing)
			tmpSlice := sszSlice2[i*416 : (1+i)*416]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("ProposerSlashings: %w", err)
			}
			c.ProposerSlashings[i] = tmp
		}
	}

	// Field 3: AttesterSlashings
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice3) > 3 {
			startOffset := ssz.ReadOffset(sszSlice3[0:4])
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
			totalVarBytes := uint64(len(sszSlice3))
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
					endOffset = ssz.ReadOffset(sszSlice3[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.AttesterSlashings", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.AttesterSlashings", endOffset, startOffset)
				}
				tmpSlice = sszSlice3[startOffset:endOffset]
				if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
					return fmt.Errorf("AttesterSlashings: %w", err)
				}
				c.AttesterSlashings[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice3) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.AttesterSlashings")
			}
			c.AttesterSlashings = make([]*AttesterSlashingGloas, 0)
		}
	}

	// Field 4: Attestations
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice4) > 3 {
			startOffset := ssz.ReadOffset(sszSlice4[0:4])
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
			totalVarBytes := uint64(len(sszSlice4))
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
					endOffset = ssz.ReadOffset(sszSlice4[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.Attestations", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.Attestations", endOffset, startOffset)
				}
				tmpSlice = sszSlice4[startOffset:endOffset]
				if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
					return fmt.Errorf("Attestations: %w", err)
				}
				c.Attestations[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice4) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Attestations")
			}
			c.Attestations = make([]*AttestationGloas, 0)
		}
	}

	// Field 5: VoluntaryExits
	{
		if len(sszSlice5)%112 != 0 {
			return fmt.Errorf("misaligned bytes: c.VoluntaryExits length is %d, which is not a multiple of 112: %w", len(sszSlice5), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice5) / 112
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.VoluntaryExits has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.VoluntaryExits = make([]*SignedVoluntaryExit, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *SignedVoluntaryExit
			tmp = new(SignedVoluntaryExit)
			tmpSlice := sszSlice5[i*112 : (1+i)*112]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("VoluntaryExits: %w", err)
			}
			c.VoluntaryExits[i] = tmp
		}
	}

	// Field 6: SyncAggregate
	c.SyncAggregate = new(SyncAggregate)
	if err = c.SyncAggregate.UnmarshalSSZ(sszSlice6); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 7: BlsToExecutionChanges
	{
		if len(sszSlice7)%172 != 0 {
			return fmt.Errorf("misaligned bytes: c.BlsToExecutionChanges length is %d, which is not a multiple of 172: %w", len(sszSlice7), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice7) / 172
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.BlsToExecutionChanges has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.BlsToExecutionChanges = make([]*SignedBLSToExecutionChange, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *SignedBLSToExecutionChange
			tmp = new(SignedBLSToExecutionChange)
			tmpSlice := sszSlice7[i*172 : (1+i)*172]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BlsToExecutionChanges: %w", err)
			}
			c.BlsToExecutionChanges[i] = tmp
		}
	}

	// Field 8: SignedExecutionPayloadBid
	c.SignedExecutionPayloadBid = new(SignedExecutionPayloadBidHeze)
	if err = c.SignedExecutionPayloadBid.UnmarshalSSZ(sszSlice8); err != nil {
		return fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}

	// Field 9: PayloadAttestations
	{
		if len(sszSlice9)%202 != 0 {
			return fmt.Errorf("misaligned bytes: c.PayloadAttestations length is %d, which is not a multiple of 202: %w", len(sszSlice9), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice9) / 202
		if numElem > 4 {
			return fmt.Errorf("ssz-max exceeded: c.PayloadAttestations has %d elements, ssz-max is 4: %w", numElem, ssz.ErrListTooBig)
		}
		c.PayloadAttestations = make([]*PayloadAttestation, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PayloadAttestation
			tmp = new(PayloadAttestation)
			tmpSlice := sszSlice9[i*202 : (1+i)*202]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PayloadAttestations: %w", err)
			}
			c.PayloadAttestations[i] = tmp
		}
	}

	// Field 10: ParentExecutionRequests
	c.ParentExecutionRequests = new(v1.ExecutionRequestsGloas)
	if err = c.ParentExecutionRequests.UnmarshalSSZ(sszSlice10); err != nil {
		return fmt.Errorf("ParentExecutionRequests: %w", err)
	}
	return err
}

func (c *BeaconBlockBodyHeze) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *BeaconBlockBodyHeze) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsBeaconBlockBodyHeze = []byte{0b10111101, 0b00011111}

func (c *BeaconBlockBodyHeze) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockBodyHeze) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Position 0: RandaoReveal
	if len(c.RandaoReveal) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.RandaoReveal)
	// Position 1: inactive (zero chunk)
	hh.PutBool(false)
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
	// Position 6: inactive (zero chunk)
	hh.PutBool(false)
	// Position 7: VoluntaryExits
	{
		subIndx := hh.Index()
		for _, o := range c.VoluntaryExits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("VoluntaryExits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.VoluntaryExits)))
	}
	// Position 8: SyncAggregate
	if err := c.SyncAggregate.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}
	// Position 9: BlsToExecutionChanges
	{
		subIndx := hh.Index()
		for _, o := range c.BlsToExecutionChanges {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BlsToExecutionChanges: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BlsToExecutionChanges)))
	}
	// Position 10: SignedExecutionPayloadBid
	if err := c.SignedExecutionPayloadBid.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}
	// Position 11: PayloadAttestations
	{
		subIndx := hh.Index()
		for _, o := range c.PayloadAttestations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PayloadAttestations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PayloadAttestations)))
	}
	// Position 12: ParentExecutionRequests
	if err := c.ParentExecutionRequests.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ParentExecutionRequests: %w", err)
	}
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsBeaconBlockBodyHeze)
	return nil
}

func (c *BeaconBlockContentsHeze) SizeSSZ() int {
	size := 16
	if c.Block == nil {
		c.Block = new(BeaconBlockHeze)
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

func (c *BeaconBlockContentsHeze) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockContentsHeze) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 16

	// Field 0: Block
	if c.Block == nil {
		c.Block = new(BeaconBlockHeze)
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

func (c *BeaconBlockContentsHeze) UnmarshalSSZ(buf []byte) error {
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
	c.Block = new(BeaconBlockHeze)
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

func (c *BeaconBlockContentsHeze) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockContentsHeze) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *BeaconBlockHeze) SizeSSZ() int {
	size := 84
	if c.Body == nil {
		c.Body = new(BeaconBlockBodyHeze)
	}
	size += c.Body.SizeSSZ()
	return size
}

func (c *BeaconBlockHeze) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockHeze) MarshalSSZTo(dst []byte) ([]byte, error) {
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
		c.Body = new(BeaconBlockBodyHeze)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Body.SizeSSZ()

	// Field 4: Body
	if dst, err = c.Body.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Body: %w", err)
	}
	return dst, err
}

func (c *BeaconBlockHeze) UnmarshalSSZ(buf []byte) error {
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
	c.Body = new(BeaconBlockBodyHeze)
	if err = c.Body.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("Body: %w", err)
	}
	return err
}

func (c *BeaconBlockHeze) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockHeze) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *BeaconStateHeze) SizeSSZ() int {
	size := 3134753
	size += len(c.HistoricalRoots) * 32
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
		c.LatestExecutionPayloadBid = new(ExecutionPayloadBidHeze)
	}
	size += c.LatestExecutionPayloadBid.SizeSSZ()
	size += len(c.PayloadExpectedWithdrawals) * 44
	return size
}

func (c *BeaconStateHeze) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconStateHeze) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 3134753

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

	// Field 8: Validators
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Validators) * 121

	// Field 9: Balances
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Balances) * 8

	// Field 10: RandaoMixes
	if len(c.RandaoMixes) != 65536 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.RandaoMixes {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 11: Slashings
	if len(c.Slashings) != 8192 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.Slashings {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 12: PreviousEpochParticipation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PreviousEpochParticipation)

	// Field 13: CurrentEpochParticipation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.CurrentEpochParticipation)

	// Field 14: JustificationBits
	if len([]byte(c.JustificationBits)) != 1 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.JustificationBits)...)

	// Field 15: PreviousJustifiedCheckpoint
	if c.PreviousJustifiedCheckpoint == nil {
		c.PreviousJustifiedCheckpoint = new(Checkpoint)
	}
	if dst, err = c.PreviousJustifiedCheckpoint.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("PreviousJustifiedCheckpoint: %w", err)
	}

	// Field 16: CurrentJustifiedCheckpoint
	if c.CurrentJustifiedCheckpoint == nil {
		c.CurrentJustifiedCheckpoint = new(Checkpoint)
	}
	if dst, err = c.CurrentJustifiedCheckpoint.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("CurrentJustifiedCheckpoint: %w", err)
	}

	// Field 17: FinalizedCheckpoint
	if c.FinalizedCheckpoint == nil {
		c.FinalizedCheckpoint = new(Checkpoint)
	}
	if dst, err = c.FinalizedCheckpoint.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("FinalizedCheckpoint: %w", err)
	}

	// Field 18: InactivityScores
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.InactivityScores) * 8

	// Field 19: CurrentSyncCommittee
	if c.CurrentSyncCommittee == nil {
		c.CurrentSyncCommittee = new(SyncCommittee)
	}
	if dst, err = c.CurrentSyncCommittee.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("CurrentSyncCommittee: %w", err)
	}

	// Field 20: NextSyncCommittee
	if c.NextSyncCommittee == nil {
		c.NextSyncCommittee = new(SyncCommittee)
	}
	if dst, err = c.NextSyncCommittee.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextSyncCommittee: %w", err)
	}

	// Field 21: LatestBlockHash
	if len(c.LatestBlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LatestBlockHash...)

	// Field 22: NextWithdrawalIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.NextWithdrawalIndex)

	// Field 23: NextWithdrawalValidatorIndex
	if dst, err = c.NextWithdrawalValidatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextWithdrawalValidatorIndex: %w", err)
	}

	// Field 24: HistoricalSummaries
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.HistoricalSummaries) * 64

	// Field 25: DepositBalanceToConsume
	if dst, err = c.DepositBalanceToConsume.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("DepositBalanceToConsume: %w", err)
	}

	// Field 26: ExitBalanceToConsume
	if dst, err = c.ExitBalanceToConsume.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ExitBalanceToConsume: %w", err)
	}

	// Field 27: EarliestExitEpoch
	if dst, err = c.EarliestExitEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("EarliestExitEpoch: %w", err)
	}

	// Field 28: ConsolidationBalanceToConsume
	if dst, err = c.ConsolidationBalanceToConsume.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ConsolidationBalanceToConsume: %w", err)
	}

	// Field 29: EarliestConsolidationEpoch
	if dst, err = c.EarliestConsolidationEpoch.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("EarliestConsolidationEpoch: %w", err)
	}

	// Field 30: PendingDeposits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingDeposits) * 192

	// Field 31: PendingPartialWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingPartialWithdrawals) * 24

	// Field 32: PendingConsolidations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingConsolidations) * 16

	// Field 33: ProposerLookahead
	if len(c.ProposerLookahead) != 64 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.ProposerLookahead {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("ProposerLookahead: %w", err)
		}
	}

	// Field 34: Builders
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Builders) * 93

	// Field 35: NextWithdrawalBuilderIndex
	if dst, err = c.NextWithdrawalBuilderIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextWithdrawalBuilderIndex: %w", err)
	}

	// Field 36: ExecutionPayloadAvailability
	if len(c.ExecutionPayloadAvailability) != 1024 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ExecutionPayloadAvailability...)

	// Field 37: BuilderPendingPayments
	if len(c.BuilderPendingPayments) != 64 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.BuilderPendingPayments {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderPendingPayments: %w", err)
		}
	}

	// Field 38: BuilderPendingWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BuilderPendingWithdrawals) * 36

	// Field 39: LatestExecutionPayloadBid
	if c.LatestExecutionPayloadBid == nil {
		c.LatestExecutionPayloadBid = new(ExecutionPayloadBidHeze)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.LatestExecutionPayloadBid.SizeSSZ()

	// Field 40: PayloadExpectedWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PayloadExpectedWithdrawals) * 44

	// Field 41: PtcWindow
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

	// Field 8: Validators
	for _, o := range c.Validators {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Validators: %w", err)
		}
	}

	// Field 9: Balances
	for _, o := range c.Balances {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 12: PreviousEpochParticipation
	dst = append(dst, c.PreviousEpochParticipation...)

	// Field 13: CurrentEpochParticipation
	dst = append(dst, c.CurrentEpochParticipation...)

	// Field 18: InactivityScores
	for _, o := range c.InactivityScores {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 24: HistoricalSummaries
	if len(c.HistoricalSummaries) > 16777216 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.HistoricalSummaries {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("HistoricalSummaries: %w", err)
		}
	}

	// Field 30: PendingDeposits
	for _, o := range c.PendingDeposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingDeposits: %w", err)
		}
	}

	// Field 31: PendingPartialWithdrawals
	for _, o := range c.PendingPartialWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingPartialWithdrawals: %w", err)
		}
	}

	// Field 32: PendingConsolidations
	for _, o := range c.PendingConsolidations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingConsolidations: %w", err)
		}
	}

	// Field 34: Builders
	for _, o := range c.Builders {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Builders: %w", err)
		}
	}

	// Field 38: BuilderPendingWithdrawals
	for _, o := range c.BuilderPendingWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderPendingWithdrawals: %w", err)
		}
	}

	// Field 39: LatestExecutionPayloadBid
	if dst, err = c.LatestExecutionPayloadBid.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}

	// Field 40: PayloadExpectedWithdrawals
	for _, o := range c.PayloadExpectedWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
		}
	}
	return dst, err
}

func (c *BeaconStateHeze) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 3134753 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]              // c.GenesisTime
	sszSlice1 := buf[8:40]             // c.GenesisValidatorsRoot
	sszSlice2 := buf[40:48]            // c.Slot
	sszSlice3 := buf[48:64]            // c.Fork
	sszSlice4 := buf[64:176]           // c.LatestBlockHeader
	sszSlice5 := buf[176:262320]       // c.BlockRoots
	sszSlice6 := buf[262320:524464]    // c.StateRoots
	sszSlice10 := buf[524476:2621628]  // c.RandaoMixes
	sszSlice11 := buf[2621628:2687164] // c.Slashings
	sszSlice14 := buf[2687172:2687173] // c.JustificationBits
	sszSlice15 := buf[2687173:2687213] // c.PreviousJustifiedCheckpoint
	sszSlice16 := buf[2687213:2687253] // c.CurrentJustifiedCheckpoint
	sszSlice17 := buf[2687253:2687293] // c.FinalizedCheckpoint
	sszSlice19 := buf[2687297:2711921] // c.CurrentSyncCommittee
	sszSlice20 := buf[2711921:2736545] // c.NextSyncCommittee
	sszSlice21 := buf[2736545:2736577] // c.LatestBlockHash
	sszSlice22 := buf[2736577:2736585] // c.NextWithdrawalIndex
	sszSlice23 := buf[2736585:2736593] // c.NextWithdrawalValidatorIndex
	sszSlice25 := buf[2736597:2736605] // c.DepositBalanceToConsume
	sszSlice26 := buf[2736605:2736613] // c.ExitBalanceToConsume
	sszSlice27 := buf[2736613:2736621] // c.EarliestExitEpoch
	sszSlice28 := buf[2736621:2736629] // c.ConsolidationBalanceToConsume
	sszSlice29 := buf[2736629:2736637] // c.EarliestConsolidationEpoch
	sszSlice33 := buf[2736649:2737161] // c.ProposerLookahead
	sszSlice35 := buf[2737165:2737173] // c.NextWithdrawalBuilderIndex
	sszSlice36 := buf[2737173:2738197] // c.ExecutionPayloadAvailability
	sszSlice37 := buf[2738197:2741525] // c.BuilderPendingPayments
	sszSlice41 := buf[2741537:3134753] // c.PtcWindow

	sszVarOffset7 := ssz.ReadOffset(buf[524464:524468]) // c.HistoricalRoots
	if sszVarOffset7 != 3134753 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset7 > size {
		return ssz.ErrOffset
	}
	sszVarOffset8 := ssz.ReadOffset(buf[524468:524472]) // c.Validators
	if sszVarOffset8 > size || sszVarOffset8 < sszVarOffset7 {
		return ssz.ErrOffset
	}
	sszVarOffset9 := ssz.ReadOffset(buf[524472:524476]) // c.Balances
	if sszVarOffset9 > size || sszVarOffset9 < sszVarOffset8 {
		return ssz.ErrOffset
	}
	sszVarOffset12 := ssz.ReadOffset(buf[2687164:2687168]) // c.PreviousEpochParticipation
	if sszVarOffset12 > size || sszVarOffset12 < sszVarOffset9 {
		return ssz.ErrOffset
	}
	sszVarOffset13 := ssz.ReadOffset(buf[2687168:2687172]) // c.CurrentEpochParticipation
	if sszVarOffset13 > size || sszVarOffset13 < sszVarOffset12 {
		return ssz.ErrOffset
	}
	sszVarOffset18 := ssz.ReadOffset(buf[2687293:2687297]) // c.InactivityScores
	if sszVarOffset18 > size || sszVarOffset18 < sszVarOffset13 {
		return ssz.ErrOffset
	}
	sszVarOffset24 := ssz.ReadOffset(buf[2736593:2736597]) // c.HistoricalSummaries
	if sszVarOffset24 > size || sszVarOffset24 < sszVarOffset18 {
		return ssz.ErrOffset
	}
	sszVarOffset30 := ssz.ReadOffset(buf[2736637:2736641]) // c.PendingDeposits
	if sszVarOffset30 > size || sszVarOffset30 < sszVarOffset24 {
		return ssz.ErrOffset
	}
	sszVarOffset31 := ssz.ReadOffset(buf[2736641:2736645]) // c.PendingPartialWithdrawals
	if sszVarOffset31 > size || sszVarOffset31 < sszVarOffset30 {
		return ssz.ErrOffset
	}
	sszVarOffset32 := ssz.ReadOffset(buf[2736645:2736649]) // c.PendingConsolidations
	if sszVarOffset32 > size || sszVarOffset32 < sszVarOffset31 {
		return ssz.ErrOffset
	}
	sszVarOffset34 := ssz.ReadOffset(buf[2737161:2737165]) // c.Builders
	if sszVarOffset34 > size || sszVarOffset34 < sszVarOffset32 {
		return ssz.ErrOffset
	}
	sszVarOffset38 := ssz.ReadOffset(buf[2741525:2741529]) // c.BuilderPendingWithdrawals
	if sszVarOffset38 > size || sszVarOffset38 < sszVarOffset34 {
		return ssz.ErrOffset
	}
	sszVarOffset39 := ssz.ReadOffset(buf[2741529:2741533]) // c.LatestExecutionPayloadBid
	if sszVarOffset39 > size || sszVarOffset39 < sszVarOffset38 {
		return ssz.ErrOffset
	}
	sszVarOffset40 := ssz.ReadOffset(buf[2741533:2741537]) // c.PayloadExpectedWithdrawals
	if sszVarOffset40 > size || sszVarOffset40 < sszVarOffset39 {
		return ssz.ErrOffset
	}
	sszSlice7 := buf[sszVarOffset7:sszVarOffset8]    // c.HistoricalRoots
	sszSlice8 := buf[sszVarOffset8:sszVarOffset9]    // c.Validators
	sszSlice9 := buf[sszVarOffset9:sszVarOffset12]   // c.Balances
	sszSlice12 := buf[sszVarOffset12:sszVarOffset13] // c.PreviousEpochParticipation
	sszSlice13 := buf[sszVarOffset13:sszVarOffset18] // c.CurrentEpochParticipation
	sszSlice18 := buf[sszVarOffset18:sszVarOffset24] // c.InactivityScores
	sszSlice24 := buf[sszVarOffset24:sszVarOffset30] // c.HistoricalSummaries
	sszSlice30 := buf[sszVarOffset30:sszVarOffset31] // c.PendingDeposits
	sszSlice31 := buf[sszVarOffset31:sszVarOffset32] // c.PendingPartialWithdrawals
	sszSlice32 := buf[sszVarOffset32:sszVarOffset34] // c.PendingConsolidations
	sszSlice34 := buf[sszVarOffset34:sszVarOffset38] // c.Builders
	sszSlice38 := buf[sszVarOffset38:sszVarOffset39] // c.BuilderPendingWithdrawals
	sszSlice39 := buf[sszVarOffset39:sszVarOffset40] // c.LatestExecutionPayloadBid
	sszSlice40 := buf[sszVarOffset40:]               // c.PayloadExpectedWithdrawals

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

	// Field 8: Validators
	{
		if len(sszSlice8)%121 != 0 {
			return fmt.Errorf("misaligned bytes: c.Validators length is %d, which is not a multiple of 121: %w", len(sszSlice8), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice8) / 121
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.Validators has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
		c.Validators = make([]*Validator, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Validator
			tmp = new(Validator)
			tmpSlice := sszSlice8[i*121 : (1+i)*121]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Validators: %w", err)
			}
			c.Validators[i] = tmp
		}
	}

	// Field 9: Balances
	{
		if len(sszSlice9)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.Balances length is %d, which is not a multiple of 8: %w", len(sszSlice9), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice9) / 8
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.Balances has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
		c.Balances = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice9[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.Balances[i] = tmp
		}
	}

	// Field 10: RandaoMixes
	{
		c.RandaoMixes = make([][]byte, 65536)
		for i := 0; i < 65536; i++ {
			var tmp []byte

			tmpSlice := sszSlice10[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.RandaoMixes[i] = tmp
		}
	}

	// Field 11: Slashings
	{
		c.Slashings = make([]uint64, 8192)
		for i := 0; i < 8192; i++ {
			var tmp uint64

			tmpSlice := sszSlice11[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.Slashings[i] = tmp
		}
	}

	// Field 12: PreviousEpochParticipation
	c.PreviousEpochParticipation = append([]byte{}, sszSlice12...)

	// Field 13: CurrentEpochParticipation
	c.CurrentEpochParticipation = append([]byte{}, sszSlice13...)

	// Field 14: JustificationBits
	c.JustificationBits = make([]byte, 0, 1)
	c.JustificationBits = append(c.JustificationBits, go_bitfield.Bitvector4(sszSlice14)...)

	// Field 15: PreviousJustifiedCheckpoint
	c.PreviousJustifiedCheckpoint = new(Checkpoint)
	if err = c.PreviousJustifiedCheckpoint.UnmarshalSSZ(sszSlice15); err != nil {
		return fmt.Errorf("PreviousJustifiedCheckpoint: %w", err)
	}

	// Field 16: CurrentJustifiedCheckpoint
	c.CurrentJustifiedCheckpoint = new(Checkpoint)
	if err = c.CurrentJustifiedCheckpoint.UnmarshalSSZ(sszSlice16); err != nil {
		return fmt.Errorf("CurrentJustifiedCheckpoint: %w", err)
	}

	// Field 17: FinalizedCheckpoint
	c.FinalizedCheckpoint = new(Checkpoint)
	if err = c.FinalizedCheckpoint.UnmarshalSSZ(sszSlice17); err != nil {
		return fmt.Errorf("FinalizedCheckpoint: %w", err)
	}

	// Field 18: InactivityScores
	{
		if len(sszSlice18)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.InactivityScores length is %d, which is not a multiple of 8: %w", len(sszSlice18), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice18) / 8
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.InactivityScores has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
		c.InactivityScores = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice18[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.InactivityScores[i] = tmp
		}
	}

	// Field 19: CurrentSyncCommittee
	c.CurrentSyncCommittee = new(SyncCommittee)
	if err = c.CurrentSyncCommittee.UnmarshalSSZ(sszSlice19); err != nil {
		return fmt.Errorf("CurrentSyncCommittee: %w", err)
	}

	// Field 20: NextSyncCommittee
	c.NextSyncCommittee = new(SyncCommittee)
	if err = c.NextSyncCommittee.UnmarshalSSZ(sszSlice20); err != nil {
		return fmt.Errorf("NextSyncCommittee: %w", err)
	}

	// Field 21: LatestBlockHash
	c.LatestBlockHash = make([]byte, 0, 32)
	c.LatestBlockHash = append(c.LatestBlockHash, sszSlice21...)

	// Field 22: NextWithdrawalIndex
	c.NextWithdrawalIndex = binary.LittleEndian.Uint64(sszSlice22)

	// Field 23: NextWithdrawalValidatorIndex
	if err = c.NextWithdrawalValidatorIndex.UnmarshalSSZ(sszSlice23); err != nil {
		return fmt.Errorf("NextWithdrawalValidatorIndex: %w", err)
	}

	// Field 24: HistoricalSummaries
	{
		if len(sszSlice24)%64 != 0 {
			return fmt.Errorf("misaligned bytes: c.HistoricalSummaries length is %d, which is not a multiple of 64: %w", len(sszSlice24), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice24) / 64
		if numElem > 16777216 {
			return fmt.Errorf("ssz-max exceeded: c.HistoricalSummaries has %d elements, ssz-max is 16777216: %w", numElem, ssz.ErrListTooBig)
		}
		c.HistoricalSummaries = make([]*HistoricalSummary, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *HistoricalSummary
			tmp = new(HistoricalSummary)
			tmpSlice := sszSlice24[i*64 : (1+i)*64]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("HistoricalSummaries: %w", err)
			}
			c.HistoricalSummaries[i] = tmp
		}
	}

	// Field 25: DepositBalanceToConsume
	if err = c.DepositBalanceToConsume.UnmarshalSSZ(sszSlice25); err != nil {
		return fmt.Errorf("DepositBalanceToConsume: %w", err)
	}

	// Field 26: ExitBalanceToConsume
	if err = c.ExitBalanceToConsume.UnmarshalSSZ(sszSlice26); err != nil {
		return fmt.Errorf("ExitBalanceToConsume: %w", err)
	}

	// Field 27: EarliestExitEpoch
	if err = c.EarliestExitEpoch.UnmarshalSSZ(sszSlice27); err != nil {
		return fmt.Errorf("EarliestExitEpoch: %w", err)
	}

	// Field 28: ConsolidationBalanceToConsume
	if err = c.ConsolidationBalanceToConsume.UnmarshalSSZ(sszSlice28); err != nil {
		return fmt.Errorf("ConsolidationBalanceToConsume: %w", err)
	}

	// Field 29: EarliestConsolidationEpoch
	if err = c.EarliestConsolidationEpoch.UnmarshalSSZ(sszSlice29); err != nil {
		return fmt.Errorf("EarliestConsolidationEpoch: %w", err)
	}

	// Field 30: PendingDeposits
	{
		if len(sszSlice30)%192 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingDeposits length is %d, which is not a multiple of 192: %w", len(sszSlice30), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice30) / 192
		if numElem > 134217728 {
			return fmt.Errorf("ssz-max exceeded: c.PendingDeposits has %d elements, ssz-max is 134217728: %w", numElem, ssz.ErrListTooBig)
		}
		c.PendingDeposits = make([]*PendingDeposit, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PendingDeposit
			tmp = new(PendingDeposit)
			tmpSlice := sszSlice30[i*192 : (1+i)*192]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingDeposits: %w", err)
			}
			c.PendingDeposits[i] = tmp
		}
	}

	// Field 31: PendingPartialWithdrawals
	{
		if len(sszSlice31)%24 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingPartialWithdrawals length is %d, which is not a multiple of 24: %w", len(sszSlice31), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice31) / 24
		if numElem > 134217728 {
			return fmt.Errorf("ssz-max exceeded: c.PendingPartialWithdrawals has %d elements, ssz-max is 134217728: %w", numElem, ssz.ErrListTooBig)
		}
		c.PendingPartialWithdrawals = make([]*PendingPartialWithdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PendingPartialWithdrawal
			tmp = new(PendingPartialWithdrawal)
			tmpSlice := sszSlice31[i*24 : (1+i)*24]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingPartialWithdrawals: %w", err)
			}
			c.PendingPartialWithdrawals[i] = tmp
		}
	}

	// Field 32: PendingConsolidations
	{
		if len(sszSlice32)%16 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingConsolidations length is %d, which is not a multiple of 16: %w", len(sszSlice32), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice32) / 16
		if numElem > 262144 {
			return fmt.Errorf("ssz-max exceeded: c.PendingConsolidations has %d elements, ssz-max is 262144: %w", numElem, ssz.ErrListTooBig)
		}
		c.PendingConsolidations = make([]*PendingConsolidation, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *PendingConsolidation
			tmp = new(PendingConsolidation)
			tmpSlice := sszSlice32[i*16 : (1+i)*16]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingConsolidations: %w", err)
			}
			c.PendingConsolidations[i] = tmp
		}
	}

	// Field 33: ProposerLookahead
	{
		c.ProposerLookahead = make([]primitives.ValidatorIndex, 64)
		for i := 0; i < 64; i++ {
			var tmp primitives.ValidatorIndex

			tmpSlice := sszSlice33[i*8 : (1+i)*8]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("ProposerLookahead: %w", err)
			}
			c.ProposerLookahead[i] = tmp
		}
	}

	// Field 34: Builders
	{
		if len(sszSlice34)%93 != 0 {
			return fmt.Errorf("misaligned bytes: c.Builders length is %d, which is not a multiple of 93: %w", len(sszSlice34), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice34) / 93
		if numElem > 1099511627776 {
			return fmt.Errorf("ssz-max exceeded: c.Builders has %d elements, ssz-max is 1099511627776: %w", numElem, ssz.ErrListTooBig)
		}
		c.Builders = make([]*Builder, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Builder
			tmp = new(Builder)
			tmpSlice := sszSlice34[i*93 : (1+i)*93]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Builders: %w", err)
			}
			c.Builders[i] = tmp
		}
	}

	// Field 35: NextWithdrawalBuilderIndex
	if err = c.NextWithdrawalBuilderIndex.UnmarshalSSZ(sszSlice35); err != nil {
		return fmt.Errorf("NextWithdrawalBuilderIndex: %w", err)
	}

	// Field 36: ExecutionPayloadAvailability
	c.ExecutionPayloadAvailability = make([]byte, 0, 1024)
	c.ExecutionPayloadAvailability = append(c.ExecutionPayloadAvailability, sszSlice36...)

	// Field 37: BuilderPendingPayments
	{
		c.BuilderPendingPayments = make([]*BuilderPendingPayment, 64)
		for i := 0; i < 64; i++ {
			var tmp *BuilderPendingPayment
			tmp = new(BuilderPendingPayment)
			tmpSlice := sszSlice37[i*52 : (1+i)*52]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderPendingPayments: %w", err)
			}
			c.BuilderPendingPayments[i] = tmp
		}
	}

	// Field 38: BuilderPendingWithdrawals
	{
		if len(sszSlice38)%36 != 0 {
			return fmt.Errorf("misaligned bytes: c.BuilderPendingWithdrawals length is %d, which is not a multiple of 36: %w", len(sszSlice38), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice38) / 36
		if numElem > 1048576 {
			return fmt.Errorf("ssz-max exceeded: c.BuilderPendingWithdrawals has %d elements, ssz-max is 1048576: %w", numElem, ssz.ErrListTooBig)
		}
		c.BuilderPendingWithdrawals = make([]*BuilderPendingWithdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *BuilderPendingWithdrawal
			tmp = new(BuilderPendingWithdrawal)
			tmpSlice := sszSlice38[i*36 : (1+i)*36]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderPendingWithdrawals: %w", err)
			}
			c.BuilderPendingWithdrawals[i] = tmp
		}
	}

	// Field 39: LatestExecutionPayloadBid
	c.LatestExecutionPayloadBid = new(ExecutionPayloadBidHeze)
	if err = c.LatestExecutionPayloadBid.UnmarshalSSZ(sszSlice39); err != nil {
		return fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}

	// Field 40: PayloadExpectedWithdrawals
	{
		if len(sszSlice40)%44 != 0 {
			return fmt.Errorf("misaligned bytes: c.PayloadExpectedWithdrawals length is %d, which is not a multiple of 44: %w", len(sszSlice40), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice40) / 44
		if numElem > 16 {
			return fmt.Errorf("ssz-max exceeded: c.PayloadExpectedWithdrawals has %d elements, ssz-max is 16: %w", numElem, ssz.ErrListTooBig)
		}
		c.PayloadExpectedWithdrawals = make([]*v1.Withdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *v1.Withdrawal
			tmp = new(v1.Withdrawal)
			tmpSlice := sszSlice40[i*44 : (1+i)*44]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
			}
			c.PayloadExpectedWithdrawals[i] = tmp
		}
	}

	// Field 41: PtcWindow
	{
		c.PtcWindow = make([]*PTCs, 96)
		for i := 0; i < 96; i++ {
			var tmp *PTCs
			tmp = new(PTCs)
			tmpSlice := sszSlice41[i*4096 : (1+i)*4096]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PtcWindow: %w", err)
			}
			c.PtcWindow[i] = tmp
		}
	}
	return err
}

func (c *BeaconStateHeze) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *BeaconStateHeze) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsBeaconStateHeze = []byte{0b11111111, 0b11111000, 0b11111111, 0b11101111, 0b11111111, 0b00111111}

func (c *BeaconStateHeze) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconStateHeze) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
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
	// Position 8: inactive (zero chunk)
	hh.PutBool(false)
	// Position 9: inactive (zero chunk)
	hh.PutBool(false)
	// Position 10: inactive (zero chunk)
	hh.PutBool(false)
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
	// Position 22: CurrentSyncCommittee
	if err := c.CurrentSyncCommittee.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("CurrentSyncCommittee: %w", err)
	}
	// Position 23: NextSyncCommittee
	if err := c.NextSyncCommittee.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextSyncCommittee: %w", err)
	}
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
	// Position 28: inactive (zero chunk)
	hh.PutBool(false)
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
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsBeaconStateHeze)
	return nil
}

func (c *ExecutionPayloadBidHeze) SizeSSZ() int {
	size := 226
	size += len(c.BlobKzgCommitments) * 48
	return size
}

func (c *ExecutionPayloadBidHeze) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ExecutionPayloadBidHeze) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 226

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

	// Field 12: InclusionListBits
	if len([]byte(c.InclusionListBits)) != 2 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.InclusionListBits)...)

	// Field 10: BlobKzgCommitments
	for _, o := range c.BlobKzgCommitments {
		if len(o) != 48 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}
	return dst, err
}

func (c *ExecutionPayloadBidHeze) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 226 {
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
	sszSlice12 := buf[224:226] // c.InclusionListBits

	sszVarOffset10 := ssz.ReadOffset(buf[188:192]) // c.BlobKzgCommitments
	if sszVarOffset10 != 226 {
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

	// Field 12: InclusionListBits
	c.InclusionListBits = make([]byte, 0, 2)
	c.InclusionListBits = append(c.InclusionListBits, go_bitfield.Bitvector16(sszSlice12)...)
	return err
}

func (c *ExecutionPayloadBidHeze) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *ExecutionPayloadBidHeze) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsExecutionPayloadBidHeze = []byte{0b11111111, 0b00011111}

func (c *ExecutionPayloadBidHeze) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ExecutionPayloadBidHeze) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Position 0: ParentBlockHash
	if len(c.ParentBlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentBlockHash)
	// Position 1: ParentBlockRoot
	if len(c.ParentBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentBlockRoot)
	// Position 2: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash)
	// Position 3: PrevRandao
	if len(c.PrevRandao) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PrevRandao)
	// Position 4: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient)
	// Position 5: GasLimit
	hh.PutUint64(c.GasLimit)
	// Position 6: BuilderIndex
	if err := c.BuilderIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("BuilderIndex: %w", err)
	}
	// Position 7: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Position 8: Value
	if err := c.Value.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Value: %w", err)
	}
	// Position 9: ExecutionPayment
	if err := c.ExecutionPayment.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExecutionPayment: %w", err)
	}
	// Position 10: BlobKzgCommitments
	{
		subIndx := hh.Index()
		for _, o := range c.BlobKzgCommitments {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BlobKzgCommitments)))
	}
	// Position 11: ExecutionRequestsRoot
	if len(c.ExecutionRequestsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ExecutionRequestsRoot)
	// Position 12: InclusionListBits
	if len([]byte(c.InclusionListBits)) != 2 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.InclusionListBits))
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsExecutionPayloadBidHeze)
	return nil
}

func (c *InclusionList) SizeSSZ() int {
	size := 52
	if c.Transactions == nil {
		c.Transactions = new(v1.ProgressiveTransactionList)
	}
	size += c.Transactions.SizeSSZ()
	return size
}

func (c *InclusionList) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *InclusionList) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 52

	// Field 0: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 1: ValidatorIndex
	if dst, err = c.ValidatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ValidatorIndex: %w", err)
	}

	// Field 2: DependentRoot
	if len(c.DependentRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.DependentRoot...)

	// Field 3: Transactions
	if c.Transactions == nil {
		c.Transactions = new(v1.ProgressiveTransactionList)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Transactions.SizeSSZ()

	// Field 3: Transactions
	if dst, err = c.Transactions.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Transactions: %w", err)
	}
	return dst, err
}

func (c *InclusionList) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 52 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.Slot
	sszSlice1 := buf[8:16]  // c.ValidatorIndex
	sszSlice2 := buf[16:48] // c.DependentRoot

	sszVarOffset3 := ssz.ReadOffset(buf[48:52]) // c.Transactions
	if sszVarOffset3 != 52 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset3 > size {
		return ssz.ErrOffset
	}
	sszSlice3 := buf[sszVarOffset3:] // c.Transactions

	// Field 0: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 1: ValidatorIndex
	if err = c.ValidatorIndex.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("ValidatorIndex: %w", err)
	}

	// Field 2: DependentRoot
	c.DependentRoot = make([]byte, 0, 32)
	c.DependentRoot = append(c.DependentRoot, sszSlice2...)

	// Field 3: Transactions
	c.Transactions = new(v1.ProgressiveTransactionList)
	if err = c.Transactions.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("Transactions: %w", err)
	}
	return err
}

func (c *InclusionList) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *InclusionList) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 1: ValidatorIndex
	if err := c.ValidatorIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ValidatorIndex: %w", err)
	}
	// Field 2: DependentRoot
	if len(c.DependentRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.DependentRoot)
	// Field 3: Transactions
	if err := c.Transactions.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Transactions: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *SignedBeaconBlockHeze) SizeSSZ() int {
	size := 100
	if c.Block == nil {
		c.Block = new(BeaconBlockHeze)
	}
	size += c.Block.SizeSSZ()
	return size
}

func (c *SignedBeaconBlockHeze) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedBeaconBlockHeze) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Block
	if c.Block == nil {
		c.Block = new(BeaconBlockHeze)
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

func (c *SignedBeaconBlockHeze) UnmarshalSSZ(buf []byte) error {
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
	c.Block = new(BeaconBlockHeze)
	if err = c.Block.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Block: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedBeaconBlockHeze) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedBeaconBlockHeze) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *SignedExecutionPayloadBidHeze) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(ExecutionPayloadBidHeze)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedExecutionPayloadBidHeze) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedExecutionPayloadBidHeze) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(ExecutionPayloadBidHeze)
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

func (c *SignedExecutionPayloadBidHeze) UnmarshalSSZ(buf []byte) error {
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
	c.Message = new(ExecutionPayloadBidHeze)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedExecutionPayloadBidHeze) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedExecutionPayloadBidHeze) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *SignedInclusionList) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(InclusionList)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *SignedInclusionList) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedInclusionList) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(InclusionList)
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

func (c *SignedInclusionList) UnmarshalSSZ(buf []byte) error {
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
	c.Message = new(InclusionList)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedInclusionList) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedInclusionList) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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
