package db

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

// eraFileLink matches a link to an era file in a directory page, for example
// href="mainnet-01886-653d1f74.era".
var eraFileLink = regexp.MustCompile(`href="(([a-z0-9]+)-(\d{5})-[0-9a-f]{8}\.era)"`)

// downloadRetryDelay is the wait before the second download attempt; the third waits twice as long.
var downloadRetryDelay = 10 * time.Second
var downloadStallTimeout = time.Minute

// listEraFiles reads the directory page at baseURL and returns the era file name of each era of
// configName.
func listEraFiles(ctx context.Context, c *http.Client, baseURL, configName string) (map[uint64]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not list era files at %s: %s", baseURL, resp.Status)
	}
	page, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, errors.Wrap(err, "could not read era file list")
	}
	files := make(map[uint64]string)
	for _, m := range eraFileLink.FindAllStringSubmatch(string(page), -1) {
		if m[2] != configName {
			continue
		}
		era, err := strconv.ParseUint(m[3], 10, 64)
		if err != nil {
			return nil, err
		}
		files[era] = m[1]
	}
	return files, nil
}

// downloadFile downloads url to path. It writes to path+".part", resumes it with a range request,
// and renames it to path when complete. It tries up to three times.
func downloadFile(ctx context.Context, c *http.Client, url, path string) error {
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			log.WithError(err).WithField("url", url).Warn("Download failed, retrying")
			select {
			case <-time.After(time.Duration(attempt-1) * downloadRetryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err = downloadOnce(ctx, c, url, path); err == nil || ctx.Err() != nil {
			return err
		}
	}
	return err
}

func downloadOnce(ctx context.Context, c *http.Client, url, path string) (err error) {
	part := path + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(part, path)
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	have := info.Size()

	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stall := time.AfterFunc(downloadStallTimeout, cancel)
	defer stall.Stop()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var total int64
	switch resp.StatusCode {
	case http.StatusPartialContent:
		var start, end int64
		if _, err := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &start, &end, &total); err != nil || start != have {
			return fmt.Errorf("unexpected Content-Range %q for a file of %d bytes", resp.Header.Get("Content-Range"), have)
		}
	case http.StatusOK:
		// The server sent the whole file.
		if err := f.Truncate(0); err != nil {
			return err
		}
		have, total = 0, resp.ContentLength
	case http.StatusRequestedRangeNotSatisfiable:
		// The partial file is complete or longer than the file.
		if _, err := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes */%d", &total); err == nil && total == have {
			return nil
		}
		if err := f.Truncate(0); err != nil {
			return err
		}
		return errors.New("partial file is longer than the file")
	default:
		return fmt.Errorf("could not download %s: %s", url, resp.Status)
	}

	n, err := io.Copy(f, &stallReader{r: resp.Body, stall: stall})
	if err != nil {
		if ctx.Err() == nil && reqCtx.Err() != nil {
			return fmt.Errorf("download stalled for %s", downloadStallTimeout)
		}
		return err
	}
	if total >= 0 && have+n != total {
		return fmt.Errorf("downloaded %d of %d bytes", have+n, total)
	}
	return nil
}

// stallReader resets the stall timer whenever data arrives.
type stallReader struct {
	r     io.Reader
	stall *time.Timer
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.stall.Reset(downloadStallTimeout)
	}
	return n, err
}
