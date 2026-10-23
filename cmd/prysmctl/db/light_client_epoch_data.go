package db

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	lightClient "github.com/OffchainLabs/prysm/v7/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v7/cmd"
	"github.com/OffchainLabs/prysm/v7/config/params"
	light_client "github.com/OffchainLabs/prysm/v7/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/era"
	"github.com/OffchainLabs/prysm/v7/io/file"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

var lightClientEpochDataFlags = struct {
	Network string
	EraURL  string
	EraDir  string
	FromEra uint64
	ToEra   uint64
	Workers int
}{}

var lightClientEpochDataCmd = &cli.Command{
	Name:  "light-client-epoch-data",
	Usage: "build light client epoch data from era files and save it to a stopped beacon node's database",
	Description: "Replays era files and saves the LightClientEpochData of each epoch. Eras whose epoch data " +
		"is already in the database are skipped, so the command can be stopped and run again. " +
		"Era k needs the era files k-1, k and k+1.",
	Action: func(cliCtx *cli.Context) error {
		if err := lightClientEpochDataAction(cliCtx); err != nil {
			log.WithError(err).Fatal("Could not build light client epoch data")
		}
		return nil
	},
	Flags: []cli.Flag{
		cmd.DataDirFlag,
		&cli.StringFlag{
			Name:        "network",
			Usage:       "network of the era files (mainnet, hoodi, sepolia)",
			Destination: &lightClientEpochDataFlags.Network,
			Value:       params.MainnetName,
		},
		&cli.StringFlag{
			Name:        "era-url",
			Usage:       "directory of era files to download from (default https://<network>.era.nimbus.team/)",
			Destination: &lightClientEpochDataFlags.EraURL,
		},
		&cli.StringFlag{
			Name:        "era-dir",
			Usage:       "directory for era files; files already there are used and kept (default <datadir>/era)",
			Destination: &lightClientEpochDataFlags.EraDir,
		},
		&cli.Uint64Flag{
			Name:        "from-era",
			Usage:       "first era to replay (default the first era after the Altair fork epoch)",
			Destination: &lightClientEpochDataFlags.FromEra,
		},
		&cli.Uint64Flag{
			Name:        "to-era",
			Usage:       "last era to replay (default the latest era the server has the next file for)",
			Destination: &lightClientEpochDataFlags.ToEra,
		},
		&cli.IntFlag{
			Name:        "workers",
			Usage:       "eras replayed at the same time; each needs about 4 GB of memory on mainnet",
			Destination: &lightClientEpochDataFlags.Workers,
			Value:       1,
		},
	},
}

func lightClientEpochDataAction(cliCtx *cli.Context) error {
	f := lightClientEpochDataFlags
	if f.Workers < 1 {
		return errors.New("--workers must be at least 1")
	}
	cfg, err := params.ByName(f.Network)
	if err != nil {
		return err
	}
	if err := params.SetActive(cfg); err != nil {
		return err
	}
	dataDir := cliCtx.String(cmd.DataDirFlag.Name)
	eraURL := f.EraURL
	if eraURL == "" {
		eraURL = fmt.Sprintf("https://%s.era.nimbus.team/", cfg.ConfigName)
	}
	eraDir := f.EraDir
	if eraDir == "" {
		eraDir = filepath.Join(dataDir, "era")
	}

	ctx, stop := signal.NotifyContext(cliCtx.Context, os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := &http.Client{Timeout: time.Hour}
	files, err := listEraFiles(ctx, client, eraURL, cfg.ConfigName)
	if err != nil {
		return err
	}
	from, to := f.FromEra, f.ToEra
	if from == 0 {
		from = firstEra()
	}
	if latest := slices.Max(append(slices.Collect(maps.Keys(files)), 0)); to == 0 && latest > 0 {
		to = latest - 1
	}
	if from == 0 || from > to {
		return fmt.Errorf("no era to replay in [%d, %d]", from, to)
	}
	for n := from - 1; n <= to+1; n++ {
		if files[n] == "" {
			return fmt.Errorf("%s has no era file %d", eraURL, n)
		}
	}

	db, err := kv.NewKVStore(ctx, filepath.Join(dataDir, kv.BeaconNodeDbDirName))
	if err != nil {
		return errors.Wrap(err, "could not open the database; stop the beacon node first")
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.WithError(err).Error("Could not close the database")
		}
	}()

	todo, err := erasToReplay(ctx, db, from, to)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		"fromEra": from, "toEra": to, "todo": len(todo), "workers": f.Workers,
	}).Info("Planned eras")
	if len(todo) == 0 {
		return nil
	}

	p := &eraProgress{total: len(todo), start: time.Now()}
	g, gctx := errgroup.WithContext(ctx)
	for i, chunk := range splitEras(todo, f.Workers) {
		w := &eraWorker{
			client:  client,
			eraURL:  eraURL,
			files:   files,
			keepDir: eraDir,
			dir:     filepath.Join(eraDir, fmt.Sprintf("worker-%d", i)),
			db:      db,
			done:    p.done,
		}
		g.Go(func() error { return w.run(gctx, chunk) })
	}
	if err := g.Wait(); err != nil {
		return errors.Wrap(err, "stopped; run the same command again to continue")
	}
	if len(p.failed) > 0 {
		return fmt.Errorf("could not replay eras %v", p.failed)
	}
	log.WithField("eras", len(todo)).Info("Saved light client epoch data")
	return nil
}

