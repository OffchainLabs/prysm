package pqkeys

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/validator/vrftree"
)

const (
	// vrfRecordSize is the size of a record of the VRF state: the VRF root, the slot, and the branch of the leaf of the slot.
	vrfRecordSize = 32 + 4 + 32*vrftree.Depth

	// vrfStateHTTPTimeout bounds the download of the VRF state.
	vrfStateHTTPTimeout = 10 * time.Minute
)

type (
	// VRFRecord is the record of a validator in the VRF state: the root of its full VRF tree, and the branch of the leaf
	// of slot 0 in it.
	VRFRecord struct {
		Root   [32]byte
		Branch [vrftree.Depth][32]byte
	}

	// VRFStateSource gives the records of the VRF state the genesis tool publishes.
	//
	// The VRF state is the concatenation of the records of the validators of the genesis, each of `vrfRecordSize` bytes:
	// the 32-byte VRF root, the slot as a little-endian uint32, always 0, and the 31 nodes of the branch of the leaf
	// of the slot, from the bottom up. This is the format Lighthouse reads with its `--vrf-state-url` flag.
	VRFStateSource interface {
		// Records returns the records of the VRF roots `roots`. A root without a single valid record is missing from the result.
		Records(ctx context.Context, roots map[[32]byte]bool) (map[[32]byte]VRFRecord, error)
	}

	// vrfStateSource reads the VRF state from an HTTP(S) URL, or from a file.
	vrfStateSource struct {
		url    string // HTTP(S) URL. Empty if the VRF state is in a file.
		path   string // Path of the file. Empty if the VRF state is at a URL.
		client *http.Client
	}
)

var _ VRFStateSource = (*vrfStateSource)(nil)

// NewVRFStateSource returns the source of the VRF state at `location`: an HTTP(S) URL, a file URI, or a path.
func NewVRFStateSource(location string) (VRFStateSource, error) {
	httpURL, path, err := parseLocation("VRF state", location)
	if err != nil {
		return nil, err
	}

	if path != "" {
		return &vrfStateSource{path: path}, nil
	}

	return &vrfStateSource{url: httpURL, client: &http.Client{Timeout: vrfStateHTTPTimeout}}, nil
}

// parseLocation returns the HTTP(S) URL or the path of the file `location` is: an HTTP(S) URL, a file URI, or a path.
// `name` names what is at `location` in the errors.
func parseLocation(name, location string) (httpURL, path string, err error) {
	if location == "" {
		return "", "", fmt.Errorf("empty %s location", name)
	}

	if !strings.Contains(location, "://") {
		return "", location, nil
	}

	parsed, err := url.Parse(location)
	if err != nil {
		return "", "", fmt.Errorf("parse %s URL: %w", name, err)
	}

	switch parsed.Scheme {
	case "http", "https":
		return location, "", nil
	case "file":
		if parsed.Path == "" {
			return "", "", fmt.Errorf("file URI %s without a path", location)
		}

		return "", parsed.Path, nil
	default:
		return "", "", fmt.Errorf("%s URL scheme %q is neither http, https nor file", name, parsed.Scheme)
	}
}

// Records reads the whole VRF state, and returns the records of `roots`.
// It streams the VRF state: it keeps the records of `roots` only.
func (s *vrfStateSource) Records(ctx context.Context, roots map[[32]byte]bool) (map[[32]byte]VRFRecord, error) {
	reader, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := reader.Close(); err != nil {
			log.WithError(err).Debug("Could not close the VRF state")
		}
	}()

	return readVRFRecords(bufio.NewReaderSize(reader, 64*vrfRecordSize), roots)
}

// open opens the VRF state.
func (s *vrfStateSource) open(ctx context.Context) (io.ReadCloser, error) {
	if s.path != "" {
		file, err := os.Open(s.path)
		if err != nil {
			return nil, fmt.Errorf("open VRF state: %w", err)
		}

		return file, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, fmt.Errorf("new VRF state request: %w", err)
	}

	// Go transparently decompresses gzip responses, but then cannot tell their length. Ask for the raw bytes.
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get VRF state: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Debug("Could not close the VRF state response")
		}

		return nil, fmt.Errorf("get VRF state: status %s", resp.Status)
	}

	return resp.Body, nil
}

// readVRFRecords reads the records of the VRF state from `reader`, and returns the records of `roots`.
// A root with several records, or whose record is not of slot 0, is missing from the result.
func readVRFRecords(reader io.Reader, roots map[[32]byte]bool) (map[[32]byte]VRFRecord, error) {
	records := make(map[[32]byte]VRFRecord, len(roots))
	invalid := make(map[[32]byte]string)

	buffer := [vrfRecordSize]byte{}
	for index := 0; ; index++ {
		if _, err := io.ReadFull(reader, buffer[:]); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			if errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, fmt.Errorf("VRF state ends with an incomplete record at index %d", index)
			}

			return nil, fmt.Errorf("read VRF state: %w", err)
		}

		root := [32]byte(buffer[:32])
		if !roots[root] {
			continue
		}

		if _, ok := records[root]; ok {
			invalid[root] = "several records"
			continue
		}

		if slot := binary.LittleEndian.Uint32(buffer[32:36]); slot != 0 {
			invalid[root] = fmt.Sprintf("record of slot %d instead of 0", slot)
			continue
		}

		record := VRFRecord{Root: root}
		for level := range record.Branch {
			start := 36 + 32*level
			record.Branch[level] = [32]byte(buffer[start : start+32])
		}

		records[root] = record
	}

	for root, reason := range invalid {
		log.WithField("vrfRoot", fmt.Sprintf("%#x", root)).WithField("reason", reason).Error("Invalid VRF state for this VRF root")
		delete(records, root)
	}

	return records, nil
}
