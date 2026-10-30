package pipewire

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// testdata/pw-dump.json is synthetic: hand-written in the shape of pw-dump.
func TestParseAndActive(t *testing.T) {
	f, err := os.Open("testdata/pw-dump.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	streams, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 8 {
		t.Fatalf("streams = %d: %+v", len(streams), streams)
	}
	byID := map[int]Stream{}
	for _, s := range streams {
		byID[s.ID] = s
	}
	if !byID[51].Monitor || !byID[52].Monitor || byID[50].Monitor {
		t.Errorf("monitor detection: %+v %+v %+v", byID[50], byID[51], byID[52])
	}
	if !byID[53].Meter || !byID[54].Meter {
		t.Errorf("meter detection: %+v %+v", byID[53], byID[54])
	}

	// Firefox is in a call; OBS/cava record outputs, pavucontrol/plasmashell
	// are meters, Slack is idle, EasyEffects is ignored.
	on, apps := Active(streams, AudioCapture, []string{"easyeffects"})
	if !on || !reflect.DeepEqual(apps, []string{"Firefox"}) {
		t.Errorf("microphone = %v %v", on, apps)
	}
	on, apps = Active(streams, VideoCapture, nil)
	if !on || !reflect.DeepEqual(apps, []string{"Firefox"}) {
		t.Errorf("camera = %v %v", on, apps)
	}
	if on, _ := Active(streams, AudioCapture, []string{"Firefox", "easyeffects"}); on {
		t.Error("ignore list not applied")
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse(strings.NewReader("not json")); err == nil {
		t.Error("expected an error")
	}
}
