package sync

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	db "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	types "github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	leakybucket "github.com/prysmaticlabs/prysm/v5/container/leaky-bucket"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

type blobsTestCase struct {
	name                string
	nblocks             int          // how many blocks to loop through in setting up test fixtures & requests
	missing             map[int]bool // skip this blob index, so that we can test different custody scenarios
	expired             map[int]bool // mark block expired to test scenarios where requests are outside retention window
	chain               *mock.ChainService
	clock               *startup.Clock // allow tests to control retention window via current slot and finalized checkpoint
	total               *int           // allow a test to specify the total number of responses received
	err                 error
	serverHandle        testHandler
	defineExpected      expectedDefiner
	requestFromSidecars requestFromSidecars
	topic               protocol.ID
	oldestSlot          oldestSlotCallback
	streamReader        expectedRequirer
}

type testHandler func(s *Service) rpcHandler
type expectedDefiner func(t *testing.T, scs []blocks.ROBlob, req interface{}) []*expectedBlobChunk
type requestFromSidecars func([]blocks.ROBlob) interface{}
type oldestSlotCallback func(t *testing.T) types.Slot
type expectedRequirer func(*testing.T, *Service, []*expectedBlobChunk) func(network.Stream)

type expectedBlobChunk struct {
	code    uint8
	sidecar *blocks.ROBlob
	message string
}

func (r *expectedBlobChunk) requireExpected(t *testing.T, s *Service, stream network.Stream) {
	encoding := s.cfg.p2p.Encoding()

	code, _, err := ReadStatusCode(stream, encoding)
	require.NoError(t, err)
	require.Equal(t, r.code, code, "unexpected response code")
	if code != responseCodeSuccess {
		return
	}

	c, err := readContextFromStream(stream)
	require.NoError(t, err)

	valRoot := s.cfg.chain.GenesisValidatorsRoot()
	ctxBytes, err := forks.ForkDigestFromEpoch(slots.ToEpoch(r.sidecar.Slot()), valRoot[:])
	require.NoError(t, err)
	require.Equal(t, ctxBytes, bytesutil.ToBytes4(c))

	sc := &ethpb.BlobSidecar{}
	require.NoError(t, encoding.DecodeWithMaxLength(stream, sc))
	rob, err := blocks.NewROBlob(sc)
	require.NoError(t, err)
	require.Equal(t, rob.BlockRoot(), r.sidecar.BlockRoot())
	require.Equal(t, rob.Index, r.sidecar.Index)
}

func (c *blobsTestCase) setup(t *testing.T) (*Service, []blocks.ROBlob, func()) {
	cfg := params.BeaconConfig()
	copiedCfg := cfg.Copy()
	repositionFutureEpochs(copiedCfg)
	copiedCfg.InitializeForkSchedule()
	params.OverrideBeaconConfig(copiedCfg)
	cleanup := func() {
		params.OverrideBeaconConfig(cfg)
	}
	maxBlobs := fieldparams.MaxBlobsPerBlock
	chain, clock := defaultMockChain(t)
	if c.chain == nil {
		c.chain = chain
	}
	if c.clock == nil {
		c.clock = clock
	}
	d := db.SetupDB(t)

	sidecars := make([]blocks.ROBlob, 0)
	oldest := c.oldestSlot(t)
	var parentRoot [32]byte
	for i := 0; i < c.nblocks; i++ {
		// check if there is a slot override for this index
		// ie to create a block outside the minimum_request_epoch
		var bs types.Slot
		if c.expired[i] {
			// the lowest possible bound of the retention period is the deneb epoch, so make sure
			// the slot of an expired block is at least one slot less than the deneb epoch.
			bs = oldest - 1 - types.Slot(i)
		} else {
			bs = oldest + types.Slot(i)
		}
		block, bsc := util.GenerateTestDenebBlockWithSidecar(t, parentRoot, bs, maxBlobs)
		sidecars = append(sidecars, bsc...)
		pb, err := block.Proto()
		require.NoError(t, err)
		util.SaveBlock(t, context.Background(), d, pb)
		parentRoot = block.Root()
	}

	client := p2ptest.NewTestP2P(t)
	s := &Service{
		cfg:         &config{p2p: client, chain: c.chain, clock: clock, beaconDB: d, blobStorage: filesystem.NewEphemeralBlobStorage(t)},
		rateLimiter: newRateLimiter(client),
	}

	byRootRate := params.BeaconConfig().MaxRequestBlobSidecars * fieldparams.MaxBlobsPerBlock
	byRangeRate := params.BeaconConfig().MaxRequestBlobSidecars * fieldparams.MaxBlobsPerBlock
	s.setRateCollector(p2p.RPCBlobSidecarsByRootTopicV1, leakybucket.NewCollector(0.000001, int64(byRootRate), time.Second, false))
	s.setRateCollector(p2p.RPCBlobSidecarsByRangeTopicV1, leakybucket.NewCollector(0.000001, int64(byRangeRate), time.Second, false))

	return s, sidecars, cleanup
}

