// Package camera finds processes that hold a V4L2 camera device open, by
// reading /proc/<pid>/fd. Apps that use the camera through PipeWire are
// found by the pipewire package instead.
package camera

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var videoDev = regexp.MustCompile(`^/dev/video[0-9]+$`)

// system processes that open cameras for every client (PipeWire's V4L2
// plugin) – the actual app is found via its PipeWire stream.
var system = []string{"pipewire", "wireplumber"}

// Users returns the names (comm) of processes with an open /dev/video*
// device. proc is the proc file system root, normally "/proc". Processes
// of other users can't be read and are skipped.
func Users(proc string, ignore []string) []string {
	entries, err := os.ReadDir(proc)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !isPID(e.Name()) {
			continue
		}
		dir := filepath.Join(proc, e.Name())
		fds, err := os.ReadDir(filepath.Join(dir, "fd"))
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(dir, "fd", fd.Name()))
			if err != nil || !videoDev.MatchString(target) {
				continue
			}
			b, _ := os.ReadFile(filepath.Join(dir, "comm"))
			name := strings.TrimSpace(string(b))
			if name != "" && !slices.Contains(system, name) && !containsFold(ignore, name) && !slices.Contains(out, name) {
				out = append(out, name)
			}
			break
		}
	}
	return out
}

func isPID(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
