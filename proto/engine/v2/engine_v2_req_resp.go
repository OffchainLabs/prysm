package enginev2

// This file hand-implements the SSZ wire codecs for the engine-API v2 top-level
// bodies/blobs request and response bodies.

import (
	ssz "github.com/prysmaticlabs/fastssz"
)

const (
	maxBodiesRequest = 32  // MAX_BODIES_REQUEST
	maxBlobsRequest  = 128 // MAX_BLOBS_REQUEST == MAX_VERSIONED_HASHES_PER_REQUEST
	hashLen          = 32  // Hash32 / VersionedHash
)

// blobV1EntrySize is the fixed serialized size of a BlobV1Entry, computed once
// (the entry is fixed-size: a bool plus a whole blob and a 48-byte proof).
var blobV1EntrySize = (&BlobV1Entry{}).SizeSSZ()

// BodiesByHashRequest is the bare SSZ List[Hash32, MAX_BODIES_REQUEST] body of
// POST /{fork}/bodies/hash. Fixed 32-byte elements, so it is a plain concatenation.
type BodiesByHashRequest [][]byte

func (r *BodiesByHashRequest) SizeSSZ() int { return len(*r) * hashLen }

func (r *BodiesByHashRequest) MarshalSSZ() ([]byte, error) {
	if len(*r) > maxBodiesRequest {
		return nil, ssz.ErrListTooBigFn("BodiesByHashRequest", len(*r), maxBodiesRequest)
	}
	dst := make([]byte, 0, len(*r)*hashLen)
	for _, h := range *r {
		if len(h) != hashLen {
			return nil, ssz.ErrBytesLengthFn("BodiesByHashRequest element", len(h), hashLen)
		}
		dst = append(dst, h...)
	}
	return dst, nil
}

func (r *BodiesByHashRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	b, err := r.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}

func (r *BodiesByHashRequest) UnmarshalSSZ(buf []byte) error {
	num, err := ssz.DivideInt2(len(buf), hashLen, maxBodiesRequest)
	if err != nil {
		return err
	}
	hashes := make([][]byte, num)
	for i := range num {
		hashes[i] = make([]byte, hashLen)
		copy(hashes[i], buf[i*hashLen:(i+1)*hashLen])
	}
	*r = hashes
	return nil
}

// BlobsRequest is the bare SSZ List[VersionedHash, MAX_BLOBS_REQUEST] body of
// POST /blobs/v{1,2,3}. Fixed 32-byte elements, so it is a plain concatenation.
type BlobsRequest [][]byte

func (r *BlobsRequest) SizeSSZ() int { return len(*r) * hashLen }

func (r *BlobsRequest) MarshalSSZ() ([]byte, error) {
	if len(*r) > maxBlobsRequest {
		return nil, ssz.ErrListTooBigFn("BlobsRequest", len(*r), maxBlobsRequest)
	}
	dst := make([]byte, 0, len(*r)*hashLen)
	for _, h := range *r {
		if len(h) != hashLen {
			return nil, ssz.ErrBytesLengthFn("BlobsRequest element", len(h), hashLen)
		}
		dst = append(dst, h...)
	}
	return dst, nil
}

func (r *BlobsRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	b, err := r.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}

func (r *BlobsRequest) UnmarshalSSZ(buf []byte) error {
	num, err := ssz.DivideInt2(len(buf), hashLen, maxBlobsRequest)
	if err != nil {
		return err
	}
	hashes := make([][]byte, num)
	for i := range num {
		hashes[i] = make([]byte, hashLen)
		copy(hashes[i], buf[i*hashLen:(i+1)*hashLen])
	}
	*r = hashes
	return nil
}

// BlobsV1Response is the bare SSZ List[BlobV1Entry, MAX_BLOBS_REQUEST] 200 body of
// POST /blobs/v1. BlobV1Entry is fixed-size, so it is a plain concatenation.
type BlobsV1Response []*BlobV1Entry

func (r *BlobsV1Response) SizeSSZ() int { return len(*r) * blobV1EntrySize }

func (r *BlobsV1Response) MarshalSSZ() ([]byte, error) {
	if len(*r) > maxBlobsRequest {
		return nil, ssz.ErrListTooBigFn("BlobsV1Response", len(*r), maxBlobsRequest)
	}
	dst := make([]byte, 0, r.SizeSSZ())
	var err error
	for _, e := range *r {
		if dst, err = e.MarshalSSZTo(dst); err != nil {
			return nil, err
		}
	}
	return dst, nil
}

func (r *BlobsV1Response) MarshalSSZTo(dst []byte) ([]byte, error) {
	b, err := r.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}

func (r *BlobsV1Response) UnmarshalSSZ(buf []byte) error {
	num, err := ssz.DivideInt2(len(buf), blobV1EntrySize, maxBlobsRequest)
	if err != nil {
		return err
	}
	entries := make([]*BlobV1Entry, num)
	for i := range num {
		entries[i] = &BlobV1Entry{}
		if err = entries[i].UnmarshalSSZ(buf[i*blobV1EntrySize : (i+1)*blobV1EntrySize]); err != nil {
			return err
		}
	}
	*r = entries
	return nil
}

