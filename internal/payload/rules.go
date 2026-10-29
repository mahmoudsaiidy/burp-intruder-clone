package payload

import (
	"encoding/base64"
	"net/url"
	"strings"
)

// Rule transforms one payload value on its way out of a generator — Burp's
// "payload processing" rules (URL-encode, base64-encode, add prefix/suffix,
// change case, ...). Rules are applied in order by Chain, each one seeing
// the previous rule's output, so e.g. Prefix then Base64Encode base64s the
// prefix along with the original value, while Base64Encode then Prefix
// does not.
type Rule interface {
	Apply(value string) string
}

// URLEncodeRule percent-encodes a value the way it would need to appear
// inside a URL query string (spaces as "+", reserved characters escaped).
// Equivalent to Burp's "URL-encode these characters" processing rule in
// its default/full mode.
type URLEncodeRule struct{}

func (URLEncodeRule) Apply(value string) string {
	return url.QueryEscape(value)
}

// Base64EncodeRule base64-encodes a value using standard (RFC 4648)
// encoding with padding, matching Burp's default base64 processing rule.
type Base64EncodeRule struct{}

func (Base64EncodeRule) Apply(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}

// PrefixRule prepends a fixed string to every payload value.
type PrefixRule struct {
	Prefix string
}

func (r PrefixRule) Apply(value string) string {
	return r.Prefix + value
}

// SuffixRule appends a fixed string to every payload value.
type SuffixRule struct {
	Suffix string
}

func (r SuffixRule) Apply(value string) string {
	return value + r.Suffix
}

// UpperCaseRule converts a value to upper case.
type UpperCaseRule struct{}

func (UpperCaseRule) Apply(value string) string {
	return strings.ToUpper(value)
}

// LowerCaseRule converts a value to lower case.
type LowerCaseRule struct{}

func (LowerCaseRule) Apply(value string) string {
	return strings.ToLower(value)
}
