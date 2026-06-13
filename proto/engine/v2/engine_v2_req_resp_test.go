package enginev2

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// blobBytes is BYTES_PER_BLOB, derived from the fixed BlobV1Entry size
// (entry = available(1) + blob + proof(48)), so the test tracks the build config.
var blobBytes = blobV1EntrySize - 1 - 48

// The request/response bodies must serialize as BARE SSZ List[...] — no
// enclosing-container 4-byte offset. These tests pin that wire shape and the
// round-trip, since the codecs are hand-written (engine_v2_req_resp.go).

func TestHashListRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 3, maxBodiesRequest} {
		hashes := make([][]byte, n)
		for i := range hashes {
			h := make([]byte, hashLen)
			h[0] = byte(i + 1)
			hashes[i] = h
		}
		req := BodiesByHashRequest(hashes)
		b, err := req.MarshalSSZ()
		if err != nil {
			t.Fatalf("n=%d marshal: %v", n, err)
		}
		// A bare List of fixed 32-byte elements is exactly n*32 bytes; a container
		// would add a 4-byte offset prefix.
		if len(b) != n*hashLen {
			t.Fatalf("n=%d: bare list is %d bytes, want %d (no container offset)", n, len(b), n*hashLen)
		}
		var got BodiesByHashRequest
		if err := got.UnmarshalSSZ(b); err != nil {
			t.Fatalf("n=%d unmarshal: %v", n, err)
		}
		if len(got) != n {
			t.Fatalf("n=%d: round-trip gave %d hashes", n, len(got))
		}
		for i := range got {
			if !bytes.Equal(got[i], hashes[i]) {
				t.Fatalf("n=%d: hash %d mismatch", n, i)
			}
		}
	}
}

func TestHashListErrors(t *testing.T) {
	// Over MAX_BODIES_REQUEST.
	over := make(BodiesByHashRequest, maxBodiesRequest+1)
	for i := range over {
		over[i] = make([]byte, hashLen)
	}
	if _, err := over.MarshalSSZ(); err == nil {
		t.Fatal("expected error marshaling over-max BodiesByHashRequest")
	}
	// Wrong element length.
	bad := BodiesByHashRequest{make([]byte, hashLen-1)}
	if _, err := bad.MarshalSSZ(); err == nil {
		t.Fatal("expected error marshaling a 31-byte hash")
	}
	// A buffer not divisible by 32 cannot decode.
	var got BodiesByHashRequest
	if err := got.UnmarshalSSZ(make([]byte, hashLen+1)); err == nil {
		t.Fatal("expected error decoding a non-multiple-of-32 buffer")
	}
	// BlobsRequest honors its own (larger) bound.
	overBlobs := make(BlobsRequest, maxBlobsRequest+1)
	for i := range overBlobs {
		overBlobs[i] = make([]byte, hashLen)
	}
	if _, err := overBlobs.MarshalSSZ(); err == nil {
		t.Fatal("expected error marshaling over-max BlobsRequest")
	}
}

func TestBlobsRequestRoundTrip(t *testing.T) {
	req := BlobsRequest{make([]byte, hashLen), make([]byte, hashLen)}
	req[1][0] = 0xff
	b, err := req.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 2*hashLen {
		t.Fatalf("bare BlobsRequest = %d bytes, want %d", len(b), 2*hashLen)
	}
	var got BlobsRequest
	if err := got.UnmarshalSSZ(b); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !bytes.Equal(got[1], req[1]) {
		t.Fatal("BlobsRequest round-trip mismatch")
	}
}

func TestBlobsV1ResponseRoundTrip(t *testing.T) {
	mk := func(avail bool) *BlobV1Entry {
		return &BlobV1Entry{Available: avail, Contents: &BlobAndProofV1{
			Blob:  make([]byte, blobBytes),
			Proof: make([]byte, 48),
		}}
	}
	resp := BlobsV1Response{mk(true), mk(false)}
	b, err := resp.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	// Fixed-size elements concatenate with no offsets: exactly n*blobV1EntrySize.
	if len(b) != len(resp)*blobV1EntrySize {
		t.Fatalf("BlobsV1Response = %d bytes, want %d (concat, no offset)", len(b), len(resp)*blobV1EntrySize)
	}
	var got BlobsV1Response
	if err := got.UnmarshalSSZ(b); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Available || got[1].Available {
		t.Fatal("BlobsV1Response round-trip mismatch")
	}
}

func TestBodiesResponseRoundTrip(t *testing.T) {
	resp := BodiesResponseGloas{
		{Available: true, Body: &ExecutionPayloadBodyGloas{
			Transactions:    [][]byte{{0x01, 0x02}},
			BlockAccessList: []byte{0x03},
		}},
		{Available: false, Body: &ExecutionPayloadBodyGloas{}},
		{Available: true, Body: &ExecutionPayloadBodyGloas{Transactions: [][]byte{{0x04}}}},
	}
	b, err := resp.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	// Variable-size elements: the bytes open with the element offset table, so the
	// first offset == 4*N. A container would prepend its own offset (=4) first.
	if got := binary.LittleEndian.Uint32(b[:4]); got != uint32(4*len(resp)) {
		t.Fatalf("first offset = %d, want %d (4*N, bare list)", got, 4*len(resp))
	}
	var got BodiesResponseGloas
	if err := got.UnmarshalSSZ(b); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[0].Available || got[1].Available || !got[2].Available {
		t.Fatal("BodiesResponseGloas available flags mismatch")
	}
	if txs := got[0].Body.Transactions; len(txs) != 1 || !bytes.Equal(txs[0], []byte{0x01, 0x02}) {
		t.Fatal("transaction did not round-trip")
	}
	if !bytes.Equal(got[0].Body.BlockAccessList, []byte{0x03}) {
		t.Fatal("block_access_list did not round-trip")
	}
}

func TestBlobsV2ResponseRoundTrip(t *testing.T) {
	mk := func() *BlobV2Entry {
		return &BlobV2Entry{Available: true, Contents: &BlobAndProofV2{
			Blob:   make([]byte, blobBytes),
			Proofs: [][]byte{make([]byte, 48), make([]byte, 48)},
		}}
	}
	resp := BlobsV2Response{mk(), mk()}
	b, err := resp.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint32(b[:4]); got != uint32(4*len(resp)) {
		t.Fatalf("BlobsV2Response first offset = %d, want %d", got, 4*len(resp))
	}
	var got BlobsV2Response
	if err := got.UnmarshalSSZ(b); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got[0].Contents.Proofs) != 2 {
		t.Fatal("BlobsV2Response round-trip mismatch")
	}
}

// Empty bodies (0 elements) must marshal to zero bytes and decode back to empty —
// the engine path relies on this for empty range/blob results.
func TestEmptyLists(t *testing.T) {
	var req BodiesByHashRequest
	b, err := req.MarshalSSZ()
	if err != nil || len(b) != 0 {
		t.Fatalf("empty request: bytes=%d err=%v", len(b), err)
	}
	var gotReq BodiesByHashRequest
	if err := gotReq.UnmarshalSSZ(nil); err != nil || len(gotReq) != 0 {
		t.Fatalf("empty request decode: len=%d err=%v", len(gotReq), err)
	}

	var resp BodiesResponseFulu
	b, err = resp.MarshalSSZ()
	if err != nil || len(b) != 0 {
		t.Fatalf("empty response: bytes=%d err=%v", len(b), err)
	}
	var gotResp BodiesResponseFulu
	if err := gotResp.UnmarshalSSZ(nil); err != nil || len(gotResp) != 0 {
		t.Fatalf("empty response decode: len=%d err=%v", len(gotResp), err)
	}
}
