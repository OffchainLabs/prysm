// Package network contains useful functions for ip address formatting.
package network

import (
	"fmt"
	"net"
	"sort"
)

const defaultIP = "127.0.0.1"

// IPAddr gets the external ipv4 address and converts into a libp2p formatted value.
func IPAddr() net.IP {
	ip, err := ExternalIP()
	if err != nil {
		panic(err) // lint:nopanic -- Only panics if a network interface is not available. This is a requirement to run the application anyway.
	}
	return net.ParseIP(ip)
}

// ExternalIPv4 returns the first IPv4 available.
func ExternalIPv4() (string, error) {
	ips, err := ipAddrs()
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return defaultIP, nil
	}
	for _, ip := range ips {
		ip = ip.To4()
		if ip == nil {
			continue // not an ipv4 address
		}
		return ip.String(), nil
	}
	return defaultIP, nil
}

// ExternalIP returns the first IPv4/IPv6 available.
func ExternalIP() (string, error) {
	ips, err := ipAddrs()
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return defaultIP, nil
	}
	return ips[0].String(), nil
}

// ExternalPublicIP returns the first public (non-private) IP available,
// preferring IPv4 over IPv6. If no public IP is found, it falls back to the
// first private IP. Returns "127.0.0.1" if no addresses are available at all.
func ExternalPublicIP() (string, error) {
	ips, err := ipAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get IP addresses: %w", err)
	}

	if len(ips) == 0 {
		return defaultIP, nil
	}

	for _, ip := range ips {
		if !ip.IsPrivate() {
			return ip.String(), nil
		}
	}

	// No public IP found, fall back to first available (private) IP.
	return ips[0].String(), nil
}

// ipAddrs returns all the valid IPs available.
func ipAddrs() ([]net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var ipAddrs []net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			ipAddrs = append(ipAddrs, ip)
		}
	}
	return SortAddresses(ipAddrs), nil
}

// SortAddresses sorts a set of addresses in the order of
// ipv4 -> ipv6.
func SortAddresses(ipAddrs []net.IP) []net.IP {
	sort.Slice(ipAddrs, func(i, j int) bool {
		return ipAddrs[i].To4() != nil && ipAddrs[j].To4() == nil
	})
	return ipAddrs
}
