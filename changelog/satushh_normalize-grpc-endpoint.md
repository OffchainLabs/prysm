### Changed

- gRPC endpoint flags (e.g., `--beacon-rpc-provider`) now accept URL-style values such as `http://localhost:4000` and normalize them to `host:port` form. Previously only bare `host:port` was accepted, so pasting a URL with an `http(s)://` scheme would fail to dial; a warning is logged on normalization because `https://` does not enable gRPC TLS.
