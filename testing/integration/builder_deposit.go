package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	contracts "github.com/OffchainLabs/prysm/v7/contracts/deposit"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const minerKeystoreJSON = `{"address":"878705ba3f8bc32fcf7f4caa1a35e72af65cf766","crypto":{"cipher":"aes-128-ctr","ciphertext":"f02daebbf456faf787c5cd61a33ce780857c1ca10b00972aa451f0e9688e4ead","cipherparams":{"iv":"ef1668814155862f0653f15dae845e58"},"kdf":"scrypt","kdfparams":{"dklen":32,"n":262144,"p":1,"r":8,"salt":"55e5ee70d3e882d2f00a073eda252ff01437abf51d7bfa76c06dcc73f7e8f1a3"},"mac":"d8d04625d0769fe286756734f946c78663961b74f0caaff1d768f0d255632f04"},"id":"5fb9083a-a221-412b-b0e0-921e22cc9645","version":3}`
const minerKeystorePassword = "password"

// depositBuilder sends a builder deposit to the deposit contract and verifies
// the builder appears in the beacon state.
func depositBuilder(t *testing.T, ctx context.Context, gethIndex, beaconIndex int) {
	t.Helper()

	// 1. Generate a builder BLS key.
	builderKey, err := bls.RandKey()
	require.NoError(t, err, "failed to generate builder key")
	builderPubkey := builderKey.PublicKey().Marshal()
	t.Logf("Builder pubkey: %#x", bytesutil.Trunc(builderPubkey))

	// 2. Build withdrawal credentials with builder prefix (0x03).
	executionAddr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	withdrawalCreds := make([]byte, 32)
	withdrawalCreds[0] = params.BeaconConfig().BuilderWithdrawalPrefixByte // 0x03
	copy(withdrawalCreds[12:], executionAddr.Bytes())

	// 3. Build deposit data and sign it.
	depositAmount := params.BeaconConfig().MinActivationBalance
	depositMessage := &ethpb.DepositMessage{
		PublicKey:             builderPubkey,
		WithdrawalCredentials: withdrawalCreds,
		Amount:                depositAmount,
	}
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	require.NoError(t, err)
	root, err := depositMessage.HashTreeRoot()
	require.NoError(t, err)
	sigRoot, err := (&ethpb.SigningData{ObjectRoot: root[:], Domain: domain}).HashTreeRoot()
	require.NoError(t, err)

	depositData := &ethpb.Deposit_Data{
		PublicKey:             builderPubkey,
		WithdrawalCredentials: withdrawalCreds,
		Amount:                depositAmount,
		Signature:             builderKey.Sign(sigRoot[:]).Marshal(),
	}
	dataRoot, err := depositData.HashTreeRoot()
	require.NoError(t, err)

	// 4. Connect to geth and send the deposit TX.
	gethURL := fmt.Sprintf("http://127.0.0.1:%d", gethHTTPPort(gethIndex))
	rpcClient, err := rpc.DialContext(ctx, gethURL)
	require.NoError(t, err, "failed to connect to geth")
	defer rpcClient.Close()
	client := ethclient.NewClient(rpcClient)

	minerKey, err := keystore.DecryptKey([]byte(minerKeystoreJSON), minerKeystorePassword)
	require.NoError(t, err, "failed to decrypt miner key")

	chainID := big.NewInt(int64(params.BeaconConfig().DepositChainID))
	txo, err := bind.NewKeyedTransactorWithChainID(minerKey.PrivateKey, chainID)
	require.NoError(t, err)
	txo.Context = ctx
	txo.GasLimit = 500000

	nonce, err := client.PendingNonceAt(ctx, txo.From)
	require.NoError(t, err)
	txo.Nonce = big.NewInt(0).SetUint64(nonce)
	txo.GasPrice = big.NewInt(2e11)
	txo.Value = new(big.Int).Mul(
		big.NewInt(0).SetUint64(depositAmount),
		big.NewInt(0).SetUint64(params.BeaconConfig().GweiPerEth),
	)

	depositContractAddr := common.HexToAddress(params.BeaconConfig().DepositContractAddress)
	contract, err := contracts.NewDepositContract(depositContractAddr, client)
	require.NoError(t, err, "failed to bind deposit contract")

	tx, err := contract.Deposit(txo, depositData.PublicKey, depositData.WithdrawalCredentials, depositData.Signature, dataRoot)
	require.NoError(t, err, "failed to send builder deposit TX")
	t.Logf("Builder deposit TX sent: %s (amount: %d gwei)", tx.Hash().Hex(), depositAmount)

	// 5. Wait for the builder to appear in the beacon state.
	t.Log("Waiting for builder to appear in beacon state...")
	deadline := time.After(60 * time.Second)
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatalf("Builder %#x not found in beacon state after 60s", bytesutil.Trunc(builderPubkey))
		case <-ticker.C:
			found, balance, err := queryBuilder(ctx, beaconIndex, builderPubkey)
			if err != nil {
				t.Logf("Builder query error: %v", err)
				continue
			}
			if found {
				t.Logf("Builder deposited! pubkey=%#x balance=%d gwei", bytesutil.Trunc(builderPubkey), balance)
				return
			}
			t.Logf("Builder not yet in state, waiting...")
		}
	}
}

// queryBuilder checks if a builder with the given pubkey exists in the beacon state.
func queryBuilder(_ context.Context, beaconIndex int, pubkey []byte) (bool, uint64, error) {
	// Use a generous per-request timeout since the full state JSON is large.
	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/eth/v2/debug/beacon/states/head", beaconGRPCPort(beaconIndex))
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return false, 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Builders []struct {
				Pubkey  string `json:"pubkey"`
				Balance string `json:"balance"`
			} `json:"builders"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, 0, err
	}

	pubkeyHex := fmt.Sprintf("%#x", pubkey)
	for _, b := range result.Data.Builders {
		if b.Pubkey == pubkeyHex {
			var bal uint64
			_, _ = fmt.Sscanf(b.Balance, "%d", &bal)
			return true, bal, nil
		}
	}
	return false, 0, nil
}
