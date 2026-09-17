### Fixed

- Derive the justified and finalized checkpoint epoch persisted by `SaveOrigin` from the origin state slot instead of the origin block slot, so checkpoint sync records the correct epoch when the checkpoint epoch's first slot is empty.
