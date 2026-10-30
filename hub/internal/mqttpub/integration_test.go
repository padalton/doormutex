package mqttpub

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/padalton/doormutex/hub/internal/engine"
	"github.com/padalton/doormutex/hub/internal/service"
	"github.com/padalton/doormutex/hub/internal/status"
)

// startMosquitto runs a throwaway broker in Docker on a free local port.
// It never touches a real broker: the container is bound to 127.0.0.1
// and removed afterwards. Set DOORMUTEX_INTEGRATION=1 to run it.
func startMosquitto(t *testing.T) string {
	t.Helper()
	if os.Getenv("DOORMUTEX_INTEGRATION") == "" {
		t.Skip("set DOORMUTEX_INTEGRATION=1 to run the MQTT integration test (needs Docker)")
	}
	out, err := exec.Command("docker", "run", "-d", "--rm", "-p", "127.0.0.1::1883",
		"eclipse-mosquitto:2", "mosquitto", "-c", "/mosquitto-no-auth.conf").CombinedOutput()
	if err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", id).Run() })
	out, err = exec.Command("docker", "port", id, "1883/tcp").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	addr := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	deadline := time.Now().Add(20 * time.Second)
	for {
		c := mqtt.NewClient(mqtt.NewClientOptions().AddBroker("tcp://" + addr).SetClientID("probe"))
		if tok := c.Connect(); tok.WaitTimeout(time.Second) && tok.Error() == nil {
			c.Disconnect(0)
			return addr
		}
		if time.Now().After(deadline) {
			t.Fatalf("broker at %s did not come up", addr)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// proxy forwards TCP to the broker and can cut all connections hard, which
// looks like a crashed hub to the broker and triggers the Last Will.
type proxy struct {
	ln    net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func newProxy(t *testing.T, target string) *proxy {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &proxy{ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			u, err := net.Dial("tcp", target)
			if err != nil {
				_ = c.Close()
				continue
			}
			p.mu.Lock()
			p.conns = append(p.conns, c, u)
			p.mu.Unlock()
			go func() { _, _ = io.Copy(u, c); _ = u.Close() }()
			go func() { _, _ = io.Copy(c, u); _ = c.Close() }()
		}
	}()
	return p
}

func (p *proxy) cut() {
	_ = p.ln.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		_ = c.Close()
	}
}

type collector struct {
	mu   sync.Mutex
	msgs map[string][][]byte
}

func (c *collector) handler(_ mqtt.Client, m mqtt.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs[m.Topic()] = append(c.msgs[m.Topic()], m.Payload())
}

func (c *collector) wait(t *testing.T, topic string, ok func([]byte) bool) []byte {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		for _, m := range c.msgs[topic] {
			if ok(m) {
				c.mu.Unlock()
				return m
			}
		}
		c.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t.Fatalf("no matching message on %s; got %q", topic, c.msgs[topic])
	return nil
}

func TestIntegrationMosquitto(t *testing.T) {
	broker := startMosquitto(t)
	px := newProxy(t, broker)

	col := &collector{msgs: map[string][][]byte{}}
	sub := mqtt.NewClient(mqtt.NewClientOptions().AddBroker("tcp://" + broker).SetClientID("observer"))
	if tok := sub.Connect(); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatalf("observer connect: %v", tok.Error())
	}
	defer sub.Disconnect(0)
	if tok := sub.Subscribe("#", 1, col.handler); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatalf("subscribe: %v", tok.Error())
	}

	cfg := Config{
		URL: "tcp://" + px.ln.Addr().String(), ClientID: "doormutex-hub-test",
		BaseTopic: "doormutex", DiscoveryPrefix: "homeassistant", Discovery: true, Version: "test",
	}
	ctl := &fakeCtl{}
	client, err := Connect(cfg, []Profile{{ID: "office", Name: "Office", Ctl: ctl}, {ID: "teen", Name: "Teen room"}},
		slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	pub := client.Publisher("office")

	until := time.Date(2026, 10, 28, 14, 30, 0, 0, time.UTC)
	pub.PublishState(engine.State{Status: status.DND, Phase: status.PhaseNormal, Light: status.LightRed,
		Reason: engine.ReasonCalendar, Since: until.Add(-30 * time.Minute), Until: &until, Updated: until.Add(-5 * time.Minute)})
	pub.PublishChange(service.Change{From: status.Working, To: status.DND, Reason: engine.ReasonCalendar, At: until.Add(-30 * time.Minute)})

	col.wait(t, "doormutex/hub/online", func(b []byte) bool { return string(b) == "online" })
	col.wait(t, "homeassistant/device/doormutex_office/config", func(b []byte) bool { return json.Valid(b) })
	col.wait(t, "homeassistant/device/doormutex_teen/config", func(b []byte) bool { return json.Valid(b) })
	col.wait(t, "doormutex/office/state", func(b []byte) bool {
		var st engine.State
		return json.Unmarshal(b, &st) == nil && st.Status == status.DND && st.Until != nil
	})
	col.wait(t, "doormutex/office/event", func(b []byte) bool {
		var ev ChangeEvent
		return json.Unmarshal(b, &ev) == nil && ev.EventType == "status_changed" && ev.To == status.DND
	})

	// The state must be retained: a late subscriber gets it immediately.
	late := &collector{msgs: map[string][][]byte{}}
	lc := mqtt.NewClient(mqtt.NewClientOptions().AddBroker("tcp://" + broker).SetClientID("late"))
	if tok := lc.Connect(); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatal(tok.Error())
	}
	defer lc.Disconnect(0)
	lc.Subscribe("doormutex/office/state", 1, late.handler).WaitTimeout(5 * time.Second)
	late.wait(t, "doormutex/office/state", func(b []byte) bool { return strings.Contains(string(b), `"status":"dnd"`) })

	// Commands from HA (select / text entities) reach the profile.
	sub.Publish("doormutex/office/override/set", 1, false, "working").WaitTimeout(5 * time.Second)
	sub.Publish("doormutex/office/message/set", 1, false, "Back at 3").WaitTimeout(5 * time.Second)
	deadline := time.Now().Add(10 * time.Second)
	for {
		o, m := ctl.get()
		if o == "working/indefinite" && m != nil && *m == "Back at 3" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("commands not applied: override %q, message %v", o, m)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Crash: cut the connection without DISCONNECT → broker sends the Last Will.
	px.cut()
	col.wait(t, "doormutex/hub/online", func(b []byte) bool { return string(b) == "offline" })
	client.client.Disconnect(0)
	fmt.Println("integration: online, discovery, retained state, event, HA commands and Last Will verified against", broker)
}
