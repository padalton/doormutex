package camera

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeProc builds a synthetic /proc tree: pid → (comm, fd targets).
func fakeProc(t *testing.T, procs map[string][2]any) string {
	t.Helper()
	root := t.TempDir()
	for pid, p := range procs {
		dir := filepath.Join(root, pid)
		if err := os.MkdirAll(filepath.Join(dir, "fd"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "comm"), []byte(p[0].(string)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for i, target := range p[1].([]string) {
			if err := os.Symlink(target, filepath.Join(dir, "fd", string(rune('0'+i)))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "self"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestUsers(t *testing.T) {
	root := fakeProc(t, map[string][2]any{
		"100": {"zoom", []string{"/dev/null", "/dev/video0"}},
		"101": {"bash", []string{"/dev/pts/1"}},
		"102": {"pipewire", []string{"/dev/video0"}},
		"103": {"obs", []string{"/dev/video2"}},
		"104": {"zoom", []string{"/dev/video0"}},
		"105": {"guvcview", []string{"/dev/video-not-a-camera"}},
	})
	got := Users(root, []string{"OBS"})
	if !reflect.DeepEqual(got, []string{"zoom"}) {
		t.Errorf("Users = %v", got)
	}
	if Users(filepath.Join(root, "missing"), nil) != nil {
		t.Error("missing proc should yield nothing")
	}
}
