### Changed

- The validator client now warns at startup when per-key proposer settings saved in the validator DB (including changes made through the keymanager API) are replaced by the configured `--proposer-settings-file`/`--proposer-settings-url`, listing the dropped and overridden keys.
