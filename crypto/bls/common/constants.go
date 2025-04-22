package common

import fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"

// ZeroSecretKey represents a zero secret key.
var ZeroSecretKey = [32]byte{}

// InfinitePublicKey represents an infinite public key (G1 Point at Infinity).
var InfinitePublicKey = [fieldparams.BLSPubkeyLength]byte{0xC0}

// InfiniteSignature represents an infinite signature (G2 Point at Infinity).
var InfiniteSignature = [96]byte{0xC0}
