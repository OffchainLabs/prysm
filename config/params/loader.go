package params

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/math"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

func isMinimal(lines []string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, "PRESET_BASE: 'minimal'") ||
			strings.HasPrefix(l, `PRESET_BASE: "minimal"`) ||
			strings.HasPrefix(l, "PRESET_BASE: minimal") ||
			strings.HasPrefix(l, "# Minimal preset") {
			return true
		}
	}
	return false
}

// reconcileSlotDuration keeps SecondsPerSlot and SlotDurationMilliseconds in agreement.
//
// The two fields describe one quantity: SLOT_DURATION_MS replaced SECONDS_PER_SLOT in the specification
// config, and Prysm keeps the older field so that configs still setting it keep working. A config
// file sets one or the other, and whichever it omits keeps the value inherited from the preset
// base, so the omitted one has to be derived here.
//
// SecondsPerSlot cannot represent a sub-second slot duration, so it truncates. Nothing outside this
// package reads it, and ConfigToYaml only writes it back out when it round-trips faithfully.
func reconcileSlotDuration(conf *BeaconChainConfig, hasSecondsPerSlot, hasSlotDurationMs bool) error {
	// The only way both values are tolerated is if they represent the same duration.
	if hasSecondsPerSlot && hasSlotDurationMs && conf.SecondsPerSlot*1000 != conf.SlotDurationMilliseconds {
		return errors.Errorf(
			"config sets contradictory slot durations: SECONDS_PER_SLOT: %d (%d ms) != SLOT_DURATION_MS: %d",
			conf.SecondsPerSlot, conf.SecondsPerSlot*1000, conf.SlotDurationMilliseconds)
	}

	// Derive whichever field the config file left at its preset base value. When the file sets both,
	// they agree by now and both assignments are no-ops.
	if hasSecondsPerSlot {
		conf.SlotDurationMilliseconds = conf.SecondsPerSlot * 1000
	}

	if hasSlotDurationMs {
		conf.SecondsPerSlot = conf.SlotDurationMilliseconds / 1000
	}

	// A zero slot duration makes every ticker panic and divides by zero in the slot arithmetic.
	if conf.SlotDurationMilliseconds == 0 {
		return errors.New("slot duration cannot be zero: set a positive SLOT_DURATION_MS")
	}

	return nil
}

func UnmarshalConfig(yamlFile []byte, conf *BeaconChainConfig) (*BeaconChainConfig, error) {
	// To track if config name is defined inside config file.
	hasConfigName := false
	// SECONDS_PER_SLOT and SLOT_DURATION_MS express the same value. Track which ones the file
	// sets so the one left at its preset default can be derived from the other.
	hasSecondsPerSlot, hasSlotDurationMs := false, false
	// Convert 0x hex inputs to fixed bytes arrays
	lines := strings.Split(string(yamlFile), "\n")
	if conf == nil {
		if isMinimal(lines) {
			conf = MinimalSpecConfig().Copy()
		} else {
			// Default to using mainnet.
			conf = MainnetConfig()
		}
	}
	for i, line := range lines {
		// No need to convert the deposit contract address to byte array (as config expects a string).
		if strings.HasPrefix(line, "DEPOSIT_CONTRACT_ADDRESS") {
			continue
		}
		if strings.HasPrefix(line, "CONFIG_NAME") {
			hasConfigName = true
		}
		if strings.HasPrefix(line, "SECONDS_PER_SLOT:") {
			hasSecondsPerSlot = true
		}
		if strings.HasPrefix(line, "SLOT_DURATION_MS:") {
			hasSlotDurationMs = true
		}
		if !strings.HasPrefix(line, "#") && strings.Contains(line, "0x") {
			parts := ReplaceHexStringWithYAMLFormat(line)
			lines[i] = strings.Join(parts, "\n")
		}
	}
	yamlFile = []byte(strings.Join(lines, "\n"))
	if err := yaml.UnmarshalStrict(yamlFile, conf); err != nil {
		var typeError *yaml.TypeError
		if !errors.As(err, &typeError) {
			return nil, errors.Wrap(err, "Failed to parse chain config yaml file.")
		} else {
			log.WithError(err).Error("There were some issues parsing the config from a yaml file")
		}
	}
	if !hasConfigName {
		conf.ConfigName = DevnetName
	}
	if err := reconcileSlotDuration(conf, hasSecondsPerSlot, hasSlotDurationMs); err != nil {
		return nil, err
	}
	// recompute SqrRootSlotsPerEpoch constant to handle non-standard values of SlotsPerEpoch
	conf.SqrRootSlotsPerEpoch = primitives.Slot(math.IntegerSquareRoot(uint64(conf.SlotsPerEpoch)))
	// Recompute the fork schedule
	conf.InitializeForkSchedule()
	log.Debugf("Config file values: %+v", conf)
	return conf, nil
}

