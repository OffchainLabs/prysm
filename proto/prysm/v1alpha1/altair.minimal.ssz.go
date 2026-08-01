//go:build minimal

package eth

import (
	binary "encoding/binary"
	"fmt"
	go_bitfield "github.com/OffchainLabs/go-bitfield"
	ssz "github.com/OffchainLabs/methodical-ssz/ssz"
)

func (c *BeaconBlockAltair) SizeSSZ() int {
	size := 84
	if c.Body == nil {
		c.Body = new(BeaconBlockBodyAltair)
	}
	size += c.Body.SizeSSZ()
	return size
}

func (c *BeaconBlockAltair) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
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
		c.Body = new(BeaconBlockBodyAltair)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Body.SizeSSZ()

	// Field 4: Body
	if dst, err = c.Body.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Body: %w", err)
	}
	return dst, err
}

func (c *BeaconBlockAltair) UnmarshalSSZ(buf []byte) error {
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
	c.Body = new(BeaconBlockBodyAltair)
	if err = c.Body.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("Body: %w", err)
	}
	return err
}

func (c *BeaconBlockAltair) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockAltair) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *BeaconBlockBodyAltair) SizeSSZ() int {
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
	size += len(c.Deposits) * 1240
	size += len(c.VoluntaryExits) * 112
	return size
}

func (c *BeaconBlockBodyAltair) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockBodyAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 320

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
	if len(c.AttesterSlashings) > 2 {
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
	if len(c.Attestations) > 128 {
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
	return dst, err
}

func (c *BeaconBlockBodyAltair) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 320 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:96]    // c.RandaoReveal
	sszSlice1 := buf[96:168]  // c.Eth1Data
	sszSlice2 := buf[168:200] // c.Graffiti
	sszSlice8 := buf[220:320] // c.SyncAggregate

	sszVarOffset3 := ssz.ReadOffset(buf[200:204]) // c.ProposerSlashings
	if sszVarOffset3 != 320 {
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
	sszSlice3 := buf[sszVarOffset3:sszVarOffset4] // c.ProposerSlashings
	sszSlice4 := buf[sszVarOffset4:sszVarOffset5] // c.AttesterSlashings
	sszSlice5 := buf[sszVarOffset5:sszVarOffset6] // c.Attestations
	sszSlice6 := buf[sszVarOffset6:sszVarOffset7] // c.Deposits
	sszSlice7 := buf[sszVarOffset7:]              // c.VoluntaryExits

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
			if listLen > 2 {
				return fmt.Errorf("ssz-max exceeded: c.AttesterSlashings has %d elements, ssz-max is 2: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice4))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.AttesterSlashings")
			}
			c.AttesterSlashings = make([]*AttesterSlashing, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp *AttesterSlashing
				tmp = new(AttesterSlashing)
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
			c.AttesterSlashings = make([]*AttesterSlashing, 0)
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
			if listLen > 128 {
				return fmt.Errorf("ssz-max exceeded: c.Attestations has %d elements, ssz-max is 128: %w", listLen, ssz.ErrListTooBig)
			}
			totalVarBytes := uint64(len(sszSlice5))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Attestations")
			}
			c.Attestations = make([]*Attestation, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp *Attestation
				tmp = new(Attestation)
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
			c.Attestations = make([]*Attestation, 0)
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
	return err
}

func (c *BeaconBlockBodyAltair) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockBodyAltair) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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
		if len(c.AttesterSlashings) > 2 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.AttesterSlashings {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("AttesterSlashings: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.AttesterSlashings)), 2)
	}
	// Field 5: Attestations
	{
		if len(c.Attestations) > 128 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.Attestations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Attestations: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.Attestations)), 128)
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
	hh.Merkleize(indx)
	return nil
}

func (c *BeaconStateAltair) SizeSSZ() int {
	size := 10229
	size += len(c.HistoricalRoots) * 32
	size += len(c.Eth1DataVotes) * 72
	size += len(c.Validators) * 121
	size += len(c.Balances) * 8
	size += len(c.PreviousEpochParticipation)
	size += len(c.CurrentEpochParticipation)
	size += len(c.InactivityScores) * 8
	return size
}

