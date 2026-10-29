// Package pipewire finds audio and video capture streams in the output of
// pw-dump. Only yes/no and application names leave this package – never
// audio or video.
package pipewire

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
)

// Media classes of capture streams.
const (
	AudioCapture = "Stream/Input/Audio"
	VideoCapture = "Stream/Input/Video"
)

// Stream is one capture stream.
type Stream struct {
	ID      int
	Class   string // AudioCapture or VideoCapture
	State   string // running, idle, suspended, …
	App     string // application name for notifications
	Binary  string // process binary, for the ignore list
	Monitor bool   // records an output (screen recording, visualiser), not a microphone
	Meter   bool   // level meter (pavucontrol, the Plasma volume applet)
}

type object struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Info *struct {
		State      string         `json:"state"`
		Props      map[string]any `json:"props"`
		OutputNode int            `json:"output-node-id"`
		InputNode  int            `json:"input-node-id"`
	} `json:"info"`
}

// Parse reads a pw-dump JSON document and returns all capture streams.
func Parse(r io.Reader) ([]Stream, error) {
	var objs []object
	if err := json.NewDecoder(r).Decode(&objs); err != nil {
		return nil, fmt.Errorf("pw-dump: %w", err)
	}
	class := map[int]string{}
	var links [][2]int // output node → input node
	for _, o := range objs {
		if o.Info == nil {
			continue
		}
		switch o.Type {
		case "PipeWire:Interface:Node":
			class[o.ID] = str(o.Info.Props["media.class"])
		case "PipeWire:Interface:Link":
			links = append(links, [2]int{o.Info.OutputNode, o.Info.InputNode})
		}
	}
	var out []Stream
	for _, o := range objs {
		if o.Type != "PipeWire:Interface:Node" || o.Info == nil {
			continue
		}
		p := o.Info.Props
		c := str(p["media.class"])
		if c != AudioCapture && c != VideoCapture {
			continue
		}
		s := Stream{
			ID: o.ID, Class: c, State: o.Info.State,
			App:    first(str(p["application.name"]), str(p["application.process.binary"]), str(p["node.name"])),
			Binary: str(p["application.process.binary"]),
			Meter:  truthy(p["stream.monitor"]) || strings.EqualFold(str(p["media.name"]), "Peak detect"),
		}
		s.Monitor = truthy(p["stream.capture.sink"])
		for _, l := range links {
			if l[1] == o.ID && class[l[0]] == "Audio/Sink" {
				s.Monitor = true // linked to a sink's monitor ports
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// Active reports whether a stream of class is capturing, and which apps.
// Monitors, level meters and ignored apps don't count.
func Active(streams []Stream, class string, ignore []string) (bool, []string) {
	var apps []string
	for _, s := range streams {
		if s.Class != class || s.State != "running" || s.Monitor || s.Meter || ignored(s, ignore) {
			continue
		}
		if !slices.Contains(apps, s.App) {
			apps = append(apps, s.App)
		}
	}
	return len(apps) > 0, apps
}

func ignored(s Stream, ignore []string) bool {
	for _, i := range ignore {
		if strings.EqualFold(i, s.App) || strings.EqualFold(i, s.Binary) {
			return true
		}
	}
	return false
}

// Dump runs pw-dump.
func Dump(ctx context.Context) ([]Stream, error) {
	out, err := exec.CommandContext(ctx, "pw-dump").Output()
	if err != nil {
		return nil, fmt.Errorf("pw-dump: %w", err)
	}
	return Parse(strings.NewReader(string(out)))
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// truthy accepts the JSON booleans and the "true" strings pw-dump writes.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	}
	return false
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return "unknown"
}
