package params

import (
	eth1Params "github.com/ethereum/go-ethereum/params"
)

// UseSepoliaNetworkConfig uses the Sepolia beacon chain specific network config.
func UseSepoliaNetworkConfig() {
	cfg := BeaconNetworkConfig().Copy()
	cfg.ContractDeploymentBlock = 1273020
	cfg.BootstrapNodes = []string{
		"enr:-KG4QCK5YeEoL55e2hoS6nCregwx0Zd6NQ3rhVDfeg5Q8ozUNmUYTskpmuqo2WYFo3z24-cWC9qrU3yYKDSJ299lh8sBgmlkgnY0gmlwhNRj2kKDaXA2kCoAHKALAA0CAAAAAAAAAF6Jc2VjcDI1NmsxoQLzqnTxu_nlM8V_semraAjfbH9HZpcbVUCXH2qanVPsroN1ZHCCTruEdWRwNoJOuw",
		"enr:-KG4QF0FvRL7Eqc4oURFhOkS0V6guntLnw54dYgTruM7z9TAMWhRpCrxZ7Pd536-q4qlwdW13czht8_UEWwGyJesu1gBgmlkgnY0gmlwhIHUpj2DaXA2kCYEqIAABAHQAAAAA2GdUACJc2VjcDI1NmsxoQL5iA7gNCs4SDmnXz8Isacq0EJbJfvV_uJlccoHxHU5ZYN1ZHCCI4yEdWRwNoIjjA",
		"enr:-KG4QI4reJ1D_BwCwg6EKAuo2HEWoIVVNjphtOTJP2gzPVLSTYM3NFwp39TAKw-7QiQ2NVts7DK4rjJR2BEcAwh3BckBgmlkgnY0gmlwhJB-_BiDaXA2kCQAYYABAADQAAAAAYEgYAGJc2VjcDI1NmsxoQLXzHa5K0M3F4pqErIhleMByA8votAUhUXylRT6SWX2HoN1ZHCCI4yEdWRwNoIjjA",
		"enr:-KG4QI1KOrogxK8u3Oc0QLdgkNTbAPuAMtixa6Vx05N-Bl7IOCVURUvqZ2N6JA97ts7YG1B4D3hQvZ9uQlCPYVjy1DABgmlkgnY0gmlwhLKc14yDaXA2kCoBBP8A9DxKAAAAAAAAAAGJc2VjcDI1NmsxoQLB0ZhHGRmVwXja_4o-GRN1VVJYRI11F45CTAlu1s00Q4N1ZHCCI4yEdWRwNoIjjA",
		"enr:-KG4QKU4YfXfB3_BVI7u0VvXnSJI6cqo-tCRm-Ggh3XxBImcYvrUoKUDbIjJjG9-QphuH6gzScdf69t597M0nHut4kABgmlkgnY0gmlwhAXfXlGDaXA2kCoBBP8C8BytAAAAAAAAAAGJc2VjcDI1NmsxoQL0y83XKpPgvY7XReWg9S8bdI2UUIe5dE0N7rjOIIj4xYN1ZHCCI4yEdWRwNoIjjA",
		"enr:-KO4QP7MmB3juk8rUjJHcUoxZDU9Np4FlW0HyDEGIjSO7GD9PbSsabu7713cWSUWKDkxIypIXg1A-6lG7ySRGOMZHeGCAmuEZXRoMpDTH2GRkAAAc___________gmlkgnY0gmlwhBSoyGOJc2VjcDI1NmsxoQNta5b_bexSSwwrGW2Re24MjfMntzFd0f2SAxQtMj3ueYN0Y3CCIyiDdWRwgiMo",
		"enr:-KG4QJejf8KVtMeAPWFhN_P0c4efuwu1pZHELTveiXUeim6nKYcYcMIQpGxxdgT2Xp9h-M5pr9gn2NbbwEAtxzu50Y8BgmlkgnY0gmlwhEEVkQCDaXA2kCoBBPnAEJg4AAAAAAAAAAGJc2VjcDI1NmsxoQLEh_eVvk07AQABvLkTGBQTrrIOQkzouMgSBtNHIRUxOIN1ZHCCIyiEdWRwNoIjKA",
		"enr:-Iq4QMCTfIMXnow27baRUb35Q8iiFHSIDBJh6hQM5Axohhf4b6Kr_cOCu0htQ5WvVqKvFgY28893DHAg8gnBAXsAVqmGAX53x8JggmlkgnY0gmlwhLKAlv6Jc2VjcDI1NmsxoQK6S-Cii_KmfFdUJL2TANL3ksaKUnNXvTCv1tLwXs0QgIN1ZHCCIyk",
		"enr:-L64QC9Hhov4DhQ7mRukTOz4_jHm4DHlGL726NWH4ojH1wFgEwSin_6H95Gs6nW2fktTWbPachHJ6rUFu0iJNgA0SB2CARqHYXR0bmV0c4j__________4RldGgykDb6UBOQAABx__________-CaWSCdjSCaXCEA-2vzolzZWNwMjU2azGhA17lsUg60R776rauYMdrAz383UUgESoaHEzMkvm4K6k6iHN5bmNuZXRzD4N0Y3CCIyiDdWRwgiMo",
	}
	OverrideBeaconNetworkConfig(cfg)
}

