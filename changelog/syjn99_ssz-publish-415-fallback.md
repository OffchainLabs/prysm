### Fixed

- REST VC now falls back to JSON on `415 Unsupported Media Type` — the code a beacon node actually returns when it rejects an SSZ request body — instead of on `406 Not Acceptable`.
- REST VC's SSZ publish requests now send `Accept: application/json`. Publish endpoints never produce SSZ response bodies, so preferring `application/octet-stream` was spec-noise that could draw a spurious `406` from servers with naive `Accept`/q-value parsing.
