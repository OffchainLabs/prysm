### Fixed

- When a payload envelope arrives, derive the head's full/empty status from forkchoice (`FullBeatsEmpty`/`choosePayloadContent`) instead of unconditionally marking the head full.
