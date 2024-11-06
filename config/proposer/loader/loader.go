package loader

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/cmd/validator/flags"
	"github.com/prysmaticlabs/prysm/v5/config"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/validator"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/validator/db/iface"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

type settingsType int

const (
	none settingsType = iota
	defaultFlag
	fileFlag
	urlFlag
	onlyDB
)

type settingsLoader struct {
	loadMethods []settingsType
	existsInDB  bool
	db          iface.ValidatorDB
	options     *flagOptions
}

type flagOptions struct {
	builderConfig *proposer.BuilderConfig
	gasLimit      *validator.Uint64
}

// SettingsLoaderOption sets additional options that affect the proposer settings
type SettingsLoaderOption func(cliCtx *cli.Context, psl *settingsLoader) error

// WithBuilderConfig applies the --enable-builder flag to proposer settings
func WithBuilderConfig() SettingsLoaderOption {
	return func(cliCtx *cli.Context, psl *settingsLoader) error {
		if cliCtx.Bool(flags.EnableBuilderFlag.Name) {
			psl.options.builderConfig = &proposer.BuilderConfig{
				Enabled:  true,
				GasLimit: validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit),
			}
		}
		return nil
	}
}

// WithGasLimit applies the --suggested-gas-limit flag to proposer settings
func WithGasLimit() SettingsLoaderOption {
	return func(cliCtx *cli.Context, psl *settingsLoader) error {
		sgl := cliCtx.String(flags.BuilderGasLimitFlag.Name)
		if sgl != "" {
			gl, err := strconv.ParseUint(sgl, 10, 64)
			if err != nil {
				return errors.Errorf("Value set by --%s is not a uint64", flags.BuilderGasLimitFlag.Name)
			}
			if gl == 0 {
				log.Warnf("Gas limit was intentionally set to 0, this will be replaced with the default gas limit of %d", params.BeaconConfig().DefaultBuilderGasLimit)
			}
			rgl := reviewGasLimit(validator.Uint64(gl))
			psl.options.gasLimit = &rgl
		}
		return nil
	}
}

// NewProposerSettingsLoader returns a new proposer settings loader that can process the proposer settings based on flag options
func NewProposerSettingsLoader(cliCtx *cli.Context, db iface.ValidatorDB, opts ...SettingsLoaderOption) (*settingsLoader, error) {
	if cliCtx.IsSet(flags.ProposerSettingsFlag.Name) && cliCtx.IsSet(flags.ProposerSettingsURLFlag.Name) {
		return nil, fmt.Errorf("cannot specify both --%s and --%s flags; choose one method for specifying proposer settings", flags.ProposerSettingsFlag.Name, flags.ProposerSettingsURLFlag.Name)
	}
	psExists, err := db.ProposerSettingsExists(cliCtx.Context)
	if err != nil {
		return nil, err
	}
	psl := &settingsLoader{db: db, existsInDB: psExists, options: &flagOptions{}}

	if cliCtx.IsSet(flags.SuggestedFeeRecipientFlag.Name) {
		psl.loadMethods = append(psl.loadMethods, defaultFlag)
	}
	if cliCtx.IsSet(flags.ProposerSettingsFlag.Name) {
		psl.loadMethods = append(psl.loadMethods, fileFlag)
	}
	if cliCtx.IsSet(flags.ProposerSettingsURLFlag.Name) {
		psl.loadMethods = append(psl.loadMethods, urlFlag)
	}
	if len(psl.loadMethods) == 0 {
		method := none
		if psExists {
			// override with db
			method = onlyDB
		}
		psl.loadMethods = append(psl.loadMethods, method)
	}

	for _, o := range opts {
		if err := o(cliCtx, psl); err != nil {
			return nil, err
		}
	}

	return psl, nil
}

