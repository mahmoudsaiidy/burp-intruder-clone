package config

import (
	"strings"
	"testing"
)

func TestLoad_RejectsUnknownFields(t *testing.T) {
	r := strings.NewReader(`{"template":"x","mode":"sniper","generators":[],"bogus":true}`)
	if _, err := Load(r); err == nil {
		t.Fatal("expected an error for an unknown field, got nil")
	}
}

func TestLoad_RoundTrip(t *testing.T) {
	raw := `{
		"template": "GET /login?password=§pass§ HTTP/1.1\r\nHost: x\r\n\r\n",
		"target": {"host": "127.0.0.1", "port": 8089},
		"mode": "sniper",
		"generators": [{"type": "wordlist", "words": ["a", "b"]}]
	}`
	cfg, err := Load(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mode != ModeSniper {
		t.Fatalf("mode = %q, want %q", cfg.Mode, ModeSniper)
	}
	if len(cfg.Generators) != 1 || len(cfg.Generators[0].Words) != 2 {
		t.Fatalf("unexpected generators: %+v", cfg.Generators)
	}
}

func TestGeneratorSpec_Build(t *testing.T) {
	tests := []struct {
		name    string
		spec    GeneratorSpec
		wantLen int
		wantErr bool
	}{
		{"wordlist", GeneratorSpec{Type: "wordlist", Words: []string{"a", "b", "c"}}, 3, false},
		{"wordlist empty", GeneratorSpec{Type: "wordlist"}, 0, true},
		{"numeric_range", GeneratorSpec{Type: "numeric_range", From: 1, To: 5, Step: 1}, 5, false},
		{"brute_force", GeneratorSpec{Type: "brute_force", Charset: "ab", MinLen: 1, MaxLen: 1}, 2, false},
		{"unknown type", GeneratorSpec{Type: "nonsense"}, 0, true},
		{"missing type", GeneratorSpec{}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := tt.spec.Build()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if g.Len() != tt.wantLen {
				t.Fatalf("Len() = %d, want %d", g.Len(), tt.wantLen)
			}
		})
	}
}

func TestGeneratorSpec_BuildWithRules(t *testing.T) {
	spec := GeneratorSpec{
		Type:  "wordlist",
		Words: []string{"hi"},
		Rules: []RuleSpec{{Type: "upper"}, {Type: "prefix", Value: ">>"}},
	}
	g, err := spec.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := g.Next()
	if !ok {
		t.Fatal("expected a value")
	}
	// upper first ("HI"), then prefix (">>HI") — rule order matters, same
	// as payload.Chain's own contract.
	if got != ">>HI" {
		t.Fatalf("got %q, want %q", got, ">>HI")
	}
}

func TestRuleSpec_UnknownType(t *testing.T) {
	if _, err := (RuleSpec{Type: "nonsense"}).Build(); err == nil {
		t.Fatal("expected error for unknown rule type")
	}
}

