### Changed

- The REST VC now caches SSZ request-body support per beacon node host and endpoint for a day, so submissions to an endpoint that previously rejected SSZ skip straight to JSON instead of paying a failed SSZ round trip on every call.
