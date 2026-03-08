
### Fixed

- Fix nil dereference (introduced by ePBS) in `signedExecutionPayloadEnvelope.IsNil()` by adding a nil check for `s.s.Message`.