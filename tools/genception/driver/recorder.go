package driver

import (
	"encoding/json"
	"os"
	"path"
	"strconv"
	"time"

	"golang.org/x/tools/go/packages"
)

type recorder struct {
	env environment
	t   time.Time
}

func newRecorder(env environment) (*recorder, error) {
	r := &recorder{env: env, t: time.Now()}
	if env.recorderPath == "" {
		log.Info(ENV_RECORDER_PATH + " not set; set this to a writable directory path if you want to record the driver response for debugging")
	} else {
		if err := r.mkdir(); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *recorder) dir() string {
	return path.Join(r.env.recorderPath, strconv.FormatInt(r.t.UTC().UnixNano(), 10))
}

func (r *recorder) mkdir() error {
	return os.MkdirAll(r.dir(), 0755)
}

func (r *recorder) recordRequest(args []string, req *packages.DriverRequest) error {
	if r.env.recorderPath == "" {
		return nil
	}
	b, err := json.Marshal(struct {
		Args    []string
		Request *packages.DriverRequest
	}{
		Args:    args,
		Request: req,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path.Join(r.dir(), "request.json"), b, 0600)
}

func (r *recorder) recordResponse(resp []byte) error {
	if r.env.recorderPath == "" {
		return nil
	}
	return os.WriteFile(path.Join(r.dir(), "response.json"), resp, 0600)
}
