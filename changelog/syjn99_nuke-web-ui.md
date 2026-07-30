### Removed

- Removed the Prysm web UI from the validator client, including the bundled `prysm-web-ui` site assets. `--web` is now a hidden deprecated no-op so existing configs keep starting; note that `--web` alone no longer starts the HTTP server, so anyone who relied on it (rather than `--rpc`) to expose the keymanager API must now pass `--rpc`.
- Removed the web-only `/v2/validator/*` HTTP API that existed solely to back the web UI (accounts, wallet, beacon, health/log streaming and slashing protection import/export endpoints). The standard keymanager API under `/eth/v1/*` is unaffected, as are the equivalent `validator accounts`, `validator wallet` and `validator slashing-protection` CLI commands.
- Removed the unused `--write-wallet-password-on-web-onboarding` flag.
- Removed the dead gRPC auth token interceptor from the validator client.
- Removed the beacon node client methods that only the web UI handlers called: `ChainClient.ChainHead`, `ChainClient.ValidatorBalances`, `ChainClient.Validators`, `NodeClient.Version` and `NodeClient.Peers`, along with the REST node client's now-unused gRPC fallback and the unused `StateValidatorsForSlot`/`StateValidatorsForHead` provider methods.

### Changed

- **Breaking:** `validator web generate-auth-token` is now `validator generate-auth-token`. This affects everyone who uses the keymanager API, not just web UI users, since it is how the API bearer token is generated. `--wallet-dir` is still accepted for compatibility but now logs a warning that it does not affect where the token is written — it never did, because `--keymanager-token-file` has a non-empty default that always took precedence. Note that the validator binary logs command errors without a non-zero exit code, so scripts still invoking `validator web generate-auth-token` will log `unrecognized argument: web` and continue.
- The validator client no longer waits for a wallet to be created at runtime; a missing wallet now fails at startup instead of after connecting to the beacon node. The web UI's wallet-create endpoint was the only way to supply one at runtime.
- The default `--http-cors-domain` no longer includes the `prysm-web-ui` development server origins (ports 3000, 4200 and 4242); it now defaults to port 7500 only.
