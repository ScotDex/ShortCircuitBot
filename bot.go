package main

import (
	"log"
	"os"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type TokenService struct {
	token string
}

func NewService(token string) *Service {
	return &TokenService{token: token}
}

// Start connects to Discord and begins listening for commands.
func (s *Service) Start(wg *sync.WaitGroup, quit chan os.Signal) {
	defer wg.Done()
	log.Println("[BOT] Starting service...")

	dg, err := discordgo.New("Bot " + s.token)
	if err != nil {
		log.Fatalf("[BOT] FATAL: Unable to create Discord session: %v", err)
	}

	dg.AddHandler(s.ready)
	dg.AddHandler(s.messageCreate)

	dg.Identify.Intents = discordgo.IntentsGuildMessages

	if err := dg.Open(); err != nil {
		log.Fatalf("[BOT] FATAL: Error opening connection: %v", err)
	}
	defer dg.Close()

	log.Println("✅ [BOT] Service is running. Press CTRL-C to exit.")

	// Wait for a shutdown signal.
	<-quit
	log.Println("[BOT] Shutdown signal received, exiting.")
}

func (s *Service) ready(sess *discordgo.Session, event *discordgo.Ready) {
	log.Printf("[BOT] Logged in as: %v#%v\n", sess.State.User.Username, sess.State.User.Discriminator)
	sess.UpdateGameStatus(0, "Planning Routes")
}

func (s *Service) messageCreate(sess *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == sess.State.User.ID {
		return
	}

	// Simple ping command to test responsiveness.
	if m.Content == "!ping" {
		sess.ChannelMessageSend(m.ChannelID, "Pong!")
	}

	// TODO: Add /route command handler here. It will read from "tripwire_data.json".
}
