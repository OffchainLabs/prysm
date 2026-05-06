package loader

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	"github.com/OffchainLabs/prysm/v7/config"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/validator/db/iface"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
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

type SettingsLoader struct {
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
type SettingsLoaderOption func(cliCtx *cli.Context, psl *SettingsLoader) error

// WithBuilderConfig applies the --enable-builder flag to proposer settings
func WithBuilderConfig() SettingsLoaderOption {
	return func(cliCtx *cli.Context, psl *SettingsLoader) error {
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
	return func(cliCtx *cli.Context, psl *SettingsLoader) error {
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
func NewProposerSettingsLoader(cliCtx *cli.Context, db iface.ValidatorDB, opts ...SettingsLoaderOption) (*SettingsLoader, error) {
	if cliCtx.IsSet(flags.ProposerSettingsFlag.Name) && cliCtx.IsSet(flags.ProposerSettingsURLFlag.Name) {
		return nil, fmt.Errorf("cannot specify both --%s and --%s flags; choose one method for specifying proposer settings", flags.ProposerSettingsFlag.Name, flags.ProposerSettingsURLFlag.Name)
	}
	psExists, err := db.ProposerSettingsExists(cliCtx.Context)
	if err != nil {
		return nil, err
	}
	psl := &SettingsLoader{db: db, existsInDB: psExists, options: &flagOptions{}}

	psl.loadMethods = determineLoadMethods(cliCtx, psl.existsInDB)

	for _, o := range opts {
		if err := o(cliCtx, psl); err != nil {
			return nil, err
		}
	}

	return psl, nil
}

func determineLoadMethods(cliCtx *cli.Context, loadedFromDB bool) []settingsType {
	var methods []settingsType

	if cliCtx.IsSet(flags.SuggestedFeeRecipientFlag.Name) {
		methods = append(methods, defaultFlag)
	}
	if cliCtx.IsSet(flags.ProposerSettingsFlag.Name) {
		methods = append(methods, fileFlag)
	}
	if cliCtx.IsSet(flags.ProposerSettingsURLFlag.Name) {
		methods = append(methods, urlFlag)
	}
	if len(methods) == 0 && loadedFromDB {
		methods = append(methods, onlyDB)
	}
	if len(methods) == 0 {
		methods = append(methods, none)
	}

	return methods
}

// Load saves the proposer settings to the database
func (psl *SettingsLoader) Load(cliCtx *cli.Context) (*proposer.Settings, error) {
	var loadedSettings, dbSettings *validatorpb.ProposerSettingsPayload

	// override settings based on other options
	psl.applyOverrides()

	// check if database has settings already
	if psl.existsInDB {
		dbps, err := psl.db.ProposerSettings(cliCtx.Context)
		if err != nil {
			return nil, err
		}
		dbSettings = dbps.ToConsensus()
		log.Debugf("DB loaded proposer settings: %s", func() string {
			b, err := json.Marshal(dbSettings)
			if err != nil {
				return err.Error()
			}
			return string(b)
		}())
	}

	// start to process based on load method
	for _, method := range psl.loadMethods {
		var err error
		switch method {
		case defaultFlag:
			loadedSettings, err = psl.loadFromDefault(cliCtx, dbSettings)
			if err != nil {
				return nil, err
			}
		case fileFlag:
			loadedSettings, err = psl.loadFromFile(cliCtx, dbSettings)
			if err != nil {
				return nil, err
			}
		case urlFlag:
			loadedSettings, err = psl.loadFromURL(cliCtx, dbSettings)
			if err != nil {
				return nil, err
			}
		case onlyDB, none:
			loadedSettings = psl.processProposerSettings(&validatorpb.ProposerSettingsPayload{}, dbSettings)
			if psl.existsInDB {
				log.Info("Proposer settings loaded from the DB")
			}
		default:
			return nil, errors.New("load method for proposer settings does not exist")
		}
	}

	// exit early if nothing is provided
	if loadedSettings == nil || (loadedSettings.ProposerConfig == nil && loadedSettings.DefaultConfig == nil) {
		log.Warn("No proposer settings were provided")
		return nil, nil
	}
	ps, err := proposer.SettingFromConsensus(loadedSettings)
	if err != nil {
		return nil, err
	}
	if err := validateSchemaVersion(ps); err != nil {
		return nil, err
	}
	if err := psl.db.SaveProposerSettings(cliCtx.Context, ps); err != nil {
		return nil, err
	}
	return ps, nil
}

// isGloasAware reports whether the running network has a configured Gloas fork epoch.
func isGloasAware() bool {
	return params.BeaconConfig().GloasForkEpoch < math.MaxUint64
}

// validateSchemaVersion rejects per-validator gas_limit on v2 settings:
// gas-limit signaling must be uniform across an operator's keys.
func validateSchemaVersion(ps *proposer.Settings) error {
	if ps == nil || ps.Version != proposer.SchemaV2 {
		return nil
	}
	for k, opt := range ps.ProposeConfig {
		if opt == nil {
			continue
		}
		if opt.GasLimit != 0 {
			label := fmt.Sprintf("proposer_config[%s]", hexutil.Encode(k[:]))
			return errors.Errorf("v2 proposer settings: per-validator 'gas_limit' on %s is not allowed; only default_config.gas_limit is honored", label)
		}
	}
	return nil
}

func (psl *SettingsLoader) applyOverrides() {
	if psl.options.builderConfig != nil && psl.options.gasLimit != nil {
		psl.options.builderConfig.GasLimit = *psl.options.gasLimit
	}
}

func (psl *SettingsLoader) loadFromDefault(cliCtx *cli.Context, dbSettings *validatorpb.ProposerSettingsPayload) (*validatorpb.ProposerSettingsPayload, error) {
	suggestedFeeRecipient := cliCtx.String(flags.SuggestedFeeRecipientFlag.Name)
	if !common.IsHexAddress(suggestedFeeRecipient) {
		return nil, errors.Errorf("--%s is not a valid Ethereum address", flags.SuggestedFeeRecipientFlag.Name)
	}
	if err := config.WarnNonChecksummedAddress(suggestedFeeRecipient); err != nil {
		return nil, err
	}

	if psl.existsInDB && len(psl.loadMethods) == 1 {
		// only log the below if default flag is the only load method
		log.Debug("Overriding previously saved proposer default settings.")
	}
	log.WithField(flags.SuggestedFeeRecipientFlag.Name, cliCtx.String(flags.SuggestedFeeRecipientFlag.Name)).Info("Proposer settings loaded from default")
	return psl.processProposerSettings(&validatorpb.ProposerSettingsPayload{DefaultConfig: &validatorpb.ProposerOptionPayload{
		FeeRecipient: suggestedFeeRecipient,
	}}, dbSettings), nil
}

func (psl *SettingsLoader) loadFromFile(cliCtx *cli.Context, dbSettings *validatorpb.ProposerSettingsPayload) (*validatorpb.ProposerSettingsPayload, error) {
	var settingFromFile *validatorpb.ProposerSettingsPayload
	if err := config.UnmarshalFromFile(cliCtx.String(flags.ProposerSettingsFlag.Name), &settingFromFile); err != nil {
		return nil, err
	}
	if settingFromFile == nil {
		return nil, errors.Errorf("proposer settings is empty after unmarshalling from file specified by %s flag", flags.ProposerSettingsFlag.Name)
	}
	log.WithField(flags.ProposerSettingsFlag.Name, cliCtx.String(flags.ProposerSettingsFlag.Name)).Info("Proposer settings loaded from file")
	return psl.processProposerSettings(settingFromFile, dbSettings), nil
}

func (psl *SettingsLoader) loadFromURL(cliCtx *cli.Context, dbSettings *validatorpb.ProposerSettingsPayload) (*validatorpb.ProposerSettingsPayload, error) {
	var settingFromURL *validatorpb.ProposerSettingsPayload
	if err := config.UnmarshalFromURL(cliCtx.Context, cliCtx.String(flags.ProposerSettingsURLFlag.Name), &settingFromURL); err != nil {
		return nil, err
	}
	if settingFromURL == nil {
		return nil, errors.Errorf("proposer settings is empty after unmarshalling from url specified by %s flag", flags.ProposerSettingsURLFlag.Name)
	}
	log.WithField(flags.ProposerSettingsURLFlag.Name, cliCtx.String(flags.ProposerSettingsURLFlag.Name)).Infof("Proposer settings loaded from URL")
	return psl.processProposerSettings(settingFromURL, dbSettings), nil
}

func (psl *SettingsLoader) processProposerSettings(loadedSettings, dbSettings *validatorpb.ProposerSettingsPayload) *validatorpb.ProposerSettingsPayload {
	if loadedSettings == nil && dbSettings == nil {
		return nil
	}

	// Merge settings with priority: loadedSettings > dbSettings
	newSettings := mergeProposerSettings(loadedSettings, dbSettings, psl.options)

	// Return nil if settings remain empty
	if newSettings.DefaultConfig == nil && len(newSettings.ProposerConfig) == 0 {
		return nil
	}

	return newSettings
}

// mergeProposerSettings merges database settings with loaded settings, giving precedence to loadedSettings
func mergeProposerSettings(loaded, db *validatorpb.ProposerSettingsPayload, options *flagOptions) *validatorpb.ProposerSettingsPayload {
	merged := &validatorpb.ProposerSettingsPayload{}

	if db != nil {
		merged.Version = db.Version
	}
	if loaded != nil && loaded.Version != 0 {
		merged.Version = loaded.Version
	}

	// Apply builder config overrides
	var builderConfig *validatorpb.BuilderConfig
	var gasLimitOnly *validator.Uint64

	if options != nil {
		if options.builderConfig != nil {
			builderConfig = options.builderConfig.ToConsensus()
		}
		if options.gasLimit != nil {
			gasLimitOnly = options.gasLimit
		}
	}

	// Capture deprecation warning intent before strip mutates the inputs.
	if isGloasAware() && merged.Version < proposerSchemaLatest &&
		(payloadHasBuilderRelay(loaded) || payloadHasBuilderRelay(db)) {
		defer log.Warn("Post-Gloas: builder/MEV-boost settings will not be honored. Remove 'builder.enabled' and 'builder.relays' from your proposer settings.")
	}

	// Merge DefaultConfig
	if db != nil && db.DefaultConfig != nil {
		merged.DefaultConfig = db.DefaultConfig
		if builderConfig == nil {
			db.DefaultConfig.Builder = stripMEVBoostRelay(db.DefaultConfig.Builder, gasLimitOnly)
		}
	}
	if loaded != nil && loaded.DefaultConfig != nil {
		merged.DefaultConfig = loaded.DefaultConfig
	}

	// Merge ProposerConfig
	if db != nil && len(db.ProposerConfig) > 0 {
		merged.ProposerConfig = db.ProposerConfig
		for _, option := range db.ProposerConfig {
			if builderConfig == nil {
				option.Builder = stripMEVBoostRelay(option.Builder, gasLimitOnly)
			}
		}
	}
	if loaded != nil && len(loaded.ProposerConfig) > 0 {
		merged.ProposerConfig = loaded.ProposerConfig
	}

	if merged.DefaultConfig != nil {
		merged.DefaultConfig.Builder = processBuilderConfig(merged.DefaultConfig.Builder, builderConfig, gasLimitOnly)
	}
	for _, option := range merged.ProposerConfig {
		if option != nil {
			option.Builder = processBuilderConfig(option.Builder, builderConfig, gasLimitOnly)
		}
	}

	if merged.DefaultConfig == nil {
		switch {
		case builderConfig != nil:
			merged.DefaultConfig = &validatorpb.ProposerOptionPayload{Builder: builderConfig}
		case gasLimitOnly != nil:
			merged.DefaultConfig = &validatorpb.ProposerOptionPayload{
				Builder: &validatorpb.BuilderConfig{
					Enabled:  false,
					GasLimit: *gasLimitOnly,
				},
			}
		}
	}

	applyProposerSchemaMigrations(merged, gasLimitOnly)
	return merged
}

// payloadHasBuilderRelay reports whether p has any BuilderConfig with
// MEV-boost relay fields set (Enabled or Relays) on default or per-validator.
func payloadHasBuilderRelay(p *validatorpb.ProposerSettingsPayload) bool {
	if p == nil {
		return false
	}
	hasRelay := func(b *validatorpb.BuilderConfig) bool {
		return b != nil && (b.Enabled || len(b.Relays) > 0)
	}
	if p.DefaultConfig != nil && hasRelay(p.DefaultConfig.Builder) {
		return true
	}
	for _, opt := range p.ProposerConfig {
		if opt != nil && hasRelay(opt.Builder) {
			return true
		}
	}
	return false
}

// proposerSchemaMigration upgrades a settings payload from one schema version
// to the next. Each migration is responsible for its own trigger logic.
type proposerSchemaMigration struct {
	from  uint32
	apply func(p *validatorpb.ProposerSettingsPayload, gasLimitOnly *validator.Uint64)
}

// proposerSchemaMigrations runs in order; add a new step here for each
// future schema bump. The dispatcher walks the chain on each load but each
// step's body fires at most once per validator lifetime — once a migration
// bumps p.Version, the early-return guard short-circuits subsequent loads.
var proposerSchemaMigrations = []proposerSchemaMigration{
	// Pre-versioned settings (proto3 zero value) and explicit v1 are the
	// same thing: legacy schema with gas limit on BuilderConfig.GasLimit.
	{from: proposer.SchemaV1Unset, apply: migrateV1ToV2},
	{from: proposer.SchemaV1, apply: migrateV1ToV2},
}

const proposerSchemaLatest = proposer.SchemaV2

func applyProposerSchemaMigrations(p *validatorpb.ProposerSettingsPayload, gasLimitOnly *validator.Uint64) {
	if p == nil || p.Version >= proposerSchemaLatest {
		return
	}
	for _, m := range proposerSchemaMigrations {
		if p.Version == m.from {
			m.apply(p, gasLimitOnly)
		}
	}
}

// migrateV1ToV2 lifts Builder.GasLimit to top-level Option.GasLimit on
// Gloas-aware networks. Builder fields are left intact for pre-Gloas MEV-boost.
func migrateV1ToV2(p *validatorpb.ProposerSettingsPayload, _ *validator.Uint64) {
	if p.DefaultConfig == nil || !isGloasAware() {
		return
	}
	if p.DefaultConfig.GasLimit == 0 && p.DefaultConfig.Builder != nil {
		p.DefaultConfig.GasLimit = p.DefaultConfig.Builder.GasLimit
	}
	if p.DefaultConfig.GasLimit != 0 {
		p.Version = proposer.SchemaV2
	}
}

// stripMEVBoostRelay clears the legacy relay fields, keeping gas_limit when
// it's still useful: --suggested-gas-limit was passed, or the network is
// Gloas-aware and the stored BuilderConfig has one. Otherwise drops the builder.
func stripMEVBoostRelay(b *validatorpb.BuilderConfig, gasLimitOnly *validator.Uint64) *validatorpb.BuilderConfig {
	if b == nil {
		return nil
	}
	keepGasLimit := gasLimitOnly != nil || (isGloasAware() && b.GasLimit != 0)
	if !keepGasLimit {
		return nil
	}
	b.Enabled = false
	b.Relays = nil
	return b
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
	if override != nil {
		return override
	}
	if gasLimitOnly != nil {
		return &validatorpb.BuilderConfig{Enabled: false, GasLimit: *gasLimitOnly}
	}
	return nil
}

func reviewGasLimit(gasLimit validator.Uint64) validator.Uint64 {
	// sets gas limit to default if not defined or set to 0
	if gasLimit == 0 {
		return validator.Uint64(params.BeaconConfig().DefaultBuilderGasLimit)
	}

	// Warning for ranges that might be problematic
	defaultGasLimit := params.BeaconConfig().DefaultBuilderGasLimit
	// If gas limit is very low (below 10% of default), warn about potential issues
	if gasLimit <= validator.Uint64(defaultGasLimit/10) {
		log.Warnf("Gas limit %d is very low compared to default %d, which may cause transactions to fail", gasLimit, defaultGasLimit)
	}
	// If gas limit is very high (above 150% of default), warn about potential block propagation issues
	if gasLimit > validator.Uint64(defaultGasLimit*3/2) {
		log.Warnf("Gas limit %d is very high compared to default %d", gasLimit, defaultGasLimit)
	}

	return gasLimit
}
