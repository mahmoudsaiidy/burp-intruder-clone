package server

import (
	"crypto/rand"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// writeMaskedFrameAny is writeMaskedFrame (websocket_test.go) without its
// 125-byte payload cap: an AttackConfig JSON document routinely runs
// longer than that, which a real client handles with the 16-bit extended
// length field RFC 6455 defines — this is that, for payloads up to 65535
// bytes (plenty for a test).
func writeMaskedFrameAny(t *testing.T, w net.Conn, opcode byte, payload []byte) {
	t.Helper()

	var maskKey [4]byte
	if _, err := rand.Read(maskKey[:]); err != nil {
		t.Fatalf("generating mask key: %v", err)
	}
	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ maskKey[i%4]
	}

	n := len(payload)
	var header []byte
	switch {
	case n <= 125:
		header = []byte{0x80 | opcode, 0x80 | byte(n)}
	case n <= 65535:
		header = []byte{0x80 | opcode, 0x80 | 126, byte(n >> 8), byte(n)}
	default:
		t.Fatalf("writeMaskedFrameAny: payload too long for this test helper (%d bytes)", n)
	}

	if _, err := w.Write(append(append(header, maskKey[:]...), masked...)); err != nil {
		t.Fatalf("writing frame: %v", err)
	}
}

// TestConfigAttack_EndToEnd dials /ws/config, sends a JSON AttackConfig as
// the first text frame (a Sniper run against a mock login handler this
// test serves itself), and checks the resulting stream: a "started"
// message with the right total, one "result" per payload, exactly one
// flagged, and a matching "done" summary — the same shape runDemoAttack's
// own test checks, now driven by a client-submitted config instead of the
// hardcoded demo.
func TestConfigAttack_EndToEnd(t *testing.T) {
	loginSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("password") == "hunter2" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer loginSrv.Close()
	host, port := splitTestServerURL(t, loginSrv.URL)

	srv := httptest.NewServer(NewHandler(""))
	defer srv.Close()

	rawConn, br := dialWebSocket(t, srv.URL+"/ws/config")
	defer rawConn.Close()
	rawConn.SetReadDeadline(time.Now().Add(10 * time.Second))

	cfgJSON := `{
		"template": "GET /login?password=` + "§pass§" + ` HTTP/1.1\r\nHost: x\r\n\r\n",
		"target": {"host": "` + host + `", "port": ` + port + `},
		"mode": "sniper",
		"generators": [{"type": "wordlist", "words": ["wrong1", "hunter2", "wrong2"]}],
		"matcher": {"type": "status", "value": 200}
	}`
	writeMaskedFrameAny(t, rawConn, opText, []byte(cfgJSON))

	var started *wsMessage
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
		case "started":
			m := msg
			started = &m
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

	if started == nil {
		t.Fatal("never got a \"started\" message")
	}
	if started.Total != 3 {
		t.Errorf("started.Total = %d, want 3", started.Total)
	}

	if len(results) != 3 {
		t.Fatalf("got %d result messages, want 3", len(results))
	}
	hits := 0
	var hitPayload string
	for _, r := range results {
		if r.Hit {
			hits++
			hitPayload = r.Payload
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
	if done.Total != 3 || done.Hits != 1 {
		t.Errorf("done = %+v, want Total=3 Hits=1", done)
	}
}

// TestConfigAttack_InvalidConfig checks that a config which fails to
// build (here: a mode/generator-count mismatch) comes back as a single
// "error" message rather than a panic or a silently empty stream.
func TestConfigAttack_InvalidConfig(t *testing.T) {
	srv := httptest.NewServer(NewHandler(""))
	defer srv.Close()

	rawConn, br := dialWebSocket(t, srv.URL+"/ws/config")
	defer rawConn.Close()
	rawConn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// no "mode" set — Build must reject this before any request fires.
	writeMaskedFrameAny(t, rawConn, opText, []byte(`{"template":"GET / HTTP/1.1\r\nHost: x\r\n\r\n"}`))

	_, op, payload, err := decodeFrame(br)
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	if op != opText {
		t.Fatalf("got opcode %#x, want text", op)
	}
	var msg wsMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("unmarshalling %q: %v", payload, err)
	}
	if msg.Type != "error" {
		t.Fatalf("message type = %q, want \"error\"", msg.Type)
	}
}

func splitTestServerURL(t *testing.T, rawURL string) (host, port string) {
	t.Helper()
	// httptest.Server.URL is always "http://127.0.0.1:PORT" — simple
	// enough to split by hand rather than pulling in net/url+net here too.
	const prefix = "http://"
	addr := rawURL[len(prefix):]
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:]
		}
	}
	t.Fatalf("could not find port in %q", rawURL)
	return "", ""
}
