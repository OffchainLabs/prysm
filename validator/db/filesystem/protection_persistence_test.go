package filesystem

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/io/file"
)

type faultProtectionFile struct {
	protectionFile
	chmod func(os.FileMode) error
	write func([]byte) (int, error)
	sync  func() error
	close func() error
}

func (f *faultProtectionFile) Chmod(m os.FileMode) error {
	if f.chmod != nil {
		return f.chmod(m)
	}
	return f.protectionFile.Chmod(m)
}
func (f *faultProtectionFile) Write(b []byte) (int, error) {
	if f.write != nil {
		return f.write(b)
	}
	return f.protectionFile.Write(b)
}
func (f *faultProtectionFile) Sync() error {
	if f.sync != nil {
		return f.sync()
	}
	return f.protectionFile.Sync()
}
func (f *faultProtectionFile) Close() error {
	if f.close != nil {
		return f.close()
	}
	return f.protectionFile.Close()
}

func TestProtectionPersistenceFailures(t *testing.T) {
	if err := protectionDurabilitySupported(); err != nil {
		t.Skip(err)
	}
	for _, existing := range []bool{false, true} {
		for _, phase := range []string{"create", "chmod", "write", "short write", "file sync", "file close", "rename", "directory open", "directory sync", "directory close", "ancestor sync", "cleanup"} {
			t.Run(phase+map[bool]string{false: "/create", true: "/replace"}[existing], func(t *testing.T) {
				s, err := NewStore(t.TempDir(), nil)
				if err != nil {
					t.Fatal(err)
				}
				key := getPubKeys(t, 1)[0]
				target := s.pubkeySlashingProtectionFilePath(key)
				dir := filepath.Dir(target)
				if err := file.MkdirAll(dir); err != nil {
					t.Fatal(err)
				}
				old := []byte("lastSignedAttestationSourceEpoch: 7\n")
				next := []byte("lastSignedAttestationSourceEpoch: 8\n")
				if existing {
					if err := file.WriteFile(target, old); err != nil {
						t.Fatal(err)
					}
				}
				ops := defaultProtectionPersistence()
				failure := errors.New("injected " + phase)
				cleanupFailure := errors.New("cleanup failed")
				create := ops.createTemp
				ops.createTemp = func(d, p string) (protectionFile, error) {
					if phase == "create" {
						return nil, failure
					}
					f, err := create(d, p)
					if err != nil {
						return nil, err
					}
					w := &faultProtectionFile{protectionFile: f}
					switch phase {
					case "chmod":
						w.chmod = func(os.FileMode) error { return failure }
					case "write", "cleanup":
						w.write = func([]byte) (int, error) { return 0, failure }
					case "short write":
						w.write = func(b []byte) (int, error) { return len(b) - 1, nil }
					case "file sync":
						w.sync = func() error { return failure }
					case "file close":
						w.close = func() error {
							_ = f.Close()
							return failure
						}
					}
					return w, nil
				}
				if phase == "rename" {
					ops.rename = func(string, string) error { return failure }
				}
				if phase == "cleanup" {
					ops.remove = func(string) error { return cleanupFailure }
				}
				open := ops.openDir
				count := 0
				ops.openDir = func(p string) (protectionFile, error) {
					count++
					if phase == "directory open" {
						return nil, failure
					}
					f, err := open(p)
					if err != nil {
						return nil, err
					}
					w := &faultProtectionFile{protectionFile: f}
					if phase == "directory sync" || (phase == "ancestor sync" && count == 2) {
						w.sync = func() error { return failure }
					}
					if phase == "directory close" {
						w.close = func() error {
							_ = f.Close()
							return failure
						}
					}
					return w, nil
				}
				err = ops.write(target, next)
				if err == nil {
					t.Fatal("expected persistence error")
				}
				wantErr := failure
				if phase == "short write" {
					wantErr = io.ErrShortWrite
				}
				if !errors.Is(err, wantErr) {
					t.Fatalf("primary error: %v", err)
				}
				if phase == "cleanup" && !errors.Is(err, cleanupFailure) {
					t.Fatalf("cleanup error lost: %v", err)
				}
				published := strings.HasPrefix(phase, "directory") || phase == "ancestor sync"
				got, readErr := os.ReadFile(target)
				if published {
					if readErr != nil || string(got) != string(next) {
						t.Fatalf("new bytes: %q %v", got, readErr)
					}
				} else if existing {
					if readErr != nil || string(got) != string(old) {
						t.Fatalf("old bytes: %q %v", got, readErr)
					}
				} else if !os.IsNotExist(readErr) {
					t.Fatalf("unexpected destination: %v", readErr)
				}
				reopened, err := NewStore(s.DatabasePath(), nil)
				if err != nil {
					t.Fatal(err)
				}
				record, err := reopened.validatorSlashingProtection(key)
				if err != nil {
					t.Fatal(err)
				}
				if published {
					if record == nil || record.LastSignedAttestationSourceEpoch != 8 {
						t.Fatalf("new record: %+v", record)
					}
				} else if existing {
					if record == nil || record.LastSignedAttestationSourceEpoch != 7 {
						t.Fatalf("old record: %+v", record)
					}
				} else if record != nil {
					t.Fatalf("unexpected record: %+v", record)
				}
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if entry.Name() != filepath.Base(target) && phase != "cleanup" {
						t.Errorf("temporary debris: %s", entry.Name())
					}
				}
			})
		}
	}
}

