package heuristics

import (
	"fmt"
	"testing"
	"time"

	"github.com/hammertrack/tracker/internal/message"
)

func createAnalyzer(rule Rule) *Analyzer {
	a := New([]Rule{rule})
	a.Compile()
	return a
}

func TestRuleNoLinks(t *testing.T) {
	t.Parallel()
	a := createAnalyzer(RuleNoLinks())

	// Good ref: https://mathiasbynens.be/demo/url-regex
	tests := []struct {
		input string
		want  bool
	}{
		// We should lean to the strict side because if this rule flags the
		// message validity as false easily it could lead to bugs that make message
		// deletions not traceable. For example, by not allowing foo.bar being ignored
		// we prevent abuse. So an ideal RegExp limits the ammount of want=false
		// (message detected as a link) limited
		{input: "hola.que", want: true},
		{input: "//hola...", want: true},
		{input: "hola...", want: true},
		{input: "//hola", want: true},
		{input: "/hola/", want: true},
		{input: "*hola/", want: true},
		{input: "*.hola./", want: true},
		{input: ".hola.", want: true},
		{input: "*hola*", want: true},
		{input: "hola/#", want: true},
		{input: "hola/", want: true},
		{input: "h.ola", want: true},
		{input: "..hola/", want: true},
		{input: ".hola/", want: true},
		{input: "😃.com", want: true},
		{input: "fail.exe", want: true},
		{input: "@hola", want: true},
		{input: "google.com", want: true},
		{input: "http://foo.com/blah_blah", want: false},
		{input: "http://foo.com/blah_blah/", want: false},
		{input: "http://✪df.ws/123", want: true},
		{input: "http://userid:password@example.com:8080", want: false},
		{input: "http://", want: true},
		{input: "http://google.com", want: false},
		{input: "https://google.com", want: false},
		{input: "ftp://google.com", want: false},
		{input: "ftps://google.com", want: false},
		{input: "file://google.com", want: false},
		{input: "http://example.com/#test", want: false},
		{input: "http://1.1.1.1", want: false},
		{input: "http://www.foo.bar./", want: false},
		{input: "http://.www.foo.bar./", want: false},
		{input: "http://.www.foo.bar/", want: false},
		{input: "https://www.reddit.com/r/sveltejs/comments/tqe4r0/svelte_cubed_normal_map/", want: false},
		{input: "https://www.youtube.com/watch?v=KAsiaDEUnlk", want: false},
		{input: "https://twitter.com/dw_espanol/status/1508489763204083721", want: false},
		{input: "drive.google.com/test", want: true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got := a.IsCompliant(Traits{
				Body: test.input,
			})
			want := test.want
			if got != want {
				t.Fatalf("input: %s, got: %t want:%t", test.input, got, want)
			}
		})
	}
}

func TestRuleMinTimeoutDuration(t *testing.T) {
	t.Parallel()
	a := createAnalyzer(RuleMinTimeoutDuration(5))

	// Good ref: https://mathiasbynens.be/demo/url-regex
	tests := []struct {
		input int
		want  bool
	}{
		{input: 5, want: false},
		{input: 1, want: false},
		{input: 2, want: false},
		{input: 6, want: true},
		{input: 800, want: true},
		{input: 10000, want: true},
		{input: 86400, want: true},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%d", test.input), func(t *testing.T) {
			got := a.IsCompliant(Traits{
				Body:            "A message",
				Type:            message.MessageTimeout,
				TimeoutDuration: test.input,
			})
			want := test.want
			if got != want {
				t.Fatalf("input: %d, got: %t want:%t", test.input, got, want)
			}
		})
	}
}

func TestOnlyHumanModerations(t *testing.T) {
	t.Parallel()
	a := createAnalyzer(RuleOnlyHumanModerations(.9))

	tests := []struct {
		input float64
		want  bool
	}{
		{input: 0.23, want: false},
		{input: 0.5, want: false},
		{input: 0.001, want: false},
		{input: 0.09, want: false},
		{input: 1, want: true},
		{input: 5.3, want: true},
		{input: 5, want: true},
		{input: 7.32, want: true},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%f", test.input), func(t *testing.T) {
			now := time.Now()
			then := now.Add(time.Duration(test.input) * time.Second)
			got := a.IsCompliant(Traits{
				Body:            "A message",
				Type:            message.MessageTimeout,
				At:              now,
				ModeratedAt:     then,
				IsMostRecentMsg: true,
			})
			want := test.want
			if got != want {
				t.Fatalf("input: %f, got: %t want:%t", test.input, got, want)
			}
		})
	}
}
