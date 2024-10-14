package util

import enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"

// HydrateSignedExecutionPayloadHeader hydrates a SignedExecutionPayloadHeader.
func HydrateSignedExecutionPayloadHeader(h *enginev1.SignedExecutionPayloadHeader) *enginev1.SignedExecutionPayloadHeader {
	if h.Message == nil {
		h.Message = &enginev1.ExecutionPayloadHeaderEPBS{}
	}
	if h.Signature == nil {
		h.Signature = make([]byte, 96)
	}
	if h.Message.ParentBlockRoot == nil {
		h.Message.ParentBlockRoot = make([]byte, 32)
	}
	if h.Message.ParentBlockHash == nil {
		h.Message.ParentBlockHash = make([]byte, 32)
	}
	if h.Message.BlockHash == nil {
		h.Message.BlockHash = make([]byte, 32)
	}
	if h.Message.BlobKzgCommitmentsRoot == nil {
		h.Message.BlobKzgCommitmentsRoot = make([]byte, 32)
	}
	return h
}