func UnmarshalConfigFile(path string, conf *BeaconChainConfig) (*BeaconChainConfig, error) {
	yamlFile, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return nil, errors.Wrap(err, "Failed to read chain config file.")
	}
	return UnmarshalConfig(yamlFile, conf)
}

// LoadChainConfigFile load, convert hex values into valid param yaml format,
// unmarshal , and apply beacon chain config file.
func LoadChainConfigFile(path string, conf *BeaconChainConfig) error {
	c, err := UnmarshalConfigFile(path, conf)
	if err != nil {
		return err
	}
	return SetActive(c)
}

// ReplaceHexStringWithYAMLFormat will replace hex strings that the yaml parser will understand.
func ReplaceHexStringWithYAMLFormat(line string) []string {
	parts := strings.Split(line, "0x")
	decoded, err := hex.DecodeString(parts[1])
	if err != nil {
		log.WithError(err).Error("Failed to decode hex string.")
	}
	switch l := len(decoded); {
	case l == 1:
		var b byte
		b = decoded[0]
		fixedByte, err := yaml.Marshal(b)
		if err != nil {
			log.WithError(err).Error("Failed to marshal config file.")
		}
		parts[0] += string(fixedByte)
		parts = parts[:1]
	case l > 1 && l <= 4:
		var arr [4]byte
		copy(arr[:], decoded)
		fixedByte, err := yaml.Marshal(arr)
		if err != nil {
			log.WithError(err).Error("Failed to marshal config file.")
		}
		parts[1] = string(fixedByte)
	case l > 4 && l <= 8:
		var arr [8]byte
		copy(arr[:], decoded)
		fixedByte, err := yaml.Marshal(arr)
		if err != nil {
			log.WithError(err).Error("Failed to marshal config file.")
		}
		parts[1] = string(fixedByte)
	case l > 8 && l <= 16:
		var arr [16]byte
		copy(arr[:], decoded)
		fixedByte, err := yaml.Marshal(arr)
		if err != nil {
			log.WithError(err).Error("Failed to marshal config file.")
		}
		parts[1] = string(fixedByte)
	case l > 16 && l <= 20:
		var arr [20]byte
		copy(arr[:], decoded)
		fixedByte, err := yaml.Marshal(arr)
		if err != nil {
			log.WithError(err).Error("Failed to marshal config file.")
		}
		parts[1] = string(fixedByte)
	case l > 20 && l <= 32:
		var arr [32]byte
		copy(arr[:], decoded)
		fixedByte, err := yaml.Marshal(arr)
		if err != nil {
			log.WithError(err).Error("Failed to marshal config file.")
		}
		parts[1] = string(fixedByte)
	case l > 32 && l <= 48:
		var arr [48]byte
		copy(arr[:], decoded)
		fixedByte, err := yaml.Marshal(arr)
		if err != nil {
			log.WithError(err).Error("Failed to marshal config file.")
		}
		parts[1] = string(fixedByte)
	case l > 48 && l <= 64:
		var arr [64]byte
		copy(arr[:], decoded)
		fixedByte, err := yaml.Marshal(arr)
		if err != nil {
			log.WithError(err).Error("Failed to marshal config file.")
		}
		parts[1] = string(fixedByte)
	case l > 64 && l <= 96:
		var arr [96]byte
		copy(arr[:], decoded)
		fixedByte, err := yaml.Marshal(arr)
		if err != nil {
			log.WithError(err).Error("Failed to marshal config file.")
		}
		parts[1] = string(fixedByte)
	}
	return parts
}

