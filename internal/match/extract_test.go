package match

import (
	"errors"
	"net/http"
	"regexp"
	"testing"

	"intruder/internal/engine"
)

func TestBodyRegexExtractor_CapturesFirstGroup(t *testing.T) {
	r := engine.Result{Body: []byte(`<input name="csrf" value="abc123xyz">`)}
	e := BodyRegexExtractor{Re: regexp.MustCompile(`name="csrf" value="([^"]+)"`)}

	v, ok := e.Extract(r)
	if !ok || v != "abc123xyz" {
		t.Fatalf("got %q ok=%v, want \"abc123xyz\" ok=true", v, ok)
	}
}

func TestBodyRegexExtractor_NoGroupReturnsFullMatch(t *testing.T) {
	r := engine.Result{Body: []byte("status: ready")}
	e := BodyRegexExtractor{Re: regexp.MustCompile(`status: \w+`)}

	v, ok := e.Extract(r)
	if !ok || v != "status: ready" {
		t.Fatalf("got %q ok=%v, want \"status: ready\" ok=true", v, ok)
	}
}

func TestBodyRegexExtractor_NoMatch(t *testing.T) {
	r := engine.Result{Body: []byte("nothing interesting here")}
	e := BodyRegexExtractor{Re: regexp.MustCompile(`token=(\w+)`)}

	_, ok := e.Extract(r)
	if ok {
		t.Fatalf("expected no extraction when the pattern doesn't match")
	}
}

func TestBodyRegexExtractor_ErroredResult(t *testing.T) {
	r := engine.Result{Body: []byte("token=abc"), Err: errors.New("timeout")}
	e := BodyRegexExtractor{Re: regexp.MustCompile(`token=(\w+)`)}

	_, ok := e.Extract(r)
	if ok {
		t.Fatalf("expected no extraction from an errored result")
	}
}

func TestHeaderRegexExtractor(t *testing.T) {
	r := engine.Result{Headers: http.Header{"Set-Cookie": []string{"sessionid=deadbeef; Path=/; HttpOnly"}}}
	e := HeaderRegexExtractor{Header: "Set-Cookie", Re: regexp.MustCompile(`sessionid=([a-f0-9]+)`)}

	v, ok := e.Extract(r)
	if !ok || v != "deadbeef" {
		t.Fatalf("got %q ok=%v, want \"deadbeef\" ok=true", v, ok)
	}
}
