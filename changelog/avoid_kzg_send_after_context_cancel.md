### Fixed

- Prevent blocked sends to the KZG batch verifier when the caller context is already canceled, avoiding useless queueing and potential hangs.
