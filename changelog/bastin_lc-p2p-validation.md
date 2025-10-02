### Changed

- Compare received LC messages over gossipsub with locally computed ones before forwarding. Also no longer save updates
  from gossipsub, just validate and forward.