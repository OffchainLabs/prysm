### Added

- New ssz-only flag for validator client to enable calling rest apis in SSZ, starting with get block endpoint.

### Changed

- when REST api is enabled the get Block api defaults to requesting and receiving SSZ instead of JSON, JSON is the fallback.