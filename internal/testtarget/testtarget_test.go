package testtarget

import (
	"net/http/httptest"
	"testing"
)

func TestLoginFunc(t *testing.T) {
	ts := httptest.NewServer(LoginHandler())
	defer ts.Close()

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"correct password", "password=" + CorrectPassword, 200},
		{"wrong password", "password=nope", 401},
		{"missing password", "", 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ts.Client().Get(ts.URL + "/login?" + tt.query)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

func TestMultiFieldLoginFunc(t *testing.T) {
	ts := httptest.NewServer(Mux())
	defer ts.Close()

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"both correct", "username=" + CorrectUsername + "&password=" + CorrectPassword, 200},
		{"right user wrong pass", "username=" + CorrectUsername + "&password=nope", 401},
		{"wrong user right pass", "username=nope&password=" + CorrectPassword, 401},
		{"both wrong", "username=nope&password=nope", 401},
		{"missing both", "", 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ts.Client().Get(ts.URL + "/login/multi?" + tt.query)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

func TestCSRFFunc(t *testing.T) {
	ts := httptest.NewServer(Mux())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/csrf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body [64]byte
	n, _ := resp.Body.Read(body[:])
	got := string(body[:n])
	want := "token=" + CSRFToken
	if got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestSecureLoginFunc(t *testing.T) {
	ts := httptest.NewServer(Mux())
	defer ts.Close()

	ok := "csrf=" + CSRFToken + "&username=" + CorrectUsername + "&password=" + CorrectPassword

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"everything correct", ok, 200},
		{"missing csrf", "username=" + CorrectUsername + "&password=" + CorrectPassword, 401},
		{"wrong csrf", "csrf=bogus&username=" + CorrectUsername + "&password=" + CorrectPassword, 401},
		{"right csrf wrong creds", "csrf=" + CSRFToken + "&username=nope&password=nope", 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ts.Client().Get(ts.URL + "/login/secure?" + tt.query)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

// TestMuxKeepsSingleFieldLoginUnchanged guards the compatibility promise
// LoginFunc's doc comment makes: Mux's /login must behave identically to
// LoginHandler's /login, so cmd/testserver (now using Mux) is still a valid
// target for internal/server's hardcoded demo attack and cmd/cli's
// no-argument demo, both of which expect exactly this single-field
// behavior at exactly this path.
func TestMuxKeepsSingleFieldLoginUnchanged(t *testing.T) {
	ts := httptest.NewServer(Mux())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/login?password=" + CorrectPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
