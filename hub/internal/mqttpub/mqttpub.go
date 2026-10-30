// Package mqttpub publishes the hub state to MQTT, announces the entities
// to Home Assistant via MQTT discovery and accepts commands from HA.
//
// Topics (base = "doormutex" by default, one set per profile):
//
//	<base>/hub/online            "online"/"offline", retained; "offline" is the Last Will
//	<base>/<profile>/state       retained JSON state, the same document as GET /api/v1/status
//	<base>/<profile>/event       status_changed events (not retained), for HA event entities
//	<base>/<profile>/override/set  command: auto | free | working | dnd | away
//	<base>/<profile>/message/set   command: e-ink message (empty clears it)
//	<base>/<profile>/display     reserved for the rendered e-ink bitmap (not published yet)
package mqttpub

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/padalton/doormutex/hub/internal/engine"
	"github.com/padalton/doormutex/hub/internal/service"
	"github.com/padalton/doormutex/hub/internal/status"
)

// Config for the MQTT connection.
type Config struct {
	URL             string // tcp://host:1883, ssl://host:8883, ws://…
	Username        string
	Password        string
	ClientID        string
	BaseTopic       string
	DiscoveryPrefix string
	Discovery       bool
	Version         string
}

// Controller is what HA commands act on; *service.Service implements it.
type Controller interface {
	SetOverride(service.OverrideRequest) (engine.State, error)
	ClearOverride() engine.State
	SetMessage(text string, ttl time.Duration) (engine.State, error)
	RemoteOverrideMode() string // mode of overrides set from HA
}

// Profile is one person and light published to MQTT.
type Profile struct {
	ID, Name string
	Ctl      Controller // nil: no HA control entities
}

// Topics of one profile.
type Topics struct {
	State, Event, OverrideSet, MessageSet, Display string
}

// TopicsFor returns the topic names of a profile.
func TopicsFor(base, profile string) Topics {
	p := base + "/" + profile
	return Topics{
		State:       p + "/state",
		Event:       p + "/event",
		OverrideSet: p + "/override/set",
		MessageSet:  p + "/message/set",
		Display:     p + "/display",
	}
}

// OnlineTopic is the hub's availability topic.
func OnlineTopic(base string) string { return base + "/hub/online" }

// Client is the MQTT connection shared by all profiles.
type Client struct {
	cfg      Config
	client   mqtt.Client
	log      *slog.Logger
	profiles map[string]*Publisher
	order    []string
}

// Publisher implements service.Publisher for one profile.
type Publisher struct {
	c       *Client
	profile Profile
	topics  Topics

	mu   sync.Mutex
	last []byte // last state payload, re-sent on reconnect
}

const qos = 1

// Connect connects to the broker. It returns once the first connection
// attempt succeeded; afterwards the client reconnects on its own.
func Connect(cfg Config, profiles []Profile, log *slog.Logger) (*Client, error) {
	if log == nil {
		log = slog.Default()
	}
	c := &Client{cfg: cfg, log: log, profiles: map[string]*Publisher{}}
	for _, p := range profiles {
		c.profiles[p.ID] = &Publisher{c: c, profile: p, topics: TopicsFor(cfg.BaseTopic, p.ID)}
		c.order = append(c.order, p.ID)
	}
	o := mqtt.NewClientOptions().
		AddBroker(cfg.URL).
		SetClientID(cfg.ClientID).
		SetUsername(cfg.Username).
		SetPassword(cfg.Password).
		SetCleanSession(true).
		SetKeepAlive(30*time.Second).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetMaxReconnectInterval(time.Minute).
		SetWill(OnlineTopic(cfg.BaseTopic), "offline", qos, true).
		SetOnConnectHandler(func(mqtt.Client) { c.onConnect() }).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) { log.Warn("mqtt connection lost", "err", err) })
	c.client = mqtt.NewClient(o)
	tok := c.client.Connect()
	if !tok.WaitTimeout(15 * time.Second) {
		return c, errors.New("mqtt: connect timeout (keeps retrying in the background)")
	}
	if err := tok.Error(); err != nil {
		return nil, fmt.Errorf("mqtt: %w", err)
	}
	return c, nil
}

// Publisher returns the publisher of a profile (nil if unknown).
func (c *Client) Publisher(profile string) *Publisher { return c.profiles[profile] }

