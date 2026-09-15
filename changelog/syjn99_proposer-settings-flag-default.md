### Fixed

- `--suggested-fee-recipient` is now applied as the default fee recipient when `--proposer-settings-file`/`--proposer-settings-url` is also set but the settings source has no `default_config`; previously the flag was silently dropped and validators not listed in the file were registered with the burn address.
