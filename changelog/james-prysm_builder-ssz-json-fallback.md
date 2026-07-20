### Added

- Builder client now automatically falls back to JSON when a remote builder rejects SSZ requests (415/406), so builders without SSZ support (e.g. Commit Boost) work without needing `--disable-builder-ssz`. Covers `GetHeader`, `RegisterValidator`, `SubmitBlindedBlock`, `SubmitBlindedBlockPostFulu`, and the Gloas `GetExecutionPayloadBid` and `SubmitBuilderPreferences` endpoints. Once a builder rejects SSZ, the client uses JSON for the remainder of its lifetime.
