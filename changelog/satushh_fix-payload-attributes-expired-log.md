### Fixed

- Stop logging skipped `payload_attributes` events for past proposal slots as an `ERROR` ("received an event it was unable to handle") in the beacon node event stream. The `errPayloadAttributeExpired` skip is now excused like `errNotRequested`, removing high-volume noise under ePBS. The `event_type` log field now prints the event type (`%T`) instead of dumping the full event struct.
