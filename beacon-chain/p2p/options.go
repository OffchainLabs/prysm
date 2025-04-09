package p2p

import (
	"crypto/ecdsa"
	"fmt"
	"net"
	"time"

	"github.com/libp2p/go-libp2p"
	mplex "github.com/libp2p/go-libp2p-mplex"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	libp2ptcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	gomplex "github.com/libp2p/go-mplex"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v5/crypto/ecdsa"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

type internetProtocol string

const (
	udp  = "udp"
	tcp  = "tcp"
	quic = "quic"
)

// MultiAddressBuilder takes in an ip address string and port to produce a go multiaddr format.
func MultiAddressBuilder(ip net.IP, tcpPort, quicPort uint) ([]ma.Multiaddr, error) {
	ipType, err := extractIpType(ip)
	if err != nil {
		return nil, errors.Wrap(err, "unable to determine IP type")
	}

	// Example: /ip4/1.2.3.4./tcp/5678
	multiaddrStr := fmt.Sprintf("/%s/%s/tcp/%d", ipType, ip, tcpPort)
	multiAddrTCP, err := ma.NewMultiaddr(multiaddrStr)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot produce TCP multiaddr format from %s:%d", ip, tcpPort)
	}

	multiaddrs := []ma.Multiaddr{multiAddrTCP}

	if features.Get().EnableQUIC {
		// Example: /ip4/1.2.3.4/udp/5678/quic-v1
		multiAddrQUIC, err := ma.NewMultiaddr(fmt.Sprintf("/%s/%s/udp/%d/quic-v1", ipType, ip, quicPort))
		if err != nil {
			return nil, errors.Wrapf(err, "cannot produce QUIC multiaddr format from %s:%d", ip, tcpPort)
		}

		multiaddrs = append(multiaddrs, multiAddrQUIC)
	}

	return multiaddrs, nil
}

// buildOptions for the libp2p host.
func (s *Service) buildOptions(ip net.IP, priKey *ecdsa.PrivateKey) ([]libp2p.Option, error) {
	cfg := s.cfg
	multiaddrs, err := MultiAddressBuilder(ip, cfg.TCPPort, cfg.QUICPort)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot produce multiaddr format from %s:%d", ip, cfg.TCPPort)
	}
	if cfg.LocalIP != "" {
		localIP := net.ParseIP(cfg.LocalIP)
		if localIP == nil {
			return nil, errors.Wrapf(err, "invalid local ip provided: %s:%d", cfg.LocalIP, cfg.TCPPort)
		}

		multiaddrs, err = MultiAddressBuilder(localIP, cfg.TCPPort, cfg.QUICPort)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot produce multiaddr format from %s:%d", cfg.LocalIP, cfg.TCPPort)
		}
	}
	ifaceKey, err := ecdsaprysm.ConvertToInterfacePrivkey(priKey)
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert private key to interface private key. (Private key not displayed in logs for security reasons)")
	}
	id, err := peer.IDFromPublicKey(ifaceKey.GetPublic())
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get ID from public key: %s", ifaceKey.GetPublic().Type().String())
	}

	log.Infof("Running node with peer id of %s ", id.String())

	options := []libp2p.Option{
		privKeyOption(priKey),
		libp2p.ListenAddrs(multiaddrs...),
		libp2p.UserAgent(version.BuildData()),
		libp2p.ConnectionGater(s),
		libp2p.Transport(libp2ptcp.NewTCPTransport),
		libp2p.DefaultMuxers,
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Ping(false), // Disable Ping Service.
	}

	if features.Get().EnableQUIC {
		options = append(options, libp2p.Transport(libp2pquic.NewTransport))
	}

	if cfg.EnableUPnP {
		options = append(options, libp2p.NATPortMap()) // Allow to use UPnP
	}

	if cfg.RelayNodeAddr != "" {
		options = append(options, libp2p.AddrsFactory(withRelayAddrs(cfg.RelayNodeAddr)))
	} else {
		// Disable relay if it has not been set.
		options = append(options, libp2p.DisableRelay())
	}

	if cfg.HostAddress != "" {
		options = append(options, libp2p.AddrsFactory(func(addrs []ma.Multiaddr) []ma.Multiaddr {
			externalMultiaddrs, err := MultiAddressBuilder(net.ParseIP(cfg.HostAddress), cfg.TCPPort, cfg.QUICPort)
			if err != nil {
				log.WithError(err).Error("Unable to create external multiaddress")
			} else {
				addrs = append(addrs, externalMultiaddrs...)
			}
			return addrs
		}))
	}

	if cfg.HostDNS != "" {
		options = append(options, libp2p.AddrsFactory(func(addrs []ma.Multiaddr) []ma.Multiaddr {
			external, err := ma.NewMultiaddr(fmt.Sprintf("/dns4/%s/tcp/%d", cfg.HostDNS, cfg.TCPPort))
			if err != nil {
				log.WithError(err).Error("Unable to create external multiaddress")
			} else {
				addrs = append(addrs, external)
			}
			return addrs
		}))
	}

	if features.Get().DisableResourceManager {
		options = append(options, libp2p.ResourceManager(&network.NullResourceManager{}))
	}

	return options, nil
}

func extractIpType(ip net.IP) (string, error) {
	if ip.To4() != nil {
		return "ip4", nil
	}

	if ip.To16() != nil {
		return "ip6", nil
	}

	return "", errors.Errorf("provided IP address is neither IPv4 nor IPv6: %s", ip)
}

func multiAddressBuilderWithID(ip net.IP, protocol internetProtocol, port uint, id peer.ID) (ma.Multiaddr, error) {
	var multiaddrStr string

	if id == "" {
		return nil, errors.Errorf("empty peer id given: %s", id)
	}

	ipType, err := extractIpType(ip)
	if err != nil {
		return nil, errors.Wrap(err, "unable to determine IP type")
	}

	switch protocol {
	case udp, tcp:
		// Example with UDP: /ip4/1.2.3.4/udp/5678/p2p/16Uiu2HAkum7hhuMpWqFj3yNLcmQBGmThmqw2ohaCRThXQuKU9ohs
		// Example with TCP: /ip6/1.2.3.4/tcp/5678/p2p/16Uiu2HAkum7hhuMpWqFj3yNLcmQBGmThmqw2ohaCRThXQuKU9ohs
		multiaddrStr = fmt.Sprintf("/%s/%s/%s/%d/p2p/%s", ipType, ip, protocol, port, id)
	case quic:
		// Example: /ip4/1.2.3.4/udp/5678/quic-v1/p2p/16Uiu2HAkum7hhuMpWqFj3yNLcmQBGmThmqw2ohaCRThXQuKU9ohs
		multiaddrStr = fmt.Sprintf("/%s/%s/udp/%d/quic-v1/p2p/%s", ipType, ip, port, id)
	default:
		return nil, errors.Errorf("unsupported protocol: %s", protocol)
	}

	return ma.NewMultiaddr(multiaddrStr)
}

// Adds a private key to the libp2p option if the option was provided.
// If the private key file is missing or cannot be read, or if the
// private key contents cannot be marshaled, an exception is thrown.
func privKeyOption(privkey *ecdsa.PrivateKey) libp2p.Option {
	return func(cfg *libp2p.Config) error {
		ifaceKey, err := ecdsaprysm.ConvertToInterfacePrivkey(privkey)
		if err != nil {
			return err
		}
		log.Debug("ECDSA private key generated")
		return cfg.Apply(libp2p.Identity(ifaceKey))
	}
}

// Configures stream timeouts on mplex.
func configureMplex() {
	gomplex.ResetStreamTimeout = 5 * time.Second
}
