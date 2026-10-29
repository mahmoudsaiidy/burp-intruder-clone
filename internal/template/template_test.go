package template

import "testing"

func TestFindMarkers_Single(t *testing.T) {
	raw := "GET /login?password=§hunter2§ HTTP/1.1\r\nHost: x\r\n\r\n"
	markers, err := FindMarkers(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(markers))
	}
	if markers[0].Original != "hunter2" {
		t.Fatalf("expected original %q, got %q", "hunter2", markers[0].Original)
	}
}

func TestFindMarkers_Multiple(t *testing.T) {
	raw := "GET /login?u=§admin§&p=§pass§ HTTP/1.1\r\n\r\n"
	markers, err := FindMarkers(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(markers) != 2 {
		t.Fatalf("expected 2 markers, got %d", len(markers))
	}
	if markers[0].Original != "admin" || markers[1].Original != "pass" {
		t.Fatalf("unexpected marker contents: %+v", markers)
	}
}

func TestFindMarkers_Unmatched(t *testing.T) {
	raw := "GET /login?u=§admin HTTP/1.1\r\n\r\n"
	if _, err := FindMarkers(raw); err == nil {
		t.Fatal("expected error for unmatched marker, got nil")
	}
}

func TestInject(t *testing.T) {
	raw := "GET /login?u=§admin§&p=§pass§ HTTP/1.1\r\n\r\n"
	markers, err := FindMarkers(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := Inject(raw, markers, []string{"root", "toor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "GET /login?u=root&p=toor HTTP/1.1\r\n\r\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestBuild(t *testing.T) {
	raw := "GET /login?password=§pass§ HTTP/1.1\r\nHost: placeholder\r\n\r\n"
	tmpl, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	req, err := tmpl.Build([]string{"hunter2"}, TargetConfig{Host: "localhost", Port: 8089})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if req.URL.Scheme != "http" || req.URL.Host != "localhost:8089" {
		t.Fatalf("unexpected URL: %+v", req.URL)
	}
	if req.URL.RawQuery != "password=hunter2" {
		t.Fatalf("unexpected query: %q", req.URL.RawQuery)
	}
	if req.RequestURI != "" {
		t.Fatalf("RequestURI must be empty for client requests, got %q", req.RequestURI)
	}
}
