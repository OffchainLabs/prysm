package p2p

import (
	"net"
	"strconv"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "p2p")

func logIPAddr(id peer.ID, addrs ...ma.Multiaddr) {
	for _, addr := range addrs {
		if !(strings.Contains(addr.String(), "/ip4/") || strings.Contains(addr.String(), "/ip6/")) {
			continue
		}

		log.WithField(
			"multiAddr",
			addr.String()+"/p2p/"+id.String(),
		).Info("Node started p2p server")
	}
}

func logExternalIPAddr(id peer.ID, addr string, tcpPort, quicPort uint) {
	if addr != "" {
		multiAddrs, err := MultiAddressBuilder(net.ParseIP(addr), tcpPort, quicPort)
		if err != nil {
			log.WithError(err).Error("Could not create multiaddress")
			return
		}

		for _, multiAddr := range multiAddrs {
			log.WithField(
				"multiAddr",
				multiAddr.String()+"/p2p/"+id.String(),
			).Info("Node started external p2p server")
		}
	}
}

func logExternalDNSAddr(id peer.ID, addr string, port uint) {
	if addr != "" {
		p := strconv.FormatUint(uint64(port), 10)

		log.WithField(
			"multiAddr",
			"/dns4/"+addr+"/tcp/"+p+"/p2p/"+id.String(),
		).Info("Node started external p2p server")
	}
}