func defaultExpectedRequirer(t *testing.T, s *Service, expect []*expectedBlobChunk) func(network.Stream) {
	return func(stream network.Stream) {
		for _, ex := range expect {
			ex.requireExpected(t, s, stream)
		}
	}
}

func (c *blobsTestCase) run(t *testing.T) {
	s, sidecars, cleanup := c.setup(t)
	defer cleanup()
	req := c.requestFromSidecars(sidecars)
	expect := c.defineExpected(t, sidecars, req)
	m := map[types.Slot][]blocks.ROBlob{}
	for i := range expect {
		sc := expect[i]
		// If define expected omits a sidecar from an expected result, we don't need to save it.
		// This can happen in particular when there are no expected results, because the nth part of the
		// response is an error (or none at all when the whole request is invalid).
		if sc.sidecar != nil {
			m[sc.sidecar.Slot()] = append(m[sc.sidecar.Slot()], *sc.sidecar)
		}
	}
	for _, blobSidecars := range m {
		v, err := verification.BlobSidecarSliceNoop(blobSidecars)
		require.NoError(t, err)
		for i := range v {
			require.NoError(t, s.cfg.blobStorage.Save(v[i]))
		}
	}
	if c.total != nil {
		require.Equal(t, *c.total, len(expect))
	}
	rht := &rpcHandlerTest{
		t:       t,
		topic:   c.topic,
		timeout: time.Second * 10,
		err:     c.err,
		s:       s,
	}
	rht.testHandler(c.streamReader(t, s, expect), c.serverHandle(s), req)
}

// we use max uints for future forks, but this causes overflows when computing slots
// so it is helpful in tests to temporarily reposition the epochs to give room for some math.
func repositionFutureEpochs(cfg *params.BeaconChainConfig) {
	if cfg.CapellaForkEpoch == math.MaxUint64 {
		cfg.CapellaForkEpoch = cfg.BellatrixForkEpoch + 100
	}
	if cfg.DenebForkEpoch == math.MaxUint64 {
		cfg.DenebForkEpoch = cfg.CapellaForkEpoch + 100
	}
}

func defaultMockChain(t *testing.T) (*mock.ChainService, *startup.Clock) {
	de := params.BeaconConfig().DenebForkEpoch
	df, err := forks.Fork(de)
	require.NoError(t, err)
	denebBuffer := params.BeaconConfig().MinEpochsForBlobsSidecarsRequest + 1000
	ce := de + denebBuffer
	fe := ce - 2
	cs, err := slots.EpochStart(ce)
	require.NoError(t, err)
	now := time.Now()
	genOffset := types.Slot(params.BeaconConfig().SecondsPerSlot) * cs
	genesis := now.Add(-1 * time.Second * time.Duration(int64(genOffset)))
	clock := startup.NewClock(genesis, [32]byte{})
	chain := &mock.ChainService{
		FinalizedCheckPoint: &ethpb.Checkpoint{Epoch: fe},
		Fork:                df,
	}

	return chain, clock
}

func TestTestcaseSetup_BlocksAndBlobs(t *testing.T) {
	ctx := context.Background()
	nblocks := 10
	c := &blobsTestCase{nblocks: nblocks}
	c.oldestSlot = c.defaultOldestSlotByRoot
	s, sidecars, cleanup := c.setup(t)
	req := blobRootRequestFromSidecars(sidecars)
	expect := c.filterExpectedByRoot(t, sidecars, req)
	defer cleanup()
	maxed := nblocks * fieldparams.MaxBlobsPerBlock
	require.Equal(t, maxed, len(sidecars))
	require.Equal(t, maxed, len(expect))
	for _, sc := range sidecars {
		blk, err := s.cfg.beaconDB.Block(ctx, sc.BlockRoot())
		require.NoError(t, err)
		var found *int
		comms, err := blk.Block().Body().BlobKzgCommitments()
		require.NoError(t, err)
		for i, cm := range comms {
			if bytesutil.ToBytes48(sc.KzgCommitment) == bytesutil.ToBytes48(cm) {
				found = &i
			}
		}
		require.Equal(t, true, found != nil)
	}
}
