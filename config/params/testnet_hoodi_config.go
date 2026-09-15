package params

// UseHoodiNetworkConfig uses the Hoodi beacon chain specific network config.
func UseHoodiNetworkConfig() {
	cfg := BeaconNetworkConfig().Copy()
	cfg.ContractDeploymentBlock = 0
	cfg.BootstrapNodes = []string{
		"enr:-KG4QEfvG40PslpTF5F0SAnDMHYwQu7u9dMxVmglDyR0iKEsTUr0MilWHWKPh_Cyo0cHt0muy2SsrWpiC2sC_TRPiMcBgmlkgnY0gmlwhNRj2kKDaXA2kCoAHKALAA0CAAAAAAAAAF6Jc2VjcDI1NmsxoQIM-dQNDiL8ldy7S8t_bkW9awktKz1HHSF2Qups_K5S64N1ZHCCTryEdWRwNoJOvA",
		"enr:-KG4QDNae3UVXdwSvWbZYotO9IpGRiBDzXr4owQ1_ONk_sOtIuZoI55Ja8EGtD-kzY5I_0bTaYpVefgRK2q2hD1T8sABgmlkgnY0gmlwhIHUpj2DaXA2kCYEqIAABAHQAAAAA2GdUACJc2VjcDI1NmsxoQMu3GRf_l288UJNQcXiLp4NbOQmigxSx14ddTal4tBp9IN1ZHCCI_CEdWRwNoIj8A",
		"enr:-KG4QOOHORt2Kmo3lgoRTcqJnxH07aELtuidFEuBzN8Xdbzkfb4MblrUOXJnDEJ8RzXpTXWBEqM3q0DRphMm8xOIVbIBgmlkgnY0gmlwhJB-_BiDaXA2kCQAYYABAADQAAAAAYEgYAGJc2VjcDI1NmsxoQOf6T6A1lri5bTBzvb3sAb42Ki9L1pSqQsNzqvBUr7BjoN1ZHCCI_CEdWRwNoIj8A",
		"enr:-KG4QM0TIrjoocAJvIY2XYOa1UzeSM1c2d3rBf1QzyxchGmzJ3OPdLKUFrjBRCPDYHhq69pEB5YKmFtKOBuF63k1pB4BgmlkgnY0gmlwhLKc14yDaXA2kCoBBP8A9DxKAAAAAAAAAAGJc2VjcDI1NmsxoQNFCY3Kl3VQfYl3lqOTN8YG0598xcIrlg1mmqKzdpLm5IN1ZHCCI_CEdWRwNoIj8A",
		"enr:-KG4QLBt5eeWOp11A7l2WfR-sC5j3SYybU0PeEepotPzpt4kZE0nDFFZCy8NPjun3dcM8D4_xmYxZCB0WTnitKj7dAMBgmlkgnY0gmlwhAXfXlGDaXA2kCoBBP8C8BytAAAAAAAAAAGJc2VjcDI1NmsxoQLhrnwm2X7ZcxLideAlCmQvGkyHXMl7KXL0K-WDOdAoLIN1ZHCCI_CEdWRwNoIj8A",
		"enr:-LK4QDwhXMitMbC8xRiNL-XGMhRyMSOnxej-zGifjv9Nm5G8EF285phTU-CAsMHRRefZimNI7eNpAluijMQP7NDC8kEMh2F0dG5ldHOIAAAAAAAABgCEZXRoMpDS8Zl_YAAJEAAIAAAAAAAAgmlkgnY0gmlwhAOIT_SJc2VjcDI1NmsxoQMoHWNL4MAvh6YpQeM2SUjhUrLIPsAVPB8nyxbmckC6KIN0Y3CCIyiDdWRwgiMo",
		"enr:-LK4QPYl2HnMPQ7b1es6Nf_tFYkyya5bj9IqAKOEj2cmoqVkN8ANbJJJK40MX4kciL7pZszPHw6vLNyeC-O3HUrLQv8Mh2F0dG5ldHOIAAAAAAAAAMCEZXRoMpDS8Zl_YAAJEAAIAAAAAAAAgmlkgnY0gmlwhAMYRG-Jc2VjcDI1NmsxoQPQ35tjr6q1qUqwAnegQmYQyfqxC_6437CObkZneI9n34N0Y3CCIyiDdWRwgiMo",
		"enr:-KG4QJk_4IQHQw3DAdKIuGcEauKU8-nmRPPMj_hIQPRHmsFGMPPeOj6_xX09aHCndOzLnOZimVRzNM56_EQWYVbEpJMBgmlkgnY0gmlwhLkvrBODaXA2kP6AAAAAAAAAAhY-__4PR6OJc2VjcDI1NmsxoQPU7g2jQGTz8BYbB2vLTb39S_PrcZAehwMM0b3bWsM5rIN1ZHCCIyiEdWRwNoIjKA",
	}

	OverrideBeaconNetworkConfig(cfg)
}

// HoodiConfig defines the config for the Hoodi beacon chain testnet.
func HoodiConfig() *BeaconChainConfig {
	cfg := MainnetConfig()
	cfg.MinGenesisTime = 1742212800
	cfg.GenesisDelay = 600
	cfg.ConfigName = HoodiName
	cfg.GenesisValidatorsRoot = [32]byte{
		0x21, 0x2f, 0x13, 0xfc, 0x4d, 0xf0, 0x78, 0xb6,
		0xcb, 0x7d, 0xb2, 0x28, 0xf1, 0xc8, 0x30, 0x75,
		0x66, 0xdc, 0xec, 0xf9, 0x00, 0x86, 0x74, 0x01,
		0xa9, 0x20, 0x23, 0xd7, 0xba, 0x99, 0xcb, 0x5f,
	}
	cfg.GenesisForkVersion = []byte{0x10, 0x00, 0x09, 0x10}
	cfg.SecondsPerETH1Block = 12
	cfg.DepositChainID = 560048
	cfg.DepositNetworkID = 560048
	cfg.AltairForkEpoch = 0
	cfg.AltairForkVersion = []byte{0x20, 0x00, 0x09, 0x10}
	cfg.BellatrixForkEpoch = 0
	cfg.BellatrixForkVersion = []byte{0x30, 0x00, 0x09, 0x10}
	cfg.CapellaForkEpoch = 0
	cfg.CapellaForkVersion = []byte{0x40, 0x00, 0x09, 0x10}
	cfg.DenebForkEpoch = 0
	cfg.DenebForkVersion = []byte{0x50, 0x00, 0x09, 0x10}
	cfg.ElectraForkEpoch = 2048
	cfg.ElectraForkVersion = []byte{0x60, 0x00, 0x09, 0x10}
	cfg.FuluForkEpoch = 50688 // 2025-10-28 18:53:12 UTC
	cfg.FuluForkVersion = []byte{0x70, 0x00, 0x09, 0x10}
	cfg.GloasForkVersion = []byte{0x80, 0x00, 0x09, 0x10}
	cfg.TerminalTotalDifficulty = "0"
	cfg.DepositContractAddress = "0x00000000219ab540356cBB839Cbe05303d7705Fa"
	cfg.BlobSchedule = []BlobScheduleEntry{
		{
			MaxBlobsPerBlock: 15,
			Epoch:            52480, // 2025-11-05 18:02:00 UTC
		},
		{
			MaxBlobsPerBlock: 21,
			Epoch:            54016, // 2025-11-12 13:52:24 UTC
		},
	}
	cfg.DefaultBuilderGasLimit = uint64(60000000)
	cfg.InitializeForkSchedule()
	return cfg
}
