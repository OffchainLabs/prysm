package driver

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/pkg/errors"
	"golang.org/x/tools/go/packages"
)

type JSONDriver struct {
	registry *registry
	recorder *recorder
}

func NewJSONDriver() (*JSONDriver, error) {
	envPtr, err := loadEnv()
	if err != nil {
		return nil, fmt.Errorf("unable to load environment: %w", err)
	}
	env := *envPtr
	configureLog(env)

	jsonFiles, err := loadJsonListing(env)
	if err != nil {
		return nil, errors.Wrap(err, "unable to lookup package")
	}
	rec, err := newRecorder(env)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize recorder: %w", err)
	}
	d := &JSONDriver{
		registry: newRegistry(env),
		recorder: rec,
	}

	for _, f := range jsonFiles {
		if err := d.load(f); err != nil {
			return nil, fmt.Errorf("unable to walk json: %w", err)
		}
	}

	if err := d.registry.resolvePackages(); err != nil {
		return nil, fmt.Errorf("unable to resolve paths: %w", err)
	}

	return d, nil
}

func (d *JSONDriver) load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("unable to open package JSON file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.WithError(err).WithField("file", f.Name()).Error("unable to close file")
		}
	}()

	decoder := json.NewDecoder(f)
	for decoder.More() {
		pkg := &flatPackage{}
		if err := decoder.Decode(&pkg); err != nil {
			return fmt.Errorf("unable to decode package in %s: %w", f.Name(), err)
		}
		d.registry.add(pkg)
	}
	return nil
}

func (d *JSONDriver) Handle(in io.Reader, queries []string) ([]byte, error) {
	req := &packages.DriverRequest{}
	if err := json.NewDecoder(in).Decode(&req); err != nil {
		return nil, fmt.Errorf("unable to decode driver request: %w", err)
	}
	if err := d.recorder.recordRequest(queries, req); err != nil {
		return nil, fmt.Errorf("unable to record request: %w", err)
	}

	r, p := d.registry.query(queries)
	resp := &driverResponse{
		NotHandled: false,
		Compiler:   "gc",
		Arch:       runtime.GOARCH,
		GoVersion:  goVersion(),
		Roots:      r,
		Packages:   p,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal response: %w", err)
	}

	if err := d.recorder.recordResponse(data); err != nil {
		return nil, fmt.Errorf("unable to record response: %w", err)
	}

	return data, nil
}
