package integration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
)

// sendBlobAndVerify sends a blob transaction to geth and verifies the beacon
// chain includes it in a block (blobKzgCommitmentCount > 0).
func sendBlobAndVerify(t *testing.T, ctx context.Context, gethIndex, beaconIndex int) {
	t.Helper()

	// 1. Connect to geth.
	gethURL := fmt.Sprintf("http://127.0.0.1:%d", gethHTTPPort(gethIndex))
	rpcClient, err := rpc.DialContext(ctx, gethURL)
	require.NoError(t, err)
	defer rpcClient.Close()
	client := ethclient.NewClient(rpcClient)

	// 2. Decrypt the miner key (funded account).
	minerKey, err := keystore.DecryptKey([]byte(minerKeystoreJSON), minerKeystorePassword)
	require.NoError(t, err)

	chainID := big.NewInt(int64(params.BeaconConfig().DepositChainID))
	nonce, err := client.PendingNonceAt(ctx, minerKey.Address)
	require.NoError(t, err)

	// 3. Create blob data (just some test bytes).
	blobData := make([]byte, 128)
	for i := range blobData {
		blobData[i] = byte(i)
	}
	blobs, commitments, proofs, versionedHashes, err := encodeBlobs(blobData)
	require.NoError(t, err, "failed to encode blobs")

	// 4. Build and sign a blob transaction (type 3).
	to := common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	tx := types.NewTx(&types.BlobTx{
		ChainID:    uint256.MustFromBig(chainID),
		Nonce:      nonce,
		GasTipCap:  uint256.NewInt(2e9),  // 2 gwei
		GasFeeCap:  uint256.NewInt(2e11), // 200 gwei
		Gas:        100000,
		To:         to,
		Value:      uint256.NewInt(0),
		BlobFeeCap: uint256.NewInt(1e12), // 1000 gwei — generous for test
		BlobHashes: versionedHashes,
		Sidecar: &types.BlobTxSidecar{
			Blobs:       blobs,
			Commitments: commitments,
			Proofs:      proofs,
		},
	})

	signer := types.NewCancunSigner(chainID)
	signedTx, err := types.SignTx(tx, signer, minerKey.PrivateKey)
	require.NoError(t, err)

	err = client.SendTransaction(ctx, signedTx)
	require.NoError(t, err, "failed to send blob TX")
	t.Logf("Blob TX sent: %s (nonce=%d, %d blob(s), gasFeeCap=%s, blobFeeCap=%s)",
		signedTx.Hash().Hex(), nonce, len(blobs), signedTx.GasFeeCap(), signedTx.BlobGasFeeCap())

	// Send to all geth nodes so whichever proposer builds can include it.
	for i := range 2 {
		if i == gethIndex {
			continue
		}
		otherURL := fmt.Sprintf("http://127.0.0.1:%d", gethHTTPPort(i))
		if rc, err := rpc.DialContext(ctx, otherURL); err == nil {
			cl := ethclient.NewClient(rc)
			_ = cl.SendTransaction(ctx, signedTx) // ignore dup error
			rc.Close()
		}
	}

	// 5. Wait for a beacon block with blob commitments.
	t.Log("Waiting for beacon block with blob commitments...")
	deadline := time.After(60 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	startSlot := queryHeadSlot(beaconIndex)
	for {
		select {
		case <-deadline:
			t.Fatal("No beacon block with blob commitments found after 60s")
		case <-ticker.C:
			head := queryHeadSlot(beaconIndex)
			for s := startSlot; s <= head; s++ {
				count := blockBlobCount(beaconIndex, s)
				if count > 0 {
					t.Logf("Block at slot %d has %d blob commitment(s)", s, count)
					return
				}
			}
			startSlot = head
		}
	}
}

// queryHeadSlot returns the head slot from the beacon REST API.
func queryHeadSlot(beaconIndex int) uint64 {
	reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://127.0.0.1:%d/eth/v1/beacon/headers/head", beaconGRPCPort(beaconIndex))
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer func() { _ = resp.Body.Close() }()
	var result struct {
		Data struct {
			Header struct {
				Message struct {
					Slot string `json:"slot"`
				} `json:"message"`
			} `json:"header"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0
	}
	var s uint64
	_, _ = fmt.Sscanf(result.Data.Header.Message.Slot, "%d", &s)
	return s
}

// blockBlobCount returns the number of blob KZG commitments in the block at the given slot.
func blockBlobCount(beaconIndex int, slot uint64) int {
	reqCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/eth/v2/beacon/blocks/%d", beaconGRPCPort(beaconIndex), slot)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0
	}

	// In Gloas, blob commitments are inside the signed execution payload bid.
	var result struct {
		Data struct {
			Message struct {
				Body struct {
					BlobKzgCommitments []string `json:"blob_kzg_commitments"`
					SignedBid          struct {
						Message struct {
							BlobKzgCommitments []string `json:"blob_kzg_commitments"`
						} `json:"message"`
					} `json:"signed_execution_payload_bid"`
				} `json:"body"`
			} `json:"message"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0
	}

	count := len(result.Data.Message.Body.BlobKzgCommitments)
	if count == 0 {
		count = len(result.Data.Message.Body.SignedBid.Message.BlobKzgCommitments)
	}
	return count
}

// encodeBlobs encodes raw data into KZG blobs with commitments and proofs.
func encodeBlobs(data []byte) ([]kzg4844.Blob, []kzg4844.Commitment, []kzg4844.Proof, []common.Hash, error) {
	var blob kzg4844.Blob
	copy(blob[:], data)

	commit, err := kzg4844.BlobToCommitment(&blob)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("BlobToCommitment: %w", err)
	}
	proof, err := kzg4844.ComputeBlobProof(&blob, commit)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("ComputeBlobProof: %w", err)
	}

	h := sha256.Sum256(commit[:])
	h[0] = 0x01 // KZG versioned hash
	vHash := common.Hash(h)

	return []kzg4844.Blob{blob},
		[]kzg4844.Commitment{commit},
		[]kzg4844.Proof{proof},
		[]common.Hash{vHash},
		nil
}
