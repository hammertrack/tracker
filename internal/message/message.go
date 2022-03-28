package message

import "time"

type MessageType string

const (
	MessagePrivmsg  MessageType = "privmsg"
	MessageBan      MessageType = "ban"
	MessageTimeout  MessageType = "timeout"
	MessageDeletion MessageType = "deletion"

	// MaxHistory represents the number of messages stored in a in-memory history
	// for each channel. It should be equal to the messages displayed in twitch or
	// at least the maximum number of messages which a moderator can take an
	// action
	MaxHistory = 150
)

// PrivateMessage represents each chat message in the IRC, i.e. twitch chat.
type PrivateMessage struct {
	ID       string
	Username string
	Body     string
	At       time.Time
	Stored   bool
}

// Message represents a message coming from the IRC client. It denormalizes the
// different MessageType types of messages in a common interface so it can be
// sent in the same go-channel.
//
// In IRC actions like deletions, timeouts or bans are also messages. For only
// plain messages, i.e. PRIVMSG, and their details refer to `PrivateMessage`
// type.
type Message struct {
	Type MessageType
	// Channel represents the twitch channel
	Channel string
	// Username represents the owner of the message
	Username string
	// Duration represents in seconds the timeout. Duration is only present for
	// messafe of type MessageTimeout and MessageBan
	Duration int
	// LastMessages contains the related PRIVMSGs. It may be multiple PRIVMSGs
	// retrieved from a history in the case of bans and timeouts or single
	// messages in the case of deletion messages or a PRIVMSG itself
	LastMessages []*PrivateMessage
	// Used in case of deletions
	TargetMsgID string
	// At represents the timestamp of the message in the case of a MessageChat
	// type or the time of the moderation (deletion/ban/timeout)
	At time.Time
}

// MessageRing is a ring buffer that contains values of `V` type in a circular
// list of messages, effectively creating a rotating window of `size` size.
//
// It is optimized for receiving millions of values. It pre-allocates the values
// provided a default value is passed down and limits the checks needed to the
// minimum.
//
// Caveats:
// Methods like `Do` and their derivates: `Find`, `All`, etc. are O(n) where n
// is the provided size and not the actual size. In other words, all elements
// are iterated, including those which are not initialized because they're
// preallocated at the start. Make sure you provide a default value which
// satisfies all nested fields used in the methods, otherwise `Do` will pass a
// nil value if the element is not initialized and it may throw nil pointer
// dereference errors.
//
// It is not optimized for short lived windows because the iterator methods will
// iterate through all elements even if you only append a few and the head
// element will be useless (the default value) in the first rotation, but when
// the window size is reached and values start to rotate, it avoids checks in
// `Append` and iterator methods with a consistent O(size) for e.g.: `Filter`.
type MessageRing[V any] struct {
	next, prev *MessageRing[V]
	val        V
	size       int
}

// Append value to the buffer. It is necessary to store the result of the
// append. When the number of messages grows to `size` it completes the circle
// and overrides old values, creating a rotating window.
func (last *MessageRing[V]) Append(val V) *MessageRing[V] {
	next := last.next
	next.val = val
	return next
}

// Do executes a `fn` function for each element. If the functions returns true
// it will stop iterating.
func (last *MessageRing[V]) Do(fn func(msg *MessageRing[V], index int) bool) {
	fn(last, 0)
	for prev, i := last.prev, 1; prev != last; prev, i = prev.prev, i+1 {
		if fn(prev, i) {
			return
		}
	}
}

// Find the first element that matches in a `fn` function
func (last *MessageRing[V]) Find(fn func(val V) bool) (v V) {
	last.Do(func(msg *MessageRing[V], _ int) bool {
		if fn(msg.val) {
			v = msg.val
			return true
		}
		return false
	})
	return
}

// Filter returns all the elements that matches a filter `fn` function
func (last *MessageRing[V]) Filter(fn func(val V) bool) []V {
	msgs := make([]V, 0, last.size)
	last.Do(func(msg *MessageRing[V], _ int) bool {
		if fn(msg.val) {
			msgs = append(msgs, msg.val)
		}
		return false
	})
	return msgs
}

func (last *MessageRing[V]) All() []V {
	all := make([]V, last.size)
	last.Do(func(msg *MessageRing[V], i int) bool {
		all[i] = msg.val
		return false
	})
	return all
}

func newRing[V any](size int, def V) *MessageRing[V] {
	return &MessageRing[V]{
		size: size,
		val:  def,
	}
}

// New creates a new MessageRing. At the given `size`, the ring will be
// completed and values will start to override old values.
//
// A default value `def` is required to preallocate all the elements in the
// ring. Make sure to pass down a default value that satisfies all the nested
// fields you will use with the iterator methods like `Filter`, otherwise you
// may encounter nil dereference errors.
func New[V any](size int, def V) *MessageRing[V] {
	msg := newRing(size, def)
	last := msg
	for i := 1; i < size; i++ {
		next := newRing(size, def)
		next.prev = last
		last.next = next
		last = next
	}
	msg.prev = last
	last.next = msg
	return msg
}
