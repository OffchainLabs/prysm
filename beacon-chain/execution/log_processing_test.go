package execution

import (
	"encoding/binary"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache/depositsnapshot"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	contracts "github.com/OffchainLabs/prysm/v7/contracts/deposit"
	"github.com/OffchainLabs/prysm/v7/contracts/deposit/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestProcessDepositLog_OK(t *testing.T) {
	hook := logTest.NewGlobal()

	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")

	beaconDB := testDB.SetupDB(t)
	depositCache, err := depositsnapshot.New()
	require.NoError(t, err)

	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(t.Context(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
		WithDepositCache(depositCache),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)
	web3Service.depositContractCaller, err = contracts.NewDepositContractCaller(testAcc.ContractAddr, testAcc.Backend.Client())
	require.NoError(t, err)

	testAcc.Backend.Commit()

	deposits, _, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)

	_, depositRoots, err := util.DeterministicDepositTrie(len(deposits))
	require.NoError(t, err)
	data := deposits[0].Data

	testAcc.TxOpts.Value = mock.Amount32Eth()
	testAcc.TxOpts.GasLimit = 1000000
	_, err = testAcc.Contract.Deposit(testAcc.TxOpts, data.PublicKey, data.WithdrawalCredentials, data.Signature, depositRoots[0])
	require.NoError(t, err, "Could not deposit to deposit contract")

	testAcc.Backend.Commit()

	query := ethereum.FilterQuery{
		Addresses: []common.Address{
			web3Service.cfg.depositContractAddr,
		},
	}

	logs, err := testAcc.Backend.Client().FilterLogs(web3Service.ctx, query)
	require.NoError(t, err, "Unable to retrieve logs")

	if len(logs) == 0 {
		t.Fatal("no logs")
	}

	err = web3Service.ProcessLog(t.Context(), &logs[0])
	require.NoError(t, err)

	require.LogsDoNotContain(t, hook, "Could not unpack log")
	require.LogsDoNotContain(t, hook, "Could not save in trie")
	require.LogsDoNotContain(t, hook, "could not deserialize validator public key")
	require.LogsDoNotContain(t, hook, "could not convert bytes to signature")
	require.LogsDoNotContain(t, hook, "could not sign root for deposit data")
	require.LogsDoNotContain(t, hook, "deposit signature did not verify")
	require.LogsDoNotContain(t, hook, "could not tree hash deposit data")
	require.LogsDoNotContain(t, hook, "deposit merkle branch of deposit root did not verify for root")
	require.LogsContain(t, hook, "Deposit registered from deposit contract")

	hook.Reset()
}

func TestProcessDepositLog_InsertsPendingDeposit(t *testing.T) {
	hook := logTest.NewGlobal()
	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	beaconDB := testDB.SetupDB(t)
	depositCache, err := depositsnapshot.New()
	require.NoError(t, err)
	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})

	web3Service, err := NewService(t.Context(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
		WithDepositCache(depositCache),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)
	web3Service.depositContractCaller, err = contracts.NewDepositContractCaller(testAcc.ContractAddr, testAcc.Backend.Client())
	require.NoError(t, err)

	testAcc.Backend.Commit()

	deposits, _, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)
	_, depositRoots, err := util.DeterministicDepositTrie(len(deposits))
	require.NoError(t, err)
	data := deposits[0].Data

	testAcc.TxOpts.Value = mock.Amount32Eth()
	testAcc.TxOpts.GasLimit = 1000000

	_, err = testAcc.Contract.Deposit(testAcc.TxOpts, data.PublicKey, data.WithdrawalCredentials, data.Signature, depositRoots[0])
	require.NoError(t, err, "Could not deposit to deposit contract")

	_, err = testAcc.Contract.Deposit(testAcc.TxOpts, data.PublicKey, data.WithdrawalCredentials, data.Signature, depositRoots[0])
	require.NoError(t, err, "Could not deposit to deposit contract")

	testAcc.Backend.Commit()

	query := ethereum.FilterQuery{
		Addresses: []common.Address{
			web3Service.cfg.depositContractAddr,
		},
	}

	logs, err := testAcc.Backend.Client().FilterLogs(web3Service.ctx, query)
	require.NoError(t, err, "Unable to retrieve logs")

	err = web3Service.ProcessDepositLog(t.Context(), &logs[0])
	require.NoError(t, err)
	err = web3Service.ProcessDepositLog(t.Context(), &logs[1])
	require.NoError(t, err)

	pendingDeposits := web3Service.cfg.depositCache.PendingDeposits(t.Context(), nil /*blockNum*/)
	require.Equal(t, 2, len(pendingDeposits), "Unexpected number of deposits")

	hook.Reset()
}