// firstEra returns the first era with epoch data, the era after the Altair fork epoch.
func firstEra() uint64 {
	cfg := params.BeaconConfig()
	epochsPerEra := uint64(slots.ToEpoch(cfg.SlotsPerHistoricalRoot))
	return uint64(cfg.AltairForkEpoch)/epochsPerEra + 1
}

// eraEpochs returns the epochs whose LightClientEpochData era k builds: those with
// start_slot(E-1) in [(k-1)*SLOTS_PER_HISTORICAL_ROOT, k*SLOTS_PER_HISTORICAL_ROOT), after the
// Altair fork epoch. ok is false when there are none.
func eraEpochs(k uint64) (first, last primitives.Epoch, ok bool) {
	cfg := params.BeaconConfig()
	if k == 0 {
		return 0, 0, false
	}
	first = slots.ToEpoch(primitives.Slot(k-1)*cfg.SlotsPerHistoricalRoot) + 1
	last = slots.ToEpoch(primitives.Slot(k) * cfg.SlotsPerHistoricalRoot)
	first = max(first, cfg.AltairForkEpoch+1)
	return first, last, first <= last
}

type epochDataReader interface {
	LightClientEpochData(ctx context.Context, epoch primitives.Epoch) (light_client.EpochData, error)
}

// erasToReplay returns the eras in [from, to] that miss the epoch data of any of their epochs.
func erasToReplay(ctx context.Context, db epochDataReader, from, to uint64) ([]uint64, error) {
	var todo []uint64
	for k := from; k <= to; k++ {
		first, last, ok := eraEpochs(k)
		if !ok {
			continue
		}
		for e := first; e <= last; e++ {
			d, err := db.LightClientEpochData(ctx, e)
			if err != nil {
				return nil, err
			}
			if d == nil {
				todo = append(todo, k)
				break
			}
		}
	}
	return todo, nil
}

// splitEras splits eras into at most n contiguous parts of nearly equal size, so that each worker
// reuses the era files of its previous era.
func splitEras(eras []uint64, n int) [][]uint64 {
	n = min(n, len(eras))
	parts := make([][]uint64, 0, n)
	for i := range n {
		parts = append(parts, eras[i*len(eras)/n:(i+1)*len(eras)/n])
	}
	return parts
}

type epochDataSaver interface {
	SaveLightClientEpochData(ctx context.Context, data []light_client.EpochData) error
	SaveLightClientPeriodSyncCommittee(ctx context.Context, period uint64, committee *ethpb.SyncCommittee) error
}

// saveEpochData saves the committees before the epoch data. A stop between the two then leaves
// the era incomplete, and the next run replays it.
func saveEpochData(ctx context.Context, db epochDataSaver, data []light_client.EpochData, committees map[uint64]*ethpb.SyncCommittee) error {
	for _, period := range slices.Sorted(maps.Keys(committees)) {
		if err := db.SaveLightClientPeriodSyncCommittee(ctx, period, committees[period]); err != nil {
			return errors.Wrapf(err, "could not save sync committee of period %d", period)
		}
	}
	return db.SaveLightClientEpochData(ctx, data)
}

