# DoctorPeer

A dashboard for debugging peer connectivity and grey-listing, served by the beacon node itself.

## Enabling it

Start the beacon node with `--enable-doctor-peer` and open `/doctorpeer/` on the HTTP API port
(`--http-host`, `--http-port`; defaults `127.0.0.1` and `3500`):

```sh
beacon-chain --enable-doctor-peer ...
open http://127.0.0.1:3500/doctorpeer/
```

For a remote node, tunnel the API port rather than exposing it:

```sh
ssh -N -L 3500:localhost:3500 user@node
open http://localhost:3500/doctorpeer/
```

## Pointing the page at another node

The rail footer shows the node URL the page is polling; click it to change it. A link can also
carry its own target with `?node=`:

```
http://localhost:3500/doctorpeer/?node=http://10.0.0.7:3500
```

A target on another origin must allow the page's origin in its `--http-cors-domain`.
