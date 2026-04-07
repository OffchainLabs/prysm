package integration

import "fmt"

// Port allocation for the integration test cluster.
// Each node gets a unique set of ports based on its index.
const (
	basePort = 30000

	// Geth ports (per node, offset by index * 100)
	gethP2POffset     = 0
	gethHTTPOffset    = 1
	gethWSOffset      = 2
	gethAuthRPCOffset = 3

	// Beacon ports (per node, offset by index * 100)
	beaconP2PUDPOffset  = 10
	beaconP2PTCPOffset  = 11
	beaconRPCOffset     = 12
	beaconGRPCOffset    = 13
	beaconHTTPOffset    = 14
	beaconMonitorOffset = 15

	// Validator ports (per node, offset by index * 100)
	validatorRPCOffset     = 20
	validatorMonitorOffset = 21
)

func gethP2PPort(index int) int     { return basePort + index*100 + gethP2POffset }
func gethHTTPPort(index int) int    { return basePort + index*100 + gethHTTPOffset }
func gethAuthRPCPort(index int) int { return basePort + index*100 + gethAuthRPCOffset }

func beaconP2PUDPPort(index int) int  { return basePort + index*100 + beaconP2PUDPOffset }
func beaconP2PTCPPort(index int) int  { return basePort + index*100 + beaconP2PTCPOffset }
func beaconRPCPort(index int) int     { return basePort + index*100 + beaconRPCOffset }
func beaconGRPCPort(index int) int    { return basePort + index*100 + beaconGRPCOffset }
func beaconHTTPPort(index int) int    { return basePort + index*100 + beaconHTTPOffset }
func beaconMonitorPort(index int) int { return basePort + index*100 + beaconMonitorOffset }

func validatorRPCPort(index int) int     { return basePort + index*100 + validatorRPCOffset }
func validatorMonitorPort(index int) int { return basePort + index*100 + validatorMonitorOffset }

func gethAuthEndpoint(index int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", gethAuthRPCPort(index))
}

func beaconGRPCEndpoint(index int) string {
	return fmt.Sprintf("127.0.0.1:%d", beaconRPCPort(index))
}
