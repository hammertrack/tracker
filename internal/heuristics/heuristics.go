package heuristics

import (
	"time"

	"pedro.to/hammertrace/tracker/internal/message"
)

type Traits struct {
	Type            message.MessageType
	Body            string
	At              time.Time
	ModeratedAt     time.Time
	TimeoutDuration int
	IsMostRecentMsg bool
}

type Rule interface {
	// If the rule needs an ahead of time compilation, do it here.
	//
	// Regular expressions and similar objects are being initialized in
	// `Compile()` methods to control from outside when this compilation is
	// happening (which may be expensive in the future). Do not initialize
	// compilations in any rule creator functions like `RuleNoLinks()`.
	// Compilation often is linked to the creation of the rule but with a
	// `Compile()` method it is more obvious that it may be an expensive task.
	Compile()
	IsCompliant(target Traits) bool
	// If final returns true, the analyzer will ignore the rest of rules. If a
	// final rule returns false it will be ignored.
	Final() bool
}

type Test interface {
	Rule
}

// Analyzer use simple heuristics to decide whether a message is valid or not,
// by applying a set of cached rules against the traits of each message.
type Analyzer struct {
	rules []Rule
}

// Compile calls the Compile() method for every rule.
func (a *Analyzer) Compile() {
	for _, rule := range a.rules {
		rule.Compile()
	}
}

// IsCompliant runs all the rules against the `target` traits of a given message.
// It returns true if it is compliant with every rule or false if a single rule
// returns false.
//
// IsCompliant requires rules to be compiled before with `Compile()` or it may
// throw a nil pointer derefence error
func (a *Analyzer) IsCompliant(target Traits) bool {
	for _, rule := range a.rules {
		v := rule.IsCompliant(target)
		if rule.Final() {
			if v {
				// target is compliant with a final rule, ignore the rest
				return true
			}
			// target is not compliant with a final rule, ignore the rule
			continue
		}
		if !v {
			return false
		}
	}
	return true
}

func New(rules []Rule) *Analyzer {
	return &Analyzer{rules}
}