func (c *Client) onConnect() {
	c.log.Info("mqtt connected", "broker", c.cfg.URL)
	c.publish(OnlineTopic(c.cfg.BaseTopic), true, []byte("online"))
	for _, id := range c.order {
		p := c.profiles[id]
		if c.cfg.Discovery {
			topic, payload := Discovery(c.cfg, p.profile)
			c.publish(topic, true, payload)
		}
		p.mu.Lock()
		last := p.last
		p.mu.Unlock()
		if last != nil {
			c.publish(p.topics.State, true, last)
		}
	}
	filters := map[string]byte{
		c.cfg.BaseTopic + "/+/override/set": qos,
		c.cfg.BaseTopic + "/+/message/set":  qos,
	}
	tok := c.client.SubscribeMultiple(filters, c.onCommand)
	go func() {
		if tok.WaitTimeout(10*time.Second) && tok.Error() != nil {
			c.log.Warn("mqtt subscribe failed", "err", tok.Error())
		}
	}()
}

// onCommand handles <base>/<profile>/override/set and …/message/set.
func (c *Client) onCommand(_ mqtt.Client, m mqtt.Message) {
	rest, ok := strings.CutPrefix(m.Topic(), c.cfg.BaseTopic+"/")
	if !ok {
		return
	}
	id, cmd, _ := strings.Cut(rest, "/")
	p := c.profiles[id]
	if p == nil || p.profile.Ctl == nil {
		c.log.Warn("mqtt command for unknown profile", "topic", m.Topic())
		return
	}
	payload := strings.TrimSpace(string(m.Payload()))
	var err error
	switch cmd {
	case "override/set":
		err = applyOverride(p.profile.Ctl, payload)
	case "message/set":
		_, err = p.profile.Ctl.SetMessage(payload, 0)
	default:
		return
	}
	if err != nil {
		c.log.Warn("mqtt command rejected", "topic", m.Topic(), "payload", payload, "err", err)
		return
	}
	c.log.Info("mqtt command", "profile", id, "command", cmd, "payload", payload)
}

// applyOverride maps the HA select: "auto" clears the override, a status
// sets it – until the next automatic change with a desktop companion,
// until changed again without one.
func applyOverride(ctl Controller, payload string) error {
	if payload == "auto" {
		ctl.ClearOverride()
		return nil
	}
	_, err := ctl.SetOverride(service.OverrideRequest{Status: status.Status(payload), Mode: ctl.RemoteOverrideMode()})
	return err
}

func (c *Client) publish(topic string, retained bool, payload []byte) {
	tok := c.client.Publish(topic, qos, retained, payload)
	go func() {
		if tok.WaitTimeout(10*time.Second) && tok.Error() != nil {
			c.log.Warn("mqtt publish failed", "topic", topic, "err", tok.Error())
		}
	}()
}

// PublishState implements service.Publisher.
func (p *Publisher) PublishState(st engine.State) {
	b, err := json.Marshal(st)
	if err != nil {
		p.c.log.Error("marshal state", "err", err)
		return
	}
	p.mu.Lock()
	p.last = b
	p.mu.Unlock()
	if p.c.client.IsConnectionOpen() {
		p.c.publish(p.topics.State, true, b)
	}
}

// ChangeEvent is the payload on <base>/<profile>/event; event_type is what
// HA's MQTT event entity expects.
type ChangeEvent struct {
	EventType string        `json:"event_type"`
	From      status.Status `json:"from"`
	To        status.Status `json:"to"`
	Reason    string        `json:"reason"`
	Detail    string        `json:"detail,omitempty"`
	At        time.Time     `json:"at"`
}

// PublishChange implements service.Publisher.
func (p *Publisher) PublishChange(ch service.Change) {
	b, _ := json.Marshal(ChangeEvent{EventType: "status_changed", From: ch.From, To: ch.To, Reason: ch.Reason, Detail: ch.Detail, At: ch.At})
	if p.c.client.IsConnectionOpen() {
		p.c.publish(p.topics.Event, false, b)
	}
}

// Close announces "offline" and disconnects cleanly.
func (c *Client) Close() {
	if c.client.IsConnectionOpen() {
		c.client.Publish(OnlineTopic(c.cfg.BaseTopic), qos, true, "offline").WaitTimeout(2 * time.Second)
	}
	c.client.Disconnect(500)
}
