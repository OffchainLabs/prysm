### Fixed

- Gloas initial sync now verifies a parent execution payload envelope and checks that its data columns are available before importing it, the same checks applied to envelopes received over gossip. Previously it was imported without them, so a payload with blobs at a batch boundary could be imported before its data was confirmed available.
- Gloas initial sync envelope range requests are capped at `MAX_REQUEST_PAYLOADS`; fork recovery could previously request more.
- Gloas initial sync now saves a verified parent payload envelope as soon as it is imported. Previously, if a later block in the same batch was rejected, the envelope was never saved even though forkchoice already treated the parent's payload as present.