func TestMatcherSpec_Build(t *testing.T) {
	tests := []struct {
		name    string
		spec    MatcherSpec
		wantErr bool
	}{
		{"status eq default op", MatcherSpec{Type: "status", Value: 200}, false},
		{"status explicit op", MatcherSpec{Type: "status", Op: "ne", Value: 404}, false},
		{"status bad op", MatcherSpec{Type: "status", Op: "nonsense", Value: 200}, true},
		{"length", MatcherSpec{Type: "length", Op: "gt", Value: 100}, false},
		{"body_regex", MatcherSpec{Type: "body_regex", Pattern: "welcome"}, false},
		{"body_regex bad pattern", MatcherSpec{Type: "body_regex", Pattern: "("}, true},
		{"header_regex", MatcherSpec{Type: "header_regex", Header: "Set-Cookie", Pattern: "session"}, false},
		{"header_regex no header", MatcherSpec{Type: "header_regex", Pattern: "session"}, true},
		{"error", MatcherSpec{Type: "error"}, false},
		{"missing type", MatcherSpec{}, true},
		{"unknown type", MatcherSpec{Type: "nonsense"}, true},
		{
			"and",
			MatcherSpec{Type: "and", Children: []MatcherSpec{
				{Type: "status", Value: 200},
				{Type: "length", Op: "gt", Value: 10},
			}},
			false,
		},
		{"and empty", MatcherSpec{Type: "and"}, true},
		{
			"or",
			MatcherSpec{Type: "or", Children: []MatcherSpec{{Type: "status", Value: 200}}},
			false,
		},
		{
			"not",
			MatcherSpec{Type: "not", Child: &MatcherSpec{Type: "error"}},
			false,
		},
		{"not missing child", MatcherSpec{Type: "not"}, true},
		{
			"nested child error propagates",
			MatcherSpec{Type: "and", Children: []MatcherSpec{{Type: "nonsense"}}},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.spec.Build()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestExtractorSpec_Build(t *testing.T) {
	tests := []struct {
		name    string
		spec    ExtractorSpec
		wantErr bool
	}{
		{"body_regex", ExtractorSpec{Type: "body_regex", Pattern: `token=(\w+)`}, false},
		{"body_regex bad pattern", ExtractorSpec{Type: "body_regex", Pattern: "("}, true},
		{"header_regex", ExtractorSpec{Type: "header_regex", Header: "Set-Cookie", Pattern: `sid=(\w+)`}, false},
		{"header_regex no header", ExtractorSpec{Type: "header_regex", Pattern: "x"}, true},
		{"missing type", ExtractorSpec{}, true},
		{"unknown type", ExtractorSpec{Type: "nonsense"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.spec.Build()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAttackConfig_Build_Validation(t *testing.T) {
	validTmpl := "GET /login?password=§pass§ HTTP/1.1\r\nHost: x\r\n\r\n"
	twoMarkerTmpl := "GET /login?u=§user§&p=§pass§ HTTP/1.1\r\nHost: x\r\n\r\n"

	tests := []struct {
		name    string
		cfg     AttackConfig
		wantErr bool
	}{
		{"missing template", AttackConfig{Mode: ModeSniper}, true},
		{"bad template (unmatched marker)", AttackConfig{Template: "GET /§x HTTP/1.1\r\n\r\n", Mode: ModeSniper}, true},
		{"missing mode", AttackConfig{Template: validTmpl}, true},
		{"unknown mode", AttackConfig{Template: validTmpl, Mode: "nonsense"}, true},
		{"no generators", AttackConfig{Template: validTmpl, Mode: ModeSniper}, true},
		{
			"sniper ok",
			AttackConfig{Template: validTmpl, Mode: ModeSniper, Generators: []GeneratorSpec{{Type: "wordlist", Words: []string{"a"}}}},
			false,
		},
		{
			"sniper with 2 generators rejected",
			AttackConfig{
				Template: validTmpl, Mode: ModeSniper,
				Generators: []GeneratorSpec{{Type: "wordlist", Words: []string{"a"}}, {Type: "wordlist", Words: []string{"b"}}},
			},
			true,
		},
		{
			"pitchfork needs one generator per marker",
			AttackConfig{
				Template: twoMarkerTmpl, Mode: ModePitchfork,
				Generators: []GeneratorSpec{{Type: "wordlist", Words: []string{"a"}}},
			},
			true,
		},
		{
			"pitchfork ok",
			AttackConfig{
				Template: twoMarkerTmpl, Mode: ModePitchfork,
				Generators: []GeneratorSpec{
					{Type: "wordlist", Words: []string{"a"}},
					{Type: "wordlist", Words: []string{"b"}},
				},
			},
			false,
		},
		{
			"bad generator spec",
			AttackConfig{Template: validTmpl, Mode: ModeSniper, Generators: []GeneratorSpec{{Type: "nonsense"}}},
			true,
		},
		{
			"bad matcher spec",
			AttackConfig{
				Template: validTmpl, Mode: ModeSniper,
				Generators: []GeneratorSpec{{Type: "wordlist", Words: []string{"a"}}},
				Matcher:    &MatcherSpec{Type: "nonsense"},
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.cfg.Build()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAttackConfig_Build_Defaults(t *testing.T) {
	cfg := AttackConfig{
		Template:   "GET /login?password=§pass§ HTTP/1.1\r\nHost: x\r\n\r\n",
		Mode:       ModeSniper,
		Generators: []GeneratorSpec{{Type: "wordlist", Words: []string{"a"}}},
	}
	built, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if built.Workers != 5 {
		t.Fatalf("default Workers = %d, want 5", built.Workers)
	}
	if built.Timeout.Seconds() != 30 {
		t.Fatalf("default Timeout = %v, want 30s", built.Timeout)
	}
	// No Matcher configured flags nothing — same as match.Or{}.
	if built.Matcher == nil {
		t.Fatal("Matcher must never be nil")
	}
}
