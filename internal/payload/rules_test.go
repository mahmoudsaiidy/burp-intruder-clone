package payload

import "testing"

func TestURLEncodeRule(t *testing.T) {
	got := URLEncodeRule{}.Apply("a b&c=d")
	want := "a+b%26c%3Dd"
	if got != want {
		t.Fatalf("URLEncodeRule.Apply: got %q, want %q", got, want)
	}
}

func TestBase64EncodeRule(t *testing.T) {
	got := Base64EncodeRule{}.Apply("hunter2")
	want := "aHVudGVyMg=="
	if got != want {
		t.Fatalf("Base64EncodeRule.Apply: got %q, want %q", got, want)
	}
}

func TestPrefixRule(t *testing.T) {
	got := PrefixRule{Prefix: "pre-"}.Apply("x")
	if got != "pre-x" {
		t.Fatalf("PrefixRule.Apply: got %q, want %q", got, "pre-x")
	}
}

func TestSuffixRule(t *testing.T) {
	got := SuffixRule{Suffix: "-post"}.Apply("x")
	if got != "x-post" {
		t.Fatalf("SuffixRule.Apply: got %q, want %q", got, "x-post")
	}
}

func TestUpperCaseRule(t *testing.T) {
	got := UpperCaseRule{}.Apply("MiXeD")
	if got != "MIXED" {
		t.Fatalf("UpperCaseRule.Apply: got %q, want %q", got, "MIXED")
	}
}

func TestLowerCaseRule(t *testing.T) {
	got := LowerCaseRule{}.Apply("MiXeD")
	if got != "mixed" {
		t.Fatalf("LowerCaseRule.Apply: got %q, want %q", got, "mixed")
	}
}
