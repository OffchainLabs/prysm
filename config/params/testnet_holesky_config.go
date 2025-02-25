package params

import "math"

// UseHoleskyNetworkConfig uses the Holesky beacon chain specific network config.
func UseHoleskyNetworkConfig() {
	cfg := BeaconNetworkConfig().Copy()
	cfg.ContractDeploymentBlock = 0
	cfg.BootstrapNodes = []string{
		"enr:-Oa4QOuWj-_JX6iJWjdS_67qQkkVoomo3YztvdOLVF4lVwc3NzFR3D8y-8CmGjmQA16DJBOZWwbc3XMOw_w_ScU6-qCCAgSHYXR0bmV0c4gAAAMAAAAAAIZjbGllbnTYikxpZ2h0aG91c2WMNy4wLjAtYmV0YS4whGV0aDKQAZ4hrQYBcAD__________4JpZIJ2NIJpcISyE90mhHF1aWOCfLuJc2VjcDI1NmsxoQK-fTVglh3wyHIcyauubuyvLeFmn6QpUwog7Aio2OSucYhzeW5jbmV0cwCDdGNwgny6g3VkcIJ8ug",
		"enr:-PW4QAOnzqnCuwuNNrUEXebSD3MFMOe-9NApsb8UkAQK-MquYtUhj35Ksz4EWcmdB0Cmj43bGBJJEpt9fYMAg1vOHXobh2F0dG5ldHOIAAAYAAAAAACGY2xpZW502IpMaWdodGhvdXNljDcuMC4wLWJldGEuMIRldGgykAGeIa0GAXAA__________-CaWSCdjSCaXCEff1tSYRxdWljgiMphXF1aWM2giMpiXNlY3AyNTZrMaECUiAFSBathSIPGhDHbZjQS5gTqaPcRkAe4HECCk-vt6KIc3luY25ldHMPg3RjcIIjKIR0Y3A2giMog3VkcIIjKA",
		"enr:-QESuEA2tFgFDu5LX9T6j1_bayowdRzrtdQcjwmTq_zOVjwe1WQOsM7-Q4qRcgc7AjpAQOcdb2F3wyPDBkbP-vxW2dLgXYdhdHRuZXRziAADAAAAAAAAhmNsaWVudNiKTGlnaHRob3VzZYw3LjAuMC1iZXRhLjCEZXRoMpABniGtBgFwAP__________gmlkgnY0gmlwhIe1ME2DaXA2kCoBBPkwgDCeAAAAAAAAAAKEcXVpY4IjKYVxdWljNoIjg4lzZWNwMjU2azGhA4oHjOmlWOfLizFFIQSI_dzn4rzvDvMG8h7zmxhmOVzXiHN5bmNuZXRzD4N0Y3CCIyiEdGNwNoIjgoN1ZHCCIyiEdWRwNoIjgg",
		"enr:-KG4QCvyykb0pA1T-EZJUPkl2P1PKYk8-4El8TqwWdKwtoI6NtIBGMJVDgGZKVy2eMszI0_ermORtQ340lj1dTHzGVVPhGV0aDKQAZ4hrQYBcAD__________4JpZIJ2NIJpcIQ5gNI3iXNlY3AyNTZrMaEDMQYffETNbuGwVzWEJSgCpA50LTxHUWU1A0TDfleEa5mDdGNwgjvFg3VkcII7xQ",
	}
	OverrideBeaconNetworkConfig(cfg)
}

// HoleskyConfig defines the config for the Holesky beacon chain testnet.
func HoleskyConfig() *BeaconChainConfig {
	cfg := MainnetConfig().Copy()
	cfg.MinGenesisTime = 1695902100
	cfg.GenesisDelay = 300
	cfg.ConfigName = HoleskyName
	cfg.GenesisValidatorsRoot = [32]byte{145, 67, 170, 124, 97, 90, 127, 113, 21, 226, 182, 170, 195, 25, 192, 53, 41, 223, 130, 66, 174, 112, 95, 186, 157, 243, 155, 121, 197, 159, 168, 177}
	cfg.GenesisForkVersion = []byte{0x01, 0x01, 0x70, 0x00}
	cfg.SecondsPerETH1Block = 14
	cfg.DepositChainID = 17000
	cfg.DepositNetworkID = 17000
	cfg.AltairForkEpoch = 0
	cfg.AltairForkVersion = []byte{0x2, 0x1, 0x70, 0x0}
	cfg.BellatrixForkEpoch = 0
	cfg.BellatrixForkVersion = []byte{0x3, 0x1, 0x70, 0x0}
	cfg.CapellaForkEpoch = 256
	cfg.CapellaForkVersion = []byte{0x4, 0x1, 0x70, 0x0}
	cfg.DenebForkEpoch = 29696
	cfg.DenebForkVersion = []byte{0x05, 0x1, 0x70, 0x0}
	cfg.ElectraForkEpoch = 115968 // Mon, Feb 24 at 21:55:12 UTC
	cfg.ElectraForkVersion = []byte{0x06, 0x1, 0x70, 0x0}
	cfg.FuluForkEpoch = math.MaxUint64
	cfg.FuluForkVersion = []byte{0x07, 0x1, 0x70, 0x0} // TODO: Define holesky fork version for fulu. This is a placeholder value.
	cfg.TerminalTotalDifficulty = "0"
	cfg.DepositContractAddress = "0x4242424242424242424242424242424242424242"
	cfg.EjectionBalance = 28000000000
	cfg.InitializeForkSchedule()
	return cfg
}
