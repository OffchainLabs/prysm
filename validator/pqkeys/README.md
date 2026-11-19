# Post-quantum keys

This package derives the post-quantum keys of a leanXMSS validator from its BLS
secret key.

## Why derive them from the BLS secret key

The genesis state of a PQ devnet holds, for each validator, an XMSS public key
and a VRF root. They are made by the ethPandaOps genesis tool, which derives them
from the BLS secret key of the validator
([`validators/mnemonic.go`](https://github.com/ethpandaops/eth-beacon-genesis/blob/ff42fce0ba5cbb351507af97d3583053df4cd165/validators/mnemonic.go#L154-L183)
and
[`validators/vrf.go`](https://github.com/ethpandaops/eth-beacon-genesis/blob/ff42fce0ba5cbb351507af97d3583053df4cd165/validators/vrf.go#L17-L19)).

This package derives the keys exactly as the genesis tool does. A validator
client holding the usual BLS keystore then gets back the XMSS key the genesis
state registered for it.

## Derivation

```text
                     BLS secret key `sk` (32 bytes, from the BLS keystore)
              ┌──────────────────────────┼───────────────────────────────┐
              │                          │                               │
        sk.PublicKey()       sha256("leanxmss-seed" || sk)    sha256("vrf-seed" || sk)
              │                          │                               │
              │            leanxmss.NewKeyFromSeed(seed)             VRF seed
              │                          │                               │
       BLS public key             XMSS secret key              VRF tree (package vrftree)
                                         │                               │
                                  XMSS public key                    VRF root
```

- The XMSS seed is `sha256("leanxmss-seed" || sk)`, where `sk` is the 32-byte
  serialized BLS secret key. The XMSS secret key is
  `leanxmss.NewKeyFromSeed(seed)`, from `github.com/nalepae/go-leanxmss`.
- The VRF seed is `sha256("vrf-seed" || sk)`. Package `vrftree` builds the VRF
  tree from it.

Nothing on chain links the two public keys. The state stores the XMSS public
key and the VRF root only. The link is that both come from the same BLS secret
key: the validator client derives the XMSS public key of each of its BLS keys,
and finds its validators in the state by it.

| Step           | Genesis tool                                     | This package                      |
| -------------- | ------------------------------------------------ | --------------------------------- |
| BLS secret key | `m/12381/3600/<index>/0/0` of the mnemonic       | the BLS keystore                  |
| XMSS seed      | `sha256("leanxmss-seed" \|\| sk)`                | `sha256("leanxmss-seed" \|\| sk)` |
| XMSS key       | `leanxmss.NewKeyFromSeed(seed)`                  | `leanxmss.NewKeyFromSeed(seed)`   |
| Library        | `go-leanxmss v0.0.0-20261116143821-bafbae50fe8b` | same version                      |

The genesis tool builds the XMSS secret key only to take its public key. It
never stores or outputs an XMSS secret key.

## What the spec says

The PQ spec
([`specs/_features/pq/validator.md`](https://github.com/ethereum/consensus-specs/blob/2f43bc0f989f47af81df390a90bc1f78e2436323/specs/_features/pq/validator.md))
says that the XMSS private key is "a 32-byte seed from which its leanXMSS key is
derived", and that it must be securely generated. It does not say where the
seed comes from, and defines no keystore format for it. It also says that "the
VRF seed is independent of the validator's XMSS private key".

Deriving both seeds from the BLS secret key is a devnet convention, not part of
the spec. The two seeds are domain-separated, but anyone holding the BLS secret
key also holds the XMSS and VRF secrets.

## Genesis keys from a precomputed file

The genesis tool does not always derive the keys. For each validator, it takes
the XMSS public key and the VRF root from the first source that has them
([`validators/xmsscache.go`](https://github.com/ethpandaops/eth-beacon-genesis/blob/ff42fce0ba5cbb351507af97d3583053df4cd165/validators/xmsscache.go#L39-L75)):

1. a local cache directory,
2. `xmss_pubkey_url`, which serves precomputed records of 64 bytes: the XMSS
   public key, then the VRF root,
3. the derivation above, for the keys still missing.

The Kurtosis setup of Lighthouse sets `xmss_pubkey_url`. On such a devnet, the
genesis keys come from the precomputed file. A validator client finds its
validators only if this file was made with the derivation above.

## Tests

`Derive` matches the test vector of Lighthouse's
`pq_ethpandaops_keys_and_vrf_tree_boundaries`: the BLS secret key
`0x2892…8834` gives the XMSS public key `0x155a…d88b`.
