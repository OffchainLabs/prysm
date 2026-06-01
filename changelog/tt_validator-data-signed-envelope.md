### Fixed

- Preserve the full signed execution payload envelope in pubsub validation by setting `msg.ValidatorData = signedEnvelope` after successful validation.
