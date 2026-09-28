### Ignored

- Read the Gloas fork-choice spectest `head.payload_status` check from the head object. The check silently never ran because it looked for a top-level `head_payload_status` key that the vectors no longer carry.
