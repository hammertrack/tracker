package errors

import (
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hammertrack/tracker/color"
)

type Generic struct {
	ID       string
	err      error
	ts       time.Time
	FuncName string
	FileName string
	Line     int
	Context  interface{}
}

// Error makes Generic comply with error interface.
//
// By printing e.err.Error(), we recursively print all the errors in the same
// stack trace. For example, imagine a stack trace (that is, an error originated
// in one function, returned to another function which wrapped the first error,
// etc. until three):
//
// Wrapped errors messages in order, were %s = message of parent error
// %s = couldn't open file bla bla
// 1. err: %s <A>
//    ^^^^^^^^^^^ will be next %s
// 2. err: err: %s <A> <B>
//         ^^^^^^^^^^^ will be next %s
// 3. err: err: err: %s <A> <B> <C>
//
// So as you see, with just e.err.Error() we have a problem: prefix gets
// repeated and suffix gets piled one after another.
//
// We fix this by trimming the prefix until (including) ">>> ". So only the most
// recent error prefix (the one that wraps the other ones in the stack trace) is
// displayed. Also we take advantage of this behaviour, including our context
// and caller information at the end, which will be piled one after the other in
// parent-to-child order.
//
// This is the resulting format:
//
// without context
// [#err:id] >>> message <main.go:21#main.test><main.go#17:main.main><etc>
//
// with Context (with caller info so you can see which one belongs to which)
// [#err:id] >>> message <main.go:21#main.test Ctx:{foo:"bar"}><main.go:17#main.main>
func (e Generic) Error() string {
	var (
		s strings.Builder
		// Trim the prefix so it doesn't repeat (because parent errors are the errors
		// of the childs)
		msg = trimUntil(e.err.Error(), ">", 4)
	)
	fmt.Fprintf(
		&s, "%s%s [%s] ► %s <%s:%d#%s",
		// prefix: this part is overwritten by the error that wraps it in the trace,
		// so only the last one will be displayed
		color.Reset, color.String(color.Red, "✗"), color.String(color.Red, e.ID),
		msg,
		// this part is carried over to each wrapper error in the trace so we take
		// advantage of this by printing the current caller info, which will be
		// concatenated one after the other. The msg, above is not carried over
		// because the parent error itself will be the error of the child (see docs
		// for Error)
		trimUntilBackwards(e.FileName, "/", 1), e.Line, e.FuncName,
	)
	if e.Context != nil {
		fmt.Fprintf(&s, " ≣:%+v", e.Context)
	}
	s.WriteString(">")
	return s.String()
}

func (e Generic) Unwrap() error {
	return e.err
}

// Cause returns the top most error of Generic type.
func (e Generic) Cause() Generic {
	return UnwrapAll(e)
}

// Trace returns the string with the breadcrumbs (step by step) containing the
// caller info of every generic error, looping through all the parent errors
// until a non-generic type error is found.
//
// The resulting string is in a minimalist format in a single line, making it
// more suitable for storage.
func (e Generic) Trace() string {
	var (
		trace strings.Builder
		err   = e
	)
	fmt.Fprintf(
		&trace, "%s:%d#%s",
		err.FileName, err.Line, err.FuncName,
	)
	for {
		nextErr, ok := err.Unwrap().(Generic)
		if !ok {
			break
		}
		fmt.Fprintf(
			&trace, "|%s:%d#%s",
			nextErr.FileName, nextErr.Line, nextErr.FuncName,
		)
		err = nextErr
	}
	return trace.String()
}

// newGeneric creates a Generic error. It is not meant to be called directly but
// from Wrap and WrapWithContext, otherwise the caller function information will
// be wrong
func newGeneric(err error, depth int, ctx interface{}) *Generic {
	if err == nil {
		panic("errors.wrap called with a nil err")
	}
	now := time.Now()
	pc, fn, line, _ := runtime.Caller(depth)
	return &Generic{
		ID:       id(now, err.Error()),
		err:      err,
		ts:       now,
		FuncName: runtime.FuncForPC(pc).Name(),
		FileName: fn,
		Line:     line,
		Context:  ctx,
	}
}

func WrapAndLog(err error) {
	log.Println(newGeneric(err, 2, nil))
}

func WrapAndLogWithContext(err error, ctx interface{}) {
	log.Println(newGeneric(err, 2, ctx))
}

func WrapFatal(err error) {
	log.Fatal(newGeneric(err, 2, nil))
}

func WrapFatalWithContext(err error, ctx interface{}) {
	log.Fatal(newGeneric(err, 2, ctx))
}

func UnwrapAll(err Generic) Generic {
	if nextErr, ok := err.Unwrap().(Generic); ok {
		return UnwrapAll(nextErr)
	}
	return err
}

func Wrap(err error) *Generic {
	return newGeneric(err, 2, nil)
}

func WrapWithContext(err error, ctx interface{}) *Generic {
	return newGeneric(err, 2, ctx)
}

// id takes a time, a message and returns the hashed id.
//
// id is not meant to be safe but fast, there is no salt and the hash algorithm
// is not cryptographically safe.
func id(t time.Time, msg string) string {
	unix := strconv.FormatInt(t.Unix(), 10)
	id := fnv64a([]byte(unix + msg))
	return base64.StdEncoding.EncodeToString([]byte(id))
}

func trimUntil(s string, del string, offset int) string {
	if i := strings.Index(s, del); i > 0 {
		return s[i+offset:]
	}
	return s
}

func trimUntilBackwards(s string, del string, offset int) string {
	if i := strings.LastIndex(s, del); i > 0 {
		return s[i+offset:]
	}
	return s
}

func fnv64a(b []byte) string {
	h := fnv.New64a()
	h.Write(b)
	return strconv.FormatUint(h.Sum64(), 10)
}

// Helpers so we don't have to import both packages

func New(msg string) error {
	return errors.New(msg)
}

func Is(err, target error) bool {
	return errors.Is(err, target)
}

func As(err error, target interface{}) bool {
	return errors.As(err, target)
}
