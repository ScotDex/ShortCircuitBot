package main

import (
	"log"
	"os"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type Service struct {
	token string
}

func NewService(token string) *Service {
	return &Service{token: token}
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

	err := sess.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: "Planning Routes",
				Type: 0, // ActivityTypePlaying = 0
			},
		},
	})
	if err != nil {
		log.Printf("[BOT] ERROR: Failed to update status: %v", err)
	}
}

func (s *Service) messageCreate(sess *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == sess.State.User.ID {
		return
	}

	// Simple ping command to test responsiveness.
	if m.Content == "!ping" {
		_, err := sess.ChannelMessageSend(m.ChannelID, "Pong!")
		if err != nil {
			log.Printf("[BOT] ERROR: Failed to send pong message: %v", err)
		}
	}

	// TODO: Add /route command handler here. It will read from "tripwire_data.json".
}

func RouteFinder() {
	log.Println("--- Running Route Finder Test ---")

	// Step 1: Load the existing Tripwire data
	tripwireData, err := loadTripwireData("tripwire_data.json")
	if err != nil {
		log.Fatalf("Failed to load tripwire data: %v", err)
	}
	log.Printf("Loaded data with %d signatures and %d wormholes.", len(tripwireData.Signatures), len(tripwireData.Wormholes))

	// Step 2: Build the graph with both stargates and wormholes
	graph, err := GraphBuilder(tripwireData)
	if err != nil {
		log.Fatalf("Failed to build graph: %v", err)
	}

	// Step 3: Define start and end points and find the path
	startSystemID := 30002523 // An example system from your previous data
	endSystemID := 30002523   // The system ID for Jita

	log.Printf("Searching for a route from %d to %d...", startSystemID, endSystemID)
	path := FindShortestPath(graph, startSystemID, endSystemID)

	// Step 4: Display the result
	if path != nil {
		log.Printf("✅ Path found! It has %d jumps.", len(path)-1)
		log.Println("Route:", path)
	} else {
		log.Println("❌ No path could be found between the two systems.")
	}
}