func (c *BeaconStateAltair) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconStateAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 10229

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
	return dst, err
}

func (c *BeaconStateAltair) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 10229 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]         // c.GenesisTime
	sszSlice1 := buf[8:40]        // c.GenesisValidatorsRoot
	sszSlice2 := buf[40:48]       // c.Slot
	sszSlice3 := buf[48:64]       // c.Fork
	sszSlice4 := buf[64:176]      // c.LatestBlockHeader
	sszSlice5 := buf[176:2224]    // c.BlockRoots
	sszSlice6 := buf[2224:4272]   // c.StateRoots
	sszSlice8 := buf[4276:4348]   // c.Eth1Data
	sszSlice10 := buf[4352:4360]  // c.Eth1DepositIndex
	sszSlice13 := buf[4368:6416]  // c.RandaoMixes
	sszSlice14 := buf[6416:6928]  // c.Slashings
	sszSlice17 := buf[6936:6937]  // c.JustificationBits
	sszSlice18 := buf[6937:6977]  // c.PreviousJustifiedCheckpoint
	sszSlice19 := buf[6977:7017]  // c.CurrentJustifiedCheckpoint
	sszSlice20 := buf[7017:7057]  // c.FinalizedCheckpoint
	sszSlice22 := buf[7061:8645]  // c.CurrentSyncCommittee
	sszSlice23 := buf[8645:10229] // c.NextSyncCommittee

	sszVarOffset7 := ssz.ReadOffset(buf[4272:4276]) // c.HistoricalRoots
	if sszVarOffset7 != 10229 {
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
	sszSlice7 := buf[sszVarOffset7:sszVarOffset9]    // c.HistoricalRoots
	sszSlice9 := buf[sszVarOffset9:sszVarOffset11]   // c.Eth1DataVotes
	sszSlice11 := buf[sszVarOffset11:sszVarOffset12] // c.Validators
	sszSlice12 := buf[sszVarOffset12:sszVarOffset15] // c.Balances
	sszSlice15 := buf[sszVarOffset15:sszVarOffset16] // c.PreviousEpochParticipation
	sszSlice16 := buf[sszVarOffset16:sszVarOffset21] // c.CurrentEpochParticipation
	sszSlice21 := buf[sszVarOffset21:]               // c.InactivityScores

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
	return err
}

func (c *BeaconStateAltair) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconStateAltair) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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
	hh.Merkleize(indx)
	return nil
}

func (c *ContributionAndProof) SizeSSZ() int {
	size := 249

	return size
}

func (c *ContributionAndProof) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ContributionAndProof) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: AggregatorIndex
	if dst, err = c.AggregatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("AggregatorIndex: %w", err)
	}

	// Field 1: Contribution
	if c.Contribution == nil {
		c.Contribution = new(SyncCommitteeContribution)
	}
	if dst, err = c.Contribution.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Contribution: %w", err)
	}

	// Field 2: SelectionProof
	if len(c.SelectionProof) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SelectionProof...)

	return dst, err
}

func (c *ContributionAndProof) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 249 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]     // c.AggregatorIndex
	sszSlice1 := buf[8:153]   // c.Contribution
	sszSlice2 := buf[153:249] // c.SelectionProof

	// Field 0: AggregatorIndex
	if err = c.AggregatorIndex.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("AggregatorIndex: %w", err)
	}

	// Field 1: Contribution
	c.Contribution = new(SyncCommitteeContribution)
	if err = c.Contribution.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Contribution: %w", err)
	}

	// Field 2: SelectionProof
	c.SelectionProof = make([]byte, 0, 96)
	c.SelectionProof = append(c.SelectionProof, sszSlice2...)
	return err
}

func (c *ContributionAndProof) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ContributionAndProof) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AggregatorIndex
	if err := c.AggregatorIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("AggregatorIndex: %w", err)
	}
	// Field 1: Contribution
	if err := c.Contribution.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Contribution: %w", err)
	}
	// Field 2: SelectionProof
	if len(c.SelectionProof) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SelectionProof)
	hh.Merkleize(indx)
	return nil
}

