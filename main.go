package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/davecgh/go-spew/spew"

	"pedro.to/hammertrace/tracker/internal/bot"
	"pedro.to/hammertrace/tracker/internal/logger"
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
