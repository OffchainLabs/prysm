### Fixed 
- `buildStatusFromStream`: Respond `statusV2` only if Fulu is enabled.
- Send our real earliest available slot when sending a Status request post Fulu instead of `0`.