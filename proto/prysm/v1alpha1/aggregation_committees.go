package eth

import (
	"encoding/binary"
	"slices"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

const (
	maxAggregationCommittees     = 64
	maxAggregatorsPerCommittee   = 2048
	aggregationCommitteeOffsetSz = 4
)

// Spec: AggregationCommittees. The methods are hand-written because the spec type is a bare list of lists, not a container.
type AggregationCommittees struct {
	Committees [][]primitives.ValidatorIndex `ssz-max:"64,2048"`
}

func (a *AggregationCommittees) Copy() *AggregationCommittees {
	if a == nil {
		return nil
	}
	committees := make([][]primitives.ValidatorIndex, len(a.Committees))
	for i, c := range a.Committees {
		committees[i] = slices.Clone(c)
	}
	return &AggregationCommittees{Committees: committees}
}

func CopyAggregationCommitteeWindow(window []*AggregationCommittees) []*AggregationCommittees {
	if window == nil {
		return nil
	}
	cp := make([]*AggregationCommittees, len(window))
	for i, a := range window {
		cp[i] = a.Copy()
	}
	return cp
}

func (a *AggregationCommittees) SizeSSZ() int {
	size := 0
	for _, c := range a.Committees {
		size += aggregationCommitteeOffsetSz + 8*len(c)
	}
	return size
}

func (a *AggregationCommittees) MarshalSSZ() ([]byte, error) {
	return a.MarshalSSZTo(make([]byte, 0, a.SizeSSZ()))
}

func (a *AggregationCommittees) MarshalSSZTo(dst []byte) ([]byte, error) {
	if len(a.Committees) > maxAggregationCommittees {
		return nil, ssz.ErrListTooBig
	}
	offset := aggregationCommitteeOffsetSz * len(a.Committees)
	for _, c := range a.Committees {
		if len(c) > maxAggregatorsPerCommittee {
			return nil, ssz.ErrListTooBig
		}
		dst = ssz.WriteOffset(dst, offset)
		offset += 8 * len(c)
	}
	for _, c := range a.Committees {
		for _, v := range c {
			dst = binary.LittleEndian.AppendUint64(dst, uint64(v))
		}
	}
	return dst, nil
}

func (a *AggregationCommittees) UnmarshalSSZ(buf []byte) error {
	size := uint64(len(buf))
	if size == 0 {
		a.Committees = [][]primitives.ValidatorIndex{}
		return nil
	}
	if size < aggregationCommitteeOffsetSz {
		return ssz.ErrSize
	}
	first := ssz.ReadOffset(buf)
	if first%aggregationCommitteeOffsetSz != 0 || first > size {
		return ssz.ErrInvalidVariableOffset
	}
	n := first / aggregationCommitteeOffsetSz
	if n > maxAggregationCommittees {
		return ssz.ErrListTooBig
	}
	committees := make([][]primitives.ValidatorIndex, n)
	start := first
	for i := range n {
		end := size
		if i+1 < n {
			end = ssz.ReadOffset(buf[(i+1)*aggregationCommitteeOffsetSz:])
		}
		if end < start || end > size || (end-start)%8 != 0 {
			return ssz.ErrOffset
		}
		count := (end - start) / 8
		if count > maxAggregatorsPerCommittee {
			return ssz.ErrListTooBig
		}
		committee := make([]primitives.ValidatorIndex, count)
		for j := range count {
			committee[j] = primitives.ValidatorIndex(binary.LittleEndian.Uint64(buf[start+8*j:]))
		}
		committees[i] = committee
		start = end
	}
	a.Committees = committees
	return nil
}

func (a *AggregationCommittees) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := a.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (a *AggregationCommittees) HashTreeRootWith(hh *ssz.Hasher) error {
	indx := hh.Index()
	for _, c := range a.Committees {
		subIndx := hh.Index()
		for _, v := range c {
			hh.AppendUint64(uint64(v))
		}
		hh.FillUpTo32()
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c)))
	}
	hh.MerkleizeProgressiveWithMixin(indx, uint64(len(a.Committees)))
	return nil
}
