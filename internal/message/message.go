package message

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

func (last *MessageRing[V]) Do(fn func(msg *MessageRing[V], index int)) {
	fn(last, 0)
	for prev, i := last.prev, 1; prev != last; prev, i = prev.prev, i+1 {
		fn(prev, i)
	}
}

func (last *MessageRing[V]) Filter(fn func(val V) bool) []V {
	msgs := make([]V, 0, last.size)
	last.Do(func(msg *MessageRing[V], _ int) {
		if fn(msg.val) {
			msgs = append(msgs, msg.val)
		}
	})
	return msgs
}

func (last *MessageRing[V]) All() []V {
	all := make([]V, last.size)
	last.Do(func(msg *MessageRing[V], i int) {
		all[i] = msg.val
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
