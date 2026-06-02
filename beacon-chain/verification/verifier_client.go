package verification

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// zkProofVerifier is an HTTP client for the zkboost proof verification endpoint.
type zkProofVerifier struct {
	endpoint string
	http     *http.Client
}

// VerifyProof calls the verifier's POST /v1/execution_proof_verifications endpoint
// to verify a single execution proof.
func (vc *zkProofVerifier) VerifyProof(proof blocks.ROSignedExecutionProof) error {
	hexRoot := "0x" + hex.EncodeToString(proof.Message.PublicInput.NewPayloadRequestRoot)
	proofTypeName := ethpb.ProofTypeName(proof.Message.ProofType[0])

	url := fmt.Sprintf(
		"%s/v1/execution_proof_verifications?new_payload_request_root=%s&proof_type=%s",
		vc.endpoint, hexRoot, proofTypeName,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(proof.Message.ProofData))
	if err != nil {
		return fmt.Errorf("%w: create request: %w", ErrProofVerificationEndpoint, err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := vc.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrProofVerificationEndpoint, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Error("Failed to close verifier response body")
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: read body: %w", ErrProofVerificationEndpoint, err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d: %s", ErrProofVerificationEndpoint, resp.StatusCode, string(body))
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("%w: unmarshal response: %w", ErrProofVerificationEndpoint, err)
	}

	if result.Status != "VALID" {
		return ErrProofVerificationFailed
	}

	return nil
}
