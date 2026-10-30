package server

// A minimal RFC 6455 WebSocket server implementation — intentionally not
// pulled in from a third-party module. This project's go.mod has stayed
// dependency-free through every package so far, and what the web UI
// actually needs is narrow: upgrade an HTTP connection, stream unmasked
// text frames from server to client, and notice when the client goes away
// (a close frame, a ping to answer, or the connection just dying). That's
// a few hundred lines of well-understood protocol, not a reason to take on
// a dependency.
//
// Deliberately NOT implemented, because nothing here needs it: message
// fragmentation (every frame this package sends or expects to receive is
// sent with FIN set), the permessage-deflate extension, and UTF-8
// validation of text payloads. A fragmented or extended frame from a
// client is reported as an error rather than silently mishandled.

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// websocketGUID is the fixed GUID RFC 6455 §1.3 has the server concatenate
// onto the client's Sec-WebSocket-Key before hashing, to prove the server
// actually understood this was a WebSocket upgrade (and isn't, say, a
// plain HTTP cache replaying a captured request).
const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Opcodes this package handles. Others (binary, reserved) are rejected.
const (
	opContinuation byte = 0x0
	opText         byte = 0x1
	opClose        byte = 0x8
	opPing         byte = 0x9
	opPong         byte = 0xA
)

// acceptKey computes the Sec-WebSocket-Accept header value for a given
// Sec-WebSocket-Key, per RFC 6455 §1.3.
func acceptKey(challengeKey string) string {
	h := sha1.New()
	h.Write([]byte(challengeKey))
	h.Write([]byte(websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// Conn is an upgraded WebSocket connection. The zero value is not usable;
// construct one with Upgrade. Not safe for concurrent writes from multiple
// goroutines (concurrent Write* and Read* from two goroutines is fine,
// since websocket.go's own callers only ever have one reader and one
// writer goroutine per Conn).
type Conn struct {
	conn net.Conn
	br   *bufio.Reader
	bw   *bufio.Writer
}

// Upgrade validates r as a WebSocket upgrade request, hijacks the
// underlying connection, and completes the handshake. On success, w and r
// must not be used again — the raw connection now belongs to the returned
// Conn.
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if r.Method != http.MethodGet {
		return nil, errors.New("websocket: upgrade request must be GET")
	}
	if !headerContainsToken(r.Header, "Connection", "upgrade") {
		return nil, errors.New("websocket: missing Connection: Upgrade")
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("websocket: missing Upgrade: websocket")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		return nil, errors.New("websocket: unsupported Sec-WebSocket-Version (want 13)")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("websocket: missing Sec-WebSocket-Key")
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("websocket: ResponseWriter does not support hijacking")
	}
	netConn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, fmt.Errorf("websocket: hijack: %w", err)
	}

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey(key) + "\r\n\r\n"
	if _, err := rw.WriteString(resp); err != nil {
		netConn.Close()
		return nil, fmt.Errorf("websocket: writing handshake response: %w", err)
	}
	if err := rw.Flush(); err != nil {
		netConn.Close()
		return nil, fmt.Errorf("websocket: flushing handshake response: %w", err)
	}

	return &Conn{conn: netConn, br: rw.Reader, bw: rw.Writer}, nil
}

// headerContainsToken reports whether header key's value is a
// comma-separated list containing token (case-insensitive) — Connection is
// typically just "Upgrade" but RFC 7230 allows a list ("keep-alive,
// Upgrade"), and some clients send it that way.
func headerContainsToken(header http.Header, key, token string) bool {
	for _, v := range header.Values(key) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// WriteText sends data as a single, unfragmented text frame.
func (c *Conn) WriteText(data []byte) error {
	return c.writeFrame(opText, data)
}

// WriteClose sends a close frame with the given status code and reason.
// Callers should stop writing to c afterward, though they may still read
// the peer's own close frame echoed back.
func (c *Conn) WriteClose(code uint16, reason string) error {
	payload := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(payload, code)
	copy(payload[2:], reason)
	return c.writeFrame(opClose, payload)
}

// Close closes the underlying TCP connection without sending a close
// frame. Prefer WriteClose for a clean shutdown; Close is for the deferred
// "definitely stop talking to this client" cleanup.
func (c *Conn) Close() error {
	return c.conn.Close()
}

// writeFrame writes a single, unfragmented, unmasked frame — per RFC 6455
// §5.1, a server never masks frames it sends; only client-to-server frames
// are masked.
func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	var header []byte
	switch n := len(payload); {
	case n <= 125:
		header = []byte{0x80 | opcode, byte(n)}
	case n <= 0xFFFF:
		header = make([]byte, 4)
		header[0] = 0x80 | opcode
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:], uint16(n))
	default:
		header = make([]byte, 10)
		header[0] = 0x80 | opcode
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:], uint64(n))
	}

	if _, err := c.bw.Write(header); err != nil {
		return fmt.Errorf("websocket: writing frame header: %w", err)
	}
	if _, err := c.bw.Write(payload); err != nil {
		return fmt.Errorf("websocket: writing frame payload: %w", err)
	}
	return c.bw.Flush()
}

// ReadMessage returns the next data-frame payload sent by the peer.
// Control frames are handled transparently: a ping is answered with a pong
// and does not return to the caller; a close frame is reported as
// (opClose, payload, nil) so the caller can stop. A fragmented frame
// (FIN not set) is reported as an error, since this package only ever
// sends and expects whole messages.
func (c *Conn) ReadMessage() (opcode byte, payload []byte, err error) {
	for {
		fin, op, data, err := decodeFrame(c.br)
		if err != nil {
			return 0, nil, err
		}
		if !fin {
			return 0, nil, errors.New("websocket: fragmented frames are not supported")
		}
		switch op {
		case opPing:
			if err := c.writeFrame(opPong, data); err != nil {
				return 0, nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			return opClose, data, nil
		case opText, opContinuation:
			return op, data, nil
		default:
			return 0, nil, fmt.Errorf("websocket: unsupported opcode %#x", op)
		}
	}
}

// decodeFrame decodes exactly one frame from br, per RFC 6455 §5.2,
// unmasking the payload if the frame declares itself masked (always true
// for a conforming client frame; this package doesn't assume it, it just
// checks the bit, so the same function also reads the unmasked frames a
// test client receives from the server — see websocket_test.go).
func decodeFrame(br *bufio.Reader) (fin bool, opcode byte, payload []byte, err error) {
	head := make([]byte, 2)
	if _, err := io.ReadFull(br, head); err != nil {
		return false, 0, nil, fmt.Errorf("websocket: reading frame header: %w", err)
	}
	fin = head[0]&0x80 != 0
	opcode = head[0] & 0x0F
	masked := head[1]&0x80 != 0
	length := uint64(head[1] & 0x7F)

	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(br, ext); err != nil {
			return false, 0, nil, fmt.Errorf("websocket: reading extended length: %w", err)
		}
		length = uint64(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(br, ext); err != nil {
			return false, 0, nil, fmt.Errorf("websocket: reading extended length: %w", err)
		}
		length = binary.BigEndian.Uint64(ext)
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(br, maskKey[:]); err != nil {
			return false, 0, nil, fmt.Errorf("websocket: reading mask key: %w", err)
		}
	}

	payload = make([]byte, length)
	if _, err := io.ReadFull(br, payload); err != nil {
		return false, 0, nil, fmt.Errorf("websocket: reading payload: %w", err)
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return fin, opcode, payload, nil
}
