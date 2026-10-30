package chain

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"intruder/internal/engine"
	"intruder/internal/match"
	"intruder/internal/template"
)

// testTarget turns an httptest.Server's URL into a template.TargetConfig,
// the same way a caller would for any other target.
func testTarget(t *testing.T, ts *httptest.Server) template.TargetConfig {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host:port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return template.TargetConfig{Host: host, Port: port}
}

func TestResolve_NoStages(t *testing.T) {
	const attackRaw = "GET /login?password=§pass§ HTTP/1.1\r\nHost: example\r\n\r\n"

	resolved, extracted, err := Resolve(context.Background(), http.DefaultClient, nil, attackRaw)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved != attackRaw {
		t.Errorf("resolved = %q, want attackRaw unchanged", resolved)
	}
	if len(extracted) != 0 {
		t.Errorf("extracted = %v, want empty", extracted)
	}
}

func TestResolve_SingleStage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "CSRFTOKEN=xyz789")
	}))
	defer ts.Close()
	target := testTarget(t, ts)

	stage := Stage{
		Name:        "csrf",
		Raw:         "GET /csrf HTTP/1.1\r\nHost: " + ts.Listener.Addr().String() + "\r\n\r\n",
		Target:      target,
		Extractor:   match.BodyRegexExtractor{Re: regexp.MustCompile(`CSRFTOKEN=(\w+)`)},
		Placeholder: "{{csrf}}",
	}

	attackRaw := "POST /login HTTP/1.1\r\n" +
		"Host: " + ts.Listener.Addr().String() + "\r\n" +
		"X-CSRF-Token: {{csrf}}\r\n\r\n" +
		"password=§pass§"

	resolved, extracted, err := Resolve(context.Background(), ts.Client(), []Stage{stage}, attackRaw)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if extracted["csrf"] != "xyz789" {
		t.Errorf("extracted[csrf] = %q, want xyz789", extracted["csrf"])
	}
	if !strings.Contains(resolved, "X-CSRF-Token: xyz789") {
		t.Errorf("resolved missing substituted header, got:\n%s", resolved)
	}
	if strings.Contains(resolved, "{{csrf}}") {
		t.Errorf("resolved still contains the placeholder, got:\n%s", resolved)
	}
	// the attack's own §marker§ must survive untouched — chain resolution
	// only ever substitutes its own placeholders
	if !strings.Contains(resolved, "§pass§") {
		t.Errorf("resolved lost the attack's own §pass§ marker, got:\n%s", resolved)
	}

	tmpl, err := template.Parse(resolved)
	if err != nil {
		t.Fatalf("resolved text doesn't parse as a template: %v", err)
	}
	if len(tmpl.Markers) != 1 {
		t.Errorf("markers = %d, want exactly 1 (§pass§)", len(tmpl.Markers))
	}
}

// TestResolve_MultiStageChaining is the real point of this package: a
// second stage's own outgoing request must carry the first stage's
// extracted value, proving values flow forward through the chain and not
// just into the final attack text.
func TestResolve_MultiStageChaining(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "SESSIONID=abc123")
	})
	mux.HandleFunc("/csrf", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "session=abc123") {
			http.Error(w, "missing session cookie from previous stage", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, "CSRFTOKEN=xyz789")
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	target := testTarget(t, ts)
	host := ts.Listener.Addr().String()

	sessionStage := Stage{
		Name:        "session",
		Raw:         "GET /session HTTP/1.1\r\nHost: " + host + "\r\n\r\n",
		Target:      target,
		Extractor:   match.BodyRegexExtractor{Re: regexp.MustCompile(`SESSIONID=(\w+)`)},
		Placeholder: "{{session}}",
	}
	csrfStage := Stage{
		Name: "csrf",
		Raw: "GET /csrf HTTP/1.1\r\n" +
			"Host: " + host + "\r\n" +
			"Cookie: session={{session}}\r\n\r\n",
		Target:      target,
		Extractor:   match.BodyRegexExtractor{Re: regexp.MustCompile(`CSRFTOKEN=(\w+)`)},
		Placeholder: "{{csrf}}",
	}

	attackRaw := "POST /login HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Cookie: session={{session}}\r\n" +
		"X-CSRF-Token: {{csrf}}\r\n\r\n" +
		"password=§pass§"

	resolved, extracted, err := Resolve(context.Background(), ts.Client(), []Stage{sessionStage, csrfStage}, attackRaw)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if extracted["session"] != "abc123" {
		t.Errorf("extracted[session] = %q, want abc123", extracted["session"])
	}
	if extracted["csrf"] != "xyz789" {
		t.Errorf("extracted[csrf] = %q, want xyz789 (proves the csrf stage's own request actually carried the session stage's value)", extracted["csrf"])
	}
	if !strings.Contains(resolved, "Cookie: session=abc123") || !strings.Contains(resolved, "X-CSRF-Token: xyz789") {
		t.Errorf("resolved missing one or both substituted values, got:\n%s", resolved)
	}
}

