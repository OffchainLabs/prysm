package timing

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/klauspost/compress/zstd"
	"github.com/pkg/errors"
)

const (
	queueSize      = 16384
	rotateBytes    = 64 << 20
	retainBytes    = 8 << 30
	rotateTime     = 5 * time.Minute
	statusInterval = 10 * time.Second
)

const (
	classReceipt = iota
	classValidation
	classWork
	classStatus
	classCount
)

var classLabels = [classCount]string{"receipt", "validation", "work", "status"}

type record map[string]any

type chunkInfo struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	First uint64 `json:"first"`
	Last  uint64 `json:"last"`
}

type recorder struct {
	dir     string
	origin  time.Time
	queue   chan record
	done    chan struct{}
	closing sync.Once
	// Emit must never block a gossip validator, so a full queue drops and counts.
	mu       sync.RWMutex
	closed   bool
	failed   atomic.Bool
	seq      atomic.Uint64
	drops    [classCount]atomic.Uint64
	dropLock sync.Mutex
	dropSeq  [2]uint64
	dropWall [2]uint64
}

func openRecorder(root, id string, metadata map[string]any) (*recorder, error) {
	dir := filepath.Join(root, id)
	if err := file.MkdirAll(dir); err != nil {
		return nil, errors.Wrap(err, "could not create the PQ timing run directory")
	}
	r := &recorder{dir: dir, origin: time.Now(), queue: make(chan record, queueSize), done: make(chan struct{})}
	metadata["recorder_status"] = r.status()
	metadata["chunks"] = []chunkInfo{}
	if err := writeMetadata(dir, metadata); err != nil {
		return nil, errors.Wrap(err, "could not write the PQ timing run metadata")
	}
	go func() {
		defer close(r.done)
		if err := r.write(metadata); err != nil {
			r.failed.Store(true)
			log.WithError(err).Error("PQ timing export disabled after a writer error")
			metadata["recorder_status"] = r.status()
			if err := writeMetadata(dir, metadata); err != nil {
				log.WithError(err).Warn("Could not record the PQ timing export failure")
			}
		}
	}()
	return r, nil
}

func (r *recorder) emit(class int, kind string, wall, mono time.Time, fields record) {
	if r.failed.Load() {
		return
	}
	seq := r.seq.Add(1)
	wallNs := uint64(wall.UnixNano()) // #nosec G115 -- wall clock is after 1970
	fields["seq"] = seq
	fields["kind"] = kind
	fields["wall_ns"] = wallNs
	fields["mono_ns"] = mono.Sub(r.origin).Nanoseconds()
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.closed {
		select {
		case r.queue <- fields:
			return
		default:
		}
	}
	r.drops[class].Add(1)
	r.dropLock.Lock()
	if r.dropSeq[0] == 0 {
		r.dropSeq[0], r.dropWall[0] = seq, wallNs
	}
	r.dropSeq[1], r.dropWall[1] = seq, wallNs
	r.dropLock.Unlock()
}

func (r *recorder) close() {
	r.closing.Do(func() {
		r.mu.Lock()
		r.closed = true
		close(r.queue)
		r.mu.Unlock()
	})
	<-r.done
}

func (r *recorder) status() map[string]any {
	dropped := make(map[string]uint64, classCount)
	for i, label := range classLabels {
		dropped[label] = r.drops[i].Load()
	}
	r.dropLock.Lock()
	defer r.dropLock.Unlock()
	var first any
	if r.dropSeq[0] != 0 {
		first = r.dropSeq[0]
	}
	return map[string]any{
		"disk_failed":            r.failed.Load(),
		"dropped":                dropped,
		"first_dropped_sequence": first,
		"last_dropped_sequence":  r.dropSeq[1],
		"submitted_sequence":     r.seq.Load(),
		"first_dropped_wall_ns":  r.dropWall[0],
		"last_dropped_wall_ns":   r.dropWall[1],
	}
}

type chunk struct {
	name    string
	partial string
	file    *os.File
	buffer  *bufio.Writer
	stream  *zstd.Encoder
	started time.Time
	bytes   int
	first   uint64
	last    uint64
}

