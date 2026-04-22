package eth1

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"
	"math/big"
	mathRand "math/rand"
	"os"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const txCount = 20

var fundedAccount *keystore.Key

type TransactionGenerator struct {
	keystore      string
	seed          int64
	started       chan struct{}
	cancel        context.CancelFunc
	paused        bool
	useLargeBlobs bool // Use large blob transactions (6 blobs per tx) for BPO testing
}

func (t *TransactionGenerator) UnderlyingProcess() *os.Process {
	// Transaction Generator runs under the same underlying process so
	// we return an empty process object.
	return &os.Process{}
}

func NewTransactionGenerator(keystore string, seed int64, useLargeBlobs bool) *TransactionGenerator {
	return &TransactionGenerator{keystore: keystore, seed: seed, useLargeBlobs: useLargeBlobs}
}

func (t *TransactionGenerator) Start(ctx context.Context) error {
	// Wrap context with a cancel func
	ctx, ccl := context.WithCancel(ctx)
	t.cancel = ccl

	client, err := rpc.DialHTTP(fmt.Sprintf("http://127.0.0.1:%d", e2e.TestParams.Ports.Eth1RPCPort))
	if err != nil {
		return err
	}
	defer client.Close()

	seed := t.seed
	newGen := rand.NewDeterministicGenerator()
	if seed == 0 {
		seed = newGen.Int63()
		logrus.WithField("Seed", seed).Info("Transaction generator")
	}
	// Set seed so that all transactions can be
	// deterministically generated.
	mathRand.Seed(seed)

	keystoreBytes, err := os.ReadFile(t.keystore) // #nosec G304
	if err != nil {
		return err
	}
	mineKey, err := keystore.DecryptKey(keystoreBytes, KeystorePassword)
	if err != nil {
		return err
	}
	newKey := keystore.NewKeyForDirectICAP(newGen)
	if err := fundAccount(client, mineKey, newKey); err != nil {
		return err
	}
	fundedAccount = newKey
	// Ensure funding tx is mined before generating txs that rely on balance.
	// Mine 1 block using the miner key to include the funding transfer.
	backend := ethclient.NewClient(client)
	defer backend.Close()

	if err := WaitForBlocks(ctx, backend, mineKey, 1); err != nil {
		return errors.Wrap(err, "failed to mine block for funding tx")
	}

	// Ensure the funded account has a comfortable minimum balance for blob and fuzzed txs.
	minWei := new(big.Int).Mul(big.NewInt(1000), big.NewInt(0).SetUint64(params.BeaconConfig().GweiPerEth))
	minWei.Mul(minWei, big.NewInt(1e9)) // 1000 ETH in wei
	if err := ensureMinBalance(ctx, client, backend, mineKey, fundedAccount, minWei); err != nil {
		return err
	}
	// Broadcast Transactions every slot
	txPeriod := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	ticker := time.NewTicker(txPeriod)
	gasPrice := big.NewInt(1e11)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if t.paused {
				continue
			}
			backend := ethclient.NewClient(client)
			err = SendTransaction(client, mineKey.PrivateKey, gasPrice, mineKey.Address.String(), txCount, backend, false, t.useLargeBlobs)
			if err != nil {
				return err
			}
			backend.Close()
		}
	}
}

// Started checks whether beacon node set is started and all nodes are ready to be queried.
func (s *TransactionGenerator) Started() <-chan struct{} {
	return s.started
}

