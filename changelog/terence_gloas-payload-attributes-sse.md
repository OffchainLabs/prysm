### Fixed

- Emit the `payload_attributes` SSE event after the Gloas fork; it previously stopped at the fork boundary because the Gloas forkchoice-update path did not fire it.
