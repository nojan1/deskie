package hass

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// fakeHA speaks just enough of the WebSocket API for the client.
func fakeHA(t *testing.T, calls chan<- map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/websocket" {
			http.NotFound(w, r)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		send := func(v any) { wsjson.Write(ctx, c, v) }

		send(map[string]any{"type": "auth_required"})
		var m map[string]any
		if wsjson.Read(ctx, c, &m) != nil || m["access_token"] != "secret" {
			send(map[string]any{"type": "auth_invalid", "message": "bad token"})
			return
		}
		send(map[string]any{"type": "auth_ok"})

		subs := map[string]float64{}
		for {
			var req map[string]any
			if wsjson.Read(ctx, c, &req) != nil {
				return
			}
			id := req["id"]
			switch req["type"] {
			case "subscribe_events":
				subs[req["event_type"].(string)] = id.(float64)
				send(map[string]any{"id": id, "type": "result", "success": true})
			case "get_states":
				send(map[string]any{"id": id, "type": "result", "success": true, "result": []any{
					map[string]any{"entity_id": "light.desk", "state": "off", "attributes": map[string]any{"friendly_name": "Desk"}},
					map[string]any{"entity_id": "sensor.noise", "state": "1"},
				}})
				// Push a change and a custom event once the client is set up.
				send(map[string]any{"id": subs["state_changed"], "type": "event", "event": map[string]any{
					"event_type": "state_changed",
					"data":       map[string]any{"entity_id": "light.desk", "new_state": map[string]any{"entity_id": "light.desk", "state": "on"}},
				}})
				send(map[string]any{"id": subs["deskie_notify"], "type": "event", "event": map[string]any{
					"event_type": "deskie_notify", "data": map[string]any{"message": "hi"},
				}})
			case "call_service":
				calls <- req
				send(map[string]any{"id": id, "type": "result", "success": true})
			}
		}
	}))
}

func TestClient(t *testing.T) {
	calls := make(chan map[string]any, 1)
	srv := fakeHA(t, calls)
	defer srv.Close()

	var mu sync.Mutex
	var states []string
	events := make(chan Event, 1)
	up := make(chan bool, 4)
	c := &Client{
		URL: srv.URL, Token: "secret",
		Entities: []string{"light.desk"},
		Events:   []string{"deskie_notify"},
		OnState: func(s State) {
			mu.Lock()
			states = append(states, s.EntityID+"="+s.State)
			mu.Unlock()
		},
		OnEvent:  func(e Event) { events <- e },
		OnStatus: func(ok bool, err error) { up <- ok },
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go c.Run(ctx)

	if !<-up {
		t.Fatal("not connected")
	}
	select {
	case e := <-events:
		var d struct{ Message string }
		json.Unmarshal(e.Data, &d)
		if e.Type != "deskie_notify" || d.Message != "hi" {
			t.Fatalf("event: %+v", e)
		}
	case <-ctx.Done():
		t.Fatal("no event")
	}
	mu.Lock()
	got := append([]string(nil), states...)
	mu.Unlock()
	if len(got) != 2 || got[0] != "light.desk=off" || got[1] != "light.desk=on" {
		t.Fatalf("states: %v (sensor should be filtered out)", got)
	}

	if err := c.CallService(ctx, "light", "toggle", "light.desk", nil); err != nil {
		t.Fatal(err)
	}
	req := <-calls
	if req["domain"] != "light" || req["target"].(map[string]any)["entity_id"] != "light.desk" {
		t.Fatalf("call: %v", req)
	}
}

func TestBadToken(t *testing.T) {
	srv := fakeHA(t, nil)
	defer srv.Close()
	errs := make(chan error, 1)
	c := &Client{URL: srv.URL, Token: "nope", OnStatus: func(_ bool, err error) {
		select {
		case errs <- err:
		default:
		}
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go c.Run(ctx)
	if err := <-errs; err == nil || ctx.Err() != nil {
		t.Fatalf("want auth error, got %v", err)
	}
	if err := c.CallService(ctx, "a", "b", "", nil); err != ErrNotConnected {
		t.Fatalf("want ErrNotConnected, got %v", err)
	}
}

func TestWebSocketURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://ha.local:8123":    "ws://ha.local:8123/api/websocket",
		"https://ha.example.com/": "wss://ha.example.com/api/websocket",
		"ws://x/api/websocket":    "ws://x/api/websocket",
		"https://proxy/ha/":       "wss://proxy/ha/api/websocket",
	} {
		if got, err := WebSocketURL(in); err != nil || got != want {
			t.Errorf("%s: got %s (%v), want %s", in, got, err, want)
		}
	}
}
