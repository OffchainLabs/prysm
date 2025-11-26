### Added

- Record data column gossip KZG batch verification latency in both the pooled worker and fallback paths so the `beacon_kzg_verification_data_column_batch_milliseconds` histogram reflects gossip traffic, annotated with `path` labels to distinguish the sources.