func TestResolve_ExtractorFindsNothing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "no token here")
	}))
	defer ts.Close()
	target := testTarget(t, ts)

	stage := Stage{
		Name:        "csrf",
		Raw:         "GET /csrf HTTP/1.1\r\nHost: " + ts.Listener.Addr().String() + "\r\n\r\n",
		Target:      target,
		Extractor:   match.BodyRegexExtractor{Re: regexp.MustCompile(`CSRFTOKEN=(\w+)`)},
		Placeholder: "{{csrf}}",
	}

	_, _, err := Resolve(context.Background(), ts.Client(), []Stage{stage}, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	if err == nil {
		t.Fatal("Resolve: want error when the extractor finds nothing, got nil")
	}
	if !strings.Contains(err.Error(), `stage "csrf"`) {
		t.Errorf("error %q doesn't name the failing stage", err)
	}
}

func TestResolve_DuplicateStageName(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "VALUE=1")
	}))
	defer ts.Close()
	target := testTarget(t, ts)

	mk := func(placeholder string) Stage {
		return Stage{
			Name:        "dup",
			Raw:         "GET / HTTP/1.1\r\nHost: " + ts.Listener.Addr().String() + "\r\n\r\n",
			Target:      target,
			Extractor:   match.BodyRegexExtractor{Re: regexp.MustCompile(`VALUE=(\w+)`)},
			Placeholder: placeholder,
		}
	}

	_, _, err := Resolve(context.Background(), ts.Client(), []Stage{mk("{{a}}"), mk("{{b}}")}, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	if err == nil || !strings.Contains(err.Error(), "duplicate stage name") {
		t.Fatalf("Resolve: want duplicate-stage-name error, got %v", err)
	}
}

func TestResolve_DuplicatePlaceholder(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "VALUE=1")
	}))
	defer ts.Close()
	target := testTarget(t, ts)
	host := ts.Listener.Addr().String()

	stageA := Stage{
		Name: "a", Raw: "GET /a HTTP/1.1\r\nHost: " + host + "\r\n\r\n",
		Target: target, Extractor: match.BodyRegexExtractor{Re: regexp.MustCompile(`VALUE=(\w+)`)},
		Placeholder: "{{shared}}",
	}
	stageB := Stage{
		Name: "b", Raw: "GET /b HTTP/1.1\r\nHost: " + host + "\r\n\r\n",
		Target: target, Extractor: match.BodyRegexExtractor{Re: regexp.MustCompile(`VALUE=(\w+)`)},
		Placeholder: "{{shared}}",
	}

	_, _, err := Resolve(context.Background(), ts.Client(), []Stage{stageA, stageB}, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	if err == nil || !strings.Contains(err.Error(), `placeholder "{{shared}}"`) {
		t.Fatalf("Resolve: want shared-placeholder error, got %v", err)
	}
}

func TestResolve_RequestFails(t *testing.T) {
	// a server that's already closed: connection refused on every request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	target := testTarget(t, ts)
	ts.Close()

	stage := Stage{
		Name:      "dead",
		Raw:       "GET / HTTP/1.1\r\nHost: x\r\n\r\n",
		Target:    target,
		Extractor: match.BodyRegexExtractor{Re: regexp.MustCompile(`.`)},
	}

	_, _, err := Resolve(context.Background(), http.DefaultClient, []Stage{stage}, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	if err == nil || !strings.Contains(err.Error(), `stage "dead"`) || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("Resolve: want a request-failed error naming the stage, got %v", err)
	}
}

func TestResolve_BuildErrorOnMarkerMismatch(t *testing.T) {
	// this stage's own template has a marker but no MarkerValues supplied
	stage := Stage{
		Name:      "bad",
		Raw:       "GET /§x§ HTTP/1.1\r\nHost: x\r\n\r\n",
		Extractor: match.BodyRegexExtractor{Re: regexp.MustCompile(`.`)},
	}

	_, _, err := Resolve(context.Background(), http.DefaultClient, []Stage{stage}, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	if err == nil || !strings.Contains(err.Error(), "building request") {
		t.Fatalf("Resolve: want a building-request error, got %v", err)
	}
}

// sanity check that execute() produces a Result shape match.Extractor
// already knows how to read, independent of Resolve's own plumbing.
func TestExecute_PopulatesResult(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "hi")
		w.WriteHeader(http.StatusTeapot)
		fmt.Fprint(w, "body")
	}))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	result := execute(context.Background(), ts.Client(), req)

	if result.Err != nil {
		t.Fatalf("execute: %v", result.Err)
	}
	if result.Status != http.StatusTeapot {
		t.Errorf("Status = %d, want %d", result.Status, http.StatusTeapot)
	}
	if string(result.Body) != "body" {
		t.Errorf("Body = %q, want %q", result.Body, "body")
	}
	if result.Headers.Get("X-Test") != "hi" {
		t.Errorf("Headers[X-Test] = %q, want hi", result.Headers.Get("X-Test"))
	}
	var _ engine.Result = result // execute's return type must stay engine.Result
}
