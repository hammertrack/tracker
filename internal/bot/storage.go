package bot

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gempir/go-twitch-irc/v3"
	"github.com/lib/pq"
	cfg "pedro.to/hammertrace/tracker/internal/config"
	"pedro.to/hammertrace/tracker/internal/database"
	"pedro.to/hammertrace/tracker/internal/errors"
)

type Storage interface {
	Start()
	Stop() error
	Channels() []string
	Save(msg *Message, recent []*twitch.PrivateMessage)
	SaveOne(msg *Message)
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
}

func (sto *Postgres) Channels() []string {
	all := make([]string, 0, 100)
	rows, err := sto.db.Query("SELECT name FROM broadcaster")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return all
		}
		errors.WrapFatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			errors.WrapFatal(err)
		}
		all = append(all, name)
	}
	if err = rows.Err(); err != nil {
		errors.WrapFatal(err)
	}
	return all
}

// Start initializes the storage batcher
func (sto *Postgres) Start() {
	ticker := time.NewTicker(5 * time.Second)
	for {
		// flush the query queue when it is full or every ticker seconds, whichever
		// comes first
		select {
		case op := <-sto.op:
			spew.Dump(op)
			if size := sto.enqueue(op); size >= sto.maxqueue {
				sto.flush()
			}
		case <-ticker.C:
			sto.flush()
		case <-sto.ctx.Done():
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

func (sto *Postgres) Save(msg *Message, recent []*twitch.PrivateMessage) {
	var sb strings.Builder
	for _, msg := range recent {
		sb.WriteString(replacer.Replace(msg.Message) + sep)
	}
	str := sb.String()

	sto.op <- &Op{
		typ:      OpInsert,
		banType:  string(msg.Type),
		username: msg.Username,
		channel:  msg.Channel,
		duration: msg.Duration,
		messages: str[:len(str)-1],
		at:       msg.At,
	}
}

func (sto *Postgres) SaveOne(msg *Message) {
	sto.op <- &Op{
		typ:      OpInsert,
		banType:  string(msg.Type),
		username: msg.Username,
		channel:  msg.Channel,
		duration: 0,
		messages: msg.DeletedMessage,
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
		pq.CopyIn("clearchat", "type", "username", "channel", "duration", "at", "messages"),
	)

	for i, l := 0, sto.size; i < l; i++ {
		op := sto.queue[i]
		_, err = stmt.Exec(op.banType, op.username, op.channel, op.duration, op.at, op.messages)
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
		queue:    make([]*Op, maxqueue),
		maxqueue: maxqueue,
	}
}
