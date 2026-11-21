package timing

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func procField(file, key string) any {
	data, err := os.ReadFile(file) // #nosec G304 -- fixed procfs paths
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(name) == key {
			return strings.TrimSpace(value)
		}
	}
	return nil
}

func hardware() map[string]any {
	cpu := procField("/proc/cpuinfo", "model name")
	if cpu == nil {
		cpu = procField("/proc/cpuinfo", "Hardware")
	}
	var memory any
	if total, ok := procField("/proc/meminfo", "MemTotal").(string); ok {
		if kib, err := strconv.ParseUint(strings.Fields(total)[0], 10, 64); err == nil {
			memory = kib * 1024
		}
	}
	var kernel any
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		kernel = strings.TrimSpace(string(data))
	}
	quota, limit := cgroupLimits()
	return map[string]any{
		"architecture":           runtime.GOARCH,
		"os":                     runtime.GOOS,
		"kernel":                 kernel,
		"cpu_model":              cpu,
		"available_parallelism":  runtime.NumCPU(),
		"cpu_affinity":           procField("/proc/self/status", "Cpus_allowed_list"),
		"cgroup_cpu_equivalents": quota,
		"visible_memory_bytes":   memory,
		"cgroup_memory_bytes":    limit,
	}
}

func cgroupLimits() (any, any) {
	const root = "/sys/fs/cgroup"
	dir := root
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if group, ok := strings.CutPrefix(line, "0::/"); ok {
				if candidate := filepath.Join(root, group); isDir(candidate) {
					dir = candidate
				}
			}
		}
	}
	var quota, limit any
	for ; strings.HasPrefix(dir, root); dir = filepath.Dir(dir) {
		if data, err := os.ReadFile(filepath.Join(dir, "cpu.max")); err == nil { // #nosec G304 -- cgroupfs path
			parts := strings.Fields(string(data))
			if len(parts) == 2 {
				q, errQ := strconv.ParseFloat(parts[0], 64)
				p, errP := strconv.ParseFloat(parts[1], 64)
				if errQ == nil && errP == nil && p > 0 {
					if current, ok := quota.(float64); !ok || q/p < current {
						quota = q / p
					}
				}
			}
		}
		if data, err := os.ReadFile(filepath.Join(dir, "memory.max")); err == nil { // #nosec G304 -- cgroupfs path
			if m, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
				if current, ok := limit.(uint64); !ok || m < current {
					limit = m
				}
			}
		}
	}
	return quota, limit
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