func newChunk(dir string, number uint64) (*chunk, error) {
	name := fmt.Sprintf("events-%06d.jsonl.zst", number)
	partial := filepath.Join(dir, name+".partial")
	file, err := os.Create(partial) // #nosec G304 -- path is built from the operator's output flag
	if err != nil {
		return nil, err
	}
	buffer := bufio.NewWriter(file)
	stream, err := zstd.NewWriter(buffer, zstd.WithEncoderLevel(zstd.SpeedFastest))
	if err != nil {
		return nil, err
	}
	return &chunk{name: name, partial: partial, file: file, buffer: buffer, stream: stream, started: time.Now()}, nil
}

func (c *chunk) write(event record) error {
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := c.stream.Write(append(line, '\n')); err != nil {
		return err
	}
	seq, _ := event["seq"].(uint64)
	if c.first == 0 {
		c.first = seq
	}
	c.first = min(c.first, seq)
	c.last = max(c.last, seq)
	c.bytes += len(line) + 1
	return nil
}

func (c *chunk) finish(dir string) (chunkInfo, error) {
	if err := c.stream.Close(); err != nil {
		return chunkInfo{}, err
	}
	if err := c.buffer.Flush(); err != nil {
		return chunkInfo{}, err
	}
	info, err := c.file.Stat()
	if err != nil {
		return chunkInfo{}, err
	}
	if err := c.file.Close(); err != nil {
		return chunkInfo{}, err
	}
	if err := os.Rename(c.partial, filepath.Join(dir, c.name)); err != nil {
		return chunkInfo{}, err
	}
	return chunkInfo{Name: c.name, Bytes: info.Size(), First: c.first, Last: c.last}, nil
}

func writeMetadata(dir string, metadata map[string]any) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	partial := filepath.Join(dir, "run.json.partial")
	if err := file.WriteFile(partial, append(data, '\n')); err != nil {
		return err
	}
	return os.Rename(partial, filepath.Join(dir, "run.json"))
}

func (r *recorder) write(metadata map[string]any) error {
	var current *chunk
	var number uint64
	retained := []chunkInfo{}
	var retainedBytes int64
	var evictedChunks, evictedFirst, evictedLast uint64
	lastStatus := time.Now()
	timer := time.NewTicker(statusInterval)
	defer timer.Stop()
	for {
		closing := false
		select {
		case event, ok := <-r.queue:
			if !ok {
				closing = true
				break
			}
			if current == nil {
				number++
				var err error
				if current, err = newChunk(r.dir, number); err != nil {
					return err
				}
			}
			if err := current.write(event); err != nil {
				return err
			}
		case <-timer.C:
		}
		rotate := current != nil && (closing || current.bytes >= rotateBytes || time.Since(current.started) >= rotateTime)
		if rotate {
			info, err := current.finish(r.dir)
			if err != nil {
				return err
			}
			current = nil
			retained = append(retained, info)
			retainedBytes += info.Bytes
			for retainedBytes > retainBytes && len(retained) > 1 {
				old := retained[0]
				retained = retained[1:]
				if err := os.Remove(filepath.Join(r.dir, old.Name)); err != nil {
					return err
				}
				retainedBytes -= old.Bytes
				if evictedChunks == 0 {
					evictedFirst = old.First
				}
				evictedChunks++
				evictedLast = old.Last
			}
		}
		if rotate || closing || time.Since(lastStatus) >= statusInterval {
			status := r.status()
			status["evicted_chunks"] = evictedChunks
			if evictedChunks != 0 {
				status["evicted_sequence_bounds"] = [2]uint64{evictedFirst, evictedLast}
			}
			status["checked_unix_ns"] = time.Now().UnixNano()
			metadata["chunks"] = retained
			metadata["recorder_status"] = status
			if closing {
				metadata["ended_unix_ns"] = time.Now().UnixNano()
			}
			if err := writeMetadata(r.dir, metadata); err != nil {
				return err
			}
			lastStatus = time.Now()
		}
		if closing {
			return nil
		}
	}
}