func TestProtectionPersistenceOrdering(t *testing.T) {
	if err := protectionDurabilitySupported(); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "record.yaml")
	ops := defaultProtectionPersistence()
	var events []string
	create := ops.createTemp
	ops.createTemp = func(d, p string) (protectionFile, error) {
		if d != dir || !strings.HasPrefix(p, ".") {
			t.Fatalf("unsafe temp: %s %s", d, p)
		}
		events = append(events, "create")
		f, err := create(d, p)
		if err != nil {
			return nil, err
		}
		return &faultProtectionFile{
			protectionFile: f,
			chmod: func(m os.FileMode) error {
				events = append(events, "chmod")
				return f.Chmod(m)
			},
			write: func(b []byte) (int, error) {
				events = append(events, "write")
				return f.Write(b)
			},
			sync: func() error {
				events = append(events, "sync")
				return f.Sync()
			},
			close: func() error {
				events = append(events, "close")
				return f.Close()
			},
		}, nil
	}
	rename := ops.rename
	ops.rename = func(a, b string) error {
		events = append(events, "rename")
		return rename(a, b)
	}
	open := ops.openDir
	ops.openDir = func(p string) (protectionFile, error) {
		events = append(events, p)
		f, err := open(p)
		if err != nil {
			return nil, err
		}
		return &faultProtectionFile{
			protectionFile: f,
			sync: func() error {
				events = append(events, "directory sync")
				return f.Sync()
			},
			close: func() error {
				events = append(events, "directory close")
				return f.Close()
			},
		}, nil
	}
	data := []byte("lastSignedAttestationSourceEpoch: 0\n")
	if err := ops.write(target, data); err != nil {
		t.Fatal(err)
	}
	want := []string{"create", "chmod", "write", "sync", "close", "rename"}
	for p := dir; ; p = filepath.Dir(p) {
		want = append(want, p, "directory sync", "directory close")
		if filepath.Dir(p) == p {
			break
		}
	}
	if !reflect.DeepEqual(want, events) {
		t.Fatalf("order %v want %v", events, want)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v", info.Mode())
	}
	if err := os.Chmod(target, 0644); err != nil {
		t.Fatal(err)
	}
	if err := ops.write(target, []byte("replacement")); err == nil || !strings.Contains(err.Error(), "0600") {
		t.Fatalf("permissions: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) || info.Mode().Perm() != 0644 {
		t.Fatal("permission rejection changed target")
	}
}

