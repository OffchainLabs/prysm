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
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	gethCrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	gethparams "github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/runtime/interop"
	e2e "github.com/prysmaticlabs/prysm/v5/testing/endtoend/params"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const txCount = 20

type txType int

const (
	RandomTx txType = iota
	ConsolidationTx
	WithdrawalTx
)

var fundedAccount *keystore.Key

type TransactionGenerator struct {
	keystore  string
	seed      int64
	started   chan struct{}
	cancel    context.CancelFunc
	paused    bool
	txGenType txType
}

func (t *TransactionGenerator) UnderlyingProcess() *os.Process {
	// Transaction Generator runs under the same underlying process so
	// we return an empty process object.
	return &os.Process{}
}

func NewTransactionGenerator(keystore string, seed int64) *TransactionGenerator {
	return &TransactionGenerator{keystore: keystore, seed: seed, txGenType: RandomTx}
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
		logrus.Infof("Seed for transaction generator is: %d", seed)
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
			switch t.txGenType {
			case ConsolidationTx:
				err = SendConsolidationTransaction(mineKey.PrivateKey, gasPrice, backend)
				if err != nil {
					return err
				}
			case WithdrawalTx:
				err = SendWithdrawalTransaction(mineKey.PrivateKey, newKey.PrivateKey, gasPrice, backend)
				if err != nil {
					return err
				}
			case RandomTx:
				err = SendTransaction(client, mineKey.PrivateKey, f, gasPrice, mineKey.Address.String(), txCount, backend, false)
				if err != nil {
					return err
				}
			default:
				logrus.Warnf("Unknown transaction type: %v", t.txGenType)
			}

			backend.Close()
		}
	}
}

// Started checks whether beacon node set is started and all nodes are ready to be queried.
func (s *TransactionGenerator) Started() <-chan struct{} {
	return s.started
}

