package heuristics

import (
	"regexp"

	"github.com/hammertrack/tracker/internal/message"
)

// NoLinks - No links stored
//
// Reason: Deleted/banned/timeout messages with links tend to be automoderated,
// doesn't help moderators to know more about the user and doesn't help users to
// know more about the moderations in the channel
type NoLinks struct {
	urlrg *regexp.Regexp
}

func (r *NoLinks) Compile() {
	r.urlrg = regexp.MustCompile(`\b(https?|ftps?|file):\/\/[\-A-Za-z0-9+&@#\/%?=~_|!:,.;]*[\-A-Za-z0-9+&@#\/%=~_|]`)
}
func (r *NoLinks) Final() bool {
	return false
}
func (r *NoLinks) IsCompliant(target Traits) bool {
	return !r.urlrg.MatchString(target.Body)
}
func RuleNoLinks() *NoLinks {
	return &NoLinks{}
}

// MinTimeoutDuration - Only store timeout messages with a ban duration greater
// than a specified minimum
//
// Reason: Bots like nightbot and moobot often are configured with timeouts of
// 5s, 1s for automatically remove links and other things. Storing this messages
// is often useless. Also, messages with low timeout duration tend to be
// unimportant. Deleted messages and bans are not affected by this rule since
// both always have a duration of 0 in our traits.
type MinTimeoutDuration struct {
	min int
}

func (r *MinTimeoutDuration) Compile() {}
func (r *MinTimeoutDuration) Final() bool {
	return false
}
func (r *MinTimeoutDuration) IsCompliant(target Traits) bool {
	if target.Type == message.MessageTimeout {
		return target.TimeoutDuration > r.min
	}
	return true
}
func RuleMinTimeoutDuration(min int) *MinTimeoutDuration {
	return &MinTimeoutDuration{min}
}

// OnlyHumanModerations - Only store messages that are moderated by humans.
//
// Reason: Bots only can delete unimportant messages (links, capital letters,
// symbols, etc.).
//
// Caveats:
// - A user may repeatedly send messages while a moderator is banning him. If
// the moderator takes action and right after another message is sent, it may
// not be stored.
type OnlyHumanModerations struct {
	minHumanlyPossible float64
}

func (r *OnlyHumanModerations) Compile() {}
func (r *OnlyHumanModerations) IsCompliant(target Traits) bool {
	if target.IsMostRecentMsg {
		return target.ModeratedAt.Sub(target.At).Seconds() > r.minHumanlyPossible
	}
	return true
}
func (r *OnlyHumanModerations) Final() bool {
	return false
}

func RuleOnlyHumanModerations(minHumanlyPossible float64) *OnlyHumanModerations {
	return &OnlyHumanModerations{minHumanlyPossible}
}

// AlwaysStoreBans - self-explanatory
//
// Reason: They are rarely automatic and almost always for a good reason,
// providing useful information about the user. Also mitigates some caveats from
// other rules or possible bugs.
//
// It should always be placed at the beginning of the rules slice
type AlwaysStoreBans struct{}

func (r *AlwaysStoreBans) Compile() {}
func (r *AlwaysStoreBans) IsCompliant(target Traits) bool {
	return target.Type == message.MessageBan
}
func (r *AlwaysStoreBans) Final() bool {
	return true
}

func RuleAlwaysStoreBans() *AlwaysStoreBans {
	return &AlwaysStoreBans{}
}
