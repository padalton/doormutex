// Package mqttpub publishes the hub state to MQTT and announces the
// entities to Home Assistant via MQTT discovery.
//
// Topics (base = "office-traffic-light" by default):
//
//	<base>/state       retained JSON state, the same document as GET /api/v1/status
//	<base>/event       status_changed events (not retained), for HA event entities
//	<base>/hub/online  "online"/"offline", retained; "offline" is the Last Will
//	<base>/display     reserved for the rendered E-Ink bitmap (not published yet)
package mqttpub

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/padalton/office-traffic-light/hub/internal/engine"
	"github.com/padalton/office-traffic-light/hub/internal/service"
	"github.com/padalton/office-traffic-light/hub/internal/status"
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

// Topics derived from the base topic.
type Topics struct {
	State, Event, Online, Display string
}

// TopicsFor returns the topic names for a base topic.
func TopicsFor(base string) Topics {
	return Topics{
		State:   base + "/state",
		Event:   base + "/event",
		Online:  base + "/hub/online",
		Display: base + "/display",
	}
}

// Publisher implements service.Publisher.
type Publisher struct {
	cfg    Config
	topics Topics
	client mqtt.Client
	log    *slog.Logger

	mu   sync.Mutex
	last []byte // last state payload, re-sent on reconnect
}

const qos = 1

// Connect connects to the broker. It returns once the first connection
// attempt succeeded; afterwards the client reconnects on its own.
func Connect(cfg Config, log *slog.Logger) (*Publisher, error) {
	if log == nil {
		log = slog.Default()
	}
	p := &Publisher{cfg: cfg, topics: TopicsFor(cfg.BaseTopic), log: log}
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
		SetWill(p.topics.Online, "offline", qos, true).
		SetOnConnectHandler(func(mqtt.Client) { p.onConnect() }).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) { log.Warn("mqtt connection lost", "err", err) })
	p.client = mqtt.NewClient(o)
	tok := p.client.Connect()
	if !tok.WaitTimeout(15 * time.Second) {
		return p, errors.New("mqtt: connect timeout (keeps retrying in the background)")
	}
	if err := tok.Error(); err != nil {
		return nil, fmt.Errorf("mqtt: %w", err)
	}
	return p, nil
}

func (p *Publisher) onConnect() {
	p.log.Info("mqtt connected", "broker", p.cfg.URL)
	p.publish(p.topics.Online, true, []byte("online"))
	if p.cfg.Discovery {
		topic, payload := Discovery(p.cfg)
		p.publish(topic, true, payload)
	}
	p.mu.Lock()
	last := p.last
	p.mu.Unlock()
	if last != nil {
		p.publish(p.topics.State, true, last)
	}
}

func (p *Publisher) publish(topic string, retained bool, payload []byte) {
	tok := p.client.Publish(topic, qos, retained, payload)
	go func() {
		if tok.WaitTimeout(10*time.Second) && tok.Error() != nil {
			p.log.Warn("mqtt publish failed", "topic", topic, "err", tok.Error())
		}
	}()
}

// PublishState implements service.Publisher.
func (p *Publisher) PublishState(st engine.State) {
	b, err := json.Marshal(st)
	if err != nil {
		p.log.Error("marshal state", "err", err)
		return
	}
	p.mu.Lock()
	p.last = b
	p.mu.Unlock()
	if p.client.IsConnectionOpen() {
		p.publish(p.topics.State, true, b)
	}
}

// ChangeEvent is the payload on <base>/event; event_type is what HA's
// MQTT event entity expects.
type ChangeEvent struct {
	EventType string        `json:"event_type"`
	From      status.Status `json:"from"`
	To        status.Status `json:"to"`
	Reason    string        `json:"reason"`
	Detail    string        `json:"detail,omitempty"`
	At        time.Time     `json:"at"`
}

// PublishChange implements service.Publisher.
func (p *Publisher) PublishChange(c service.Change) {
	b, _ := json.Marshal(ChangeEvent{EventType: "status_changed", From: c.From, To: c.To, Reason: c.Reason, Detail: c.Detail, At: c.At})
	if p.client.IsConnectionOpen() {
		p.publish(p.topics.Event, false, b)
	}
}

// Close announces "offline" and disconnects cleanly.
func (p *Publisher) Close() {
	if p.client.IsConnectionOpen() {
		p.client.Publish(p.topics.Online, qos, true, "offline").WaitTimeout(2 * time.Second)
	}
	p.client.Disconnect(500)
}
