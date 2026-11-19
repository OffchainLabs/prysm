package pq

import "github.com/pkg/errors"

const xmssSignatureLength = 2404

// Spec: process_execution_payload_bid, signed_bid.signature == XMSSSignature()
func VerifySelfBuildBidSignature(sig []byte) error {
	if len(sig) != xmssSignatureLength {
		return errors.Errorf("self-build signature has length %d", len(sig))
	}
	for _, b := range sig {
		if b != 0 {
			return errors.New("self-build signature must be the zero XMSS signature")
		}
	}
	return nil
}
