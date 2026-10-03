### Ignored
- enforce ssz limits that have migrated to the stf at unmarshal time.
- skip spectest vectors whose only defect is an over-limit list, since that limit now rejects them at unmarshal time.
- drop the Electra deposit request cap from the gloas `ExecutionRequests` SSZ decoder and REST conversion, since gloas has no `MAX_DEPOSIT_REQUESTS_PER_PAYLOAD` (consensus-specs #5436).