// Load saves the proposer settings to the database
func (psl *settingsLoader) Load(cliCtx *cli.Context) (*proposer.Settings, error) {
	loadConfig := &validatorpb.ProposerSettingsPayload{}

	// override settings based on other options
	if psl.options.builderConfig != nil && psl.options.gasLimit != nil {
		psl.options.builderConfig.GasLimit = *psl.options.gasLimit
	}

	// check if database has settings already
	if psl.existsInDB {
		dbps, err := psl.db.ProposerSettings(cliCtx.Context)
		if err != nil {
			return nil, err
		}
		loadConfig = dbps.ToConsensus()
		log.Debugf("DB loaded proposer settings: %s", func() string {
			b, err := json.Marshal(loadConfig)
			if err != nil {
				return err.Error()
			}
			return string(b)
		}())
	}

	// start to process based on load method
	for _, method := range psl.loadMethods {
		switch method {
		case defaultFlag:
			suggestedFeeRecipient := cliCtx.String(flags.SuggestedFeeRecipientFlag.Name)
			if !common.IsHexAddress(suggestedFeeRecipient) {
				return nil, errors.Errorf("--%s is not a valid Ethereum address", flags.SuggestedFeeRecipientFlag.Name)
			}
			if err := config.WarnNonChecksummedAddress(suggestedFeeRecipient); err != nil {
				return nil, err
			}
			defaultConfig := &validatorpb.ProposerOptionPayload{
				FeeRecipient: suggestedFeeRecipient,
			}
			if psl.options.builderConfig != nil {
				defaultConfig.Builder = psl.options.builderConfig.ToConsensus()
			}
			if psl.existsInDB && len(psl.loadMethods) == 1 {
				// only log the below if default flag is the only load method
				log.Debug("Overriding previously saved proposer default settings.")
			}
			loadConfig.DefaultConfig = defaultConfig
		case fileFlag:
			var settingFromFile *validatorpb.ProposerSettingsPayload
			if err := config.UnmarshalFromFile(cliCtx.String(flags.ProposerSettingsFlag.Name), &settingFromFile); err != nil {
				return nil, err
			}
			if settingFromFile == nil {
				return nil, errors.Errorf("proposer settings is empty after unmarshalling from file specified by %s flag", flags.ProposerSettingsFlag.Name)
			}
			loadConfig = psl.processProposerSettings(settingFromFile, loadConfig)
			log.WithField(flags.ProposerSettingsFlag.Name, cliCtx.String(flags.ProposerSettingsFlag.Name)).Info("Proposer settings loaded from file")
		case urlFlag:
			var settingFromURL *validatorpb.ProposerSettingsPayload
			if err := config.UnmarshalFromURL(cliCtx.Context, cliCtx.String(flags.ProposerSettingsURLFlag.Name), &settingFromURL); err != nil {
				return nil, err
			}
			if settingFromURL == nil {
				return nil, errors.New("proposer settings is empty after unmarshalling from url")
			}
			loadConfig = psl.processProposerSettings(settingFromURL, loadConfig)
			log.WithField(flags.ProposerSettingsURLFlag.Name, cliCtx.String(flags.ProposerSettingsURLFlag.Name)).Infof("Proposer settings loaded from URL")
		case onlyDB:
			loadConfig = psl.processProposerSettings(nil, loadConfig)
			log.Info("Proposer settings loaded from the DB")
		case none:
			if psl.existsInDB {
				log.Info("Proposer settings loaded from the DB")
			}
			if psl.options.builderConfig != nil {
				// if there are no proposer settings provided, create a default where fee recipient is not populated, this will be skipped for validator registration on validators that don't have a fee recipient set.
				// skip saving to DB if only builder settings are provided until a trigger like keymanager API updates with fee recipient values
				option := &proposer.Option{
					BuilderConfig: psl.options.builderConfig.Clone(),
				}
				loadConfig.DefaultConfig = option.ToConsensus()
			}
		default:
			return nil, errors.New("load method for proposer settings does not exist")
		}
	}

	// exit early if nothing is provided
	if loadConfig == nil || (loadConfig.ProposerConfig == nil && loadConfig.DefaultConfig == nil) {
		log.Warn("No proposer settings were provided")
		return nil, nil
	}
	ps, err := proposer.SettingFromConsensus(loadConfig)
	if err != nil {
		return nil, err
	}
	if err := psl.db.SaveProposerSettings(cliCtx.Context, ps); err != nil {
		return nil, err
	}
	return ps, nil
}

func (psl *settingsLoader) processProposerSettings(loadedSettings, dbSettings *validatorpb.ProposerSettingsPayload) *validatorpb.ProposerSettingsPayload {
	if loadedSettings == nil && dbSettings == nil {
		return nil
	}

	// loaded settings have higher priority than db settings
	newSettings := &validatorpb.ProposerSettingsPayload{}

	var builderConfig *validatorpb.BuilderConfig
	var gasLimitOnly *validator.Uint64

	if psl.options != nil {
		if psl.options.builderConfig != nil {
			builderConfig = psl.options.builderConfig.ToConsensus()
		}
		if psl.options.gasLimit != nil {
			gasLimitOnly = psl.options.gasLimit
		}
	}

	if dbSettings != nil && dbSettings.DefaultConfig != nil {
		if builderConfig == nil {
			dbSettings.DefaultConfig.Builder = nil
		}
		newSettings.DefaultConfig = dbSettings.DefaultConfig
	}
	if loadedSettings != nil && loadedSettings.DefaultConfig != nil {
		newSettings.DefaultConfig = loadedSettings.DefaultConfig
	}

	// process any builder overrides on defaults
	if newSettings.DefaultConfig != nil {
		newSettings.DefaultConfig.Builder = processBuilderConfig(newSettings.DefaultConfig.Builder, builderConfig, gasLimitOnly)
	}

	if dbSettings != nil && len(dbSettings.ProposerConfig) != 0 {
		for _, option := range dbSettings.ProposerConfig {
			if builderConfig == nil {
				option.Builder = nil
			}
		}
		newSettings.ProposerConfig = dbSettings.ProposerConfig
	}
	if loadedSettings != nil && len(loadedSettings.ProposerConfig) != 0 {
		newSettings.ProposerConfig = loadedSettings.ProposerConfig
	}

	// process any overrides for proposer config
	for _, option := range newSettings.ProposerConfig {
		if option != nil {
			option.Builder = processBuilderConfig(option.Builder, builderConfig, gasLimitOnly)
		}
	}

	// if default and proposer configs are both missing even after db setting
	if newSettings.DefaultConfig == nil && newSettings.ProposerConfig == nil {
		return nil
	}

	return newSettings
}

func processBuilderConfig(current *validatorpb.BuilderConfig, override *validatorpb.BuilderConfig, gasLimitOnly *validator.Uint64) *validatorpb.BuilderConfig {
	if current != nil {
		current.GasLimit = reviewGasLimit(current.GasLimit)
		if override != nil {
			current.Enabled = override.Enabled
		}
		if gasLimitOnly != nil {
			current.GasLimit = *gasLimitOnly
		}
		return current
	}
	return override
}

func reviewGasLimit(gasLimit validator.Uint64) validator.Uint64 {
	// sets gas limit to default if not defined or set to 0
	if gasLimit == 0 {
		return validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	}
	// TODO(10810): add in warning for ranges
	return gasLimit
}
