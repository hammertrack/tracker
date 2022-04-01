package main

import (
	"embed"
	"fmt"
	"log"

	"github.com/hammertrack/tracker/internal/config"
	"github.com/hammertrack/tracker/utils"
)

//go:embed banner.txt
var f embed.FS

// It's a stupid banner with a stupid useless embed file, but it's my stupid banner
func printBanner() {
	b, err := f.ReadFile("banner.txt")
	if err != nil {
		panic(err)
	}
	fmt.Print(utils.ByteToStr(b))
	fmt.Printf("v%s\n\n", config.Version)
	log.Print("Initializing server tracker...")
}
