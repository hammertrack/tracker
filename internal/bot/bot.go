package bot

import (
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/gempir/go-twitch-irc/v3"
	cfg "pedro.to/hammertrace/tracker/internal/config"
	"pedro.to/hammertrace/tracker/internal/database"
	"pedro.to/hammertrace/tracker/internal/errors"
	"pedro.to/hammertrace/tracker/internal/message"
)

// noopPrivmsg is used as default
var noopPrivmsg = &message.PrivateMessage{
	ID:       "",
	Username: "%noop%",
	Body:     "",
}

// tracked is a hashtable which contains each go-channel for each twitch
// tracked channel
var tracked map[string]chan *message.Message

// handleClearChat is called when a new timeout or ban message is received
func handleClearChat(msg twitch.ClearChatMessage) {
	var (
		d        = msg.BanDuration
		ch       = msg.Channel
		typ      = message.MessageBan
		username = msg.TargetUsername
	)
	if username == "" {
		// ignore a CLEARCHAT of all messages with no specific user
		return
	}
	if d != 0 {
		// ignore everything but bans
		return
	}

	log.Printf("->[#%s] :%s", msg.Channel, msg.TargetUsername)
	tracked[ch] <- &message.Message{
		Type:     typ,
		Duration: d,
		Username: msg.TargetUsername,
		Channel:  ch,
		At:       msg.Time,
	}
}

// handleClearChat is called when a new deletion is received
func handleClear(msg twitch.ClearMessage) {
	tracked[msg.Channel] <- &message.Message{
		TargetMsgID: msg.TargetMsgID,
		Type:        message.MessageDeletion,
		Username:    msg.Login,
		Channel:     msg.Channel,
		At:          time.Now(),
	}
}

// handlePrivmsg is called when a new message in the twitch chat of any of the
// tracked twitch channels is received
func handlePrivmsg(msg twitch.PrivateMessage) {
	sub, _ := strconv.Atoi(msg.Tags["suscriber"])
	privmsg := &message.PrivateMessage{
		ID:         msg.ID,
		Username:   msg.User.Name,
		Body:       msg.Message,
		At:         msg.Time,
		Subscribed: message.SubscribedStatus(sub),
	}
	tracked[msg.Channel] <- &message.Message{
		Type:         message.MessagePrivmsg,
		Username:     msg.User.Name,
		Channel:      msg.Channel,
		LastMessages: []*message.PrivateMessage{privmsg},
		At:           msg.Time,
	}
}

type Bot struct {
	sto *Storage
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
func (b *Bot) StartClient(channels []Channel) error {
	b.client = twitch.NewClient(cfg.ClientUsername, cfg.ClientToken)
	b.client.OnClearChatMessage(handleClearChat)
	// b.client.OnClearMessage(handleClear)
	b.client.OnPrivateMessage(handlePrivmsg)
	b.client.OnConnect(func() {
		b.ircReady <- struct{}{}
	})

	for _, ch := range channels {
		b.client.Join(string(ch))
	}

	if err := b.client.Connect(); err != nil {
		return err
	}
	return nil
}

// StartTracker initializes the channels tracker
func (b *Bot) StartTracker(channels []Channel) {
	var w sync.WaitGroup

	for _, ch := range channels {
		msgch := make(chan *message.Message, 100)
		tracked[string(ch)] = msgch

		w.Add(1)
		go func(msgch chan *message.Message) {
			// history is scoped to each go-routine, per twitch channel.
			history := message.New(message.MaxHistory, noopPrivmsg)

			for msg := range msgch {
				switch msg.Type {
				case message.MessageBan:
					fallthrough
				case message.MessageTimeout:
					// find in the history previous messages related to the ban/timeout,
					// if the message is already `Stored` ignore it.
					msg.LastMessages = history.Filter(func(privmsg *message.PrivateMessage) bool {
						if privmsg.Username == msg.Username && !privmsg.Stored {
							// mutate the message so we never store it again
							privmsg.Stored = true
							return true
						}
						return false
					})
					b.sto.Save(msg)
				case message.MessageDeletion:
					// find the message in the history with the corresponding ID, if the
					// message is already `Stored` ignore it. We could retrieve the body
					// of the message from the CLEARCHAT message but then we couldn't
					// figure out the time span between the message and the deletion
					privmsg := history.Find(func(privmsg *message.PrivateMessage) bool {
						if privmsg.ID == msg.TargetMsgID && !privmsg.Stored {
							privmsg.Stored = true
							return true
						}
						return false
					})
					if privmsg != nil {
						msg.LastMessages = []*message.PrivateMessage{privmsg}
						b.sto.Save(msg)
					}
				case message.MessagePrivmsg:
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
	sess := database.New(cfg.DBMigrate)
	driver := NewCassandraStorage(sess)
	b.SetStorage(NewStorage(driver))
	w.Add(1)
	go func() {
		b.sto.Start()
		w.Done()
	}()

	chs, err := b.sto.Channels()
	if err != nil {
		errors.WrapFatal(err)
	}
	log.Printf("channels about to be tracked: %v", chs)
	log.Print("initializing channel tracker...")
	w.Add(1)
	go func(chs []Channel) {
		b.StartTracker(chs)
		w.Done()
	}(chs)
	<-b.trackerReady
	log.Print("tracker ready")

	log.Print("initializing IRC client...")
	w.Add(1)
	go func(chs []Channel) {
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

func (b *Bot) SetStorage(sto *Storage) {
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
	b.sto.Stop()
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
	tracked = make(map[string]chan *message.Message)
}
