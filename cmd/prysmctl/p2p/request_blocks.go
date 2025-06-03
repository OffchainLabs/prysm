package p2p

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/config/params"
	consensus_types "github.com/OffchainLabs/prysm/v6/consensus-types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/types/known/emptypb"
)

var requestBlocksFlags = struct {
	Network        string
	Fork           string
	Peers          string
	ClientPortTCP  uint
	ClientPortQUIC uint
	APIEndpoints   string
	StartSlot      uint64
	Count          uint64
	Step           uint64
}{}

var requestBlocksCmd = &cli.Command{
	Name:  "beacon-blocks-by-range",
	Usage: "Request a range of blocks from a beacon node via a p2p connection",
	Action: func(cliCtx *cli.Context) error {
		if err := cliActionRequestBlocks(cliCtx); err != nil {
			log.WithError(err).Fatal("Could not request blocks by range")
		}
		return nil
	},
	Flags: []cli.Flag{
		cmd.ChainConfigFileFlag,
		&cli.StringFlag{
			Name:        "network",
			Usage:       "network to run on (mainnet, sepolia, holesky, hoodi)",
			Destination: &requestBlocksFlags.Network,
			Value:       "mainnet",
		},
		&cli.StringFlag{
			Name:        "fork",
			Usage:       "fork version to use (phase0, altair, bellatrix, capella, deneb). If not specified, will auto-detect from chain state",
			Destination: &requestBlocksFlags.Fork,
			Value:       "",
		},
		&cli.StringFlag{
			Name:        "peer-multiaddrs",
			Usage:       "comma-separated, peer multiaddr(s) to connect to for p2p requests",
			Destination: &requestBlocksFlags.Peers,
			Value:       "",
		},
		&cli.UintFlag{
			Name:        "client-port-tcp",
			Aliases:     []string{"client-port"},
			Usage:       "TCP port to use for the client as a libp2p host",
			Destination: &requestBlocksFlags.ClientPortTCP,
			Value:       13001,
		},
		&cli.UintFlag{
			Name:        "client-port-quic",
			Usage:       "QUIC port to use for the client as a libp2p host",
			Destination: &requestBlocksFlags.ClientPortQUIC,
			Value:       13001,
		},
		&cli.StringFlag{
			Name:        "prysm-api-endpoints",
			Usage:       "comma-separated, gRPC API endpoint(s) for Prysm beacon node(s)",
			Destination: &requestBlocksFlags.APIEndpoints,
			Value:       "localhost:4000",
		},
		&cli.Uint64Flag{
			Name:        "start-slot",
			Usage:       "start slot for blocks by range request. If unset, will use start_slot(current_epoch-1)",
			Destination: &requestBlocksFlags.StartSlot,
			Value:       0,
		},
		&cli.Uint64Flag{
			Name:        "count",
			Usage:       "number of blocks to request, (default 32)",
			Destination: &requestBlocksFlags.Count,
			Value:       32,
		},
		&cli.Uint64Flag{
			Name:        "step",
			Usage:       "number of steps of blocks in the range request, (default 1)",
			Destination: &requestBlocksFlags.Step,
			Value:       1,
		},
	},
}

