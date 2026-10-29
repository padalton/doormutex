// Package desktop wraps the D-Bus services the companion uses: logind for
// the screen lock and for sleep/shutdown, and the freedesktop
// notification service.
package desktop

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	login1      = "org.freedesktop.login1"
	login1Path  = dbus.ObjectPath("/org/freedesktop/login1")
	managerIfc  = "org.freedesktop.login1.Manager"
	sessionIfc  = "org.freedesktop.login1.Session"
	userIfc     = "org.freedesktop.login1.User"
	notifyName  = "org.freedesktop.Notifications"
	notifyPath  = dbus.ObjectPath("/org/freedesktop/Notifications")
	notifyIfc   = "org.freedesktop.Notifications"
	inhibitWhat = "sleep:shutdown"
)

// Logind reads the lock state and announces sleep and shutdown.
type Logind struct {
	conn    *dbus.Conn
	session dbus.BusObject
	inhibit *os.File
}

// NewLogind connects to the system bus and finds the user's graphical
// session. A systemd user service has no session of its own, so the
// user's "Display" session is used.
func NewLogind() (*Logind, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("system bus: %w", err)
	}
	l := &Logind{conn: conn}
	path, err := l.displaySession()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	l.session = conn.Object(login1, path)
	return l, nil
}

func (l *Logind) displaySession() (dbus.ObjectPath, error) {
	user := l.conn.Object(login1, "/org/freedesktop/login1/user/self")
	v, err := user.GetProperty(userIfc + ".Display")
	if err == nil {
		if d, ok := v.Value().([]any); ok && len(d) == 2 {
			if p, ok := d[1].(dbus.ObjectPath); ok && p != "/" {
				return p, nil
			}
		}
	}
	if id := os.Getenv("XDG_SESSION_ID"); id != "" {
		var p dbus.ObjectPath
		if err := l.conn.Object(login1, login1Path).Call(managerIfc+".GetSession", 0, id).Store(&p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("logind: no graphical session found for this user")
}

// Locked reports logind's LockedHint of the session (set by the screen locker).
func (l *Logind) Locked() (bool, error) {
	v, err := l.session.GetProperty(sessionIfc + ".LockedHint")
	if err != nil {
		return false, err
	}
	b, _ := v.Value().(bool)
	return b, nil
}

// Power events from logind.
type Power int

const (
	Suspend Power = iota + 1
	Resume
	Shutdown
)

// Watch delivers sleep/shutdown events. Before suspend and shutdown the
// companion holds a delay inhibitor, so it can sign off at the hub; call
// Release once the goodbye is sent.
func (l *Logind) Watch() (<-chan Power, error) {
	if err := l.conn.AddMatchSignal(dbus.WithMatchInterface(managerIfc), dbus.WithMatchObjectPath(login1Path)); err != nil {
		return nil, err
	}
	sig := make(chan *dbus.Signal, 8)
	l.conn.Signal(sig)
	if err := l.Inhibit(); err != nil {
		return nil, err
	}
	out := make(chan Power, 4)
	go func() {
		defer close(out)
		for s := range sig {
			if len(s.Body) == 0 {
				continue
			}
			start, _ := s.Body[0].(bool)
			switch s.Name {
			case managerIfc + ".PrepareForSleep":
				if start {
					out <- Suspend
				} else {
					out <- Resume
				}
			case managerIfc + ".PrepareForShutdown":
				if start {
					out <- Shutdown
				}
			}
		}
	}()
	return out, nil
}

// Inhibit takes a delay lock for sleep and shutdown (again after resume).
func (l *Logind) Inhibit() error {
	if l.inhibit != nil {
		return nil
	}
	var fd dbus.UnixFD
	err := l.conn.Object(login1, login1Path).Call(managerIfc+".Inhibit", 0,
		inhibitWhat, "doormutex", "Sign off at the doormutex hub", "delay").Store(&fd)
	if err != nil {
		return fmt.Errorf("logind inhibit: %w", err)
	}
	l.inhibit = os.NewFile(uintptr(fd), "doormutex-inhibit")
	return nil
}

// Release lets a pending suspend or shutdown continue.
func (l *Logind) Release() {
	if l.inhibit != nil {
		_ = l.inhibit.Close()
		l.inhibit = nil
	}
}

// Close disconnects.
func (l *Logind) Close() {
	l.Release()
	_ = l.conn.Close()
}

// Notifier shows desktop notifications.
type Notifier struct {
	conn *dbus.Conn

	mu      sync.Mutex
	last    uint32
	pending map[uint32]func(string) // notification id → action callback
}

// NewNotifier connects to the session bus.
func NewNotifier() (*Notifier, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	n := &Notifier{conn: conn, pending: map[uint32]func(string){}}
	if err := conn.AddMatchSignal(dbus.WithMatchInterface(notifyIfc), dbus.WithMatchObjectPath(notifyPath)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	sig := make(chan *dbus.Signal, 8)
	conn.Signal(sig)
	go n.dispatch(sig)
	return n, nil
}

func (n *Notifier) dispatch(sig <-chan *dbus.Signal) {
	for s := range sig {
		if len(s.Body) < 2 {
			continue
		}
		id, _ := s.Body[0].(uint32)
		n.mu.Lock()
		fn := n.pending[id]
		delete(n.pending, id) // invoked or closed: either way it's done
		n.mu.Unlock()
		if key, ok := s.Body[1].(string); ok && fn != nil && s.Name == notifyIfc+".ActionInvoked" {
			go fn(key)
		}
	}
}

// Notify replaces the previous doormutex notification. actions maps an
// action key to its label; onAction is called with the key if clicked.
func (n *Notifier) Notify(summary, body string, actions map[string]string, onAction func(string)) error {
	var list []string
	for k, v := range actions {
		list = append(list, k, v)
	}
	hints := map[string]dbus.Variant{"desktop-entry": dbus.MakeVariant("doormutex-companion")}
	n.mu.Lock()
	replaces := n.last
	n.mu.Unlock()
	var id uint32
	err := n.conn.Object(notifyName, notifyPath).Call(notifyIfc+".Notify", 0,
		"doormutex", replaces, "traffic-light", summary, body, list, hints, int32(8000)).Store(&id)
	if err != nil {
		return err
	}
	n.mu.Lock()
	n.last = id
	if onAction != nil && len(actions) > 0 {
		n.pending[id] = onAction
	}
	n.mu.Unlock()
	return nil
}

// Close disconnects.
func (n *Notifier) Close() { _ = n.conn.Close() }