// ConfigToYaml takes a provided config and outputs its contents
// in yaml. This allows prysm's custom configs to be read by other clients.
func ConfigToYaml(cfg *BeaconChainConfig) []byte {
	lines := []string{
		fmt.Sprintf("PRESET_BASE: '%s'", cfg.PresetBase),
		fmt.Sprintf("CONFIG_NAME: '%s'", cfg.ConfigName),
		fmt.Sprintf("MIN_GENESIS_ACTIVE_VALIDATOR_COUNT: %d", cfg.MinGenesisActiveValidatorCount),
		fmt.Sprintf("GENESIS_DELAY: %d", cfg.GenesisDelay),
		fmt.Sprintf("MIN_GENESIS_TIME: %d", cfg.MinGenesisTime),
		fmt.Sprintf("GENESIS_FORK_VERSION: %#x", cfg.GenesisForkVersion),
		fmt.Sprintf("CHURN_LIMIT_QUOTIENT: %d", cfg.ChurnLimitQuotient),
		fmt.Sprintf("SLOT_DURATION_MS: %d", cfg.SlotDurationMillis()),
		fmt.Sprintf("SLOTS_PER_EPOCH: %d", cfg.SlotsPerEpoch),
		fmt.Sprintf("SECONDS_PER_ETH1_BLOCK: %d", cfg.SecondsPerETH1Block),
		fmt.Sprintf("ETH1_FOLLOW_DISTANCE: %d", cfg.Eth1FollowDistance),
		fmt.Sprintf("EPOCHS_PER_ETH1_VOTING_PERIOD: %d", cfg.EpochsPerEth1VotingPeriod),
		fmt.Sprintf("SHARD_COMMITTEE_PERIOD: %d", cfg.ShardCommitteePeriod),
		fmt.Sprintf("MIN_VALIDATOR_WITHDRAWABILITY_DELAY: %d", cfg.MinValidatorWithdrawabilityDelay),
		fmt.Sprintf("MAX_VALIDATORS_PER_WITHDRAWALS_SWEEP: %d", cfg.MaxValidatorsPerWithdrawalsSweep),
		fmt.Sprintf("MAX_BUILDERS_PER_WITHDRAWALS_SWEEP: %d", cfg.MaxBuildersPerWithdrawalsSweep),
		fmt.Sprintf("MAX_SEED_LOOKAHEAD: %d", cfg.MaxSeedLookahead),
		fmt.Sprintf("EJECTION_BALANCE: %d", cfg.EjectionBalance),
		fmt.Sprintf("MIN_PER_EPOCH_CHURN_LIMIT: %d", cfg.MinPerEpochChurnLimit),
		fmt.Sprintf("DEPOSIT_CHAIN_ID: %d", cfg.DepositChainID),
		fmt.Sprintf("DEPOSIT_NETWORK_ID: %d", cfg.DepositNetworkID),
		fmt.Sprintf("ALTAIR_FORK_EPOCH: %d", cfg.AltairForkEpoch),
		fmt.Sprintf("ALTAIR_FORK_VERSION: %#x", cfg.AltairForkVersion),
		fmt.Sprintf("BELLATRIX_FORK_EPOCH: %d", cfg.BellatrixForkEpoch),
		fmt.Sprintf("BELLATRIX_FORK_VERSION: %#x", cfg.BellatrixForkVersion),
		fmt.Sprintf("CAPELLA_FORK_EPOCH: %d", cfg.CapellaForkEpoch),
		fmt.Sprintf("CAPELLA_FORK_VERSION: %#x", cfg.CapellaForkVersion),
		fmt.Sprintf("INACTIVITY_SCORE_BIAS: %d", cfg.InactivityScoreBias),
		fmt.Sprintf("INACTIVITY_SCORE_RECOVERY_RATE: %d", cfg.InactivityScoreRecoveryRate),
		fmt.Sprintf("TERMINAL_TOTAL_DIFFICULTY: %s", cfg.TerminalTotalDifficulty),
		fmt.Sprintf("TERMINAL_BLOCK_HASH: %#x", cfg.TerminalBlockHash),
		fmt.Sprintf("TERMINAL_BLOCK_HASH_ACTIVATION_EPOCH: %d", cfg.TerminalBlockHashActivationEpoch),
		fmt.Sprintf("DEPOSIT_CONTRACT_ADDRESS: %s", cfg.DepositContractAddress),
		fmt.Sprintf("MAX_PER_EPOCH_ACTIVATION_CHURN_LIMIT: %d", cfg.MaxPerEpochActivationChurnLimit),
		fmt.Sprintf("MIN_EPOCHS_FOR_BLOB_SIDECARS_REQUESTS: %d", cfg.MinEpochsForBlobsSidecarsRequest),
		fmt.Sprintf("MAX_REQUEST_BLOCKS_DENEB: %d", cfg.MaxRequestBlocksDeneb),
		fmt.Sprintf("MAX_REQUEST_BLOB_SIDECARS: %d", cfg.MaxRequestBlobSidecars),
		fmt.Sprintf("MAX_REQUEST_BLOB_SIDECARS_ELECTRA: %d", cfg.MaxRequestBlobSidecarsElectra),
		fmt.Sprintf("BLOB_SIDECAR_SUBNET_COUNT: %d", cfg.BlobsidecarSubnetCount),
		fmt.Sprintf("BLOB_SIDECAR_SUBNET_COUNT_ELECTRA: %d", cfg.BlobsidecarSubnetCountElectra),
		fmt.Sprintf("DENEB_FORK_EPOCH: %d", cfg.DenebForkEpoch),
		fmt.Sprintf("DENEB_FORK_VERSION: %#x", cfg.DenebForkVersion),
		fmt.Sprintf("ELECTRA_FORK_EPOCH: %d", cfg.ElectraForkEpoch),
		fmt.Sprintf("ELECTRA_FORK_VERSION: %#x", cfg.ElectraForkVersion),
		fmt.Sprintf("FULU_FORK_EPOCH: %d", cfg.FuluForkEpoch),
		fmt.Sprintf("FULU_FORK_VERSION: %#x", cfg.FuluForkVersion),
		fmt.Sprintf("GLOAS_FORK_VERSION: %#x", cfg.GloasForkVersion),
		fmt.Sprintf("GLOAS_FORK_EPOCH: %d", cfg.GloasForkEpoch),
		fmt.Sprintf("EPOCHS_PER_SUBNET_SUBSCRIPTION: %d", cfg.EpochsPerSubnetSubscription),
		fmt.Sprintf("ATTESTATION_SUBNET_EXTRA_BITS: %d", cfg.AttestationSubnetExtraBits),
		fmt.Sprintf("ATTESTATION_SUBNET_PREFIX_BITS: %d", cfg.AttestationSubnetPrefixBits),
		fmt.Sprintf("SUBNETS_PER_NODE: %d", cfg.SubnetsPerNode),
		fmt.Sprintf("NODE_ID_BITS: %d", cfg.NodeIdBits),
		fmt.Sprintf("MAX_PAYLOAD_SIZE: %d", cfg.MaxPayloadSize),
		fmt.Sprintf("ATTESTATION_SUBNET_COUNT: %d", cfg.AttestationSubnetCount),
		fmt.Sprintf("ATTESTATION_PROPAGATION_SLOT_RANGE: %d", cfg.AttestationPropagationSlotRange),
		fmt.Sprintf("MAX_REQUEST_BLOCKS: %d", cfg.MaxRequestBlocks),
		fmt.Sprintf("TTFB_TIMEOUT: %d", int(cfg.TtfbTimeout)),
		fmt.Sprintf("RESP_TIMEOUT: %d", int(cfg.RespTimeout)),
		fmt.Sprintf("MAXIMUM_GOSSIP_CLOCK_DISPARITY: %d", int(cfg.MaximumGossipClockDisparity)),
		fmt.Sprintf("MESSAGE_DOMAIN_INVALID_SNAPPY:  %#x", cfg.MessageDomainInvalidSnappy),
		fmt.Sprintf("MESSAGE_DOMAIN_VALID_SNAPPY: %#x", cfg.MessageDomainValidSnappy),
		fmt.Sprintf("MIN_EPOCHS_FOR_BLOCK_REQUESTS: %d", int(cfg.MinEpochsForBlockRequests)),
		fmt.Sprintf("MIN_PER_EPOCH_CHURN_LIMIT_ELECTRA: %d", cfg.MinPerEpochChurnLimitElectra),
		fmt.Sprintf("MAX_BLOBS_PER_BLOCK: %d", cfg.DeprecatedMaxBlobsPerBlock),
		fmt.Sprintf("PROPOSER_REORG_CUTOFF_BPS: %d", cfg.ProposerReorgCutoffBPS),
		fmt.Sprintf("ATTESTATION_DUE_BPS: %d", cfg.AttestationDueBPS),
		fmt.Sprintf("AGGREGATE_DUE_BPS: %d", cfg.AggregateDueBPS),
		fmt.Sprintf("SYNC_MESSAGE_DUE_BPS: %d", cfg.SyncMessageDueBPS),
		fmt.Sprintf("CONTRIBUTION_DUE_BPS: %d", cfg.ContributionDueBPS),
		fmt.Sprintf("ATTESTATION_DUE_BPS_GLOAS: %d", cfg.AttestationDueBPSGloas),
		fmt.Sprintf("AGGREGATE_DUE_BPS_GLOAS: %d", cfg.AggregateDueBPSGloas),
		fmt.Sprintf("SYNC_MESSAGE_DUE_BPS_GLOAS: %d", cfg.SyncMessageDueBPSGloas),
		fmt.Sprintf("CONTRIBUTION_DUE_BPS_GLOAS: %d", cfg.ContributionDueBPSGloas),
		fmt.Sprintf("PAYLOAD_ATTESTATION_DUE_BPS: %d", cfg.PayloadAttestationDueBPS),
		fmt.Sprintf("PAYLOAD_DUE_BPS: %d", cfg.PayloadDueBPS),
	}

	if ms := cfg.SlotDurationMillis(); ms%1000 == 0 {
		lines = append(lines, fmt.Sprintf("SECONDS_PER_SLOT: %d", ms/1000))
	}

	if len(cfg.BlobSchedule) > 0 {
		lines = append(lines, "BLOB_SCHEDULE:")
		for _, entry := range cfg.BlobSchedule {
			lines = append(lines,
				"  - EPOCH: "+strconv.FormatUint(uint64(entry.Epoch), 10),
				"    MAX_BLOBS_PER_BLOCK: "+strconv.FormatUint(entry.MaxBlobsPerBlock, 10),
			)
		}
	}

	if len(cfg.GasLimitSchedule) > 0 {
		lines = append(lines, "GAS_LIMIT_SCHEDULE:")
		for _, entry := range cfg.GasLimitSchedule {
			lines = append(lines,
				"  - EPOCH: "+strconv.FormatUint(uint64(entry.Epoch), 10),
				"    GAS_LIMIT: "+strconv.FormatUint(entry.GasLimit, 10),
			)
		}
	}

	yamlFile := []byte(strings.Join(lines, "\n"))
	return yamlFile
}
