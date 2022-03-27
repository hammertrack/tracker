package bot

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/gempir/go-twitch-irc/v3"
	cfg "pedro.to/hammertrace/tracker/internal/config"
	"pedro.to/hammertrace/tracker/internal/errors"
	"pedro.to/hammertrace/tracker/internal/message"
)

type MessageType string

const (
	MessageChat     MessageType = "chat"
	MessageBan      MessageType = "ban"
	MessageTimeout  MessageType = "timeout"
	MessageDeletion MessageType = "deletion"
)

var dummyMessage = &twitch.PrivateMessage{
	User: twitch.User{
		Name: "%invalid%",
	},
}

// Message represents a message coming from the IRC client. It standarizes the
// different MessageType types of messages in a common interface
type Message struct {
	Type MessageType
	// Channel represents the twitch channel
	Channel string
	// Username represents the owner of the message
	Username string
	// Duration represents in seconds the timeout. Duration is only present for
	// messafe of type MessageTimeout and MessageBan
	Duration int
	// Original contains the underlying message from the IRC client. Original is only
	// present for messages of type MessageChat
	Original *twitch.PrivateMessage
	// DeletedMessage contains the message for MessageDeletion type
	DeletedMessage string
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
	tracked[msg.Channel] <- &Message{
		Type:           MessageDeletion,
		Username:       msg.Login,
		Channel:        msg.Channel,
		DeletedMessage: msg.Message,
		At:             time.Now(),
	}
}

// handleChat is called when a new message in the twitch chat of any of the
// tracked twitch channels is received
func handleChat(msg twitch.PrivateMessage) {
	tracked[msg.Channel] <- &Message{
		Type:     MessageChat,
		Username: msg.User.Name,
		Channel:  msg.Channel,
		Original: &msg,
		At:       msg.Time,
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
	b.client.OnPrivateMessage(handleChat)
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
			history := message.New(100, dummyMessage)

			for msg := range msgch {
				switch msg.Type {
				case MessageBan:
					fallthrough
				case MessageTimeout:
					b.sto.Save(msg, history.Filter(func(el *twitch.PrivateMessage) bool {
						return el.User.Name == msg.Username
					}))
				case MessageDeletion:
					b.sto.SaveOne(msg)
				case MessageChat:
					// extend the history with the received message
					history = history.Append(msg.Original)
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
	w.Add(1)
	go func() {
		b.sto.Start()
		w.Done()
	}()

	chs := b.Channels()

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

func (b *Bot) Channels() []string {
	return b.sto.Channels()
}

func New() *Bot {
	b := &Bot{
		trackerReady: make(chan struct{}, 1),
		ircReady:     make(chan struct{}, 1),
		done:         make(chan struct{}, 1),
	}
	b.SetStorage(NewPostgresStorage())
	return b
}

func init() {
	tracked = make(map[string]chan *Message)
}