func (c *LightClientHeaderAltair) SizeSSZ() int {
	size := 112

	return size
}

func (c *LightClientHeaderAltair) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *LightClientHeaderAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Beacon
	if c.Beacon == nil {
		c.Beacon = new(BeaconBlockHeader)
	}
	if dst, err = c.Beacon.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Beacon: %w", err)
	}

	return dst, err
}

func (c *LightClientHeaderAltair) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 112 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:112] // c.Beacon

	// Field 0: Beacon
	c.Beacon = new(BeaconBlockHeader)
	if err = c.Beacon.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Beacon: %w", err)
	}
	return err
}

func (c *LightClientHeaderAltair) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *LightClientHeaderAltair) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Beacon
	if err := c.Beacon.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Beacon: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *LightClientBootstrapAltair) SizeSSZ() int {
	size := 1856

	return size
}

func (c *LightClientBootstrapAltair) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *LightClientBootstrapAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Header
	if c.Header == nil {
		c.Header = new(LightClientHeaderAltair)
	}
	if dst, err = c.Header.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Header: %w", err)
	}

	// Field 1: CurrentSyncCommittee
	if c.CurrentSyncCommittee == nil {
		c.CurrentSyncCommittee = new(SyncCommittee)
	}
	if dst, err = c.CurrentSyncCommittee.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("CurrentSyncCommittee: %w", err)
	}

	// Field 2: CurrentSyncCommitteeBranch
	if len(c.CurrentSyncCommitteeBranch) != 5 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.CurrentSyncCommitteeBranch {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	return dst, err
}

func (c *LightClientBootstrapAltair) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 1856 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:112]     // c.Header
	sszSlice1 := buf[112:1696]  // c.CurrentSyncCommittee
	sszSlice2 := buf[1696:1856] // c.CurrentSyncCommitteeBranch

	// Field 0: Header
	c.Header = new(LightClientHeaderAltair)
	if err = c.Header.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Header: %w", err)
	}

	// Field 1: CurrentSyncCommittee
	c.CurrentSyncCommittee = new(SyncCommittee)
	if err = c.CurrentSyncCommittee.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("CurrentSyncCommittee: %w", err)
	}

	// Field 2: CurrentSyncCommitteeBranch
	{
		c.CurrentSyncCommitteeBranch = make([][]byte, 5)
		for i := 0; i < 5; i++ {
			var tmp []byte

			tmpSlice := sszSlice2[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.CurrentSyncCommitteeBranch[i] = tmp
		}
	}
	return err
}

func (c *LightClientBootstrapAltair) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *LightClientBootstrapAltair) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Header
	if err := c.Header.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Header: %w", err)
	}
	// Field 1: CurrentSyncCommittee
	if err := c.CurrentSyncCommittee.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("CurrentSyncCommittee: %w", err)
	}
	// Field 2: CurrentSyncCommitteeBranch
	{
		if len(c.CurrentSyncCommitteeBranch) != 5 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.CurrentSyncCommitteeBranch {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.Merkleize(subIndx)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *LightClientUpdateAltair) SizeSSZ() int {
	size := 2268

	return size
}

func (c *LightClientUpdateAltair) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *LightClientUpdateAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: AttestedHeader
	if c.AttestedHeader == nil {
		c.AttestedHeader = new(LightClientHeaderAltair)
	}
	if dst, err = c.AttestedHeader.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("AttestedHeader: %w", err)
	}

	// Field 1: NextSyncCommittee
	if c.NextSyncCommittee == nil {
		c.NextSyncCommittee = new(SyncCommittee)
	}
	if dst, err = c.NextSyncCommittee.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextSyncCommittee: %w", err)
	}

	// Field 2: NextSyncCommitteeBranch
	if len(c.NextSyncCommitteeBranch) != 5 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.NextSyncCommitteeBranch {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 3: FinalizedHeader
	if c.FinalizedHeader == nil {
		c.FinalizedHeader = new(LightClientHeaderAltair)
	}
	if dst, err = c.FinalizedHeader.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("FinalizedHeader: %w", err)
	}

	// Field 4: FinalityBranch
	if len(c.FinalityBranch) != 6 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.FinalityBranch {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 5: SyncAggregate
	if c.SyncAggregate == nil {
		c.SyncAggregate = new(SyncAggregate)
	}
	if dst, err = c.SyncAggregate.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 6: SignatureSlot
	if dst, err = c.SignatureSlot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SignatureSlot: %w", err)
	}

	return dst, err
}