func cliActionRequestBlocks(cliCtx *cli.Context) error {
	// Set network configuration first
	switch requestBlocksFlags.Network {
	case params.SepoliaName:
		if err := params.SetActive(params.SepoliaConfig()); err != nil {
			log.Fatal(err)
		}
	case params.HoleskyName:
		if err := params.SetActive(params.HoleskyConfig()); err != nil {
			log.Fatal(err)
		}
	case params.HoodiName:
		if err := params.SetActive(params.HoodiConfig()); err != nil {
			log.Fatal(err)
		}
	case params.MainnetName:
		// Do nothing - mainnet is default
	default:
		log.Fatalf("Unknown network provided: %s", requestBlocksFlags.Network)
	}

	// Load custom chain config if provided
	if cliCtx.IsSet(cmd.ChainConfigFileFlag.Name) {
		chainConfigFileName := cliCtx.String(cmd.ChainConfigFileFlag.Name)
		if err := params.LoadChainConfigFile(chainConfigFileName, nil); err != nil {
			return err
		}
	}

	// Parse and validate fork flag if provided
	var forkOverride *pb.Fork
	if requestBlocksFlags.Fork != "" {
		var err error
		forkOverride, err = parseForkFlag(requestBlocksFlags.Fork)
		if err != nil {
			return errors.Wrap(err, "invalid fork flag")
		}
		log.WithField("fork", requestBlocksFlags.Fork).Info("Using fork override from --fork flag")
	}

	p2ptypes.InitializeDataMaps()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	allAPIEndpoints := make([]string, 0)
	if requestBlocksFlags.APIEndpoints != "" {
		allAPIEndpoints = strings.Split(requestBlocksFlags.APIEndpoints, ",")
	}
	var err error
	c, err := newClient(allAPIEndpoints, requestBlocksFlags.ClientPortTCP, requestBlocksFlags.ClientPortQUIC)
	if err != nil {
		return err
	}
	defer c.Close()

	allPeers := make([]string, 0)
	if requestBlocksFlags.Peers != "" {
		allPeers = strings.Split(requestBlocksFlags.Peers, ",")
	}
	if len(allPeers) == 0 {
		allPeers, err = c.retrievePeerAddressesViaRPC(ctx, allAPIEndpoints)
		if err != nil {
			return err
		}
	}
	if len(allPeers) == 0 {
		return errors.New("no peers found")
	}
	log.WithField("peers", allPeers).Info("List of peers")

	// Initialize chain service with optional fork override
	chain, err := c.initializeMockChainServiceWithFork(ctx, forkOverride)
	if err != nil {
		return err
	}
	c.registerHandshakeHandlers()

	c.registerRPCHandler(p2p.RPCBlocksByRangeTopicV1, func(
		ctx context.Context, i interface{}, stream libp2pcore.Stream,
	) error {
		return nil
	})
	c.registerRPCHandler(p2p.RPCBlocksByRangeTopicV2, func(
		ctx context.Context, i interface{}, stream libp2pcore.Stream,
	) error {
		return nil
	})

	if err := c.connectToPeers(ctx, allPeers...); err != nil {
		return err
	}

	startSlot := primitives.Slot(requestBlocksFlags.StartSlot)
	var headSlot *primitives.Slot
	if startSlot == 0 {
		headResp, err := c.beaconClient.GetChainHead(ctx, &emptypb.Empty{})
		if err != nil {
			return err
		}
		startSlot, err = slots.EpochStart(headResp.HeadEpoch.Sub(1))
		if err != nil {
			return err
		}
		headSlot = &headResp.HeadSlot
	}

	// Submit requests.
	for _, pr := range c.host.Peerstore().Peers() {
		if pr.String() == c.host.ID().String() {
			continue
		}
		req := &pb.BeaconBlocksByRangeRequest{
			StartSlot: startSlot,
			Count:     requestBlocksFlags.Count,
			Step:      requestBlocksFlags.Step,
		}
		fields := logrus.Fields{
			"startSlot": startSlot,
			"count":     requestBlocksFlags.Count,
			"step":      requestBlocksFlags.Step,
			"peer":      pr.String(),
			"fork":      chain.currentFork,
		}
		if headSlot != nil {
			fields["headSlot"] = *headSlot
		}
		log.WithFields(fields).Info("Sending blocks by range p2p request to peer")
		start := time.Now()
		blocks, err := sync.SendBeaconBlocksByRangeRequest(
			ctx,
			chain,
			c,
			pr,
			req,
			nil, /* no extra block processing */
		)
		if err != nil {
			return err
		}
		end := time.Since(start)
		totalExecutionBlocks := 0
		for _, blk := range blocks {
			exec, err := blk.Block().Body().Execution()
			switch {
			case errors.Is(err, consensus_types.ErrUnsupportedField):
				continue
			case err != nil:
				log.WithError(err).Error("Could not read execution data from block body")
				continue
			default:
			}
			_, err = exec.Transactions()
			switch {
			case errors.Is(err, consensus_types.ErrUnsupportedField):
				continue
			case err != nil:
				log.WithError(err).Error("Could not read transactions block execution payload")
				continue
			default:
			}
			totalExecutionBlocks++
		}
		log.WithFields(logrus.Fields{
			"numBlocks":                           len(blocks),
			"peer":                                pr.String(),
			"timeFromSendingToProcessingResponse": end,
			"totalBlocksWithExecutionPayloads":    totalExecutionBlocks,
		}).Info("Received blocks from peer")
	}
	return nil
}

// parseForkFlag parses the fork flag and returns the corresponding Fork struct
func parseForkFlag(forkName string) (*pb.Fork, error) {
	switch strings.ToLower(forkName) {
	case "phase0", "phase_0":
		return &pb.Fork{
			PreviousVersion: bytesutil.PadTo([]byte{0, 0, 0, 0}, 4),
			CurrentVersion:  bytesutil.PadTo([]byte{0, 0, 0, 0}, 4),
			Epoch:           0,
		}, nil
	case "altair":
		return &pb.Fork{
			PreviousVersion: bytesutil.PadTo([]byte{0, 0, 0, 0}, 4),
			CurrentVersion:  bytesutil.PadTo([]byte{1, 0, 0, 0}, 4),
			Epoch:           74240, // Mainnet Altair epoch
		}, nil
	case "bellatrix", "merge":
		return &pb.Fork{
			PreviousVersion: bytesutil.PadTo([]byte{1, 0, 0, 0}, 4),
			CurrentVersion:  bytesutil.PadTo([]byte{2, 0, 0, 0}, 4),
			Epoch:           144896, // Mainnet Bellatrix epoch
		}, nil
	case "capella":
		return &pb.Fork{
			PreviousVersion: bytesutil.PadTo([]byte{2, 0, 0, 0}, 4),
			CurrentVersion:  bytesutil.PadTo([]byte{3, 0, 0, 0}, 4),
			Epoch:           194048, // Mainnet Capella epoch
		}, nil
	case "deneb":
		return &pb.Fork{
			PreviousVersion: bytesutil.PadTo([]byte{3, 0, 0, 0}, 4),
			CurrentVersion:  bytesutil.PadTo([]byte{4, 0, 0, 0}, 4),
			Epoch:           269568, // Mainnet Deneb epoch
		}, nil
	default:
		return nil, fmt.Errorf("unknown fork: %s. Supported forks: phase0, altair, bellatrix, capella, deneb", forkName)
	}
}
