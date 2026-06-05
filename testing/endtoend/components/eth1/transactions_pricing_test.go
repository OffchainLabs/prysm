package eth1

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestNextBlobPricingInitializesFromSuggestedGasPrice(t *testing.T) {
	txGen := NewTransactionGenerator("", 1, false)
	gasPrice, blobFeeCap := txGen.nextBlobPricing(7, big.NewInt(100))
	require.Equal(t, "100", gasPrice.String())
	require.Equal(t, "200", blobFeeCap.String())
}

func TestNextBlobPricingBumpsReplacementPrices(t *testing.T) {
	txGen := NewTransactionGenerator("", 1, false)
	txGen.hasBlobState = true
	txGen.blobNonce = 10
	txGen.blobGasPrice = big.NewInt(100)
	txGen.blobFeeCap = big.NewInt(200)

	gasPrice, blobFeeCap := txGen.nextBlobPricing(9, big.NewInt(90))
	require.Equal(t, "120", gasPrice.String())
	require.Equal(t, "240", blobFeeCap.String())
}

func TestNextBlobPricingResetsAfterAcceptedNonceAdvances(t *testing.T) {
	txGen := NewTransactionGenerator("", 1, false)
	txGen.hasBlobState = true
	txGen.blobNonce = 10
	txGen.blobGasPrice = big.NewInt(100)
	txGen.blobFeeCap = big.NewInt(200)

	gasPrice, blobFeeCap := txGen.nextBlobPricing(10, big.NewInt(90))
	require.Equal(t, "90", gasPrice.String())
	require.Equal(t, "180", blobFeeCap.String())
}

func TestNextTxPricingBumpsAndPinsPendingNonce(t *testing.T) {
	txGen := NewTransactionGenerator("", 1, false)
	txGen.hasTxState = true
	txGen.txNonce = 10
	txGen.txGasPrice = big.NewInt(100)

	nonce, gasPrice := txGen.nextTxPricing(8, big.NewInt(90))
	require.Equal(t, uint64(10), nonce)
	require.Equal(t, "120", gasPrice.String())
}

func TestNextTxPricingResetsAfterNonceAdvances(t *testing.T) {
	txGen := NewTransactionGenerator("", 1, false)
	txGen.hasTxState = true
	txGen.txNonce = 10
	txGen.txGasPrice = big.NewInt(100)

	nonce, gasPrice := txGen.nextTxPricing(10, big.NewInt(90))
	require.Equal(t, uint64(10), nonce)
	require.Equal(t, "90", gasPrice.String())
}

func TestNextTxPricingUsesFreshNonceWhenChainMovesAhead(t *testing.T) {
	txGen := NewTransactionGenerator("", 1, false)
	txGen.hasTxState = true
	txGen.txNonce = 10
	txGen.txGasPrice = big.NewInt(100)

	nonce, gasPrice := txGen.nextTxPricing(12, big.NewInt(90))
	require.Equal(t, uint64(12), nonce)
	require.Equal(t, "90", gasPrice.String())
}

func TestSyncBlobModeResetsPendingBlobStateOnModeChange(t *testing.T) {
	txGen := NewTransactionGenerator("", 1, false)
	txGen.hasBlobState = true
	txGen.blobNonce = 10
	txGen.blobGasPrice = big.NewInt(100)
	txGen.blobFeeCap = big.NewInt(200)
	txGen.blobMode = blobTxModeSidecar

	txGen.syncBlobMode(blobTxModeCellProof)

	require.Equal(t, blobTxModeCellProof, txGen.blobMode)
	require.Equal(t, false, txGen.hasBlobState)
	require.Equal(t, uint64(0), txGen.blobNonce)
	require.Equal(t, (*big.Int)(nil), txGen.blobGasPrice)
	require.Equal(t, (*big.Int)(nil), txGen.blobFeeCap)
}

func TestBlobAccountUsesModeSpecificFundedAccount(t *testing.T) {
	txGen := NewTransactionGenerator("", 1, false)
	txGen.sidecarAccount = &keystore.Key{Address: common.HexToAddress("0x100")}
	txGen.cellProofAccount = &keystore.Key{Address: common.HexToAddress("0x200")}

	require.Equal(t, txGen.sidecarAccount, txGen.blobAccount(blobTxModeSidecar))
	require.Equal(t, txGen.cellProofAccount, txGen.blobAccount(blobTxModeCellProof))
}