func (c *LightClientUpdateAltair) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 2268 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:112]     // c.AttestedHeader
	sszSlice1 := buf[112:1696]  // c.NextSyncCommittee
	sszSlice2 := buf[1696:1856] // c.NextSyncCommitteeBranch
	sszSlice3 := buf[1856:1968] // c.FinalizedHeader
	sszSlice4 := buf[1968:2160] // c.FinalityBranch
	sszSlice5 := buf[2160:2260] // c.SyncAggregate
	sszSlice6 := buf[2260:2268] // c.SignatureSlot

	// Field 0: AttestedHeader
	c.AttestedHeader = new(LightClientHeaderAltair)
	if err = c.AttestedHeader.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("AttestedHeader: %w", err)
	}

	// Field 1: NextSyncCommittee
	c.NextSyncCommittee = new(SyncCommittee)
	if err = c.NextSyncCommittee.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("NextSyncCommittee: %w", err)
	}

	// Field 2: NextSyncCommitteeBranch
	{
		c.NextSyncCommitteeBranch = make([][]byte, 5)
		for i := 0; i < 5; i++ {
			var tmp []byte

			tmpSlice := sszSlice2[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.NextSyncCommitteeBranch[i] = tmp
		}
	}

	// Field 3: FinalizedHeader
	c.FinalizedHeader = new(LightClientHeaderAltair)
	if err = c.FinalizedHeader.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("FinalizedHeader: %w", err)
	}

	// Field 4: FinalityBranch
	{
		c.FinalityBranch = make([][]byte, 6)
		for i := 0; i < 6; i++ {
			var tmp []byte

			tmpSlice := sszSlice4[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.FinalityBranch[i] = tmp
		}
	}

	// Field 5: SyncAggregate
	c.SyncAggregate = new(SyncAggregate)
	if err = c.SyncAggregate.UnmarshalSSZ(sszSlice5); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 6: SignatureSlot
	if err = c.SignatureSlot.UnmarshalSSZ(sszSlice6); err != nil {
		return fmt.Errorf("SignatureSlot: %w", err)
	}
	return err
}

func (c *LightClientUpdateAltair) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *LightClientUpdateAltair) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AttestedHeader
	if err := c.AttestedHeader.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("AttestedHeader: %w", err)
	}
	// Field 1: NextSyncCommittee
	if err := c.NextSyncCommittee.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextSyncCommittee: %w", err)
	}
	// Field 2: NextSyncCommitteeBranch
	{
		if len(c.NextSyncCommitteeBranch) != 5 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.NextSyncCommitteeBranch {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 3: FinalizedHeader
	if err := c.FinalizedHeader.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("FinalizedHeader: %w", err)
	}
	// Field 4: FinalityBranch
	{
		if len(c.FinalityBranch) != 6 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.FinalityBranch {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 5: SyncAggregate
	if err := c.SyncAggregate.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}
	// Field 6: SignatureSlot
	if err := c.SignatureSlot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SignatureSlot: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *LightClientFinalityUpdateAltair) SizeSSZ() int {
	size := 524

	return size
}

func (c *LightClientFinalityUpdateAltair) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *LightClientFinalityUpdateAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: AttestedHeader
	if c.AttestedHeader == nil {
		c.AttestedHeader = new(LightClientHeaderAltair)
	}
	if dst, err = c.AttestedHeader.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("AttestedHeader: %w", err)
	}

	// Field 1: FinalizedHeader
	if c.FinalizedHeader == nil {
		c.FinalizedHeader = new(LightClientHeaderAltair)
	}
	if dst, err = c.FinalizedHeader.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("FinalizedHeader: %w", err)
	}

	// Field 2: FinalityBranch
	if len(c.FinalityBranch) != 6 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.FinalityBranch {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 3: SyncAggregate
	if c.SyncAggregate == nil {
		c.SyncAggregate = new(SyncAggregate)
	}
	if dst, err = c.SyncAggregate.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 4: SignatureSlot
	if dst, err = c.SignatureSlot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SignatureSlot: %w", err)
	}

	return dst, err
}

