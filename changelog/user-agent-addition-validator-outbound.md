### Added

- All outbound HTTP requests from the validator client now include a custom `User-Agent` header in the format `Prysm/<name>/<version>`. This enhances observability and enables upstream systems to correctly identify Prysm validator clients by their name and version.
- Fixes [#15435](https://github.com/OffchainLabs/prysm/issues/15435).