type eraProgress struct {
	mu     sync.Mutex
	total  int
	count  int
	failed []uint64
	start  time.Time
}

func (p *eraProgress) done(k uint64, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.count++
	if err != nil {
		p.failed = append(p.failed, k)
		log.WithError(err).WithField("era", k).Error("Could not replay era")
		return
	}
	elapsed := time.Since(p.start)
	log.WithFields(logrus.Fields{
		"era":  k,
		"done": fmt.Sprintf("%d/%d", p.count, p.total),
		"eta":  (elapsed / time.Duration(p.count) * time.Duration(p.total-p.count)).Round(time.Minute),
	}).Info("Saved era")
}

type eraWorker struct {
	client  *http.Client
	eraURL  string
	files   map[uint64]string
	keepDir string // era files already here are used and kept
	dir     string // this worker's downloads
	db      epochDataSaver
	done    func(k uint64, err error)
}

// run replays eras in order. A failed era is retried once with freshly downloaded files. It
// returns an error only when ctx is done.
func (w *eraWorker) run(ctx context.Context, eras []uint64) error {
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return err
	}
	// Keep the downloads of an earlier run that these eras need, including partial ones.
	needed := make(map[string]bool)
	for _, k := range eras {
		for _, n := range []uint64{k - 1, k, k + 1} {
			needed[w.files[n]] = true
		}
	}
	if err := removeUnneeded(w.dir, needed); err != nil {
		return err
	}

	for i, k := range eras {
		err := w.replay(ctx, k)
		if err != nil && ctx.Err() == nil {
			log.WithError(err).WithField("era", k).Warn("Could not replay era, retrying with new downloads")
			w.remove(k-1, k, k+1)
			err = w.replay(ctx, k)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		w.done(k, err)

		// Keep the files the next era needs.
		var next []uint64
		if i+1 < len(eras) {
			next = []uint64{eras[i+1] - 1, eras[i+1], eras[i+1] + 1}
		}
		for _, n := range []uint64{k - 1, k, k + 1} {
			if !slices.Contains(next, n) {
				w.remove(n)
			}
		}
	}
	_ = os.Remove(w.dir)
	return nil
}

// removeUnneeded deletes the files in dir that are not needed, counting a partial download of a
// needed file as needed.
func removeUnneeded(dir string, needed map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !needed[strings.TrimSuffix(e.Name(), ".part")] {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *eraWorker) replay(ctx context.Context, k uint64) error {
	readers := make([]*era.Reader, 0, 3)
	defer func() {
		for _, r := range readers {
			_ = r.Close()
		}
	}()
	for _, n := range []uint64{k - 1, k, k + 1} {
		path, err := w.fetch(ctx, n)
		if err != nil {
			return err
		}
		r, err := era.Open(path)
		if err != nil {
			return errors.Wrapf(err, "could not open %s", path)
		}
		readers = append(readers, r)
	}
	data, committees, err := lightClient.NewLightClientEpochDataFromEra(ctx, readers[0], readers[1], readers[2])
	if err != nil {
		return err
	}
	return saveEpochData(ctx, w.db, data, committees)
}

// fetch returns the path of era file n, downloading it into the worker's directory if it is not in
// keepDir or the worker's directory.
func (w *eraWorker) fetch(ctx context.Context, n uint64) (string, error) {
	name := w.files[n]
	for _, path := range []string{filepath.Join(w.keepDir, name), filepath.Join(w.dir, name)} {
		ok, err := file.Exists(path, file.Regular)
		if err != nil {
			return "", err
		}
		if ok {
			return path, nil
		}
	}
	fileURL, err := url.JoinPath(w.eraURL, name)
	if err != nil {
		return "", err
	}
	path := filepath.Join(w.dir, name)
	if err := downloadFile(ctx, w.client, fileURL, path); err != nil {
		return "", errors.Wrapf(err, "could not download %s", name)
	}
	return path, nil
}

// remove deletes the worker's downloads of era files ns.
func (w *eraWorker) remove(ns ...uint64) {
	for _, n := range ns {
		_ = os.Remove(filepath.Join(w.dir, w.files[n]))
	}
}