// SepoliaConfig defines the config for the Sepolia beacon chain testnet.
func SepoliaConfig() *BeaconChainConfig {
	cfg := MainnetConfig()
	cfg.MinGenesisTime = 1655647200
	cfg.GenesisDelay = 86400
	cfg.MinGenesisActiveValidatorCount = 1300
	cfg.GenesisValidatorsRoot = [32]byte{216, 234, 23, 31, 60, 148, 174, 162, 30, 188, 66, 161, 237, 97, 5, 42, 207, 63, 146, 9, 192, 14, 78, 251, 170, 221, 172, 9, 237, 155, 128, 120}
	cfg.ConfigName = SepoliaName
	cfg.GenesisForkVersion = []byte{0x90, 0x00, 0x00, 0x69}
	cfg.SecondsPerETH1Block = 14
	cfg.DepositChainID = eth1Params.SepoliaChainConfig.ChainID.Uint64()
	cfg.DepositNetworkID = eth1Params.SepoliaChainConfig.ChainID.Uint64()
	cfg.AltairForkEpoch = 50
	cfg.AltairForkVersion = []byte{0x90, 0x00, 0x00, 0x70}
	cfg.BellatrixForkEpoch = 100
	cfg.BellatrixForkVersion = []byte{0x90, 0x00, 0x00, 0x71}
	cfg.CapellaForkEpoch = 56832
	cfg.CapellaForkVersion = []byte{0x90, 0x00, 0x00, 0x72}
	cfg.DenebForkEpoch = 132608
	cfg.DenebForkVersion = []byte{0x90, 0x00, 0x00, 0x73}
	cfg.ElectraForkEpoch = 222464 // Wed, Mar 5 at 07:29:36 UTC
	cfg.ElectraForkVersion = []byte{0x90, 0x00, 0x00, 0x74}
	cfg.FuluForkEpoch = 272640 // 2025-10-14 07:36:00 UTC
	cfg.FuluForkVersion = []byte{0x90, 0x00, 0x00, 0x75}
	cfg.GloasForkVersion = []byte{0x90, 0x00, 0x00, 0x76}
	cfg.TerminalTotalDifficulty = "17000000000000000"
	cfg.DepositContractAddress = "0x7f02C3E3c98b133055B8B348B2Ac625669Ed295D"
	cfg.DefaultBuilderGasLimit = uint64(60000000)
	cfg.BlobSchedule = []BlobScheduleEntry{
		{
			MaxBlobsPerBlock: 15,
			Epoch:            274176, // 2025-10-21 03:26:24 UTC
		},
		{
			MaxBlobsPerBlock: 21,
			Epoch:            275712, // 2025-10-27 23:16:48 UTC
		},
	}
	cfg.InitializeForkSchedule()
	return cfg
}
