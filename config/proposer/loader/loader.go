package loader

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	"github.com/OffchainLabs/prysm/v7/config"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/io/logs"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/validator/db/iface"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/proto"
)

// maxLoggedKeys caps the key lists in the DB-replacement warning.
const maxLoggedKeys = 10

// builderDefaultFlags write v2 builder content into default_config and make the flags a v2 source.
var builderDefaultFlags = []cli.Flag{
	flags.BuilderURLsFlag,
	flags.BuilderMinBidFlag,
	flags.BuilderBoostFactorFlag,
	flags.BuilderMaxExecutionPaymentFlag,
}

type settingsType int

const (
	none settingsType = iota
	defaultFlag
	fileFlag
	urlFlag
	onlyDB
)

type SettingsLoader struct {
	loadMethods    []settingsType
	existsInDB     bool
	replacesDBKeys bool
	db             iface.ValidatorDB
	options        *flagOptions
}

type flagOptions struct {
	builderConfig   *proposer.BuilderConfig
	gasLimit        *validator.Uint64
	builderFlagsSet bool
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
		if !cliCtx.IsSet(flags.BuilderGasLimitFlag.Name) {
			return nil
		}
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
	psl := &SettingsLoader{
		db:         db,
		existsInDB: psExists,
		options:    &flagOptions{builderFlagsSet: len(setBuilderFlagNames(cliCtx)) > 0},
	}

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

	if cliCtx.IsSet(flags.SuggestedFeeRecipientFlag.Name) || len(setBuilderFlagNames(cliCtx)) > 0 {
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
	var dbps *proposer.Settings

	// override settings based on other options
	psl.applyOverrides()

	// check if database has settings already
	if psl.existsInDB {
		var err error
		dbps, err = psl.db.ProposerSettings(cliCtx.Context)
		if err != nil {
			return nil, err
		}
		dbSettings = dbps.ToConsensus()
		log.WithField("version", dbSettings.Version).
			WithField("proposerConfigCount", len(dbSettings.ProposerConfig)).
			Debug("Loaded proposer settings from DB")
	}
	// Captured before the merges below rewrite the DB payload in place.
	hadDefaultBuilders := hasDefaultBuilders(dbSettings)

	// start to process based on load method,
	// each method merges onto the previous method's result.
	base := dbSettings
	for _, method := range psl.loadMethods {
		var err error
		switch method {
		case defaultFlag:
			loadedSettings, err = psl.loadFromDefault(cliCtx, base)
			if err != nil {
				return nil, err
			}
		case fileFlag:
			loadedSettings, err = psl.loadFromFile(cliCtx, base)
			if err != nil {
				return nil, err
			}
		case urlFlag:
			loadedSettings, err = psl.loadFromURL(cliCtx, base)
			if err != nil {
				return nil, err
			}
		case onlyDB, none:
			loadedSettings = psl.processProposerSettings(&validatorpb.ProposerSettingsPayload{}, base)
			if psl.existsInDB {
				log.Info("Proposer settings loaded from the DB")
			}
		default:
			return nil, errors.New("load method for proposer settings does not exist")
		}
		base = loadedSettings
	}
	if hadDefaultBuilders && !hasDefaultBuilders(loadedSettings) {
		log.Warn("Dropped the default builder settings a previous run stored in the validator DB because neither builder flags nor a settings source configured a builders list this run; pass --" +
			flags.BuilderURLsFlag.Name + " or the settings file on every start")
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
	ps.WarnDeprecatedSchema()
	ps.WarnUnsetMaxExecutionPayment()
	if psl.replacesDBKeys {
		warnReplacedDBKeys(dbps, ps)
	}
	// Flag-only builder defaults are rebuilt every run and never persisted on their own.
	if !ps.ShouldBeSaved() {
		log.Debug("Proposer settings carry nothing to persist; validator DB left unchanged")
		return ps, nil
	}
	if err := psl.db.SaveProposerSettings(cliCtx.Context, ps); err != nil {
		return nil, err
	}
	return ps, nil
}

// warnReplacedDBKeys lists the DB per-key entries the settings file/URL replaced.
// Comparing normalized settings keeps an unchanged restart quiet.
func warnReplacedDBKeys(db, merged *proposer.Settings) {
	if db == nil || len(db.ProposeConfig) == 0 {
		return
	}
	var dropped, overridden []string
	for key, dbOpt := range db.ProposeConfig {
		opt, ok := merged.ProposeConfig[key]
		switch {
		case !ok:
			dropped = append(dropped, fmt.Sprintf("%#x", key))
		case !proto.Equal(opt.ToConsensus(), dbOpt.ToConsensus()):
			overridden = append(overridden, fmt.Sprintf("%#x", key))
		}
	}
	if len(dropped) == 0 && len(overridden) == 0 {
		return
	}
	log.WithField("droppedKeys", capKeys(dropped)).
		WithField("droppedCount", len(dropped)).
		WithField("overriddenKeys", capKeys(overridden)).
		WithField("overriddenCount", len(overridden)).
		Warn("Per-key proposer settings saved in the validator DB by a previous run differ from the configured settings file/URL; " +
			"the settings source is authoritative and the DB entries are replaced. " +
			"Changes made through the keymanager API do not survive a restart while a settings file or URL is configured")
}

// hasDefaultBuilders reports whether default_config names at least one builder.
func hasDefaultBuilders(p *validatorpb.ProposerSettingsPayload) bool {
	return p != nil && p.DefaultConfig != nil && len(p.DefaultConfig.Builder.GetBuilders()) > 0
}

// capKeys renders a sorted key list, truncated to maxLoggedKeys with a "+N more" tail.
func capKeys(keys []string) string {
	sort.Strings(keys)
	if len(keys) > maxLoggedKeys {
		return fmt.Sprintf("%s +%d more", strings.Join(keys[:maxLoggedKeys], ","), len(keys)-maxLoggedKeys)
	}
	return strings.Join(keys, ",")
}

func (psl *SettingsLoader) applyOverrides() {
	if psl.options.builderConfig != nil && psl.options.gasLimit != nil {
		psl.options.builderConfig.GasLimit = *psl.options.gasLimit
	}
}

// loadFromDefault builds default_config from the flags; the builder flags make it a v2 source.
func (psl *SettingsLoader) loadFromDefault(cliCtx *cli.Context, dbSettings *validatorpb.ProposerSettingsPayload) (*validatorpb.ProposerSettingsPayload, error) {
	option := &validatorpb.ProposerOptionPayload{}
	loaded := &validatorpb.ProposerSettingsPayload{DefaultConfig: option}
	logEntry := log
	if cliCtx.IsSet(flags.SuggestedFeeRecipientFlag.Name) {
		suggestedFeeRecipient := cliCtx.String(flags.SuggestedFeeRecipientFlag.Name)
		if !common.IsHexAddress(suggestedFeeRecipient) {
			return nil, errors.Errorf("--%s is not a valid Ethereum address", flags.SuggestedFeeRecipientFlag.Name)
		}
		if err := config.WarnNonChecksummedAddress(suggestedFeeRecipient); err != nil {
			return nil, err
		}
		option.FeeRecipient = suggestedFeeRecipient
		logEntry = logEntry.WithField(flags.SuggestedFeeRecipientFlag.Name, suggestedFeeRecipient)
	} else if dbSettings != nil && dbSettings.DefaultConfig != nil {
		// Builder flags alone replace only the builder half of the persisted default.
		option.FeeRecipient = dbSettings.DefaultConfig.FeeRecipient
		option.GasLimit = dbSettings.DefaultConfig.GasLimit
	}
	builder, err := builderConfigFromFlags(cliCtx)
	if err != nil {
		return nil, err
	}
	if builder != nil {
		option.Builder = builder.ToConsensus()
		loaded.Version = proposer.SchemaV2
		if len(builder.Builders) > 0 {
			logEntry = logEntry.WithField("builders", maskedBuilderURLs(builder.Builders))
		}
		if !params.GloasEnabled() {
			log.Warnf("%s configure Gloas builders, but this network has no Gloas fork scheduled", strings.Join(setBuilderFlagNames(cliCtx), ", "))
		}
	}

	if psl.existsInDB && len(psl.loadMethods) == 1 {
		// only log the below if default flag is the only load method
		log.Debug("Overriding previously saved proposer default settings.")
	}
	logEntry.Info("Proposer settings loaded from default")
	return psl.processProposerSettings(loaded, dbSettings), nil
}

// setBuilderFlagNames lists the builder default flags present on the command line, "--" prefixed.
func setBuilderFlagNames(cliCtx *cli.Context) []string {
	var names []string
	for _, f := range builderDefaultFlags {
		if name := f.Names()[0]; cliCtx.IsSet(name) {
			names = append(names, "--"+name)
		}
	}
	return names
}

// builderConfigFromFlags assembles the default_config builder from the builder flags; nil when none is set.
func builderConfigFromFlags(cliCtx *cli.Context) (*proposer.BuilderConfig, error) {
	if len(setBuilderFlagNames(cliCtx)) == 0 {
		return nil, nil
	}
	bc := &proposer.BuilderConfig{}
	if cliCtx.IsSet(flags.BuilderURLsFlag.Name) {
		raw := cliCtx.StringSlice(flags.BuilderURLsFlag.Name)
		if len(raw) > proposer.MaxBuilderEntries {
			return nil, errors.Errorf("--%s lists more than %d builders", flags.BuilderURLsFlag.Name, proposer.MaxBuilderEntries)
		}
		seen := make(map[proposer.EntryIdentity]bool, len(raw))
		bc.Builders = make([]*proposer.BuilderEntry, 0, len(raw))
		for _, r := range raw {
			be, err := parseBuilderURL(r)
			if err != nil {
				return nil, errors.Wrapf(err, "--%s", flags.BuilderURLsFlag.Name)
			}
			if seen[be.Identity()] {
				return nil, errors.Errorf("--%s lists %s more than once", flags.BuilderURLsFlag.Name, logs.MaskCredentialsLogging(be.URL))
			}
			seen[be.Identity()] = true
			bc.Builders = append(bc.Builders, be)
		}
	}
	bc.MinBid = uint64FlagValue(cliCtx, flags.BuilderMinBidFlag)
	bc.BuilderBoostFactor = uint64FlagValue(cliCtx, flags.BuilderBoostFactorFlag)
	bc.MaxExecutionPayment = uint64FlagValue(cliCtx, flags.BuilderMaxExecutionPaymentFlag)
	return bc, nil
}

// uint64FlagValue returns nil for an unset flag so an explicit 0 stays distinct from "inherit".
func uint64FlagValue(cliCtx *cli.Context, f *cli.Uint64Flag) *validator.Uint64 {
	if !cliCtx.IsSet(f.Name) {
		return nil
	}
	v := validator.Uint64(cliCtx.Uint64(f.Name))
	return &v
}

// parseBuilderURL splits a --builder-urls entry into its URL and optional "#0x..." auth fragment.
func parseBuilderURL(raw string) (*proposer.BuilderEntry, error) {
	rawURL, fragment, hasFragment := strings.Cut(strings.TrimSpace(raw), "#")
	if rawURL == "" {
		return nil, errors.New("empty builder url")
	}
	be := &proposer.BuilderEntry{URL: rawURL}
	if hasFragment {
		auth, err := hexutil.Decode(fragment)
		if err != nil {
			return nil, errors.Errorf("auth fragment of %s is not 0x-prefixed hex", logs.MaskCredentialsLogging(rawURL))
		}
		be.AuthData = auth
	}
	if err := be.Validate(); err != nil {
		return nil, errors.Wrap(err, logs.MaskCredentialsLogging(rawURL))
	}
	return be, nil
}

func maskedBuilderURLs(entries []*proposer.BuilderEntry) string {
	urls := make([]string, 0, len(entries))
	for _, be := range entries {
		urls = append(urls, logs.MaskCredentialsLogging(be.URL))
	}
	return strings.Join(urls, ",")
}

// A source's default_config replaces the flag-built one whole, builder flags included.
func warnBuilderFlagsReplaced(cliCtx *cli.Context, loaded *validatorpb.ProposerSettingsPayload, source string) {
	names := setBuilderFlagNames(cliCtx)
	if loaded.DefaultConfig == nil || len(names) == 0 {
		return
	}
	log.Warnf("default_config from --%s replaces the builder defaults set by %s", source, strings.Join(names, ", "))
}

func (psl *SettingsLoader) loadFromFile(cliCtx *cli.Context, dbSettings *validatorpb.ProposerSettingsPayload) (*validatorpb.ProposerSettingsPayload, error) {
	var settingFromFile *validatorpb.ProposerSettingsPayload
	if err := config.UnmarshalFromFile(cliCtx.String(flags.ProposerSettingsFlag.Name), &settingFromFile); err != nil {
		return nil, err
	}
	if settingFromFile == nil {
		return nil, errors.Errorf("proposer settings is empty after unmarshalling from file specified by %s flag", flags.ProposerSettingsFlag.Name)
	}
	markExplicitEmptyBuilders(settingFromFile)
	inferSchemaVersion(settingFromFile)
	warnBuilderFlagsReplaced(cliCtx, settingFromFile, flags.ProposerSettingsFlag.Name)
	psl.replacesDBKeys = len(settingFromFile.ProposerConfig) > 0
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
	markExplicitEmptyBuilders(settingFromURL)
	inferSchemaVersion(settingFromURL)
	warnBuilderFlagsReplaced(cliCtx, settingFromURL, flags.ProposerSettingsURLFlag.Name)
	psl.replacesDBKeys = len(settingFromURL.ProposerConfig) > 0
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

// mergeProposerSettings merges database settings with loaded settings, giving
// precedence to loadedSettings. Dispatches by schema version: v1 still flows
// through Builder; v2 lives on Option directly.
func mergeProposerSettings(loaded, db *validatorpb.ProposerSettingsPayload, options *flagOptions) *validatorpb.ProposerSettingsPayload {
	merged := &validatorpb.ProposerSettingsPayload{}
	if db != nil {
		merged.Version = db.Version
	}
	if loaded != nil && loaded.Version > merged.Version {
		merged.Version = loaded.Version
	}

	var builderConfig *validatorpb.BuilderConfig
	var gasLimitOnly *validator.Uint64
	builderFlagsSet := false
	if options != nil {
		if options.builderConfig != nil {
			builderConfig = options.builderConfig.ToConsensus()
		}
		gasLimitOnly = options.gasLimit
		builderFlagsSet = options.builderFlagsSet
	}

	if merged.Version == proposer.SchemaV2 {
		return mergeProposerSettingsV2(merged, loaded, db, builderConfig, gasLimitOnly, builderFlagsSet)
	}
	return mergeProposerSettingsV1(merged, loaded, db, builderConfig, gasLimitOnly)
}

// hasGloasBuilderFields reports whether a payload builder configures the Gloas builder API; an explicit empty list counts.
func hasGloasBuilderFields(b *validatorpb.BuilderConfig) bool {
	return b != nil && (len(b.Builders) > 0 || b.BuildersSet || b.MinBid != nil ||
		b.BuilderBoostFactor != nil || b.MaxExecutionPayment != nil)
}

// clearBuilderFlagFields clears the fields the builder flags own; a config left with zero legacy fields disappears.
func clearBuilderFlagFields(b *validatorpb.BuilderConfig) *validatorpb.BuilderConfig {
	if b == nil {
		return nil
	}
	b.Builders, b.BuildersSet, b.MinBid, b.BuilderBoostFactor, b.MaxExecutionPayment = nil, false, nil, nil, nil
	if !b.Enabled && b.GasLimit == 0 {
		return nil
	}
	return b
}

// enableLegacyPerKeyBuilders opts legacy-only per-key blocks in, as v1 --enable-builder
// does. Blocks with v2 content, including builders: [], keep their own choice.
func enableLegacyPerKeyBuilders(merged *validatorpb.ProposerSettingsPayload) {
	for _, opt := range merged.ProposerConfig {
		if opt != nil && opt.Builder != nil && !hasGloasBuilderFields(opt.Builder) {
			opt.Builder.Enabled = true
		}
	}
}

// markExplicitEmptyBuilders stamps the persistence marker for a user source's
// explicit "builders": [] (opt-out), which yaml keeps distinct from absent.
func markExplicitEmptyBuilders(p *validatorpb.ProposerSettingsPayload) {
	mark := func(opt *validatorpb.ProposerOptionPayload) {
		if opt == nil || opt.Builder == nil {
			return
		}
		if opt.Builder.Builders != nil {
			opt.Builder.BuildersSet = true
		}
	}
	mark(p.DefaultConfig)
	for _, opt := range p.ProposerConfig {
		mark(opt)
	}
}

// inferSchemaVersion stamps version 2 on an unversioned source carrying v2-only
// builder fields, so a forgotten "version" cannot get gloas content dropped as v1.
func inferSchemaVersion(p *validatorpb.ProposerSettingsPayload) {
	if p.Version != proposer.SchemaV1Unset {
		return
	}
	hasV2 := func(opt *validatorpb.ProposerOptionPayload) bool {
		return opt != nil && hasGloasBuilderFields(opt.Builder)
	}
	found := hasV2(p.DefaultConfig)
	for _, opt := range p.ProposerConfig {
		if found {
			break
		}
		found = hasV2(opt)
	}
	if !found {
		return
	}
	p.Version = proposer.SchemaV2
	log.Info("Proposer settings contain v2 builder fields but no version; treating the source as version 2")
}

// selectProposerConfig keeps the pre-v2 source precedence: a loaded per-key
// section replaces the DB's entirely, so restarting with a file resets the DB.
// Load reports what that replaced through warnReplacedDBKeys.
func selectProposerConfig(db, loaded *validatorpb.ProposerSettingsPayload) map[string]*validatorpb.ProposerOptionPayload {
	if loaded != nil && len(loaded.ProposerConfig) > 0 {
		return loaded.ProposerConfig
	}
	if db != nil && len(db.ProposerConfig) > 0 {
		return db.ProposerConfig
	}
	return nil
}

func mergeProposerSettingsV1(merged, loaded, db *validatorpb.ProposerSettingsPayload, builderConfig *validatorpb.BuilderConfig, gasLimitOnly *validator.Uint64) *validatorpb.ProposerSettingsPayload {
	stripDBBuilder := builderConfig == nil

	if db != nil && db.DefaultConfig != nil {
		merged.DefaultConfig = db.DefaultConfig
		if stripDBBuilder {
			db.DefaultConfig.Builder = nil
		}
	}
	if loaded != nil && loaded.DefaultConfig != nil {
		merged.DefaultConfig = loaded.DefaultConfig
	}

	if db != nil && stripDBBuilder {
		for _, option := range db.ProposerConfig {
			option.Builder = nil
		}
	}
	merged.ProposerConfig = selectProposerConfig(db, loaded)

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
				Builder: &validatorpb.BuilderConfig{GasLimit: *gasLimitOnly},
			}
		}
	}
	return merged
}

