package bot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/lib/pq"
	cfg "pedro.to/hammertrace/tracker/internal/config"
	"pedro.to/hammertrace/tracker/internal/database"
	"pedro.to/hammertrace/tracker/internal/errors"
	"pedro.to/hammertrace/tracker/internal/heuristics"
	"pedro.to/hammertrace/tracker/internal/message"
)

const (
	// FlushInterval is the time span between flushes if the queue is not full
	FlushInterval = 5
	// Size of the database insert/etc. batches
	OpQueueSize = 100
	// Exclusive minimum duration for storing timeout messages
	MinTimeoutDuration = 5
	// Exclusive minimum number of seconds that has to happen for the moderation
	// to be considered human
	MinHumanlyPossible float64 = .9
)

var ErrUncachedChannels = errors.New("Postgres storage layer requires to be called with OptimizeChannels() before starting")

type Storage interface {
	Start()
	Stop() error
	Channels() []Channel
	Save(msg *message.Message)
	OptimizeChannels() []string
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

type Channel struct {
	ID   int    `db:"broadcaster.broadcaster_id"`
	Name string `db:"broadcaster.name"`
}

func (sto *Postgres) Channels() []Channel {
	all := make([]Channel, 0, 100)
	rows, err := sto.db.Query("SELECT broadcaster_id, name FROM broadcaster")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return all
		}
		errors.WrapFatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var ch Channel
		if err := rows.Scan(&ch.ID, &ch.Name); err != nil {
			errors.WrapFatal(err)
		}
		all = append(all, ch)
	}
	if err = rows.Err(); err != nil {
		errors.WrapFatal(err)
	}
	return all
}

// OptimizeChannels queries for the tracked channels, caches them into a map of
// name to id for later use and returns a slice with the names of all the
// tracked channels, useful for use outside the storage.
func (sto *Postgres) OptimizeChannels() []string {
	chs := sto.Channels()

	strs := make([]string, len(chs))
	sto.chanIds = make(map[string]int, len(chs))
	for i, ch := range chs {
		strs[i] = ch.Name
		sto.chanIds[ch.Name] = ch.ID
	}
	return strs
}

// Start initializes the storage batcher
func (sto *Postgres) Start() {
	if len(sto.chanIds) == 0 {
		errors.WrapFatal(ErrUncachedChannels)
	}

	ticker := time.NewTicker(FlushInterval * time.Second)
	for {
		// flush the query queue when it is full or every ticker seconds, whichever
		// comes first
		select {
		case op := <-sto.op:
			if size := sto.enqueue(op); size >= sto.maxqueue {
				sto.flush()
			}
		case <-ticker.C:
			sto.flush()
		case <-sto.ctx.Done():
			log.Print("Stopping tracking")
			ticker.Stop()
			return
		}
	}
}

func (sto *Postgres) Stop() error {
	sto.cancel()
	// flush any remaining op in the queue before closing the connection
	sto.flush()
	if err := sto.db.Close(); err != nil {
		return err
	}
	return nil
}

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

func (sto *Postgres) enqueue(op *Op) int {
	sto.queue[sto.size] = op
	sto.size++
	return sto.size
}

func (sto *Postgres) flush() {
	if sto.size <= 0 {
		sto.size = 0
		return
	}

	tx, err := sto.db.Begin()
	if err != nil {
		errors.WrapFatal(err)
	}

	stmt, err := tx.Prepare(
		pq.CopyIn("clearchat", "type", "username", "channel_name", "channel_id", "duration", "at", "messages"),
	)

	for i, l := 0, sto.size; i < l; i++ {
		op := sto.queue[i]
		_, err = stmt.Exec(
			op.banType,
			op.username,
			op.channel,
			sto.chanIds[op.channel],
			op.duration,
			op.at,
			op.messages,
		)
		if err != nil {
			errors.WrapFatal(err)
		}
	}

	if _, err = stmt.Exec(); err != nil {
		errors.WrapFatal(err)
	}

	if err = stmt.Close(); err != nil {
		errors.WrapFatal(err)
	}

	if err = tx.Commit(); err != nil {
		errors.WrapFatal(err)
	}

	sto.size = 0
}

func NewPostgresStorage() Storage {
	return NewPostgresStorageWithCancel(
		context.WithCancel(context.Background()),
	)
}

func NewPostgresStorageWithCancel(
	ctx context.Context,
	cancel context.CancelFunc,
) Storage {
	// rules for storing messages
	a := heuristics.New([]heuristics.Rule{
		heuristics.RuleAlwaysStoreBans(),
		heuristics.RuleOnlyHumanModerations(MinHumanlyPossible),
		heuristics.RuleMinTimeoutDuration(MinTimeoutDuration),
		heuristics.RuleNoLinks(),
	})
	a.Compile()
	return &Postgres{
		analyzer: a,
		db:       database.New(cfg.DBMigrate),
		ctx:      ctx,
		cancel:   cancel,
		op:       make(chan *Op, OpQueueSize),
		queue:    make([]*Op, OpQueueSize),
		maxqueue: OpQueueSize,
	}
}
