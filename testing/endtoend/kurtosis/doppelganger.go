package kurtosis

import "fmt"

// PrysmDoppelgangerValidatorConfig identifies the duplicate-key validator
// service to add for the doppelganger protection check.
type PrysmDoppelgangerValidatorConfig struct {
	ServiceName      string
	Image            string
	KeystoreArtifact string
	BeaconRPC        string
	BeaconREST       string
}

// validate checks whether the config has all required fields set, returning an error if not.
func (cfg PrysmDoppelgangerValidatorConfig) validate() error {
	if cfg.ServiceName == "" {
		return fmt.Errorf("doppelganger service name is required")
	}
	if cfg.Image == "" {
		return fmt.Errorf("doppelganger validator image is required")
	}
	if cfg.KeystoreArtifact == "" {
		return fmt.Errorf("doppelganger keystore artifact is required")
	}
	if cfg.BeaconRPC == "" {
		return fmt.Errorf("doppelganger beacon RPC provider is required")
	}
	if cfg.BeaconREST == "" {
		return fmt.Errorf("doppelganger beacon REST provider is required")
	}
	return nil
}

// AddPrysmDoppelgangerValidator adds a late validator client with node 1's
// keys, pointed at another Prysm beacon node.
func (kw *KurtosisWrapper) AddPrysmDoppelgangerValidator(cfg PrysmDoppelgangerValidatorConfig) error {
	if kw.enclaveCtx == nil {
		return fmt.Errorf("enclave context is nil")
	}
	if err := cfg.validate(); err != nil {
		return err
	}

	if err := kw.runStarlarkScript(prysmDoppelgangerValidatorScript(cfg)); err != nil {
		return fmt.Errorf("failed to add doppelganger validator service %q: %w", cfg.ServiceName, err)
	}
	return nil
}

// prysmDoppelgangerValidatorScript returns a Starlark script that adds a Prysm validator service with the given config.
func prysmDoppelgangerValidatorScript(cfg PrysmDoppelgangerValidatorConfig) string {
	return fmt.Sprintf(`def run(plan):
    plan.add_service(
        name=%q,
        config=ServiceConfig(
            image=%q,
            files={
                "/network-configs": Directory(artifact_names=["el_cl_genesis_data"]),
                "/validator-keys": Directory(artifact_names=[%q]),
                "/prysm-password": Directory(artifact_names=["prysm-password"]),
            },
            cmd=[
                "--accept-terms-of-use=true",
                "--chain-config-file=/network-configs/config.yaml",
                "--wallet-dir=/validator-keys/prysm",
                "--wallet-password-file=/prysm-password/prysm-password.txt",
                "--beacon-rpc-provider=%s",
                "--beacon-rest-api-provider=%s",
                "--enable-doppelganger",
                "--disable-monitoring=true",
            ],
            tty_enabled=True,
        ),
    )
`, cfg.ServiceName, cfg.Image, cfg.KeystoreArtifact, cfg.BeaconRPC, cfg.BeaconREST)
}
