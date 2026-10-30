package server

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestAcceptKey checks against the worked example in RFC 6455 §1.3 itself,
// so this isn't just testing the implementation against itself.
func TestAcceptKey(t *testing.T) {
	got := acceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Fatalf("acceptKey() = %q, want %q", got, want)
	}
}

func TestUpgrade_RejectsNonWebSocketRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil) // no upgrade headers
	if _, err := Upgrade(rec, req); err == nil {
		t.Fatal("Upgrade succeeded on a request with no WebSocket headers")
	}
}

// TestUpgrade_FullHandshake drives a real TCP connection through
// http.Server -> NewHandler -> Upgrade, using a hand-rolled client (the
// stdlib has no WebSocket client), then exercises a text frame, a ping
// round-trip, and a close frame over the upgraded connection.
func TestUpgrade_FullHandshake(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			t.Errorf("server Upgrade: %v", err)
			return
		}
		defer conn.Close()

		if err := conn.WriteText([]byte("hello")); err != nil {
			t.Errorf("WriteText: %v", err)
			return
		}
		// Exercise the ping->pong path and the close path on the same
		// connection the "client" below drives.
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Errorf("server ReadMessage (expected the client's close): %v", err)
		}
	}))
	defer srv.Close()

	rawConn, br := dialWebSocket(t, srv.URL+"/ws")
	defer rawConn.Close()

	fin, op, payload, err := decodeFrame(br)
	if err != nil {
		t.Fatalf("decoding server's text frame: %v", err)
	}
	if !fin || op != opText || string(payload) != "hello" {
		t.Fatalf("got fin=%v op=%#x payload=%q, want fin=true op=text payload=%q", fin, op, payload, "hello")
	}

	// Client sends a masked close frame; server's ReadMessage above should
	// return it without erroring out of the test handler.
	writeMaskedFrame(t, rawConn, opClose, []byte{})
}

// TestConn_ReadMessage_AnswersPing drives the server-side Conn directly
// (via net.Pipe, no real TCP/HTTP needed) to check that a ping is answered
// with a pong and never surfaced to the caller.
func TestConn_ReadMessage_AnswersPing(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	conn := &Conn{conn: serverSide, br: bufio.NewReader(serverSide), bw: bufio.NewWriter(serverSide)}

	done := make(chan struct{})
	go func() {
		defer close(done)
		writeMaskedFrame(t, clientSide, opPing, []byte("ping-data"))

		fin, op, payload, err := decodeFrame(bufio.NewReader(clientSide))
		if err != nil {
			t.Errorf("decoding expected pong: %v", err)
			return
		}
		if !fin || op != opPong || string(payload) != "ping-data" {
			t.Errorf("got fin=%v op=%#x payload=%q, want pong echoing %q", fin, op, payload, "ping-data")
		}

		writeMaskedFrame(t, clientSide, opClose, []byte{})
	}()

	op, _, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if op != opClose {
		t.Fatalf("ReadMessage returned opcode %#x, want opClose (ping should have been absorbed)", op)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for client goroutine")
	}
}

// --- test-only WebSocket client helpers ---------------------------------
//
// The stdlib has no WebSocket client, so these act as the minimum needed
// to drive the server from a test: a handshake, and masked frame writes
// (RFC 6455 requires client->server frames to be masked). Frame *decoding*
// reuses the package's own decodeFrame, since it already handles both
// masked and unmasked frames by checking the bit on the wire rather than
// assuming a direction.

func dialWebSocket(t *testing.T, url string) (net.Conn, *bufio.Reader) {
	t.Helper()
	host := url[len("http://"):]

	rawConn, err := net.Dial("tcp", host[:indexOfSlash(host)])
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("generating Sec-WebSocket-Key: %v", err)
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)

	path := host[indexOfSlash(host):]
	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + host[:indexOfSlash(host)] + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := rawConn.Write([]byte(req)); err != nil {
		t.Fatalf("writing handshake request: %v", err)
	}

	br := bufio.NewReader(rawConn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("reading handshake response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d, want 101", resp.StatusCode)
	}
	if want := acceptKey(key); resp.Header.Get("Sec-WebSocket-Accept") != want {
		t.Fatalf("Sec-WebSocket-Accept = %q, want %q", resp.Header.Get("Sec-WebSocket-Accept"), want)
	}

	return rawConn, br
}

func indexOfSlash(s string) int {
	for i, c := range s {
		if c == '/' {
			return i
		}
	}
	return len(s)
}

// writeMaskedFrame writes a single, unfragmented, masked frame — what a
// conforming client sends. Only used by tests acting as a WebSocket
// client; the server itself never masks (see writeFrame).
func writeMaskedFrame(t *testing.T, w net.Conn, opcode byte, payload []byte) {
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
	default:
		t.Fatalf("writeMaskedFrame: payload too long for this test helper (%d bytes)", n)
	}

	if _, err := w.Write(append(append(header, maskKey[:]...), masked...)); err != nil {
		t.Fatalf("writing frame: %v", err)
	}
}
