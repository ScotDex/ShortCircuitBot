// main.go
package main

import (
	"firehawk/internal/bot"
	"firehawk/internal/config"
	"firehawk/internal/tripwire"
	"fmt"
)

func main() {
	// 1. Load configuration from environment variables
	cfg, err := config.Load()
	if err != nil {
		panic("could not load configuration")
	}

	// 2. Create a new Tripwire scraper instance
	scraper := tripwire.NewScraper(cfg.TripwireUser, cfg.TripwirePass)

	// 3. Create and start the bot, passing it the scraper and token
	firehawkBot, err := bot.New(cfg.BotToken, scraper)
	if err != nil {
		panic("could not create bot")
	}

	fmt.Println("Starting Firehawk Bot...")
	firehawkBot.Run()
}
