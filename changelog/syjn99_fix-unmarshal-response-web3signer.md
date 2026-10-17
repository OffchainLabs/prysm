### Fixed

- Return the decode error from the remote web3signer client's response unmarshalling. A shadowed variable made it return `nil` on malformed responses, so `GetPublicKeys` reported zero public keys and no error.