func TestProtectionInitializationRetries(t *testing.T) {
	if err := protectionDurabilitySupported(); err != nil {
		t.Skip(err)
	}
	for _, kind := range []string{"fresh", "empty", "history"} {
		for _, phase := range []string{"directory open", "directory sync", "ancestor sync"} {
			t.Run(kind+"/"+phase, func(t *testing.T) {
				s, err := NewStore(filepath.Join(t.TempDir(), "new", "parent"), nil)
				if err != nil {
					t.Fatal(err)
				}
				keys := getPubKeys(t, 1)
				target := s.pubkeySlashingProtectionFilePath(keys[0])
				var before []byte
				if kind == "empty" {
					keys = nil
				}
				if kind == "history" {
					if err := file.MkdirAll(s.slashingProtectionDirPath()); err != nil {
						t.Fatal(err)
					}
					before = []byte("lastSignedAttestationSourceEpoch: 17\nlatestSignedBlockSlot: 29\nlastSignedAttestationTargetEpoch: 20\n")
					if err := file.WriteFile(target, before); err != nil {
						t.Fatal(err)
					}
				}
				ops := defaultProtectionPersistence()
				s.protectionPersistence = &ops
				open := ops.openDir
				calls := 0
				ops.openDir = func(p string) (protectionFile, error) {
					failure := errors.New("directory barrier")
					if phase == "directory open" {
						calls++
						return nil, failure
					}
					f, err := open(p)
					if err != nil {
						return nil, err
					}
					if phase == "directory sync" || p == filepath.Dir(s.slashingProtectionDirPath()) {
						return &faultProtectionFile{
							protectionFile: f,
							sync: func() error {
								calls++
								return failure
							},
						}, nil
					}
					return f, nil
				}
				for i := 0; i < 2; i++ {
					if err := s.UpdatePublicKeysBuckets(keys); err == nil {
						t.Fatal("barrier skipped")
					}
					if kind != "empty" {
						got, err := os.ReadFile(target)
						if err != nil {
							t.Fatal(err)
						}
						if before == nil {
							before = got
						} else if string(got) != string(before) {
							t.Fatal("retry changed record")
						}
					}
				}
				if calls != 2 {
					t.Fatalf("barriers %d", calls)
				}
				ops.openDir = open
				if err := s.UpdatePublicKeysBuckets(keys); err != nil {
					t.Fatal(err)
				}
				if kind != "empty" {
					got, err := os.ReadFile(target)
					if err != nil {
						t.Fatal(err)
					}
					if string(got) != string(before) {
						t.Fatal("history reset")
					}
					reopened, err := NewStore(s.DatabasePath(), nil)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := reopened.validatorSlashingProtection(keys[0]); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestProtectionPersistenceInterruption(t *testing.T) {
	if err := protectionDurabilitySupported(); err != nil {
		t.Skip(err)
	}
	if stage := os.Getenv("PROTECTION_INTERRUPT_STAGE"); stage != "" {
		ops := defaultProtectionPersistence()
		pause := func() {
			_, _ = os.Stdout.WriteString("ready\n")
			for {
				time.Sleep(time.Hour)
			}
		}
		if stage == "partial" {
			create := ops.createTemp
			ops.createTemp = func(d, p string) (protectionFile, error) {
				f, err := create(d, p)
				if err != nil {
					return nil, err
				}
				return &faultProtectionFile{protectionFile: f, write: func(b []byte) (int, error) {
					n, err := f.Write(b[:len(b)/2])
					if err != nil {
						return n, err
					}
					pause()
					return n, nil
				}}, nil
			}
		} else {
			rename := ops.rename
			ops.rename = func(a, b string) error {
				if stage == "before rename" {
					pause()
				}
				if err := rename(a, b); err != nil {
					return err
				}
				pause()
				return nil
			}
		}
		if err := ops.write(os.Getenv("PROTECTION_INTERRUPT_PATH"), []byte("lastSignedAttestationSourceEpoch: 8\n")); err != nil {
			t.Fatal(err)
		}
		t.Fatal("child did not stop")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, existing := range []bool{false, true} {
		for _, stage := range []string{"partial", "before rename", "after rename"} {
			t.Run(stage+map[bool]string{false: "/create", true: "/replace"}[existing], func(t *testing.T) {
				s, err := NewStore(t.TempDir(), nil)
				if err != nil {
					t.Fatal(err)
				}
				key := getPubKeys(t, 1)[0]
				target := s.pubkeySlashingProtectionFilePath(key)
				if err := file.MkdirAll(filepath.Dir(target)); err != nil {
					t.Fatal(err)
				}
				if existing {
					if err := file.WriteFile(target, []byte("lastSignedAttestationSourceEpoch: 7\n")); err != nil {
						t.Fatal(err)
					}
				}
				cmd := exec.Command(executable, "-test.run=^TestProtectionPersistenceInterruption$")
				cmd.Env = append(os.Environ(), "PROTECTION_INTERRUPT_STAGE="+stage, "PROTECTION_INTERRUPT_PATH="+target)
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				cmd.Stderr = os.Stderr
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = cmd.Process.Kill() })
				ready := make(chan string, 1)
				go func() {
					line, _ := bufio.NewReader(stdout).ReadString('\n')
					ready <- line
				}()
				select {
				case line := <-ready:
					if line != "ready\n" {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
						t.Fatalf("handshake: %q", line)
					}
				case <-time.After(30 * time.Second):
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					t.Fatal("handshake timeout")
				}
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = cmd.Wait()
				reopened, err := NewStore(s.DatabasePath(), nil)
				if err != nil {
					t.Fatal(err)
				}
				record, err := reopened.validatorSlashingProtection(key)
				if err != nil {
					t.Fatal(err)
				}
				if stage == "after rename" {
					if record == nil || record.LastSignedAttestationSourceEpoch != 8 {
						t.Fatalf("new record: %+v", record)
					}
				} else if existing {
					if record == nil || record.LastSignedAttestationSourceEpoch != 7 {
						t.Fatalf("old record: %+v", record)
					}
				} else if record != nil {
					t.Fatalf("unexpected record: %+v", record)
				}
				indexed, err := reopened.publicKeys()
				if err != nil {
					t.Fatal(err)
				}
				want := 0
				if existing || stage == "after rename" {
					want = 1
				}
				if len(indexed) != want {
					t.Fatalf("debris indexed: %v", indexed)
				}
			})
		}
	}
}

func TestProtectionPersistenceNormalizesTemporaryPermissions(t *testing.T) {
	if err := protectionDurabilitySupported(); err != nil {
		t.Skip(err)
	}
	target := filepath.Join(t.TempDir(), "record.yaml")
	ops := defaultProtectionPersistence()
	create := ops.createTemp
	ops.createTemp = func(d, p string) (protectionFile, error) {
		f, err := create(d, p)
		if err != nil {
			return nil, err
		}
		if err := f.Chmod(0400); err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
			return nil, err
		}
		return f, nil
	}
	for i := 0; i < 2; i++ {
		if err := ops.write(target, []byte("lastSignedAttestationSourceEpoch: 0\n")); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("mode: %v", info.Mode())
		}
	}
}

func TestProtectionInitializationPreservesInvalidRecords(t *testing.T) {
	if err := protectionDurabilitySupported(); err != nil {
		t.Skip(err)
	}
	for _, data := range []string{"", "lastSignedAttestationSourceEpoch: [\n"} {
		t.Run(data, func(t *testing.T) {
			s, err := NewStore(t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			keys := getPubKeys(t, 1)
			target := s.pubkeySlashingProtectionFilePath(keys[0])
			if err := file.MkdirAll(filepath.Dir(target)); err != nil {
				t.Fatal(err)
			}
			if err := file.WriteFile(target, []byte(data)); err != nil {
				t.Fatal(err)
			}
			if err := s.UpdatePublicKeysBuckets(keys); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != data {
				t.Fatal("invalid bytes changed")
			}
			if _, err := s.validatorSlashingProtection(keys[0]); err == nil {
				t.Fatal("invalid record accepted")
			}
		})
	}
}

func TestProtectionPersistencePathFailures(t *testing.T) {
	if err := protectionDurabilitySupported(); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := file.WriteFile(blocker, []byte("unchanged")); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(blocker, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePublicKeysBuckets(nil); err == nil {
		t.Fatal("created directory through file")
	}
	if err := defaultProtectionPersistence().write(filepath.Join(dir, "absent", "record"), []byte("data")); err == nil {
		t.Fatal("created temporary in absent directory")
	}
	target := filepath.Join(dir, "directory")
	if err := file.MkdirAll(target); err != nil {
		t.Fatal(err)
	}
	if err := defaultProtectionPersistence().write(target, []byte("data")); err == nil {
		t.Fatal("replaced directory")
	}
	t.Setenv("PROTECTION_TEST_PATH", dir)
	if err := defaultProtectionPersistence().write("$PROTECTION_TEST_PATH/expanded.yaml", []byte("lastSignedAttestationSourceEpoch: 0\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "expanded.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestProtectionUnsupportedPlatform(t *testing.T) {
	if protectionDurabilitySupported() == nil {
		t.Skip("native platform supports durable protection writes")
	}
	s, err := NewStore(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	keys := getPubKeys(t, 1)
	if err := s.UpdatePublicKeysBuckets(keys); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("init: %v", err)
	}
	if err := s.saveValidatorSlashingProtection(keys[0], &ValidatorSlashingProtection{}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("update: %v", err)
	}
	if _, err := os.Stat(s.slashingProtectionDirPath()); !os.IsNotExist(err) {
		t.Fatalf("unsupported write mutated directory: %v", err)
	}
}
