package message

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func reverse[V any](s []V) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func TestWindowedValues(t *testing.T) {
	t.Parallel()
	var initialMsgIds = []int{10, 20, 30, 40, 50}
	tests := []struct {
		desc  string
		input int
		want  []int
	}{
		{desc: "10,[20,30,40,50,60],70,80,90", input: 60, want: []int{60, 50, 40, 30, 20}},
		{desc: "10,20,[30,40,50,60,70],80,90", input: 70, want: []int{70, 60, 50, 40, 30}},
		{desc: "10,20,30,[40,50,60,70,80],90", input: 80, want: []int{80, 70, 60, 50, 40}},
		{desc: "10,20,30,40,[50,60,70,80,90]", input: 90, want: []int{90, 80, 70, 60, 50}},
	}

	msgRing := New(5, 0)
	got, want := msgRing.All(), []int{0, 0, 0, 0, 0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong initial state: got %v, want %v", got, want)
	}

	for _, msgId := range initialMsgIds {
		msgRing = msgRing.Append(msgId)
	}

	got, want = msgRing.All(), initialMsgIds
	reverse(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong initial messages ids: got %v want %v", got, want)
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			msgRing = msgRing.Append(test.input)
			got, want = msgRing.All(), test.want
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got: %v, want: %v", got, want)
			}
		})
	}
}

func TestFindValues(t *testing.T) {
	t.Parallel()
	type NestedVal struct {
		val int
	}
	type User struct {
		username string
	}
	type Msg struct {
		val  *NestedVal
		user *User
	}
	var initialMsgs = []Msg{
		{val: &NestedVal{10}, user: &User{"aaa"}},
		{val: &NestedVal{20}, user: &User{"bbb"}},
		{val: &NestedVal{30}, user: &User{"ccc"}},
		{val: &NestedVal{40}, user: &User{"aaa"}},
		{val: &NestedVal{50}, user: &User{"aaa"}},
		{val: &NestedVal{60}, user: &User{"ccc"}},
		{val: &NestedVal{70}, user: &User{"ccc"}},
		{val: &NestedVal{80}, user: &User{"ccc"}},
		{val: &NestedVal{90}, user: &User{"ddd"}},
		{val: &NestedVal{100}, user: &User{"ddd"}},
		{val: &NestedVal{10}, user: &User{"eee"}},
	}
	tests := []struct {
		desc  string
		input string
		want  []Msg
	}{
		{desc: "find:aaa", input: "aaa", want: []Msg{
			{val: &NestedVal{10}, user: &User{"aaa"}},
			{val: &NestedVal{40}, user: &User{"aaa"}},
			{val: &NestedVal{50}, user: &User{"aaa"}},
		}},
		{desc: "find:bbb", input: "bbb", want: []Msg{
			{val: &NestedVal{20}, user: &User{"bbb"}},
		}},
		{desc: "find:ccc", input: "ccc", want: []Msg{
			{val: &NestedVal{30}, user: &User{"ccc"}},
			{val: &NestedVal{60}, user: &User{"ccc"}},
			{val: &NestedVal{70}, user: &User{"ccc"}},
			{val: &NestedVal{80}, user: &User{"ccc"}},
		}},
		{desc: "find:ddd", input: "ddd", want: []Msg{
			{val: &NestedVal{90}, user: &User{"ddd"}},
			{val: &NestedVal{100}, user: &User{"ddd"}},
		}},
		{desc: "find:eee", input: "eee", want: []Msg{
			{val: &NestedVal{10}, user: &User{"eee"}},
		}},
	}

	msgRing := New(15, Msg{user: &User{""}})

	for _, msg := range initialMsgs {
		msgRing = msgRing.Append(msg)
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got := msgRing.Filter(func(msg Msg) bool {
				t.Logf("val: %v, user: %v", msg.val, msg.user)
				return msg.user.username == test.input
			})
			want := test.want
			reverse(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got: %v, want: %v", got, want)
			}
		})
	}
}

func TestFind(t *testing.T) {
	t.Parallel()
	type NestedVal struct {
		val int
	}
	type User struct {
		username string
	}
	type Msg struct {
		val  *NestedVal
		user *User
		id   string
	}
	var initialMsgs = []Msg{
		{val: &NestedVal{10}, user: &User{"aaa"}, id: "1"},
		{val: &NestedVal{20}, user: &User{"bbb"}, id: "2"},
		{val: &NestedVal{30}, user: &User{"ccc"}, id: "3"},
		{val: &NestedVal{40}, user: &User{"aaa"}, id: "4"},
		{val: &NestedVal{50}, user: &User{"aaa"}, id: "5"},
		{val: &NestedVal{60}, user: &User{"ccc"}, id: "6"},
		{val: &NestedVal{70}, user: &User{"ccc"}, id: "7"},
		{val: &NestedVal{50}, user: &User{"mmm"}, id: "4"},
		{val: &NestedVal{80}, user: &User{"ccc"}, id: "8"},
		{val: &NestedVal{80}, user: &User{"ccc"}, id: "8"},
		{val: &NestedVal{90}, user: &User{"ddd"}, id: "9"},
		{val: &NestedVal{100}, user: &User{"ddd"}, id: "10"},
		{val: &NestedVal{10}, user: &User{"eee"}, id: "11"},
	}
	tests := []struct {
		desc  string
		input string
		want  Msg
	}{
		{desc: "find:4", input: "4", want: initialMsgs[7]},
		{desc: "find:8", input: "8", want: initialMsgs[9]},
		{desc: "find:10", input: "10", want: initialMsgs[11]},
		{desc: "find:100", input: "100", want: Msg{}},
	}

	msgRing := New(15, Msg{user: &User{""}})

	for _, msg := range initialMsgs {
		msgRing = msgRing.Append(msg)
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got := msgRing.Find(func(msg Msg) bool {
				return msg.id == test.input
			})

			want := test.want
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got: %v, want: %v", spew.Sdump(got), spew.Sdump(want))
			}
		})
	}

}
