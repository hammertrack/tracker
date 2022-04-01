package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/davecgh/go-spew/spew"

	"github.com/hammertrack/tracker/internal/bot"
	"github.com/hammertrack/tracker/logger"
)

func waitSignInt() {
	sigint := make(chan os.Signal, 1)
	signal.Notify(
		sigint,
		os.Interrupt,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGABRT,
		syscall.SIGQUIT,
	)
	<-sigint
	log.Print("Stopping hammertrack tracker")
}

// TODO - Clean and re-structure some logs
// TODO - Tests
// TODO - Rename everything from hammertrace to hammertrack
func main() {
	b := bot.New()
	go func() {
		b.Start()
	}()
	waitSignInt()
	b.Stop()
}

func init() {
	spew.Config.Indent = "\t"
	log.SetFlags(0)
	log.SetOutput(logger.New())
	printBanner()
}
