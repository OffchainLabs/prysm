package eth1

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"
	"math/big"
	mathRand "math/rand"
	"os"
	"time"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	txfuzz "github.com/MariusVanDerWijden/tx-fuzz"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	e2e "github.com/OffchainLabs/prysm/v6/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v6/time/slots"
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
	keystore string
	seed     int64
	started  chan struct{}
	cancel   context.CancelFunc
	paused   bool
}

func (t *TransactionGenerator) UnderlyingProcess() *os.Process {
	// Transaction Generator runs under the same underlying process so
	// we return an empty process object.
	return &os.Process{}
}

func NewTransactionGenerator(keystore string, seed int64) *TransactionGenerator {
	return &TransactionGenerator{keystore: keystore, seed: seed}
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
	rnd := make([]byte, 10000)
	_, err = mathRand.Read(rnd) // #nosec G404
	if err != nil {
		return err
	}
	f := filler.NewFiller(rnd)
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
			err = SendTransaction(client, mineKey.PrivateKey, f, gasPrice, mineKey.Address.String(), txCount, backend, false)
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

func SendTransaction(client *rpc.Client, key *ecdsa.PrivateKey, f *filler.Filler, gasPrice *big.Int, addr string, txCount uint64, backend *ethclient.Client, al bool) error {
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

	// Check if we're near or post-Fulu fork
	currentSlot := slots.CurrentSlot(e2e.TestParams.CLGenesisTime)
	currentEpoch := slots.ToEpoch(currentSlot)
	fuluForkEpoch := params.BeaconConfig().FuluForkEpoch

	// Stop sending blob transactions 1 epoch before Fulu fork to avoid execution client bug at fork boundary
	nearFuluFork := fuluForkEpoch > 0 && currentEpoch == (fuluForkEpoch-1)
	isPostFulu := currentEpoch >= fuluForkEpoch

	g, _ := errgroup.WithContext(context.Background())
	txs := make([]*types.Transaction, 10)

	// Send blob transactions - skip near fork boundary, use different versions pre/post Fulu
	if nearFuluFork && !isPostFulu {
		logrus.WithFields(logrus.Fields{
			"currentEpoch":  currentEpoch,
			"fuluForkEpoch": fuluForkEpoch,
		}).Infof("Near Fulu fork boundary: Skipping blob transactions to avoid execution client bug")
	} else if isPostFulu {
		logrus.Info("Post-Fulu: Sending blob transactions with Version 1 sidecars (cell proofs)")
		// Reduced from 10 to 5 to conserve funds during extended test runs
		for index := range uint64(5) {

			g.Go(func() error {
				tx, err := RandomBlobCellTx(client, f, fundedAccount.Address, nonce+index, gasPrice, chainid, al)
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
	} else {
		logrus.Info("Pre-Fulu: Sending standard blob transactions with Version 0 sidecars")
		// Reduced from 10 to 5 to conserve funds during extended test runs
		for index := range uint64(5) {

			g.Go(func() error {
				tx, err := RandomBlobTx(client, f, fundedAccount.Address, nonce+index, gasPrice, chainid, al)
				if err != nil {
					logrus.WithError(err).Error("Could not create blob tx")
					// In the event the transaction constructed is not valid, we continue with the routine
					// rather than complete stop it.
					//nolint:nilerr
					return nil
				}

				signedTx, err := types.SignTx(tx, types.NewCancunSigner(chainid), fundedAccount.PrivateKey)
				if err != nil {
					logrus.WithError(err).Error("Could not sign blob tx")
					// We continue on in the event there is a reason we can't sign this
					// transaction(unlikely).
					//nolint:nilerr
					return nil
				}

				txs[index] = signedTx
				return nil
			})
		}
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

	nonce, err = backend.PendingNonceAt(context.Background(), sender)
	if err != nil {
		return err
	}

	txs = make([]*types.Transaction, txCount)
	for index := range txCount {

		g.Go(func() error {
			tx, err := txfuzz.RandomValidTx(client, f, sender, nonce+index, gasPrice, chainid, al)
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

func RandomBlobCellTx(rpc *rpc.Client, f *filler.Filler, sender common.Address, nonce uint64, gasPrice, chainID *big.Int, al bool) (*types.Transaction, error) {
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
	code := txfuzz.RandomCode(f)
	value := big.NewInt(0)

	if len(code) > 128 {
		code = code[:128]
	}

	mod := 2
	if al {
		mod = 1
	}

	switch f.Byte() % byte(mod) {
	case 0:
		// Blob transaction with cell proofs (Version 1 sidecar)
		tip, feecap, err := getCaps(rpc, gasPrice)
		if err != nil {
			return nil, errors.Wrap(err, "getCaps")
		}

		data, err := randomBlobData()
		if err != nil {
			return nil, errors.Wrap(err, "randomBlobData")
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
		data, err := randomBlobData()
		if err != nil {
			return nil, errors.Wrap(err, "randomBlobData")
		}

		return New4844CellTx(nonce, &to, gas, chainID, tip, feecap, value, code, big.NewInt(1000000), data, *al)
	}

	return nil, nil
}

func RandomBlobTx(rpc *rpc.Client, f *filler.Filler, sender common.Address, nonce uint64, gasPrice, chainID *big.Int, al bool) (*types.Transaction, error) {
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
	code := txfuzz.RandomCode(f)
	value := big.NewInt(0)
	if len(code) > 128 {
		code = code[:128]
	}
	mod := 2
	if al {
		mod = 1
	}
	switch f.Byte() % byte(mod) {
	case 0:
		// 4844 transaction without AL

		tip, feecap, err := getCaps(rpc, gasPrice)
		if err != nil {
			return nil, errors.Wrap(err, "getCaps")
		}

		data, err := randomBlobData()
		if err != nil {
			return nil, errors.Wrap(err, "randomBlobData")
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
		data, err := randomBlobData()
		if err != nil {
			return nil, errors.Wrap(err, "randomBlobData")
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
	for i := 0; i < len(data); i += 31 {
		fieldIndex++
		if fieldIndex == numOfElems {
			if blobIndex >= 1 {
				break
			}
			blobs = append(blobs, kzg4844.Blob{})
			blobIndex++
			fieldIndex = 0
		}
		max := i + 31
		if max > len(data) {
			max = len(data)
		}
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
