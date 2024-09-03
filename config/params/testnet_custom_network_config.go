package params

func UseCustomNetworkConfig() {
	cfg := BeaconNetworkConfig().Copy()
	cfg.ContractDeploymentBlock = 0
	cfg.BootstrapNodes = []string{}

	OverrideBeaconNetworkConfig(cfg)
}
