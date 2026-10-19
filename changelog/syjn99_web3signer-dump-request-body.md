### Fixed

- Web3signer: Return the status-specific error (400, 404, 412 slashing protection, 500) and log the failed request again. A failed request dump no longer replaces the error or leaks the response body.
