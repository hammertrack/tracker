package bot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hammertrack/tracker/errors"
	"github.com/hammertrack/tracker/internal/heuristics"
	"github.com/hammertrack/tracker/internal/message"
)

const (
	QueueSize = 200

	// Exclusive minimum duration for storing timeout messages
	MinTimeoutDuration = 5
	// Exclusive minimum number of seconds that has to happen for the moderation
	// to be considered human
	MinHumanlyPossible float64 = .9
)

var ErrUncachedChannels = errors.New("Postgres storage layer requires to be called with OptimizeChannels() before starting")

type Driver interface {
	Insert(msg *message.Message)
	Channels() ([]Channel, error)
	Close() error
}

type Storage struct {
	queue  chan *message.Message
	ctx    context.Context
	cancel context.CancelFunc
	driver Driver
}

func (s *Storage) Start() {
	for {
		select {
		case msg := <-s.queue:
			s.driver.Insert(msg)
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Storage) Stop() {
	s.cancel()
	s.driver.Close()
}

func (s *Storage) Save(msg *message.Message) {
	s.driver.Insert(msg)
}

func (s *Storage) Channels() ([]Channel, error) {
	return s.driver.Channels()
}

func NewStorage(d Driver) *Storage {
	ctx, cancel := context.WithCancel(context.Background())
	return &Storage{
		ctx:    ctx,
		cancel: cancel,
		queue:  make(chan *message.Message, QueueSize),
		driver: d,
	}
}

type OpType int

const (
	OpInsert OpType = iota + 1
)

type Op struct {
	typ      OpType
	banType  string
	username string
	channel  string
	duration int
	messages string
	at       time.Time
}

type Postgres struct {
	db *sql.DB
	// note: op may be a bottleneck in the future as it receives the database
	// insertions from all the channels and all the goroutines will block until
	// their messages are handled in the op channel, it could need multiple
	// workers in different threads to be able to handle the overload.
	//
	// An idea for the future: first if the bottleneck starts being noticeable in
	// latency increase the queue size (and maybe the RAM of the instance). if it
	// consumes too much ram or the queue is delaying the insertions of the
	// messages too much then start scaling by splitting the logic of the op
	// channel into workers, one per core in the instance (maybe increase the
	// cores in the instance) and one database connection per instance. If the
	// number of ops is too high, then scale by creating more instances, dividing
	// the number of channels tracked between all the instances. The database may
	// also need different partitions and instances.
	op       chan *Op
	queue    []*Op
	maxqueue int
	size     int
	ctx      context.Context
	cancel   context.CancelFunc
	// In memory hash-table to cache and map twitch channels to internal ids to
	// avoid querying in every query
	chanIds  map[string]int
	analyzer *heuristics.Analyzer
}

type Channel string

const sep = "|"

// replacer is safe for concurrent use
var replacer = strings.NewReplacer(sep, "\\"+sep)

func (sto *Postgres) Save(msg *message.Message) {
	var (
		sb     strings.Builder
		logmsg strings.Builder
		t      = heuristics.Traits{}
	)
	if len(msg.LastMessages) > 0 {
		privmsg := msg.LastMessages[0]
		logmsg.WriteString(fmt.Sprintf("%s: %s; T-%f", msg.Username, privmsg.Body, msg.At.Sub(msg.LastMessages[0].At).Seconds()))
	}

	// flag to identify most recent message (=msg.LastMessages[0])
	t.IsMostRecentMsg = true
	for _, privmsg := range msg.LastMessages {
		// reuse trait object for every recent message
		t.Body = privmsg.Body
		t.At = privmsg.At
		t.ModeratedAt = msg.At
		t.Type = msg.Type
		t.TimeoutDuration = msg.Duration
		if !sto.analyzer.IsCompliant(t) {
			// if a single message of all the ones cleared is not compliant, abort
			return
		}
		t.IsMostRecentMsg = false

		sb.WriteString(replacer.Replace(privmsg.Body) + sep)
	}
	str := sb.String()
	if len(str) > 0 {
		// Trim last sep
		str = str[:len(str)-1]
	}

	sto.op <- &Op{
		typ:      OpInsert,
		banType:  string(msg.Type),
		username: msg.Username,
		channel:  msg.Channel,
		duration: msg.Duration,
		messages: str,
		at:       msg.At,
	}
	logmsg.WriteString(" [S]")
	log.Print(logmsg.String())
}
