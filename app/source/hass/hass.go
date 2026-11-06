// Package hass is a small Home Assistant WebSocket API client: it mirrors
// entity states, forwards selected bus events and calls services. It
// reconnects on its own.
//
// Callbacks run on the client's goroutines, not the UI goroutine; hand the
// data over (see main's post) before touching widgets.
package hass

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// State is one entity's state as Home Assistant reports it.
type State struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"last_changed"`
}

// Name is the entity's friendly_name, falling back to its id.
func (s State) Name() string {
	if n, ok := s.Attributes["friendly_name"].(string); ok && n != "" {
		return n
	}
	return s.EntityID
}

// Event is a Home Assistant bus event (other than state_changed, which is
// delivered as a State).
type Event struct {
	Type      string          `json:"event_type"`
	Data      json.RawMessage `json:"data"`
	TimeFired time.Time       `json:"time_fired"`
}

type Client struct {
	// URL is the Home Assistant base URL, e.g. http://homeassistant.local:8123.
	URL   string
	Token string // long-lived access token
	// Entities limits OnState to these entity ids; empty means all entities.
	Entities []string
	// Events lists extra event types to subscribe to (e.g. a custom
	// "deskie_notify" fired from automations).
	Events []string

	OnState  func(State)                     // initial states, then every change
	OnEvent  func(Event)                     // events of the types in Events
	OnStatus func(connected bool, err error) // connection up/down

	mu      sync.Mutex
	conn    *websocket.Conn
	done    chan struct{} // closed when the current session ends
	nextID  int
	pending map[int]pendingCall
	watch   map[string]bool
}

type message struct {
	ID      int             `json:"id"`
	Type    string          `json:"type"`
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Event   json.RawMessage `json:"event"`
	Error   *errorInfo      `json:"error"`
	Message string          `json:"message"` // auth_invalid reason
}

type errorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrNotConnected is returned by calls made while the connection is down.
var ErrNotConnected = errors.New("hass: not connected")

// Run connects and keeps the connection alive until ctx is cancelled.
func (c *Client) Run(ctx context.Context) {
	c.watch = map[string]bool{}
	for _, e := range c.Entities {
		c.watch[e] = true
	}
	backoff := time.Second
	for {
		start := time.Now()
		err := c.session(ctx)
		if ctx.Err() != nil {
			return
		}
		c.status(false, err)
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

func (c *Client) status(up bool, err error) {
	if c.OnStatus != nil {
		c.OnStatus(up, err)
	}
}

// WebSocketURL turns a Home Assistant base URL into its WebSocket API URL.
func WebSocketURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("hass: unsupported URL scheme %q", u.Scheme)
	}
	if !strings.HasSuffix(u.Path, "/api/websocket") {
		u.Path = strings.TrimSuffix(u.Path, "/") + "/api/websocket"
	}
	return u.String(), nil
}

func (c *Client) session(ctx context.Context) error {
	wsURL, err := WebSocketURL(c.URL)
	if err != nil {
		return err
	}
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	conn, _, err := websocket.Dial(dctx, wsURL, nil)
	cancel()
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(32 << 20) // get_states can be large

	if err := c.auth(ctx, conn); err != nil {
		return err
	}

	done := make(chan struct{})
	c.mu.Lock()
	c.conn, c.done, c.nextID, c.pending = conn, done, 0, map[int]pendingCall{}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.conn, c.pending = nil, nil
		c.mu.Unlock()
		close(done)
	}()

	// Requests need the read loop running to see their replies.
	setupErr := make(chan error, 1)
	go func() {
		err := c.setup(ctx)
		setupErr <- err
		if err != nil {
			conn.CloseNow()
		} else {
			c.status(true, nil)
		}
	}()

	readErr := c.readLoop(ctx, conn)
	select {
	case err := <-setupErr:
		if err != nil {
			return err
		}
	default:
	}
	return readErr
}

func (c *Client) auth(ctx context.Context, conn *websocket.Conn) error {
	actx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var m message
	if err := wsjson.Read(actx, conn, &m); err != nil {
		return err
	}
	if m.Type != "auth_required" {
		return fmt.Errorf("hass: expected auth_required, got %q", m.Type)
	}
	if err := wsjson.Write(actx, conn, map[string]string{"type": "auth", "access_token": c.Token}); err != nil {
		return err
	}
	if err := wsjson.Read(actx, conn, &m); err != nil {
		return err
	}
	switch m.Type {
	case "auth_ok":
		return nil
	case "auth_invalid":
		return fmt.Errorf("hass: authentication failed: %s", m.Message)
	default:
		return fmt.Errorf("hass: unexpected auth reply %q", m.Type)
	}
}

// setup subscribes first, then fetches states, so no change can slip
// between the snapshot and the subscription. The snapshot is applied on the
// read loop (inline), so it is ordered correctly against state_changed
// events that arrive after it.
func (c *Client) setup(ctx context.Context) error {
	for _, t := range append([]string{"state_changed"}, c.Events...) {
		if _, err := c.call(ctx, map[string]any{"type": "subscribe_events", "event_type": t}, nil); err != nil {
			return fmt.Errorf("subscribe %s: %w", t, err)
		}
	}
	_, err := c.call(ctx, map[string]any{"type": "get_states"}, func(res json.RawMessage) error {
		var states []State
		if err := json.Unmarshal(res, &states); err != nil {
			return err
		}
		for _, s := range states {
			c.state(s)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("get_states: %w", err)
	}
	return nil
}

// pendingCall is a request waiting for its result. inline, if set, runs on
// the read loop before any later message is processed.
type pendingCall struct {
	ch     chan message
	inline func(json.RawMessage) error
}

func (c *Client) state(s State) {
	if c.OnState != nil && (len(c.watch) == 0 || c.watch[s.EntityID]) {
		c.OnState(s)
	}
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		var m message
		if err := wsjson.Read(ctx, conn, &m); err != nil {
			return err
		}
		switch m.Type {
		case "result":
			c.mu.Lock()
			p, ok := c.pending[m.ID]
			delete(c.pending, m.ID)
			c.mu.Unlock()
			if !ok {
				continue
			}
			if p.inline != nil && m.Success {
				if err := p.inline(m.Result); err != nil {
					m.Success, m.Error = false, &errorInfo{Code: "client", Message: err.Error()}
				}
			}
			p.ch <- m
		case "event":
			c.event(m.Event)
		}
	}
}

func (c *Client) event(raw json.RawMessage) {
	var e Event
	if err := json.Unmarshal(raw, &e); err != nil {
		return
	}
	if e.Type != "state_changed" {
		if c.OnEvent != nil {
			c.OnEvent(e)
		}
		return
	}
	var d struct {
		NewState *State `json:"new_state"`
	}
	if json.Unmarshal(e.Data, &d) == nil && d.NewState != nil {
		c.state(*d.NewState)
	}
}

// call sends one request and waits for its result. See pendingCall for inline.
func (c *Client) call(ctx context.Context, req map[string]any, inline func(json.RawMessage) error) (json.RawMessage, error) {
	ch := make(chan message, 1)
	c.mu.Lock()
	conn, done := c.conn, c.done
	if conn == nil {
		c.mu.Unlock()
		return nil, ErrNotConnected
	}
	c.nextID++
	id := c.nextID
	c.pending[id] = pendingCall{ch: ch, inline: inline}
	c.mu.Unlock()

	req["id"] = id
	if err := wsjson.Write(ctx, conn, req); err != nil {
		return nil, err
	}
	select {
	case m := <-ch:
		if !m.Success {
			if m.Error != nil {
				return nil, fmt.Errorf("hass: %s: %s", m.Error.Code, m.Error.Message)
			}
			return nil, errors.New("hass: request failed")
		}
		return m.Result, nil
	case <-done:
		return nil, ErrNotConnected
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CallService calls e.g. ("light", "toggle", "light.desk", nil).
// entityID may be empty for services without a target.
func (c *Client) CallService(ctx context.Context, domain, service, entityID string, data map[string]any) error {
	req := map[string]any{"type": "call_service", "domain": domain, "service": service}
	if entityID != "" {
		req["target"] = map[string]any{"entity_id": entityID}
	}
	if data != nil {
		req["service_data"] = data
	}
	_, err := c.call(ctx, req, nil)
	return err
}
