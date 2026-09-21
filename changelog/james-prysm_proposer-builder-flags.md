### Added

- Validator client flags `--builder-urls`, `--builder-min-bid`, `--builder-boost-factor` and `--builder-max-execution-payment` to configure Gloas builders for all validators from the command line. Builder auth data can be appended to a URL as a `#0x...` hex fragment. Before Gloas a non-empty `--builder-urls` list also enables builder registration. Default builder settings, from these flags or a settings file, apply per run; a restart without them does not carry them over from the validator DB.
