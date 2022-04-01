package bot

import (
	"context"

	"github.com/gocql/gocql"

	"github.com/hammertrack/tracker/internal/errors"
	"github.com/hammertrack/tracker/internal/message"
)

type Cassandra struct {
	s      *gocql.Session
	ctx    context.Context
	cancel context.CancelFunc
}

func (c *Cassandra) Close() error {
	// Cancel all queries
	c.cancel()
	// Close all sessions
	c.s.Close()
	return nil
}

func (c *Cassandra) Insert(msg *message.Message) {
	recent := msg.LastMessages

	// We cannot know whether it is sub with no messages in history
	sub := message.SubscribedStatusUnknown
	if len(recent) > 0 {
		sub = recent[0].Subscribed
	}

	msgs := make([]string, len(recent))
	for i, m := range recent {
		msgs[i] = m.Body
	}

	if err := c.s.Query(`INSERT INTO hammertrack.mod_messages_by_user_name (user_name, channel_name, at, messages, sub)
  VALUES (?, ?, ?, ?, ?)`, msg.Username, msg.Channel, msg.At, msgs, sub).
		WithContext(c.ctx).
		Exec(); err != nil {
		errors.WrapAndLog(err)
		return
	}
	// We don't care about atomicity for this use case. The overhead of a batch is
	// worse than a dangling user in by_channel_name table if the previous insert
	// fails
	if err := c.s.Query(`INSERT INTO hammertrack.mod_messages_by_channel_name (month, channel_name, user_name, at, messages, sub)
    VALUES (?, ?, ?, ?, ?, ?)`, msg.At.Month(), msg.Channel, msg.Username, msg.At, msgs, sub).
		WithContext(c.ctx).
		Exec(); err != nil {
		errors.WrapAndLog(err)
		return
	}
}

func (c *Cassandra) Channels() ([]Channel, error) {
	scanner := c.s.Query(`SELECT user_name FROM tracked_channels WHERE shard_id=1`).
		WithContext(c.ctx).
		Iter().
		Scanner()

	var (
		all = make([]Channel, 0, 20)
		err error
		ch  string
	)
	for scanner.Next() {
		if err = scanner.Scan(&ch); err != nil {
			return nil, errors.Wrap(err)
		}
		all = append(all, Channel(ch))
	}
	if err = scanner.Err(); err != nil {
		return nil, errors.Wrap(err)
	}
	return all, nil
}

func NewCassandraStorage(s *gocql.Session) Driver {
	// Instead of taking a ctx we create a new one and expose Close() because
	// some db drivers don't have contexts
	ctx, cancel := context.WithCancel(context.Background())
	return &Cassandra{s: s, ctx: ctx, cancel: cancel}
}
