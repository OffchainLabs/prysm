package params

// UseHoodiNetworkConfig uses the Hoodi beacon chain specific network config.
func UseHoodiNetworkConfig() {
	cfg := BeaconNetworkConfig().Copy()
	cfg.ContractDeploymentBlock = 0
	cfg.BootstrapNodes = []string{
		"enr:-Mq4QLkmuSwbGBUph1r7iHopzRpdqE-gcm5LNZfcE-6T37OCZbRHi22bXZkaqnZ6XdIyEDTelnkmMEQB8w6NbnJUt9GGAZWaowaYh2F0dG5ldHOIABgAAAAAAACEZXRoMpDS8Zl_YAAJEAAIAAAAAAAAgmlkgnY0gmlwhNEmfKCEcXVpY4IyyIlzZWNwMjU2azGhA0hGa4jZJZYQAS-z6ZFK-m4GCFnWS8wfjO0bpSQn6hyEiHN5bmNuZXRzAIN0Y3CCIyiDdWRwgiMo",
		"enr:-Ku4QLVumWTwyOUVS4ajqq8ZuZz2ik6t3Gtq0Ozxqecj0qNZWpMnudcvTs-4jrlwYRQMQwBS8Pvtmu4ZPP2Lx3i2t7YBh2F0dG5ldHOIAAAAAAAAAACEZXRoMpBd9cEGEAAJEP__________gmlkgnY0gmlwhNEmfKCJc2VjcDI1NmsxoQLdRlI8aCa_ELwTJhVN8k7km7IDc3pYu-FMYBs5_FiigIN1ZHCCIyk",
		"enr:-LK4QAYuLujoiaqCAs0-qNWj9oFws1B4iy-Hff1bRB7wpQCYSS-IIMxLWCn7sWloTJzC1SiH8Y7lMQ5I36ynGV1ASj4Eh2F0dG5ldHOIYAAAAAAAAACEZXRoMpDS8Zl_YAAJEAAIAAAAAAAAgmlkgnY0gmlwhIbRilSJc2VjcDI1NmsxoQOmI5MlAu3f5WEThAYOqoygpS2wYn0XS5NV2aYq7T0a04N0Y3CCIyiDdWRwgiMo",
		"enr:-Ku4QIC89sMC0o-irosD4_23lJJ4qCGOvdUz7SmoShWx0k6AaxCFTKviEHa-sa7-EzsiXpDp0qP0xzX6nKdXJX3X-IQBh2F0dG5ldHOIAAAAAAAAAACEZXRoMpBd9cEGEAAJEP__________gmlkgnY0gmlwhIbRilSJc2VjcDI1NmsxoQK_m0f1DzDc9Cjrspm36zuRa7072HSiMGYWLsKiVSbP34N1ZHCCIyk",
		"enr:-Ku4QNkWjw5tNzo8DtWqKm7CnDdIq_y7xppD6c1EZSwjB8rMOkSFA1wJPLoKrq5UvA7wcxIotH6Usx3PAugEN2JMncIBh2F0dG5ldHOIAAAAAAAAAACEZXRoMpBd9cEGEAAJEP__________gmlkgnY0gmlwhIbHuBeJc2VjcDI1NmsxoQP3FwrhFYB60djwRjAoOjttq6du94DtkQuaN99wvgqaIYN1ZHCCIyk",
		"enr:-OS4QMJGE13xEROqvKN1xnnt7U-noc51VXyM6wFMuL9LMhQDfo1p1dF_zFdS4OsnXz_vIYk-nQWnqJMWRDKvkSK6_CwDh2F0dG5ldHOIAAAAADAAAACGY2xpZW502IpMaWdodGhvdXNljDcuMC4wLWJldGEuM4RldGgykNLxmX9gAAkQAAgAAAAAAACCaWSCdjSCaXCEhse4F4RxdWljgiMqiXNlY3AyNTZrMaECef77P8k5l3PC_raLw42OAzdXfxeQ-58BJriNaqiRGJSIc3luY25ldHMAg3RjcIIjKIN1ZHCCIyg",
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
