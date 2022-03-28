package bot

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gempir/go-twitch-irc/v3"
	cfg "pedro.to/hammertrace/tracker/internal/config"
	"pedro.to/hammertrace/tracker/internal/errors"
	"pedro.to/hammertrace/tracker/internal/message"
)

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

var dummyMessage = &twitch.PrivateMessage{
	User: twitch.User{
		Name: "%invalid%",
	},
}

// noopPrivmsg is used as default
var noopPrivmsg = &PrivateMessage{
	ID:       "",
	Username: "%noop%",
	Body:     "",
}

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

// tracked is a hashtable which contains each go-channel for each twitch
// tracked channel
var tracked map[string]chan *Message

// handleClearChat is called when a new timeout or ban message is received
func handleClearChat(msg twitch.ClearChatMessage) {
	var (
		d   = msg.BanDuration
		ch  = msg.Channel
		typ = MessageTimeout
	)
	if d == 0 {
		typ = MessageBan
	}
	log.Printf("OnClearChat channel:%s duration:%d user:%s", ch, d, msg.TargetUsername)

	tracked[ch] <- &Message{
		Type:     typ,
		Duration: d,
		Username: msg.TargetUsername,
		Channel:  ch,
		At:       msg.Time,
	}
}

// handleClearChat is called when a new deletion is received
func handleClear(msg twitch.ClearMessage) {
	log.Printf("OnClear channel:%s user:%s", msg.Channel, msg.Login)
	tracked[msg.Channel] <- &Message{
		TargetMsgID: msg.TargetMsgID,
		Type:        MessageDeletion,
		Username:    msg.Login,
		Channel:     msg.Channel,
		At:          time.Now(),
	}
}

// handlePrivmsg is called when a new message in the twitch chat of any of the
// tracked twitch channels is received
func handlePrivmsg(msg twitch.PrivateMessage) {
	privmsg := &PrivateMessage{
		ID:       msg.ID,
		Username: msg.User.Name,
		Body:     msg.Message,
		At:       msg.Time,
	}
	tracked[msg.Channel] <- &Message{
		Type:         MessagePrivmsg,
		Username:     msg.User.Name,
		Channel:      msg.Channel,
		LastMessages: []*PrivateMessage{privmsg},
		At:           msg.Time,
	}
}

type Bot struct {
	sto Storage
	db  *sql.DB
	// client is the IRC Client
	client *twitch.Client
	// trackerReady is a channel for signaling when all the go-routine are spawned and
	// trackerReady to get messages
	trackerReady chan struct{}
	// ircReady is a channel for signaling when the IRC client is connected to the
	// server and listening for messages
	ircReady chan struct{}
	// done is a channel for signaling when all the go-routines spawned by Bot
	// have finished
	done chan struct{}
}

// StartClient initializes the IRC client and connects to the IRC server
func (b *Bot) StartClient(channels []string) error {
	b.client = twitch.NewClient(cfg.ClientUsername, cfg.ClientToken)
	b.client.OnClearChatMessage(handleClearChat)
	b.client.OnClearMessage(handleClear)
	b.client.OnPrivateMessage(handlePrivmsg)
	b.client.OnConnect(func() {
		b.ircReady <- struct{}{}
	})

	b.client.Join(channels...)
	if err := b.client.Connect(); err != nil {
		return err
	}
	return nil
}

// StartTracker initializes the channels tracker
func (b *Bot) StartTracker(channels []string) {
	var w sync.WaitGroup

	for _, ch := range channels {
		msgch := make(chan *Message, 100)
		tracked[ch] = msgch

		w.Add(1)
		go func(msgch chan *Message) {
			// history is scoped to each go-routine, per twitch channel.
			history := message.New(MaxHistory, noopPrivmsg)

			for msg := range msgch {
				switch msg.Type {
				case MessageBan:
					fallthrough
				case MessageTimeout:
					// find in the history previous messages related to the ban/timeout,
					// if the message is already `Stored` ignore it.
					spew.Dump(history.All())
					msg.LastMessages = history.Filter(func(privmsg *PrivateMessage) bool {
						if privmsg.Username == msg.Username && !privmsg.Stored {
							// mutate the message so we never store it again
							privmsg.Stored = true
							return true
						}
						return false
					})
					b.sto.Save(msg)
				case MessageDeletion:
					// find the message in the history with the corresponding ID, if the
					// message is already `Stored` ignore it. We could retrieve the body
					// of the message from the CLEARCHAT message but then we couldn't
					// figure out the time span between the message and the deletion
					privmsg := history.Find(func(privmsg *PrivateMessage) bool {
						if privmsg.ID == msg.TargetMsgID && !privmsg.Stored {
							privmsg.Stored = true
							return true
						}
						return false
					})
					if privmsg != nil {
						msg.LastMessages = []*PrivateMessage{privmsg}
						b.sto.Save(msg)
					}
				case MessagePrivmsg:
					// extend the history with the received message
					history = history.Append(msg.LastMessages[0])
				}
			}
			w.Done()
		}(msgch)
	}
	// Signal that we spawned all the go-routines and are ready to start receiving
	// messages
	b.trackerReady <- struct{}{}
	w.Wait()
	// Signal that all go-routines are finished
	b.done <- struct{}{}
}

func (b *Bot) Start() {
	var w sync.WaitGroup
	log.Print("initializing storage...")
	b.SetStorage(NewPostgresStorage())
	chs := b.sto.OptimizeChannels()
	w.Add(1)
	go func() {
		b.sto.Start()
		log.Print("storage stopped ")
		w.Done()
	}()

	log.Print("initializing channel tracker...")
	w.Add(1)
	go func(chs []string) {
		b.StartTracker(chs)
		w.Done()
	}(chs)
	<-b.trackerReady
	log.Print("tracker ready")

	log.Print("initializing IRC client...")
	w.Add(1)
	go func(chs []string) {
		if err := b.StartClient(chs); err != nil {
			if !errors.Is(err, twitch.ErrClientDisconnected) {
				errors.WrapFatal(err)
			}
		}
		w.Done()
	}(chs)
	<-b.ircReady
	log.Print("connected to IRC server")

	w.Wait()
}

func (b *Bot) SetStorage(sto Storage) {
	b.sto = sto
}

func (b *Bot) Stop() error {
	// Stop IRC Client
	log.Print("stopping IRC client")
	if err := b.client.Disconnect(); err != nil {
		return err
	}
	log.Print("IRC client stopped")

	// Close all channels
	log.Print("stopping tracker")
	for _, ch := range tracked {
		close(ch)
	}
	// Wait for all the go-routines spawned by the bot to finish
	<-b.done
	log.Print("tracker stopped")

	// Gracefully close storage and underlying database
	log.Print("stopping storage")
	if err := b.sto.Stop(); err != nil {
		return err
	}
	log.Print("storage stopped")

	return nil
}

func New() *Bot {
	b := &Bot{
		trackerReady: make(chan struct{}, 1),
		ircReady:     make(chan struct{}, 1),
		done:         make(chan struct{}, 1),
	}
	return b
}

func init() {
	tracked = make(map[string]chan *Message)
}
