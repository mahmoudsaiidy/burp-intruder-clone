package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestDemoAttack_EndToEnd dials NewHandler's /ws over a real httptest
// server and checks the full stream it produces: one "result" message per
// demoWordlist entry, exactly one of them flagged (the "hunter2" hit), and
// a final "done" summary matching those counts.
func TestDemoAttack_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(NewHandler(""))
	defer srv.Close()

	rawConn, br := dialWebSocket(t, srv.URL+"/ws")
	defer rawConn.Close()
	rawConn.SetReadDeadline(time.Now().Add(10 * time.Second))

	var results []wsMessage
	var done *wsMessage

	for {
		fin, op, payload, err := decodeFrame(br)
		if err != nil {
			t.Fatalf("decodeFrame: %v", err)
		}
		if !fin {
			t.Fatalf("got a fragmented frame, not expected from this server")
		}
		if op == opClose {
			break
		}
		if op != opText {
			t.Fatalf("got opcode %#x, want text or close", op)
		}

		var msg wsMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatalf("unmarshalling %q: %v", payload, err)
		}

		switch msg.Type {
		case "result":
			results = append(results, msg)
		case "done":
			m := msg
			done = &m
		case "error":
			t.Fatalf("got an error message: %s", msg.Message)
		default:
			t.Fatalf("unexpected message type %q", msg.Type)
		}
	}

	if len(results) != len(demoWordlist) {
		t.Fatalf("got %d result messages, want %d (one per demoWordlist entry)", len(results), len(demoWordlist))
	}

	hits := 0
	var hitPayload string
	for _, r := range results {
		if r.Hit {
			hits++
			hitPayload = r.Payload
		}
		if r.Error == "" && r.Status == 0 {
			t.Errorf("result for payload %q has neither Status nor Error set", r.Payload)
		}
	}
	if hits != 1 {
		t.Fatalf("got %d flagged results, want exactly 1", hits)
	}
	if hitPayload != "hunter2" {
		t.Fatalf("flagged payload = %q, want %q", hitPayload, "hunter2")
	}

	if done == nil {
		t.Fatal("never got a \"done\" message")
	}
	if done.Total != len(demoWordlist) {
		t.Errorf("done.Total = %d, want %d", done.Total, len(demoWordlist))
	}
	if done.Hits != 1 {
		t.Errorf("done.Hits = %d, want 1", done.Hits)
	}
}

// TestNewHandler_ServesStaticFiles checks the webDir branch separately from
// the WebSocket demo above, since most tests pass "" to skip it.
func TestNewHandler_ServesStaticFiles(t *testing.T) {
	srv := httptest.NewServer(NewHandler("testdata"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/hello.txt")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil && body == "" {
		t.Fatalf("reading body: %v", err)
	}
	if body != "hello from testdata\n" {
		t.Fatalf("body = %q, want %q", body, "hello from testdata\n")
	}
}
