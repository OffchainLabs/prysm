package p2p

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/kv"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/wrapper"
	ecdsaprysm "github.com/OffchainLabs/prysm/v6/crypto/ecdsa"
	"github.com/OffchainLabs/prysm/v6/io/file"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/metadata"
	"github.com/btcsuite/btcd/btcec/v2"
	gCrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/sirupsen/logrus"
)

const keyPath = "network-keys"

const dialTimeout = 1 * time.Second

// SerializeENR takes the enr record in its key-value form and serializes it.
func SerializeENR(record *enr.Record) (string, error) {
	if record == nil {
		return "", errors.New("could not serialize nil record")
	}
	buf := bytes.NewBuffer([]byte{})
	if err := record.EncodeRLP(buf); err != nil {
		return "", errors.Wrap(err, "could not encode ENR record to bytes")
	}
	enrString := base64.RawURLEncoding.EncodeToString(buf.Bytes())
	return enrString, nil
}

// Determines a private key for p2p networking from the p2p service's
// configuration struct. If no key is found, it generates a new one.
func privKey(cfg *Config) (*ecdsa.PrivateKey, error) {
	defaultKeyPath := path.Join(cfg.DataDir, keyPath)
	privateKeyPath := cfg.PrivateKey

	// PrivateKey cli flag takes highest precedence.
	if privateKeyPath != "" {
		return privKeyFromFile(cfg.PrivateKey)
	}

	// Default keys have the next highest precedence, if they exist.
	_, err := os.Stat(defaultKeyPath)
	defaultKeysExist := !os.IsNotExist(err)
	if err != nil && defaultKeysExist {
		return nil, err
	}

	if defaultKeysExist {
		log.WithField("filePath", defaultKeyPath).Info("Reading static P2P private key from a file. To generate a new random private key at every start, please remove this file.")
		return privKeyFromFile(defaultKeyPath)
	}

	// There are no keys on the filesystem, so we need to generate one.
	priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	// If the StaticPeerID flag is not set or the Fulu epoch is not set, return the private key.
	// Starting at Fulu, we don't want to generate a new key every time, to avoid custody columns changes.
	if !(cfg.StaticPeerID || params.FuluEnabled()) {
		return ecdsaprysm.ConvertFromInterfacePrivKey(priv)
	}

	// Save the generated key as the default key, so that it will be used by
	// default on the next node start.
	rawbytes, err := priv.Raw()
	if err != nil {
		return nil, err
	}

	dst := make([]byte, hex.EncodedLen(len(rawbytes)))
	hex.Encode(dst, rawbytes)
	if err := file.WriteFile(defaultKeyPath, dst); err != nil {
		return nil, err
	}

	log.WithField("path", defaultKeyPath).Info("Wrote network key to file")
	// Read the key from the defaultKeyPath file just written
	// for the strongest guarantee that the next start will be the same as this one.
	return privKeyFromFile(defaultKeyPath)
}

// Retrieves a p2p networking private key from a file path.
func privKeyFromFile(path string) (*ecdsa.PrivateKey, error) {
	src, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		log.WithError(err).Error("Error reading private key from file")
		return nil, err
	}
	dst := make([]byte, hex.DecodedLen(len(src)))
	_, err = hex.Decode(dst, src)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode hex string")
	}
	unmarshalledKey, err := crypto.UnmarshalSecp256k1PrivateKey(dst)
	if err != nil {
		return nil, err
	}
	return ecdsaprysm.ConvertFromInterfacePrivKey(unmarshalledKey)
}

// Retrieves metadata sequence number from DB and returns a Metadata(V0) object
func metaDataFromDB(ctx context.Context, db db.ReadOnlyDatabaseWithSeqNum) (metadata.Metadata, error) {
	seqNum, err := db.MetadataSeqNum(ctx)
	// We can proceed if error is `kv.ErrNotFoundMetadataSeqNum` by using default value of 0 for sequence number.
	if err != nil && !errors.Is(err, kv.ErrNotFoundMetadataSeqNum) {
		return nil, err
	}

	// NOTE: Load V0 metadata because:
	// - As the p2p service accesses metadata as an interface, and all versions implement the interface,
	//   there is no error in calling the fields of higher versions. It just returns the default value.
	// - This approach allows us to avoid unnecessary code changes when the metadata version bumps.
	// - `RefreshPersistentSubnets` runs twice every slot and it manages updating and saving metadata.
	metadata := wrapper.WrappedMetadataV0(&pb.MetaDataV0{
		SeqNumber: seqNum,
		Attnets:   bitfield.NewBitvector64(),
	})
	return metadata, nil
}

// Attempt to dial an address to verify its connectivity
func verifyConnectivity(addr string, port uint, protocol string) {
	if addr != "" {
		a := net.JoinHostPort(addr, fmt.Sprintf("%d", port))
		fields := logrus.Fields{
			"protocol": protocol,
			"address":  a,
		}
		conn, err := net.DialTimeout(protocol, a, dialTimeout)
		if err != nil {
			log.WithError(err).WithFields(fields).Warn("IP address is not accessible")
			return
		}
		if err := conn.Close(); err != nil {
			log.WithError(err).Debug("Could not close connection")
		}
	}
}

// ConvertPeerIDToNodeID converts a peer ID (libp2p) to a node ID (devp2p).
func ConvertPeerIDToNodeID(pid peer.ID) (enode.ID, error) {
	// Retrieve the public key object of the peer under "crypto" form.
	pubkeyObjCrypto, err := pid.ExtractPublicKey()
	if err != nil {
		return [32]byte{}, errors.Wrapf(err, "extract public key from peer ID `%s`", pid)
	}

	// Extract the bytes representation of the public key.
	compressedPubKeyBytes, err := pubkeyObjCrypto.Raw()
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "public key raw")
	}

	// Retrieve the public key object of the peer under "SECP256K1" form.
	pubKeyObjSecp256k1, err := btcec.ParsePubKey(compressedPubKeyBytes)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "parse public key")
	}

	newPubkey := &ecdsa.PublicKey{
		Curve: gCrypto.S256(),
		X:     pubKeyObjSecp256k1.X(),
		Y:     pubKeyObjSecp256k1.Y(),
	}

	return enode.PubkeyToIDV4(newPubkey), nil
}
