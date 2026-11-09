### Fixed

- Redact basic-auth credentials from Web3Signer URLs in client errors, tracing attributes, validator start-up errors (`--validators-external-signer-url` and `--validators-external-signer-public-keys`) and the `GET /eth/v1/remotekeys` response, so they are no longer written to the validator logs or served to keymanager API clients. Discovered by Anthropic's OSS Scanner.