func (c *LightClientFinalityUpdateAltair) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 524 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:112]   // c.AttestedHeader
	sszSlice1 := buf[112:224] // c.FinalizedHeader
	sszSlice2 := buf[224:416] // c.FinalityBranch
	sszSlice3 := buf[416:516] // c.SyncAggregate
	sszSlice4 := buf[516:524] // c.SignatureSlot

	// Field 0: AttestedHeader
	c.AttestedHeader = new(LightClientHeaderAltair)
	if err = c.AttestedHeader.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("AttestedHeader: %w", err)
	}

	// Field 1: FinalizedHeader
	c.FinalizedHeader = new(LightClientHeaderAltair)
	if err = c.FinalizedHeader.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("FinalizedHeader: %w", err)
	}

	// Field 2: FinalityBranch
	{
		c.FinalityBranch = make([][]byte, 6)
		for i := 0; i < 6; i++ {
			var tmp []byte

			tmpSlice := sszSlice2[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.FinalityBranch[i] = tmp
		}
	}

	// Field 3: SyncAggregate
	c.SyncAggregate = new(SyncAggregate)
	if err = c.SyncAggregate.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 4: SignatureSlot
	if err = c.SignatureSlot.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("SignatureSlot: %w", err)
	}
	return err
}

func (c *LightClientFinalityUpdateAltair) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *LightClientFinalityUpdateAltair) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AttestedHeader
	if err := c.AttestedHeader.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("AttestedHeader: %w", err)
	}
	// Field 1: FinalizedHeader
	if err := c.FinalizedHeader.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("FinalizedHeader: %w", err)
	}
	// Field 2: FinalityBranch
	{
		if len(c.FinalityBranch) != 6 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.FinalityBranch {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 3: SyncAggregate
	if err := c.SyncAggregate.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}
	// Field 4: SignatureSlot
	if err := c.SignatureSlot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SignatureSlot: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *LightClientOptimisticUpdateAltair) SizeSSZ() int {
	size := 220

	return size
}

func (c *LightClientOptimisticUpdateAltair) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *LightClientOptimisticUpdateAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: AttestedHeader
	if c.AttestedHeader == nil {
		c.AttestedHeader = new(LightClientHeaderAltair)
	}
	if dst, err = c.AttestedHeader.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("AttestedHeader: %w", err)
	}

	// Field 1: SyncAggregate
	if c.SyncAggregate == nil {
		c.SyncAggregate = new(SyncAggregate)
	}
	if dst, err = c.SyncAggregate.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 2: SignatureSlot
	if dst, err = c.SignatureSlot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SignatureSlot: %w", err)
	}

	return dst, err
}

func (c *LightClientOptimisticUpdateAltair) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 220 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:112]   // c.AttestedHeader
	sszSlice1 := buf[112:212] // c.SyncAggregate
	sszSlice2 := buf[212:220] // c.SignatureSlot

	// Field 0: AttestedHeader
	c.AttestedHeader = new(LightClientHeaderAltair)
	if err = c.AttestedHeader.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("AttestedHeader: %w", err)
	}

	// Field 1: SyncAggregate
	c.SyncAggregate = new(SyncAggregate)
	if err = c.SyncAggregate.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 2: SignatureSlot
	if err = c.SignatureSlot.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("SignatureSlot: %w", err)
	}
	return err
}

func (c *LightClientOptimisticUpdateAltair) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *LightClientOptimisticUpdateAltair) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AttestedHeader
	if err := c.AttestedHeader.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("AttestedHeader: %w", err)
	}
	// Field 1: SyncAggregate
	if err := c.SyncAggregate.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}
	// Field 2: SignatureSlot
	if err := c.SignatureSlot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SignatureSlot: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *SignedBeaconBlockAltair) SizeSSZ() int {
	size := 100
	if c.Block == nil {
		c.Block = new(BeaconBlockAltair)
	}
	size += c.Block.SizeSSZ()
	return size
}

