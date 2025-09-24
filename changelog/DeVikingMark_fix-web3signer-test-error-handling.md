### Fixed

- Replace fmt.Printf with proper test error handling in web3signer keymanager tests, using require.NoError(t, err) instead of t.Fatalf for better error handling and debugging.
