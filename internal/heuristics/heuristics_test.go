package heuristics

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hammertrack/tracker/internal/message"
)

type RuleTest struct {
	callCompile     int
	callIsCompliant bool
	compliant       bool
}

func (r *RuleTest) Compile() {
	r.callCompile++
}
func (r *RuleTest) Final() bool {
	return false
}
func (r *RuleTest) IsCompliant(target Traits) bool {
	return r.compliant
}

func TestAnalyzer(t *testing.T) {
	t.Parallel()

	ruleA := &RuleTest{}
	ruleB := &RuleTest{}
	a := New([]Rule{
		ruleA, ruleB,
	})

	if ruleA.callCompile != 0 {
		t.Fatal("bad initialized RuleTest")
	}
	if ruleB.callCompile != 0 {
		t.Fatal("bad initialized RuleTest")
	}
	a.Compile()
	if ruleA.callCompile != 1 {
		t.Fatal("expected RuleTest.Compile() to have been called")
	}
	if ruleB.callCompile != 1 {
		t.Fatal("expected RuleTest.Compile() to have been called")
	}

	ruleA.compliant = false
	ruleB.compliant = true
	if a.IsCompliant(Traits{}) {
		t.Fatal("expect analyzer not to be compliant")
	}

	ruleA.compliant = true
	if !a.IsCompliant(Traits{}) {
		t.Fatal("expect analyzer to be compliant")
	}
}

func TestFinalRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc   string
		traits Traits
		msg    string
		rules  []Rule
		want   bool
	}{
		{
			desc:   "Final=false;others=non-compliant",
			traits: Traits{Type: message.MessageDeletion, Body: "https://example.com"},
			rules:  []Rule{RuleAlwaysStoreBans(), RuleNoLinks()},
			want:   false,
		},
		{
			desc:   "Final=false;others=non-compliant-2",
			traits: Traits{Type: message.MessageTimeout, Body: "hola", TimeoutDuration: 4},
			rules:  []Rule{RuleAlwaysStoreBans(), RuleNoLinks(), RuleMinTimeoutDuration(5)},
			want:   false,
		},
		{
			desc:   "Final=true;others=non-compliant",
			traits: Traits{Type: message.MessageBan, Body: "https://example.com"},
			rules:  []Rule{RuleAlwaysStoreBans(), RuleNoLinks()},
			want:   true,
		},
		{
			desc:   "Final=false;others=compliant",
			traits: Traits{Type: message.MessageDeletion, Body: "I am a compliant msg"},
			rules:  []Rule{RuleAlwaysStoreBans(), RuleNoLinks()},
			want:   true,
		},
		{
			desc:   "Final=true;others=compliant",
			traits: Traits{Type: message.MessageBan, Body: "I am a compliant msg"},
			rules:  []Rule{RuleAlwaysStoreBans(), RuleNoLinks()},
			want:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			a := New(test.rules)
			a.Compile()
			want := test.want
			got := a.IsCompliant(test.traits)
			if got != want {
				t.Logf("Traits: %s\n Rules: %s\n", spew.Sdump(test.traits), spew.Sdump(test.rules))
				t.Fatalf("got: %t; want: %t", got, want)
			}
		})
	}
}