func (c *SignedBeaconBlockAltair) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedBeaconBlockAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Block
	if c.Block == nil {
		c.Block = new(BeaconBlockAltair)
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

func (c *SignedBeaconBlockAltair) UnmarshalSSZ(buf []byte) error {
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
	c.Block = new(BeaconBlockAltair)
	if err = c.Block.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Block: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedBeaconBlockAltair) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedBeaconBlockAltair) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *SignedContributionAndProof) SizeSSZ() int {
	size := 345

	return size
}

func (c *SignedContributionAndProof) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedContributionAndProof) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(ContributionAndProof)
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

func (c *SignedContributionAndProof) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 345 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:249]   // c.Message
	sszSlice1 := buf[249:345] // c.Signature

	// Field 0: Message
	c.Message = new(ContributionAndProof)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice1...)
	return err
}

func (c *SignedContributionAndProof) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedContributionAndProof) HashTreeRootWith(hh *ssz.Hasher) (err error) {
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

func (c *SyncAggregate) SizeSSZ() int {
	size := 100

	return size
}

func (c *SyncAggregate) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SyncAggregate) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SyncCommitteeBits
	if len([]byte(c.SyncCommitteeBits)) != 4 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.SyncCommitteeBits)...)

	// Field 1: SyncCommitteeSignature
	if len(c.SyncCommitteeSignature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SyncCommitteeSignature...)

	return dst, err
}

func (c *SyncAggregate) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 100 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:4]   // c.SyncCommitteeBits
	sszSlice1 := buf[4:100] // c.SyncCommitteeSignature

	// Field 0: SyncCommitteeBits
	c.SyncCommitteeBits = make([]byte, 0, 4)
	c.SyncCommitteeBits = append(c.SyncCommitteeBits, go_bitfield.Bitvector32(sszSlice0)...)

	// Field 1: SyncCommitteeSignature
	c.SyncCommitteeSignature = make([]byte, 0, 96)
	c.SyncCommitteeSignature = append(c.SyncCommitteeSignature, sszSlice1...)
	return err
}

func (c *SyncAggregate) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SyncAggregate) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SyncCommitteeBits
	if len([]byte(c.SyncCommitteeBits)) != 4 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.SyncCommitteeBits))
	// Field 1: SyncCommitteeSignature
	if len(c.SyncCommitteeSignature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SyncCommitteeSignature)
	hh.Merkleize(indx)
	return nil
}

func (c *SyncAggregatorSelectionData) SizeSSZ() int {
	size := 16

	return size
}

func (c *SyncAggregatorSelectionData) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SyncAggregatorSelectionData) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 1: SubcommitteeIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.SubcommitteeIndex)

	return dst, err
}

func (c *SyncAggregatorSelectionData) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 16 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.Slot
	sszSlice1 := buf[8:16] // c.SubcommitteeIndex

	// Field 0: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 1: SubcommitteeIndex
	c.SubcommitteeIndex = binary.LittleEndian.Uint64(sszSlice1)
	return err
}

func (c *SyncAggregatorSelectionData) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SyncAggregatorSelectionData) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 1: SubcommitteeIndex
	hh.PutUint64(c.SubcommitteeIndex)
	hh.Merkleize(indx)
	return nil
}

func (c *SyncCommittee) SizeSSZ() int {
	size := 1584

	return size
}

func (c *SyncCommittee) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SyncCommittee) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Pubkeys
	if len(c.Pubkeys) != 32 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.Pubkeys {
		if len(o) != 48 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 1: AggregatePubkey
	if len(c.AggregatePubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.AggregatePubkey...)

	return dst, err
}

func (c *SyncCommittee) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 1584 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:1536]    // c.Pubkeys
	sszSlice1 := buf[1536:1584] // c.AggregatePubkey

	// Field 0: Pubkeys
	{
		c.Pubkeys = make([][]byte, 32)
		for i := 0; i < 32; i++ {
			var tmp []byte

			tmpSlice := sszSlice0[i*48 : (1+i)*48]
			tmp = make([]byte, 0, 48)
			tmp = append(tmp, tmpSlice...)
			c.Pubkeys[i] = tmp
		}
	}

	// Field 1: AggregatePubkey
	c.AggregatePubkey = make([]byte, 0, 48)
	c.AggregatePubkey = append(c.AggregatePubkey, sszSlice1...)
	return err
}