func SendTransaction(client *rpc.Client, key *ecdsa.PrivateKey, gasPrice *big.Int, addr string, txCount uint64, backend *ethclient.Client, al bool, useLargeBlobs bool) error {
	sender := common.HexToAddress(addr)
	nonce, err := backend.PendingNonceAt(context.Background(), fundedAccount.Address)
	if err != nil {
		return err
	}
	chainid, err := backend.ChainID(context.Background())
	if err != nil {
		return err
	}
	expectedPrice, err := backend.SuggestGasPrice(context.Background())
	if err != nil {
		return err
	}
	if expectedPrice.Cmp(gasPrice) > 0 {
		gasPrice = expectedPrice
	}

	clock := startup.NewClock(e2e.TestParams.CLGenesisTime, [32]byte{})
	isPostFulu := clock.CurrentEpoch() >= params.BeaconConfig().FuluForkEpoch

	g, _ := errgroup.WithContext(context.Background())
	var txs []*types.Transaction

	// Pre-Fulu V0 sidecars become invalid once the EL crosses Osaka and then
	// permanently occupy the blobpool (maxTxsPerAccount=16), so skip them.
	if isPostFulu {
		logrus.Info("Sending blob transactions with cell proofs")
		txs = make([]*types.Transaction, 5)
		for index := range uint64(5) {

			g.Go(func() error {
				tx, err := RandomBlobCellTx(client, fundedAccount.Address, nonce+index, gasPrice, chainid, al, useLargeBlobs)
				if err != nil {
					return errors.Wrap(err, "Could not create blob cell tx")
				}

				signedTx, err := types.SignTx(tx, types.NewCancunSigner(chainid), fundedAccount.PrivateKey)
				if err != nil {
					return errors.Wrap(err, "Could not sign blob cell tx")
				}

				txs[index] = signedTx
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}
		// Stop on first failure: any nonce gap trips geth's gapped-queue
		// allowance=log10(nonce+1), which rejects every subsequent nonce.
		for i, tx := range txs {
			if tx == nil {
				continue
			}
			if err := backend.SendTransaction(context.Background(), tx); err != nil {
				entry := logrus.WithError(err).WithField("index", i).WithField("nonce", tx.Nonce())
				if strings.Contains(err.Error(), "account limit exceeded") {
					entry.Debug("Blob tx send stopped: pool full (backpressure)")
				} else {
					entry.Warn("Blob tx send failed; stopping batch to avoid nonce gap")
				}
				break
			}
		}
	}

	nonce, err = backend.PendingNonceAt(context.Background(), sender)
	if err != nil {
		return err
	}

	txs = make([]*types.Transaction, txCount)
	for index := range txCount {

		g.Go(func() error {
			tx, err := randomValidTx(sender, nonce+index, gasPrice, chainid, al)
			if err != nil {
				// In the event the transaction constructed is not valid, we continue with the routine
				// rather than complete stop it.
				//nolint:nilerr
				return nil
			}
			// Clamp gas to avoid exceeding common EL per-tx gas caps (e.g. 16,777,216) due to EIP-7825: Transaction Gas Limit Cap
			tx = clampTxGas(tx, 16_000_000)

			signedTx, err := types.SignTx(tx, types.NewLondonSigner(chainid), key)
			if err != nil {
				// We continue on in the event there is a reason we can't sign this
				// transaction(unlikely).
				//nolint:nilerr
				return nil
			}
			txs[index] = signedTx
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	for _, tx := range txs {
		if tx == nil {
			continue
		}
		err = backend.SendTransaction(context.Background(), tx)
		if err != nil {
			// Do nothing
			continue
		}
	}
	return nil
}

// Pause pauses the component and its underlying process.
func (t *TransactionGenerator) Pause() error {
	t.paused = true
	return nil
}

// Resume resumes the component and its underlying process.
func (t *TransactionGenerator) Resume() error {
	t.paused = false
	return nil
}

// Stop stops the component and its underlying process.
func (t *TransactionGenerator) Stop() error {
	t.cancel()
	return nil
}

func RandomBlobCellTx(rpc *rpc.Client, sender common.Address, nonce uint64, gasPrice, chainID *big.Int, al bool, useLargeBlobs bool) (*types.Transaction, error) {
	// Set fields if non-nil
	if rpc != nil {
		client := ethclient.NewClient(rpc)

		var err error

		if gasPrice == nil {
			gasPrice, err = client.SuggestGasPrice(context.Background())
			if err != nil {
				gasPrice = big.NewInt(1)
			}
		}
		if chainID == nil {
			chainID, err = client.ChainID(context.Background())
			if err != nil {
				chainID = big.NewInt(1)
			}
		}
	}

	gas := uint64(100000)
	to := randomAddress()
	// Generate random EVM bytecode (similar to what tx-fuzz RandomCode did)
	code := generateRandomEVMCode(mathRand.Intn(128)) // #nosec G404
	value := big.NewInt(0)

	mod := 2
	if al {
		mod = 1
	}

	// Helper to get blob data based on config
	getBlobData := func() ([]byte, error) {
		if useLargeBlobs {
			return randomBlobDataLarge()
		}
		return randomBlobData()
	}

	// #nosec G404 -- Test code uses deterministic randomness
	switch mathRand.Intn(mod) {
	case 0:
		// Blob transaction with cell proofs (Version 1 sidecar)
		tip, feecap, err := getCaps(rpc, gasPrice)
		if err != nil {
			return nil, errors.Wrap(err, "getCaps")
		}

		data, err := getBlobData()
		if err != nil {
			return nil, errors.Wrap(err, "getBlobData")
		}

		return New4844CellTx(nonce, &to, gas, chainID, tip, feecap, value, code, big.NewInt(1000000), data, make(types.AccessList, 0))
	case 1:
		// Blob transaction with cell proofs and access list
		tx := types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			To:       &to,
			Value:    value,
			Gas:      gas,
			GasPrice: gasPrice,
			Data:     code,
		})

		// Use legacy GasPrice for access list simulation to satisfy post-London requirement.
		msg := ethereum.CallMsg{
			From:       sender,
			To:         tx.To(),
			Gas:        tx.Gas(),
			GasPrice:   gasPrice,
			Value:      tx.Value(),
			Data:       tx.Data(),
			AccessList: nil,
		}
		geth := gethclient.New(rpc)

		al, _, _, err := geth.CreateAccessList(context.Background(), msg)
		if err != nil {
			return nil, errors.Wrap(err, "CreateAccessList")
		}
		tip, feecap, err := getCaps(rpc, gasPrice)
		if err != nil {
			return nil, errors.Wrap(err, "getCaps")
		}
		data, err := getBlobData()
		if err != nil {
			return nil, errors.Wrap(err, "getBlobData")
		}

		return New4844CellTx(nonce, &to, gas, chainID, tip, feecap, value, code, big.NewInt(1000000), data, *al)
	}

	return nil, nil
}

func RandomBlobTx(rpc *rpc.Client, sender common.Address, nonce uint64, gasPrice, chainID *big.Int, al bool, useLargeBlobs bool) (*types.Transaction, error) {
	// Set fields if non-nil
	if rpc != nil {
		client := ethclient.NewClient(rpc)
		var err error
		if gasPrice == nil {
			gasPrice, err = client.SuggestGasPrice(context.Background())
			if err != nil {
				gasPrice = big.NewInt(1)
			}
		}

		if chainID == nil {
			chainID, err = client.ChainID(context.Background())
			if err != nil {
				chainID = big.NewInt(1)
			}
		}
	}
	gas := uint64(100000)
	to := randomAddress()
	// Generate random EVM bytecode (similar to what tx-fuzz RandomCode did)
	code := generateRandomEVMCode(mathRand.Intn(128)) // #nosec G404
	value := big.NewInt(0)
	mod := 2
	if al {
		mod = 1
	}

	// Helper to get blob data based on config
	getBlobData := func() ([]byte, error) {
		if useLargeBlobs {
			return randomBlobDataLarge()
		}
		return randomBlobData()
	}

	// #nosec G404 -- Test code uses deterministic randomness
	switch mathRand.Intn(mod) {
	case 0:
		// 4844 transaction without AL

		tip, feecap, err := getCaps(rpc, gasPrice)
		if err != nil {
			return nil, errors.Wrap(err, "getCaps")
		}

		data, err := getBlobData()
		if err != nil {
			return nil, errors.Wrap(err, "getBlobData")
		}
		return New4844Tx(nonce, &to, gas, chainID, tip, feecap, value, code, big.NewInt(1000000), data, make(types.AccessList, 0)), nil
	case 1:
		// 4844 transaction with AL nonce, to, value, gas, gasPrice, code
		tx := types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			To:       &to,
			Value:    value,
			Gas:      gas,
			GasPrice: gasPrice,
			Data:     code,
		})

		// Use legacy GasPrice for access list simulation to satisfy post-London requirement.
		msg := ethereum.CallMsg{
			From:       sender,
			To:         tx.To(),
			Gas:        tx.Gas(),
			GasPrice:   gasPrice,
			Value:      tx.Value(),
			Data:       tx.Data(),
			AccessList: nil,
		}
		geth := gethclient.New(rpc)

		al, _, _, err := geth.CreateAccessList(context.Background(), msg)
		if err != nil {
			return nil, errors.Wrap(err, "CreateAccessList")
		}
		tip, feecap, err := getCaps(rpc, gasPrice)
		if err != nil {
			return nil, errors.Wrap(err, "getCaps")
		}
		data, err := getBlobData()
		if err != nil {
			return nil, errors.Wrap(err, "getBlobData")
		}
		return New4844Tx(nonce, &to, gas, chainID, tip, feecap, value, code, big.NewInt(1000000), data, *al), nil
	}
	return nil, errors.New("asdf")
}

func New4844CellTx(nonce uint64, to *common.Address, gasLimit uint64, chainID, tip, feeCap, value *big.Int, code []byte, blobFeeCap *big.Int, blobData []byte, al types.AccessList) (*types.Transaction, error) {
	blobs, comms, _, versionedHashes, err := EncodeBlobs(blobData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode blobs")
	}

	// Create a Version 0 sidecar first
	sidecar := &types.BlobTxSidecar{
		Version:     types.BlobSidecarVersion0,
		Blobs:       blobs,
		Commitments: comms,
		Proofs:      make([]kzg4844.Proof, len(blobs)), // Placeholder, will be replaced by ToV1
	}

	// Convert to Version 1 which will compute and attach cell proofs
	if err := sidecar.ToV1(); err != nil {
		return nil, errors.Wrap(err, "failed to convert sidecar to V1")
	}

	tx := types.NewTx(&types.BlobTx{
		ChainID:    uint256.MustFromBig(chainID),
		Nonce:      nonce,
		GasTipCap:  uint256.MustFromBig(tip),
		GasFeeCap:  uint256.MustFromBig(feeCap),
		Gas:        gasLimit,
		To:         *to,
		Value:      uint256.MustFromBig(value),
		Data:       code,
		AccessList: al,
		BlobFeeCap: uint256.MustFromBig(blobFeeCap),
		BlobHashes: versionedHashes,
		Sidecar:    sidecar,
	})

	return tx, nil
}

func New4844Tx(nonce uint64, to *common.Address, gasLimit uint64, chainID, tip, feeCap, value *big.Int, code []byte, blobFeeCap *big.Int, blobData []byte, al types.AccessList) *types.Transaction {
	blobs, comms, proofs, versionedHashes, err := EncodeBlobs(blobData)
	if err != nil {
		panic(err) // lint:nopanic -- Test code.
	}
	tx := types.NewTx(&types.BlobTx{
		ChainID:    uint256.MustFromBig(chainID),
		Nonce:      nonce,
		GasTipCap:  uint256.MustFromBig(tip),
		GasFeeCap:  uint256.MustFromBig(feeCap),
		Gas:        gasLimit,
		To:         *to,
		Value:      uint256.MustFromBig(value),
		Data:       code,
		AccessList: al,
		BlobFeeCap: uint256.MustFromBig(blobFeeCap),
		BlobHashes: versionedHashes,
		Sidecar: &types.BlobTxSidecar{
			Blobs:       blobs,
			Commitments: comms,
			Proofs:      proofs,
		},
	})
	return tx
}

// clampTxGas returns a copy of tx with Gas reduced to cap if it exceeds cap.
// This avoids EL errors like "transaction gas limit too high" on networks with
// per-transaction gas caps (commonly ~16,777,216).
func clampTxGas(tx *types.Transaction, gasCap uint64) *types.Transaction {
	if tx == nil || tx.Gas() <= gasCap {
		return tx
	}

	to := tx.To()
	switch tx.Type() {
	case types.LegacyTxType:
		return types.NewTx(&types.LegacyTx{
			Nonce:    tx.Nonce(),
			To:       to,
			Value:    tx.Value(),
			Gas:      gasCap,
			GasPrice: tx.GasPrice(),
			Data:     tx.Data(),
		})
	case types.AccessListTxType:
		return types.NewTx(&types.AccessListTx{
			ChainID:    tx.ChainId(),
			Nonce:      tx.Nonce(),
			To:         to,
			Value:      tx.Value(),
			Gas:        gasCap,
			GasPrice:   tx.GasPrice(),
			Data:       tx.Data(),
			AccessList: tx.AccessList(),
		})
	case types.DynamicFeeTxType:
		return types.NewTx(&types.DynamicFeeTx{
			ChainID:    tx.ChainId(),
			Nonce:      tx.Nonce(),
			To:         to,
			Value:      tx.Value(),
			Gas:        gasCap,
			Data:       tx.Data(),
			AccessList: tx.AccessList(),
			GasTipCap:  tx.GasTipCap(),
			GasFeeCap:  tx.GasFeeCap(),
		})
	case types.BlobTxType:
		// Leave blob txs unchanged here; blob tx construction paths set gas explicitly.
		return tx
	default:
		return tx
	}
}

// ensureMinBalance tops up dest account from miner if its balance is below minWei.
func ensureMinBalance(ctx context.Context, rpcCli *rpc.Client, backend *ethclient.Client, minerKey, destKey *keystore.Key, minWei *big.Int) error {
	bal, err := backend.BalanceAt(ctx, destKey.Address, nil)
	if err != nil {
		return err
	}

	if bal.Cmp(minWei) >= 0 {
		return nil
	}

	if err := fundAccount(rpcCli, minerKey, destKey); err != nil {
		return err
	}

	if err := WaitForBlocks(ctx, backend, minerKey, 1); err != nil {
		return errors.Wrap(err, "failed to mine block for top-up tx")
	}

	return nil
}

func encodeBlobs(data []byte) []kzg4844.Blob {
	blobs := []kzg4844.Blob{{}}
	blobIndex := 0
	fieldIndex := -1
	numOfElems := fieldparams.BlobLength / 32
	// Allow up to 6 blobs per transaction to properly test BPO limits.
	// With 10 blob txs per slot × 6 blobs = 60 max blobs submitted,
	// which exceeds the highest BPO limit (21) and ensures we can hit it.
	const maxBlobsPerTx = 6
	for i := 0; i < len(data); i += 31 {
		fieldIndex++
		if fieldIndex == numOfElems {
			if blobIndex >= maxBlobsPerTx-1 {
				break
			}
			blobs = append(blobs, kzg4844.Blob{})
			blobIndex++
			fieldIndex = 0
		}
		max := min(i+31, len(data))
		copy(blobs[blobIndex][fieldIndex*32+1:], data[i:max])
	}
	return blobs
}

func EncodeBlobs(data []byte) ([]kzg4844.Blob, []kzg4844.Commitment, []kzg4844.Proof, []common.Hash, error) {
	var (
		blobs           = encodeBlobs(data)
		commits         []kzg4844.Commitment
		proofs          []kzg4844.Proof
		versionedHashes []common.Hash
	)
	for _, blob := range blobs {
		b := blob
		commit, err := kzg4844.BlobToCommitment(&b)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		commits = append(commits, commit)

		proof, err := kzg4844.ComputeBlobProof(&b, commit)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := kzg4844.VerifyBlobProof(&b, commit, proof); err != nil {
			return nil, nil, nil, nil, err
		}
		proofs = append(proofs, proof)

		versionedHashes = append(versionedHashes, kZGToVersionedHash(commit))
	}
	return blobs, commits, proofs, versionedHashes, nil
}

var blobCommitmentVersionKZG uint8 = 0x01

// kZGToVersionedHash implements kzg_to_versioned_hash from EIP-4844
func kZGToVersionedHash(kzg kzg4844.Commitment) common.Hash {
	h := sha256.Sum256(kzg[:])
	h[0] = blobCommitmentVersionKZG

	return h
}

func randomBlobData() ([]byte, error) {
	// Generate random data for up to 1 blob. This is used for pre-Fulu tests.
	size := mathRand.Intn(fieldparams.BlobSize) // #nosec G404
	data := make([]byte, size)
	n, err := mathRand.Read(data) // #nosec G404
	if err != nil {
		return nil, err
	}
	if n != size {
		return nil, fmt.Errorf("could not create random blob data with size %d: %w", size, err)
	}
	return data, nil
}

// randomBlobDataLarge generates 6 blobs worth of data for BPO testing.
// This is used post-Fulu to ensure we can test increased blob limits.
// With 5 blob txs per slot × 6 blobs = 30 max blobs submitted,
// which exceeds the highest BPO limit (21) and ensures we can hit it.
// The data is mostly zeros with only the first 1KB randomized for uniqueness,
// which is sufficient for testing without the overhead of generating ~786KB of random data.
func randomBlobDataLarge() ([]byte, error) {
	const numBlobs = 6
	size := (numBlobs-1)*fieldparams.BlobSize + 1
	data := make([]byte, size) // Zero-initialized by Go

	// Only randomize first 1KB for uniqueness - no need for full randomness in tests
	const randomSize = 1024
	n, err := mathRand.Read(data[:randomSize]) // #nosec G404
	if err != nil {
		return nil, err
	}
	if n != randomSize {
		return nil, fmt.Errorf("could not create random blob data: %w", err)
	}
	return data, nil
}

func randomAddress() common.Address {
	rNum := mathRand.Int31n(5) // #nosec G404
	switch rNum {
	case 0, 1, 2:
		b := make([]byte, 20)
		_, err := mathRand.Read(b) // #nosec G404
		if err != nil {
			panic(err) // lint:nopanic -- Test code.
		}
		return common.BytesToAddress(b)
	case 3:
		return common.Address{}
	case 4:
		return common.HexToAddress("0xb02A2EdA1b317FBd16760128836B0Ac59B560e9D")
	}
	return common.Address{}
}

func getCaps(rpc *rpc.Client, defaultGasPrice *big.Int) (*big.Int, *big.Int, error) {
	if rpc == nil {
		tip := new(big.Int).Mul(big.NewInt(1), big.NewInt(0).SetUint64(params.BeaconConfig().GweiPerEth))
		if defaultGasPrice.Cmp(tip) >= 0 {
			feeCap := new(big.Int).Sub(defaultGasPrice, tip)
			return tip, feeCap, nil
		}
		return big.NewInt(0), defaultGasPrice, nil
	}
	client := ethclient.NewClient(rpc)
	tip, err := client.SuggestGasTipCap(context.Background())
	if err != nil {
		return nil, nil, err
	}
	feeCap, err := client.SuggestGasPrice(context.Background())
	return tip, feeCap, err
}

func fundAccount(client *rpc.Client, sourceKey, destKey *keystore.Key) error {
	backend := ethclient.NewClient(client)
	defer backend.Close()
	nonce, err := backend.PendingNonceAt(context.Background(), sourceKey.Address)
	if err != nil {
		return err
	}
	chainid, err := backend.ChainID(context.Background())
	if err != nil {
		return err
	}
	expectedPrice, err := backend.SuggestGasPrice(context.Background())
	if err != nil {
		return err
	}
	// Increased funding to 100 million ETH to handle extended test runs with blob transactions
	val, ok := big.NewInt(0).SetString("100000000000000000000000000", 10)
	if !ok {
		return errors.New("could not set big int for value")
	}
	tx := types.NewTransaction(nonce, destKey.Address, val, 100000, expectedPrice, nil)
	signedTx, err := types.SignTx(tx, types.NewLondonSigner(chainid), sourceKey.PrivateKey)
	if err != nil {
		return err
	}
	return backend.SendTransaction(context.Background(), signedTx)
}

// generateRandomEVMCode generates random but valid-looking EVM bytecode
// This mimics what tx-fuzz's RandomCode did (which used FuzzyVM's generator)
func generateRandomEVMCode(maxLen int) []byte {
	if maxLen == 0 {
		return []byte{}
	}

	// Common EVM opcodes that are safe for testing
	// Including: PUSH, DUP, SWAP, arithmetic, logic, and STOP
	safeOpcodes := []byte{
		0x00, // STOP
		0x01, // ADD
		0x02, // MUL
		0x03, // SUB
		0x04, // DIV
		0x10, // LT
		0x11, // GT
		0x14, // EQ
		0x16, // AND
		0x17, // OR
		0x18, // XOR
		0x50, // POP
		0x52, // MSTORE
		0x54, // SLOAD
		0x55, // SSTORE
		0x56, // JUMP
		0x57, // JUMPI
		0x58, // PC
		0x59, // MSIZE
		0x5A, // GAS
		0x60, // PUSH1
		0x80, // DUP1
		0x90, // SWAP1
	}

	code := make([]byte, 0, maxLen)
	for i := 0; i < maxLen; i++ {
		opcode := safeOpcodes[mathRand.Intn(len(safeOpcodes))] // #nosec G404
		code = append(code, opcode)

		// If PUSH1, add a random byte
		if opcode == 0x60 && i+1 < maxLen {
			code = append(code, byte(mathRand.Intn(256))) // #nosec G404
			i++
		}
	}

	return code
}

// randomValidTx generates a random valid transaction
// This replaces tx-fuzz's RandomValidTx functionality
func randomValidTx(sender common.Address, nonce uint64, gasPrice, chainID *big.Int, forceAccessList bool) (*types.Transaction, error) {
	gas := uint64(21000 + mathRand.Intn(100000)) // #nosec G404
	to := randomAddress()
	code := generateRandomEVMCode(mathRand.Intn(256)) // #nosec G404
	value := big.NewInt(0)

	// Randomly choose transaction type
	// 0: Legacy, 1: AccessList, 2: DynamicFee
	txType := mathRand.Intn(3) // #nosec G404
	if forceAccessList {
		txType = 1 // Force AccessList type
	}

	switch txType {
	case 0:
		// Legacy transaction
		return types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			To:       &to,
			Value:    value,
			Gas:      gas,
			GasPrice: gasPrice,
			Data:     code,
		}), nil
	case 1:
		// AccessList transaction
		accessList := make(types.AccessList, 0)
		// Optionally add some random access list entries
		// #nosec G404 -- Test code uses deterministic randomness
		if mathRand.Intn(2) == 0 {
			// #nosec G404 -- Test code uses deterministic randomness
			numEntries := mathRand.Intn(3) + 1
			for range numEntries {
				addr := randomAddress()
				storageKeys := make([]common.Hash, mathRand.Intn(3)) // #nosec G404
				for j := range storageKeys {
					b := make([]byte, 32)
					_, _ = mathRand.Read(b) // #nosec G404
					storageKeys[j] = common.BytesToHash(b)
				}
				accessList = append(accessList, types.AccessTuple{
					Address:     addr,
					StorageKeys: storageKeys,
				})
			}
		}
		return types.NewTx(&types.AccessListTx{
			ChainID:    chainID,
			Nonce:      nonce,
			To:         &to,
			Value:      value,
			Gas:        gas,
			GasPrice:   gasPrice,
			Data:       code,
			AccessList: accessList,
		}), nil
	case 2:
		// DynamicFee transaction (EIP-1559)
		tip := new(big.Int).Div(gasPrice, big.NewInt(10)) // 10% tip
		feeCap := new(big.Int).Add(gasPrice, tip)
		accessList := make(types.AccessList, 0)
		return types.NewTx(&types.DynamicFeeTx{
			ChainID:    chainID,
			Nonce:      nonce,
			To:         &to,
			Value:      value,
			Gas:        gas,
			GasTipCap:  tip,
			GasFeeCap:  feeCap,
			Data:       code,
			AccessList: accessList,
		}), nil
	}

	return nil, errors.New("invalid transaction type")
}
