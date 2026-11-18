### Fixed

- Fixed light client bootstrap P2P requests being reset before reaching the handler, because the request type lacked SSZ decoding.
- Fixed light client bootstrap P2P requests returning ServerError instead of ResourceUnavailable when the bootstrap is missing.
- Fixed light client updates-by-range P2P requests being reset before reaching the handler, because the request type lacked SSZ decoding.
