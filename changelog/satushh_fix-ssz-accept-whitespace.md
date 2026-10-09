### Fixed

- Trim whitespace when parsing `Accept` header media types for SSZ responses, so types listed after a comma and space (e.g. `application/json;q=0, application/octet-stream`) are no longer ignored.