func (c *SyncCommittee) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SyncCommittee) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Pubkeys
	{
		if len(c.Pubkeys) != 32 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.Pubkeys {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 1: AggregatePubkey
	if len(c.AggregatePubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.AggregatePubkey)
	hh.Merkleize(indx)
	return nil
}

func (c *SyncCommitteeContribution) SizeSSZ() int {
	size := 145

	return size
}

func (c *SyncCommitteeContribution) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SyncCommitteeContribution) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 1: BlockRoot
	if len(c.BlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockRoot...)

	// Field 2: SubcommitteeIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.SubcommitteeIndex)

	// Field 3: AggregationBits
	if len([]byte(c.AggregationBits)) != 1 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.AggregationBits)...)

	// Field 4: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	return dst, err
}

func (c *SyncCommitteeContribution) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 145 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]    // c.Slot
	sszSlice1 := buf[8:40]   // c.BlockRoot
	sszSlice2 := buf[40:48]  // c.SubcommitteeIndex
	sszSlice3 := buf[48:49]  // c.AggregationBits
	sszSlice4 := buf[49:145] // c.Signature

	// Field 0: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 1: BlockRoot
	c.BlockRoot = make([]byte, 0, 32)
	c.BlockRoot = append(c.BlockRoot, sszSlice1...)

	// Field 2: SubcommitteeIndex
	c.SubcommitteeIndex = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: AggregationBits
	c.AggregationBits = make([]byte, 0, 1)
	c.AggregationBits = append(c.AggregationBits, go_bitfield.Bitvector8(sszSlice3)...)

	// Field 4: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice4...)
	return err
}

func (c *SyncCommitteeContribution) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SyncCommitteeContribution) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 1: BlockRoot
	if len(c.BlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockRoot)
	// Field 2: SubcommitteeIndex
	hh.PutUint64(c.SubcommitteeIndex)
	// Field 3: AggregationBits
	if len([]byte(c.AggregationBits)) != 1 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.AggregationBits))
	// Field 4: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}

func (c *SyncCommitteeMessage) SizeSSZ() int {
	size := 144

	return size
}

func (c *SyncCommitteeMessage) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SyncCommitteeMessage) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Slot
	if dst, err = c.Slot.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Slot: %w", err)
	}

	// Field 1: BlockRoot
	if len(c.BlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockRoot...)

	// Field 2: ValidatorIndex
	if dst, err = c.ValidatorIndex.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ValidatorIndex: %w", err)
	}

	// Field 3: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature...)

	return dst, err
}

func (c *SyncCommitteeMessage) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 144 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]    // c.Slot
	sszSlice1 := buf[8:40]   // c.BlockRoot
	sszSlice2 := buf[40:48]  // c.ValidatorIndex
	sszSlice3 := buf[48:144] // c.Signature

	// Field 0: Slot
	if err = c.Slot.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}

	// Field 1: BlockRoot
	c.BlockRoot = make([]byte, 0, 32)
	c.BlockRoot = append(c.BlockRoot, sszSlice1...)

	// Field 2: ValidatorIndex
	if err = c.ValidatorIndex.UnmarshalSSZ(sszSlice2); err != nil {
		return fmt.Errorf("ValidatorIndex: %w", err)
	}

	// Field 3: Signature
	c.Signature = make([]byte, 0, 96)
	c.Signature = append(c.Signature, sszSlice3...)
	return err
}

func (c *SyncCommitteeMessage) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SyncCommitteeMessage) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Slot
	if err := c.Slot.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Slot: %w", err)
	}
	// Field 1: BlockRoot
	if len(c.BlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockRoot)
	// Field 2: ValidatorIndex
	if err := c.ValidatorIndex.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ValidatorIndex: %w", err)
	}
	// Field 3: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature)
	hh.Merkleize(indx)
	return nil
}