func mergeProposerSettingsV2(merged, loaded, db *validatorpb.ProposerSettingsPayload, builderConfig *validatorpb.BuilderConfig, gasLimitOnly *validator.Uint64, builderFlagsSet bool) *validatorpb.ProposerSettingsPayload {
	// Builder flags are per-run: a run without them drops the v2 builder fields an
	// earlier flag run persisted in default_config. Legacy fields follow their own flags.
	if db != nil && db.DefaultConfig != nil && !builderFlagsSet {
		db.DefaultConfig.Builder = clearBuilderFlagFields(db.DefaultConfig.Builder)
	}
	if db != nil && db.DefaultConfig != nil {
		merged.DefaultConfig = db.DefaultConfig
	}
	if loaded != nil && loaded.DefaultConfig != nil {
		merged.DefaultConfig = loaded.DefaultConfig
	}
	merged.ProposerConfig = selectProposerConfig(db, loaded)

	if builderFlagsSet && merged.DefaultConfig != nil && len(merged.DefaultConfig.Builder.GetBuilders()) > 0 {
		enableLegacyPerKeyBuilders(merged)
	}

	// --enable-builder is legacy content: it still forces the default mev-boost
	// toggle on for pre-gloas registrations, and is inert from the fork onward.
	if builderConfig != nil {
		if merged.DefaultConfig == nil {
			merged.DefaultConfig = &validatorpb.ProposerOptionPayload{}
		}
		if merged.DefaultConfig.Builder == nil {
			merged.DefaultConfig.Builder = &validatorpb.BuilderConfig{}
		}
		merged.DefaultConfig.Builder.Enabled = true
		log.Warnf("--%s is legacy (pre-gloas) mev-boost content and has no effect after the gloas fork; configure builders via the settings source or keymanager API", flags.EnableBuilderFlag.Name)
	}

	// --suggested-gas-limit is likewise legacy content: it applies to the
	// pre-gloas builder gas limit and never overrides v2 or schedule values.
	if gasLimitOnly != nil {
		if merged.DefaultConfig == nil {
			merged.DefaultConfig = &validatorpb.ProposerOptionPayload{}
		}
		if merged.DefaultConfig.Builder == nil {
			merged.DefaultConfig.Builder = &validatorpb.BuilderConfig{}
		}
		merged.DefaultConfig.Builder.GasLimit = *gasLimitOnly
		log.Warnf("--%s is legacy (pre-gloas) content and has no effect after the gloas fork; set gas limits in v2 proposer settings or via the keymanager API", flags.BuilderGasLimitFlag.Name)
	}
	return merged
}

func processBuilderConfig(current *validatorpb.BuilderConfig, override *validatorpb.BuilderConfig, gasLimitOnly *validator.Uint64) *validatorpb.BuilderConfig {
	if current != nil {
		if gasLimitOnly != nil {
			current.GasLimit = *gasLimitOnly
		} else {
			current.GasLimit = reviewGasLimit(current.GasLimit)
		}
		if override != nil {
			current.Enabled = override.Enabled
		}
		return current
	}
	if override != nil {
		return override
	}
	if gasLimitOnly != nil {
		return &validatorpb.BuilderConfig{GasLimit: *gasLimitOnly}
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
