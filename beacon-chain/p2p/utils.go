package p2p

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	gCrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/wrapper"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v5/crypto/ecdsa"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/metadata"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

const keyPath = "network-keys"
const metaDataPath = "metaData"

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

	// If the StaticPeerID flag is not set and if peerDAS is not enabled, return the private key.
	if !(cfg.StaticPeerID || params.PeerDASEnabled()) {
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

// Retrieves node p2p metadata from a set of configuration values
// from the p2p service.
// TODO: Figure out how to do a v1/v2 check.
func metaDataFromConfig(cfg *Config) (metadata.Metadata, error) {
	defaultKeyPath := path.Join(cfg.DataDir, metaDataPath)
	metaDataPath := cfg.MetaDataDir

	_, err := os.Stat(defaultKeyPath)
	defaultMetadataExist := !os.IsNotExist(err)
	if err != nil && defaultMetadataExist {
		return nil, err
	}
	if metaDataPath == "" && !defaultMetadataExist {
		metaData := &pb.MetaDataV0{
			SeqNumber: 0,
			Attnets:   bitfield.NewBitvector64(),
		}
		dst, err := proto.Marshal(metaData)
		if err != nil {
			return nil, err
		}
		if err := file.WriteFile(defaultKeyPath, dst); err != nil {
			return nil, err
		}
		return wrapper.WrappedMetadataV0(metaData), nil
	}
	if defaultMetadataExist && metaDataPath == "" {
		metaDataPath = defaultKeyPath
	}
	src, err := os.ReadFile(metaDataPath) // #nosec G304
	if err != nil {
		log.WithError(err).Error("Error reading metadata from file")
		return nil, err
	}
	metaData := &pb.MetaDataV0{}
	if err := proto.Unmarshal(src, metaData); err != nil {
		return nil, err
	}
	return wrapper.WrappedMetadataV0(metaData), nil
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

	newPubkey := &ecdsa.PublicKey{Curve: gCrypto.S256(), X: pubKeyObjSecp256k1.X(), Y: pubKeyObjSecp256k1.Y()}
	return enode.PubkeyToIDV4(newPubkey), nil
}
