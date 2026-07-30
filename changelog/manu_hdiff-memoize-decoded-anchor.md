### Changed

- State diff: keep the last decoded anchor state in the anchor cache. Saves at deeper levels read the
  same anchor repeatedly, and each read was a full SSZ unmarshal of the beacon state (~1GB of
  allocation and over 2 seconds on mainnet). The anchor is also no longer decoded while the cache lock
  is held.
