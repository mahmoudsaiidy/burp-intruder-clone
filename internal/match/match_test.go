package match

import (
	"errors"
	"net/http"
	"regexp"
	"testing"

	"intruder/internal/engine"
)

func TestStatusMatcher(t *testing.T) {
	ok200 := engine.Result{Status: 200}
	ok404 := engine.Result{Status: 404}
	errored := engine.Result{Status: 200, Err: errors.New("boom")}

	cases := []struct {
		name string
		m    Matcher
		r    engine.Result
		want bool
	}{
		{"equal match", StatusMatcher{Op: Equal, Value: 200}, ok200, true},
		{"equal no match", StatusMatcher{Op: Equal, Value: 200}, ok404, false},
		{"not equal", StatusMatcher{Op: NotEqual, Value: 200}, ok404, true},
		{"greater than", StatusMatcher{Op: GreaterThan, Value: 300}, ok404, true},
		{"less than", StatusMatcher{Op: LessThan, Value: 300}, ok200, true},
		{"errored result never matches", StatusMatcher{Op: Equal, Value: 200}, errored, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.m.Match(c.r); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestLengthMatcher(t *testing.T) {
	short := engine.Result{Length: 10}
	long := engine.Result{Length: 5000}

	if !(LengthMatcher{Op: LessThan, Value: 100}).Match(short) {
		t.Errorf("expected short result to match LessThan 100")
	}
	if (LengthMatcher{Op: LessThan, Value: 100}).Match(long) {
		t.Errorf("expected long result not to match LessThan 100")
	}
	if !(LengthMatcher{Op: GreaterOrEqual, Value: 5000}).Match(long) {
		t.Errorf("expected exact-length match with GreaterOrEqual")
	}
}

func TestBodyRegexMatcher(t *testing.T) {
	r := engine.Result{Body: []byte("welcome back, admin")}
	m := BodyRegexMatcher{Re: regexp.MustCompile(`welcome back`)}
	if !m.Match(r) {
		t.Errorf("expected body regex to match")
	}
	m2 := BodyRegexMatcher{Re: regexp.MustCompile(`invalid credentials`)}
	if m2.Match(r) {
		t.Errorf("expected body regex not to match")
	}
}

func TestHeaderRegexMatcher(t *testing.T) {
	r := engine.Result{Headers: http.Header{"Set-Cookie": []string{"session=abc123; Path=/"}}}
	m := HeaderRegexMatcher{Header: "Set-Cookie", Re: regexp.MustCompile(`session=\w+`)}
	if !m.Match(r) {
		t.Errorf("expected header regex to match")
	}
	missing := HeaderRegexMatcher{Header: "X-Not-There", Re: regexp.MustCompile(`.`)}
	if missing.Match(r) {
		t.Errorf("expected no match against a missing header")
	}
}

func TestErrorMatcher(t *testing.T) {
	if !(ErrorMatcher{}).Match(engine.Result{Err: errors.New("timeout")}) {
		t.Errorf("expected ErrorMatcher to match an errored result")
	}
	if (ErrorMatcher{}).Match(engine.Result{Status: 200}) {
		t.Errorf("expected ErrorMatcher not to match a successful result")
	}
}

func TestCombinators(t *testing.T) {
	r := engine.Result{Status: 200, Length: 42}

	and := And{
		StatusMatcher{Op: Equal, Value: 200},
		LengthMatcher{Op: Equal, Value: 42},
	}
	if !and.Match(r) {
		t.Errorf("expected And of two true matchers to match")
	}
	andFalse := And{
		StatusMatcher{Op: Equal, Value: 200},
		LengthMatcher{Op: Equal, Value: 999},
	}
	if andFalse.Match(r) {
		t.Errorf("expected And with one false matcher not to match")
	}

	or := Or{
		StatusMatcher{Op: Equal, Value: 404},
		LengthMatcher{Op: Equal, Value: 42},
	}
	if !or.Match(r) {
		t.Errorf("expected Or with one true matcher to match")
	}
	if (Or{}).Match(r) {
		t.Errorf("expected empty Or to match nothing")
	}

	not := Not{Matcher: StatusMatcher{Op: Equal, Value: 404}}
	if !not.Match(r) {
		t.Errorf("expected Not to invert a false match to true")
	}
}