// BodiesResponseFulu is the bare SSZ List[BodyEntryFulu, MAX_BODIES_REQUEST] body
// of /osaka/bodies. BodyEntryFulu is variable-size, so the elements are preceded
// by a table of 4-byte offsets (relative to the list start).
type BodiesResponseFulu []*BodyEntryFulu

func (r *BodiesResponseFulu) SizeSSZ() int {
	size := 4 * len(*r)
	for _, e := range *r {
		size += e.SizeSSZ()
	}
	return size
}

func (r *BodiesResponseFulu) MarshalSSZ() ([]byte, error) {
	if len(*r) > maxBodiesRequest {
		return nil, ssz.ErrListTooBigFn("BodiesResponseFulu", len(*r), maxBodiesRequest)
	}
	dst := make([]byte, 0, r.SizeSSZ())
	offset := 4 * len(*r)
	for _, e := range *r {
		dst = ssz.WriteOffset(dst, offset)
		offset += e.SizeSSZ()
	}
	var err error
	for _, e := range *r {
		if dst, err = e.MarshalSSZTo(dst); err != nil {
			return nil, err
		}
	}
	return dst, nil
}

func (r *BodiesResponseFulu) MarshalSSZTo(dst []byte) ([]byte, error) {
	b, err := r.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}

func (r *BodiesResponseFulu) UnmarshalSSZ(buf []byte) error {
	num, err := ssz.DecodeDynamicLength(buf, maxBodiesRequest)
	if err != nil {
		return err
	}
	entries := make([]*BodyEntryFulu, num)
	if err = ssz.UnmarshalDynamic(buf, num, func(i int, b []byte) error {
		entries[i] = &BodyEntryFulu{}
		return entries[i].UnmarshalSSZ(b)
	}); err != nil {
		return err
	}
	*r = entries
	return nil
}

// BodiesResponseGloas is the bare SSZ List[BodyEntryGloas, MAX_BODIES_REQUEST]
// body of /amsterdam/bodies. BodyEntryGloas is variable-size (offset table).
type BodiesResponseGloas []*BodyEntryGloas

func (r *BodiesResponseGloas) SizeSSZ() int {
	size := 4 * len(*r)
	for _, e := range *r {
		size += e.SizeSSZ()
	}
	return size
}

func (r *BodiesResponseGloas) MarshalSSZ() ([]byte, error) {
	if len(*r) > maxBodiesRequest {
		return nil, ssz.ErrListTooBigFn("BodiesResponseGloas", len(*r), maxBodiesRequest)
	}
	dst := make([]byte, 0, r.SizeSSZ())
	offset := 4 * len(*r)
	for _, e := range *r {
		dst = ssz.WriteOffset(dst, offset)
		offset += e.SizeSSZ()
	}
	var err error
	for _, e := range *r {
		if dst, err = e.MarshalSSZTo(dst); err != nil {
			return nil, err
		}
	}
	return dst, nil
}

func (r *BodiesResponseGloas) MarshalSSZTo(dst []byte) ([]byte, error) {
	b, err := r.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}

func (r *BodiesResponseGloas) UnmarshalSSZ(buf []byte) error {
	num, err := ssz.DecodeDynamicLength(buf, maxBodiesRequest)
	if err != nil {
		return err
	}
	entries := make([]*BodyEntryGloas, num)
	if err = ssz.UnmarshalDynamic(buf, num, func(i int, b []byte) error {
		entries[i] = &BodyEntryGloas{}
		return entries[i].UnmarshalSSZ(b)
	}); err != nil {
		return err
	}
	*r = entries
	return nil
}

// BlobsV2Response is the bare SSZ List[BlobV2Entry, MAX_BLOBS_REQUEST] 200 body of
// POST /blobs/v2 and /blobs/v3. BlobV2Entry is variable-size (it carries cell
// proofs), so the elements are preceded by a table of 4-byte offsets.
type BlobsV2Response []*BlobV2Entry

func (r *BlobsV2Response) SizeSSZ() int {
	size := 4 * len(*r)
	for _, e := range *r {
		size += e.SizeSSZ()
	}
	return size
}

func (r *BlobsV2Response) MarshalSSZ() ([]byte, error) {
	if len(*r) > maxBlobsRequest {
		return nil, ssz.ErrListTooBigFn("BlobsV2Response", len(*r), maxBlobsRequest)
	}
	dst := make([]byte, 0, r.SizeSSZ())
	offset := 4 * len(*r)
	for _, e := range *r {
		dst = ssz.WriteOffset(dst, offset)
		offset += e.SizeSSZ()
	}
	var err error
	for _, e := range *r {
		if dst, err = e.MarshalSSZTo(dst); err != nil {
			return nil, err
		}
	}
	return dst, nil
}

func (r *BlobsV2Response) MarshalSSZTo(dst []byte) ([]byte, error) {
	b, err := r.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}

func (r *BlobsV2Response) UnmarshalSSZ(buf []byte) error {
	num, err := ssz.DecodeDynamicLength(buf, maxBlobsRequest)
	if err != nil {
		return err
	}
	entries := make([]*BlobV2Entry, num)
	if err = ssz.UnmarshalDynamic(buf, num, func(i int, b []byte) error {
		entries[i] = &BlobV2Entry{}
		return entries[i].UnmarshalSSZ(b)
	}); err != nil {
		return err
	}
	*r = entries
	return nil
}
