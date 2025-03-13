# prysmctl - Prysm Control Tool

The `prysmctl` command line tool helps operators and developers perform various utility operations with Prysm, such as:

- Extracting information from the beacon node's database
- Performing low-level P2P operations
- Generating test data
- Managing validator keys and accounts

## Fork Version Support

Many commands in `prysmctl` now support a `--fork` flag that allows you to explicitly set which consensus layer fork version to use:

```
--fork value  fork version to use (phase0, altair, bellatrix, capella, deneb, electra, fulu)
```

This is particularly useful for testing P2P operations with different protocol versions, analyzing chain data from different forks, or testing compatibility between nodes running different fork versions.

### Example Usage:

Request blocks using Deneb fork:
```
prysmctl p2p send beacon-blocks-by-range --fork=deneb --count=10
```

Request blobs (requires Deneb or later):
```
prysmctl p2p send blobs-by-range --fork=deneb --count=5
```

Test future fork compatibility:
```
prysmctl p2p send beacon-blocks-by-range --fork=electra
```

If the `--fork` flag is not specified, the tool will auto-detect the appropriate fork version based on the current epoch.