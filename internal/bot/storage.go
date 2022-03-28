package bot

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	"github.com/lib/pq"
	cfg "pedro.to/hammertrace/tracker/internal/config"
	"pedro.to/hammertrace/tracker/internal/database"
	"pedro.to/hammertrace/tracker/internal/errors"
)

const (
	// FlushInterval is the time span between flushes if the queue is not full
	FlushInterval = 5
)

var ErrUncachedChannels = errors.New("Postgres storage layer requires to be called with OptimizeChannels() before starting")

type Storage interface {
	Start()
	Stop() error
	Channels() []Channel
	Save(msg *Message)
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
	db       *sql.DB
	op       chan *Op
	queue    []*Op
	maxqueue int
	size     int
	ctx      context.Context
	cancel   context.CancelFunc
	// In memory hash-table to cache and map twitch channel to internal ids, to
	// avoid querying for them in every query
	chanIds map[string]int
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

var replacer = strings.NewReplacer(sep, "\\"+sep)

func (sto *Postgres) Save(msg *Message) {
	var sb strings.Builder
	for _, privmsg := range msg.LastMessages {
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
	maxqueue := 100
	return &Postgres{
		db:       database.New(cfg.DBMigrate),
		ctx:      ctx,
		cancel:   cancel,
		op:       make(chan *Op, maxqueue),
		queue:    make([]*Op, maxqueue),
		maxqueue: maxqueue,
	}
}