func TestUnpackDepositLogData_OK(t *testing.T) {
	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	beaconDB := testDB.SetupDB(t)
	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(t.Context(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)
	web3Service.depositContractCaller, err = contracts.NewDepositContractCaller(testAcc.ContractAddr, testAcc.Backend.Client())
	require.NoError(t, err)

	testAcc.Backend.Commit()

	deposits, _, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)
	_, depositRoots, err := util.DeterministicDepositTrie(len(deposits))
	require.NoError(t, err)
	data := deposits[0].Data

	testAcc.TxOpts.Value = mock.Amount32Eth()
	testAcc.TxOpts.GasLimit = 1000000
	_, err = testAcc.Contract.Deposit(testAcc.TxOpts, data.PublicKey, data.WithdrawalCredentials, data.Signature, depositRoots[0])
	require.NoError(t, err, "Could not deposit to deposit contract")
	testAcc.Backend.Commit()

	query := ethereum.FilterQuery{
		Addresses: []common.Address{
			web3Service.cfg.depositContractAddr,
		},
	}

	logz, err := testAcc.Backend.Client().FilterLogs(web3Service.ctx, query)
	require.NoError(t, err, "Unable to retrieve logs")

	loggedPubkey, withCreds, _, loggedSig, index, err := contracts.UnpackDepositLogData(logz[0].Data)
	require.NoError(t, err, "Unable to unpack logs")

	require.Equal(t, uint64(0), binary.LittleEndian.Uint64(index), "Retrieved merkle tree index is incorrect")
	require.DeepEqual(t, data.PublicKey, loggedPubkey, "Pubkey is not the same as the data that was put in")
	require.DeepEqual(t, data.Signature, loggedSig, "Proof of Possession is not the same as the data that was put in")
	require.DeepEqual(t, data.WithdrawalCredentials, withCreds, "Withdrawal Credentials is not the same as the data that was put in")
}

func TestProcessPastLogs_ProcessesAllDeposits(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	kvStore := testDB.SetupDB(t)
	depositCache, err := depositsnapshot.New()
	require.NoError(t, err)
	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})

	web3Service, err := NewService(t.Context(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(kvStore),
		WithDepositCache(depositCache),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)
	web3Service.depositContractCaller, err = contracts.NewDepositContractCaller(testAcc.ContractAddr, testAcc.Backend.Client())
	require.NoError(t, err)
	web3Service.rpcClient = &mockExecution.RPCClient{Backend: testAcc.Backend}
	web3Service.httpLogger = testAcc.Backend.Client()
	web3Service.latestEth1Data.LastRequestedBlock = 0
	block, err := testAcc.Backend.Client().BlockByNumber(t.Context(), nil)
	require.NoError(t, err)
	web3Service.latestEth1Data.BlockHeight = block.NumberU64()
	web3Service.latestEth1Data.BlockTime = block.Time()
	bConfig := params.MinimalSpecConfig().Copy()
	bConfig.MinGenesisTime = 0
	bConfig.SecondsPerETH1Block = 1
	params.OverrideBeaconConfig(bConfig)
	nConfig := params.BeaconNetworkConfig()
	nConfig.ContractDeploymentBlock = 0
	params.OverrideBeaconNetworkConfig(nConfig)

	testAcc.Backend.Commit()

	totalNumOfDeposits := 94

	deposits, _, err := util.DeterministicDepositsAndKeys(uint64(totalNumOfDeposits))
	require.NoError(t, err)
	_, depositRoots, err := util.DeterministicDepositTrie(len(deposits))
	require.NoError(t, err)
	depositOffset := 5

	for i := range totalNumOfDeposits {
		data := deposits[i].Data
		testAcc.TxOpts.Value = mock.Amount32Eth()
		testAcc.TxOpts.GasLimit = 1000000
		_, err = testAcc.Contract.Deposit(testAcc.TxOpts, data.PublicKey, data.WithdrawalCredentials, data.Signature, depositRoots[i])
		require.NoError(t, err, "Could not deposit to deposit contract")
		// pack 8 deposits into a block with an offset of
		// 5
		if (i+1)%8 == depositOffset {
			testAcc.Backend.Commit()
		}
	}
	// Mine the trailing deposits, then forward the chain past the follow distance.
	testAcc.Backend.Commit()
	for i := uint64(0); i < params.BeaconConfig().Eth1FollowDistance; i++ {
		testAcc.Backend.Commit()
	}
	b, err := testAcc.Backend.Client().BlockByNumber(t.Context(), nil)
	require.NoError(t, err)
	web3Service.latestEth1Data.BlockHeight = b.NumberU64()
	web3Service.latestEth1Data.BlockTime = b.Time()

	require.NoError(t, web3Service.processPastLogs(t.Context()))

	// Every deposit log behind the follow distance lands in the trie and the pending cache.
	require.Equal(t, int64(totalNumOfDeposits-1), web3Service.lastReceivedMerkleIndex)
	require.Equal(t, totalNumOfDeposits, len(web3Service.cfg.depositCache.PendingDeposits(t.Context(), nil)))
}

