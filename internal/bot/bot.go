// bot.go
package bot

import (
	"firehawk/internal/tripwire"

	"github.com/bwmarrin/discordgo"
)

// Bot represents the Discord bot application.
type Bot struct {
	session *discordgo.Session
	scraper *tripwire.Scraper
}

// New creates a new Bot instance.
func New(token string, scraper *tripwire.Scraper) (*Bot, error) {
	// TODO: Create Discord session, add handlers
	// Handlers will call methods defined in commands.go
	return &Bot{scraper: scraper}, nil
}

// Run opens the connection and waits for a shutdown signal.
func (b *Bot) Run() {
	// TODO: Open the Discord session and handle graceful shutdown
}
