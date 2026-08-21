### Changed

- Block requests now carry resolved builder entries (url, auth, pubkeys, limits) instead of bare request auths, request auths sign the entry's auth data rather than its URL, and the beacon node routes builder requests by the entry url.