func TestProcessLogs_DepositRequestsStarted(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	hook := logTest.NewGlobal()
	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	kvStore := testDB.SetupDB(t)
	depositCache, err := depositsnapshot.New()
	require.NoError(t, err)
	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})

	web3Service, err := NewService(t.Context(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(kvStore),
		WithDepositCache(depositCache),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)
	web3Service.depositContractCaller, err = contracts.NewDepositContractCaller(testAcc.ContractAddr, testAcc.Backend.Client())
	require.NoError(t, err)
	web3Service.rpcClient = &mockExecution.RPCClient{Backend: testAcc.Backend}
	web3Service.httpLogger = testAcc.Backend.Client()
	web3Service.latestEth1Data.LastRequestedBlock = 0
	block, err := testAcc.Backend.Client().BlockByNumber(t.Context(), nil)
	require.NoError(t, err)
	web3Service.latestEth1Data.BlockHeight = block.NumberU64()
	web3Service.latestEth1Data.BlockTime = block.Time()
	bConfig := params.MinimalSpecConfig().Copy()
	bConfig.MinGenesisTime = 0
	bConfig.SecondsPerETH1Block = 1
	params.OverrideBeaconConfig(bConfig)
	nConfig := params.BeaconNetworkConfig()
	nConfig.ContractDeploymentBlock = 0
	params.OverrideBeaconNetworkConfig(nConfig)

	testAcc.Backend.Commit()

	totalNumOfDeposits := 94

	deposits, _, err := util.DeterministicDepositsAndKeys(uint64(totalNumOfDeposits))
	require.NoError(t, err)
	_, depositRoots, err := util.DeterministicDepositTrie(len(deposits))
	require.NoError(t, err)
	depositOffset := 5

	for i := range totalNumOfDeposits {
		data := deposits[i].Data
		testAcc.TxOpts.Value = mock.Amount32Eth()
		testAcc.TxOpts.GasLimit = 1000000
		_, err = testAcc.Contract.Deposit(testAcc.TxOpts, data.PublicKey, data.WithdrawalCredentials, data.Signature, depositRoots[i])
		require.NoError(t, err, "Could not deposit to deposit contract")
		// pack 8 deposits into a block with an offset of
		// 5
		if (i+1)%8 == depositOffset {
			testAcc.Backend.Commit()
		}
	}
	// Forward the chain to account for the follow distance
	for i := uint64(0); i < params.BeaconConfig().Eth1FollowDistance; i++ {
		testAcc.Backend.Commit()
	}
	b, err := testAcc.Backend.Client().BlockByNumber(t.Context(), nil)
	require.NoError(t, err)
	web3Service.latestEth1Data.BlockHeight = b.NumberU64()
	web3Service.latestEth1Data.BlockTime = b.Time()

	web3Service.depositRequestsStarted = true
	web3Service.initPOWService()
	require.NoError(t, err)

	require.Equal(t, int64(-1), web3Service.lastReceivedMerkleIndex, "Processed deposit logs even when requests are active")

	hook.Reset()
}
