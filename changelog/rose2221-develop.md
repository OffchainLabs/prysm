### Added

- **Gzip Compression for Beacon API:**  
  Fixed an issue where the beacon chain server ignored the `Accept-Encoding: gzip` header and returned uncompressed JSON responses. With this change, endpoints that use the `AcceptHeaderHandler` now also compress responses when a client requests gzip encoding. 
  Fixes [#14593](https://github.com/prysmaticlabs/prysm/issues/14593).
