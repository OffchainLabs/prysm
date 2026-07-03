### Changed

- REST Validator client now subscribes to the `head_v2` beacon API event by default instead of the deprecated `head` event. If the beacon node does not support `head_v2` (it rejects the subscription with HTTP 400), the validator client automatically falls back to the legacy `head` event, so it keeps working against older beacon nodes with no behavior change.
