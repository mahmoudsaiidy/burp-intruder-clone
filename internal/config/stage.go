package config

import (
	"fmt"
	"regexp"

	"intruder/internal/chain"
	"intruder/internal/match"
)

// StageSpec is the JSON form of one chain.Stage — a setup request run
// before the main attack, whose extracted value can feed into a later
// stage or into the attack template itself.
type StageSpec struct {
	// Name identifies the stage; must be non-empty and unique among a
	// Chain's stages (chain.Resolve enforces this at run time).
	Name string `json:"name"`
	// Raw is this stage's own raw HTTP request template text, same format
	// as AttackConfig.Template, §marker§ delimiters included if needed.
	Raw string `json:"raw"`
	// MarkerValues fills any §markers§ Raw contains, in marker order. Most
	// setup requests have none, so this is usually omitted.
	MarkerValues []string `json:"marker_values,omitempty"`
	// Target is the connection info for this stage's own request.
	Target TargetSpec `json:"target"`
	// Extractor pulls this stage's value out of its response. Required.
	Extractor ExtractorSpec `json:"extractor"`
	// Placeholder is the literal substring, elsewhere in this Chain's
	// later stages' Raw and in the main attack template, that gets
	// replaced with this stage's extracted value. Leave empty to extract
	// a value purely for inspection/logging.
	Placeholder string `json:"placeholder,omitempty"`
}

// Build turns the spec into a live chain.Stage.
func (s StageSpec) Build() (chain.Stage, error) {
	ext, err := s.Extractor.Build()
	if err != nil {
		return chain.Stage{}, fmt.Errorf("stage %q: %w", s.Name, err)
	}
	return chain.Stage{
		Name:         s.Name,
		Raw:          s.Raw,
		MarkerValues: s.MarkerValues,
		Target:       s.Target.Build(),
		Extractor:    ext,
		Placeholder:  s.Placeholder,
	}, nil
}

// ExtractorSpec is the JSON form of one match.Extractor.
type ExtractorSpec struct {
	// Type selects the extractor: "body_regex" or "header_regex".
	Type string `json:"type"`
	// Pattern is the regular expression used to find the value. If it has
	// a capture group, the first group is extracted; otherwise the whole
	// match is.
	Pattern string `json:"pattern"`
	// Header is the response header name for "header_regex".
	Header string `json:"header,omitempty"`
}

// Build turns the spec into a live match.Extractor.
func (e ExtractorSpec) Build() (match.Extractor, error) {
	switch e.Type {
	case "body_regex":
		re, err := regexp.Compile(e.Pattern)
		if err != nil {
			return nil, fmt.Errorf("extractor pattern: %w", err)
		}
		return match.BodyRegexExtractor{Re: re}, nil
	case "header_regex":
		if e.Header == "" {
			return nil, fmt.Errorf("header_regex extractor needs \"header\"")
		}
		re, err := regexp.Compile(e.Pattern)
		if err != nil {
			return nil, fmt.Errorf("extractor pattern: %w", err)
		}
		return match.HeaderRegexExtractor{Header: e.Header, Re: re}, nil
	case "":
		return nil, fmt.Errorf("extractor missing \"type\"")
	default:
		return nil, fmt.Errorf("unknown extractor type %q", e.Type)
	}
}