func SendTransaction(client *rpc.Client, key *ecdsa.PrivateKey, f *filler.Filler, gasPrice *big.Int, addr string, N uint64, backend *ethclient.Client, al bool) error {
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
	g, _ := errgroup.WithContext(context.Background())
	txs := make([]*types.Transaction, 10)
	for i := uint64(0); i < 10; i++ {
		index := i
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

	txs = make([]*types.Transaction, N)
	for i := uint64(0); i < N; i++ {
		index := i
		g.Go(func() error {
			tx, err := txfuzz.RandomValidTx(client, f, sender, nonce+index, gasPrice, chainid, al)
			if err != nil {
				// In the event the transaction constructed is not valid, we continue with the routine
				// rather than complete stop it.
				//nolint:nilerr
				return nil
			}
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

func SendConsolidationTransaction(key *ecdsa.PrivateKey, gasPrice *big.Int, backend *ethclient.Client) error {
	totalCreds := e2e.TestParams.NumberOfExecutionCreds
	_, pKeys, err := interop.DeterministicallyGenerateKeys(0, totalCreds)
	if err != nil {
		return err
	}
	compoundedKey := pKeys[len(pKeys)-1].Marshal()

	// Create compounding credentials
	if err := createAndSendConsolidation(compoundedKey, compoundedKey, key, gasPrice, backend); err != nil {
		return err
	}
	for _, k := range pKeys {
		if err := createAndSendConsolidation(k.Marshal(), compoundedKey, key, gasPrice, backend); err != nil {
			return err
		}
	}

	// Junk Requests
	for i := 0; i < 2; i++ {
		sourcePubkey := [48]byte{byte(i), 0xFF, 0x34, 0xEE}
		targetPubkey := compoundedKey
		if err := createAndSendConsolidation(sourcePubkey[:], targetPubkey, key, gasPrice, backend); err != nil {
			return err
		}
	}
	return nil
}

func createAndSendConsolidation(sourceKey, targetKey []byte, key *ecdsa.PrivateKey, gasPrice *big.Int, backend *ethclient.Client) error {
	publicKey := key.Public().(*ecdsa.PublicKey)
	fromAddress := gethCrypto.PubkeyToAddress(*publicKey)
	// Get nonce
	nonce, err := backend.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return err
	}
	chainid, err := backend.ChainID(context.Background())
	if err != nil {
		return err
	}
	gasLimit := uint64(200000)

	sourcePubkey := sourceKey
	targetPubkey := targetKey

	consolidationData := []byte{}
	consolidationData = append(consolidationData, sourcePubkey...)
	consolidationData = append(consolidationData, targetPubkey...)

	ret, err := backend.CallContract(context.Background(), ethereum.CallMsg{To: &gethparams.ConsolidationQueueAddress}, nil)
	if err != nil {
		return errors.Wrapf(err, "%s", string(ret))
	}
	fee := new(big.Int).SetBytes(ret)
	fee = fee.Mul(fee, big.NewInt(2))

	// Create transaction
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &gethparams.ConsolidationQueueAddress,
		Value:    fee,
		Gas:      gasLimit,
		GasPrice: gasPrice,
		Data:     consolidationData,
	})

	// Sign transaction
	signedTx, err := types.SignTx(tx, types.NewCancunSigner(chainid), key)
	if err != nil {
		return err
	}
	return backend.SendTransaction(context.Background(), signedTx)
}

func SendWithdrawalTransaction(key, newKey *ecdsa.PrivateKey, gasPrice *big.Int, backend *ethclient.Client) error {
	totalCreds := e2e.TestParams.NumberOfExecutionCreds
	_, pKeys, err := interop.DeterministicallyGenerateKeys(0, totalCreds)
	if err != nil {
		return err
	}
	compoundedKey := pKeys[len(pKeys)-1].Marshal()

	_, invalidWithdrawalKeys, err := interop.DeterministicallyGenerateKeys(totalCreds, totalCreds+4)
	if err != nil {
		return err
	}

	publicKey := key.Public().(*ecdsa.PublicKey)
	fromAddress := gethCrypto.PubkeyToAddress(*publicKey)
	nonce, err := backend.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return err
	}

	var withdrawalTxs []*types.Transaction
	// Create Withdrawal for compounded key.
	tx, err := createWithdrawal(compoundedKey, 0, nonce, key, gasPrice, backend)
	if err != nil {
		return err
	}
	withdrawalTxs = append(withdrawalTxs, tx)
	nonce++

	rGen := rand.NewDeterministicGenerator()
	for _, k := range pKeys {
		tx, err := createWithdrawal(k.Marshal(), uint64(rGen.Int63n(32000000000)), nonce, key, gasPrice, backend)
		if err != nil {
			return err
		}
		withdrawalTxs = append(withdrawalTxs, tx)
		nonce++
	}

	// Junk Requests
	for _, k := range invalidWithdrawalKeys {
		tx, err := createWithdrawal(k.Marshal(), uint64(rGen.Int63n(32000000000)), nonce, newKey, gasPrice, backend)
		if err != nil {
			return err
		}
		withdrawalTxs = append(withdrawalTxs, tx)
		nonce++
	}
	currExecHead := uint64(0)
	// Batch And Send Withdrawals
	for len(withdrawalTxs) > 0 {
		currBlock, err := backend.BlockNumber(context.Background())
		if err != nil {
			return err
		}
		if currBlock > currExecHead {
			currExecHead = currBlock
			maxWithdrawalPerPayload := params.BeaconConfig().MaxWithdrawalRequestsPerPayload
			if maxWithdrawalPerPayload > uint64(len(withdrawalTxs)) {
				maxWithdrawalPerPayload = uint64(len(withdrawalTxs))
			}
			for _, tx := range withdrawalTxs[:maxWithdrawalPerPayload] {
				if err := backend.SendTransaction(context.Background(), tx); err != nil {
					return err
				}
			}
			// Shift slice to only have unsent transactions
			withdrawalTxs = withdrawalTxs[maxWithdrawalPerPayload:]
			time.Sleep(2 * time.Second)
		}
	}
	return nil
}

func createWithdrawal(sourceKey []byte, amount, nonce uint64, key *ecdsa.PrivateKey, gasPrice *big.Int, backend *ethclient.Client) (*types.Transaction, error) {
	chainid, err := backend.ChainID(context.Background())
	if err != nil {
		return nil, err
	}
	gasLimit := uint64(200000)

	withdrawalData := []byte{}
	withdrawalData = append(withdrawalData, sourceKey...)
	withdrawalData = append(withdrawalData, bytesutil.Uint64ToBytesBigEndian(amount)...)

	ret, err := backend.CallContract(context.Background(), ethereum.CallMsg{To: &gethparams.WithdrawalQueueAddress}, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "%s", string(ret))
	}
	fee := new(big.Int).SetBytes(ret)
	fee = fee.Mul(fee, big.NewInt(2))

	// Create transaction
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &gethparams.WithdrawalQueueAddress,
		Value:    fee,
		Gas:      gasLimit,
		GasPrice: gasPrice,
		Data:     withdrawalData,
	})

	// Sign transaction
	signedTx, err := types.SignTx(tx, types.NewCancunSigner(chainid), key)
	if err != nil {
		return nil, err
	}
	return signedTx, nil
}

func (t *TransactionGenerator) SetTxType(typ txType) {
	t.txGenType = typ
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

		// TODO: replace call with al, err := txfuzz.CreateAccessList(rpc, tx, sender) when txfuzz is fixed in new release
		// an error occurs mentioning error="CreateAccessList: both gasPrice and (maxFeePerGas or maxPriorityFeePerGas) specified"
		msg := ethereum.CallMsg{
			From:       sender,
			To:         tx.To(),
			Gas:        tx.Gas(),
			GasPrice:   tx.GasPrice(),
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
	val, ok := big.NewInt(0).SetString("10000000000000000000000000", 10)
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
