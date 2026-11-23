package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"intruder/internal/configstore"
)

const validConfigJSON = `{"template":"x","mode":"sniper","generators":[]}`

func newTestStoreServer(t *testing.T) (*httptest.Server, *configstore.Store) {
	t.Helper()
	store, err := configstore.New(t.TempDir())
	if err != nil {
		t.Fatalf("configstore.New: %v", err)
	}
	srv := httptest.NewServer(NewHandler("", store))
	t.Cleanup(srv.Close)
	return srv, store
}

func doRequest(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestConfigStoreRoutes_NotRegisteredWhenStoreNil(t *testing.T) {
	srv := httptest.NewServer(NewHandler("", nil))
	defer srv.Close()

	resp := doRequest(t, http.MethodGet, srv.URL+"/api/configs", "")
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("GET /api/configs with a nil store: got 200, want anything but a handled 200")
	}
}

func TestConfigStoreRoutes_SaveListLoadDelete(t *testing.T) {
	srv, _ := newTestStoreServer(t)

	// Freshly created store: empty list.
	resp := doRequest(t, http.MethodGet, srv.URL+"/api/configs", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/configs (empty): status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("GET /api/configs (empty) = %s, want []", body)
	}

	// Save one.
	resp = doRequest(t, http.MethodPut, srv.URL+"/api/configs/my-sniper", validConfigJSON)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT /api/configs/my-sniper: status = %d, body = %s", resp.StatusCode, b)
	}

	// List now shows it.
	resp = doRequest(t, http.MethodGet, srv.URL+"/api/configs", "")
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "my-sniper") {
		t.Fatalf("GET /api/configs after save = %s, want it to contain \"my-sniper\"", body)
	}

	// Load it back byte-for-byte.
	resp = doRequest(t, http.MethodGet, srv.URL+"/api/configs/my-sniper", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/configs/my-sniper: status = %d", resp.StatusCode)
	}
	body, _ = io.ReadAll(resp.Body)
	if string(body) != validConfigJSON {
		t.Fatalf("GET /api/configs/my-sniper = %s, want %s", body, validConfigJSON)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	// Delete it.
	resp = doRequest(t, http.MethodDelete, srv.URL+"/api/configs/my-sniper", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /api/configs/my-sniper: status = %d", resp.StatusCode)
	}

	// Gone.
	resp = doRequest(t, http.MethodGet, srv.URL+"/api/configs/my-sniper", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/configs/my-sniper after delete: status = %d, want 404", resp.StatusCode)
	}
}

func TestConfigStoreRoutes_LoadMissing404(t *testing.T) {
	srv, _ := newTestStoreServer(t)
	resp := doRequest(t, http.MethodGet, srv.URL+"/api/configs/nope", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestConfigStoreRoutes_DeleteMissing404(t *testing.T) {
	srv, _ := newTestStoreServer(t)
	resp := doRequest(t, http.MethodDelete, srv.URL+"/api/configs/nope", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestConfigStoreRoutes_SaveInvalidJSON400(t *testing.T) {
	srv, _ := newTestStoreServer(t)
	resp := doRequest(t, http.MethodPut, srv.URL+"/api/configs/bad", "not json")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestConfigStoreRoutes_SaveInvalidName400(t *testing.T) {
	srv, _ := newTestStoreServer(t)
	// "a/b" as a path segment doesn't even reach our handler as a single
	// {name} — net/http's own path cleaning means this exercises a
	// differently-shaped name instead; use one that's a single path
	// segment but fails configstore.ValidateName.
	resp := doRequest(t, http.MethodPut, srv.URL+"/api/configs/"+strings.Repeat("x", 101), validConfigJSON)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestConfigStoreRoutes_SaveOverwrites(t *testing.T) {
	srv, _ := newTestStoreServer(t)
	const updated = `{"template":"y","mode":"sniper","generators":[]}`

	doRequest(t, http.MethodPut, srv.URL+"/api/configs/cfg", validConfigJSON)
	resp := doRequest(t, http.MethodPut, srv.URL+"/api/configs/cfg", updated)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("overwrite PUT: status = %d", resp.StatusCode)
	}

	resp = doRequest(t, http.MethodGet, srv.URL+"/api/configs/cfg", "")
	body, _ := io.ReadAll(resp.Body)
	if string(body) != updated {
		t.Fatalf("GET after overwrite = %s, want %s", body, updated)
	}
}
