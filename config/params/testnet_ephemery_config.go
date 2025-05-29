package params

import (
	"math"
	"time"
)

// UseEphemeryNetworkConfig uses the Ephemery beacon chain specific network config.
func UseEphemeryNetworkConfig() {
	cfg := BeaconNetworkConfig().Copy()
	cfg.ContractDeploymentBlock = 0
	cfg.BootstrapNodes = []string{
		// Teku
		"enr:-Iq4QNMYHuJGbnXyBj6FPS2UkOQ-hnxT-mIdNMMr7evR9UYtLemaluorL6J10RoUG1V4iTPTEbl3huijSNs5_ssBWFiGAYhBNHOzgmlkgnY0gmlwhIlKy_CJc2VjcDI1NmsxoQNULnJBzD8Sakd9EufSXhM4rQTIkhKBBTmWVJUtLCp8KoN1ZHCCIyk",
		// pk910
		"enr:-Iq4QIc297-de1P6hznMX2cIdVsQkve9BD9NUsJ7vVQa7eh5UpekA9rLid5A-yLiS3gZwOGugYZPi58x76zNs2cEQFCGAYhBJlTYgmlkgnY0gmlwhEFtmi6Jc2VjcDI1NmsxoQJDyix-IHa_mVwLBEN9NeG8I-RUjNQK_MGxk9OqRQUAtIN1ZHCCIyg",
	}
	OverrideBeaconNetworkConfig(cfg)
}

// EphemeryConfig defines the config for the Ephemery beacon chain testnet.
func EphemeryConfig() *BeaconChainConfig {
	cfg := MainnetConfig()
	cfg.ConfigName = EphemeryName
	cfg.MinGenesisActiveValidatorCount = 64

	// Time parameters
	cfg.GenesisDelay = 600
	cfg.SecondsPerETH1Block = 12
	cfg.Eth1FollowDistance = 12
	cfg.Period = 2419200

	// Calculate the number of ephemery iterations that have elapsed since genesis 0 to get the latest ChainID
	genesisZero := int64(1393527600)
	now := time.Now().Unix()
	difference := now - genesisZero
	iteration := difference / int64(cfg.Period)

	// Calculate the MinGenesisTime of the iteration
	cfg.MinGenesisTime = uint64(genesisZero+iteration*int64(cfg.Period)) + cfg.GenesisDelay

	// Validator cycle
	cfg.InactivityScoreBias = 4
	cfg.InactivityScoreRecoveryRate = 16
	cfg.EjectionBalance = 30000000000
	cfg.MinPerEpochChurnLimit = 4
	cfg.ChurnLimitQuotient = 65536
	cfg.MaxPerEpochActivationChurnLimit = 8

	// Deposit contract
	cfg.DepositChainID = uint64(iteration + 39438000)   // 39438000 is the genesisZero chainID
	cfg.DepositNetworkID = uint64(iteration + 39438000) // 39438000 is the genesisZero networkId
	cfg.DepositContractAddress = "0x00000000219ab540356cBB839Cbe05303d7705Fa"

	// Fork versions
	cfg.GenesisValidatorsRoot = [32]byte{}
	cfg.GenesisForkVersion = []byte{0x10, 0x00, 0x10, 0x1b}
	cfg.AltairForkEpoch = 0
	cfg.AltairForkVersion = []byte{0x20, 0x00, 0x10, 0x1b}
	cfg.BellatrixForkEpoch = 0
	cfg.BellatrixForkVersion = []byte{0x30, 0x00, 0x10, 0x1b}
	cfg.CapellaForkEpoch = 0
	cfg.CapellaForkVersion = []byte{0x40, 0x00, 0x10, 0x1b}
	cfg.DenebForkEpoch = 0
	cfg.DenebForkVersion = []byte{0x50, 0x00, 0x10, 0x1b}
	cfg.ElectraForkEpoch = 10
	cfg.ElectraForkVersion = []byte{0x60, 0x00, 0x10, 0x1b}
	cfg.FuluForkEpoch = math.MaxUint64
	cfg.FuluForkVersion = []byte{0x70, 0x00, 0x10, 0x1b}

	cfg.TerminalTotalDifficulty = "0"
	cfg.TerminalBlockHash = [32]byte{}
	cfg.TerminalBlockHashActivationEpoch = math.MaxUint64

	cfg.InitializeForkSchedule()
	return cfg
}
