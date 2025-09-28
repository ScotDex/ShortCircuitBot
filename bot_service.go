package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Service struct {
	token         string
	universeGraph map[int][]int
	graphMutex    *sync.RWMutex
	esiClient     *ESIClient
}

func NewService(token string, graph map[int][]int, mutex *sync.RWMutex, esi *ESIClient) *Service {
	return &Service{
		token:         token,
		universeGraph: graph,
		graphMutex:    mutex,
		esiClient:     esi,
	}
}

func (s *Service) Start(wg *sync.WaitGroup, quit chan os.Signal) {
	defer wg.Done()
	log.Println("[BOT] Starting service...")

	dg, err := discordgo.New("Bot " + s.token)
	if err != nil {
		log.Fatalf("[BOT] FATAL: Unable to create Discord session: %v", err)
	}

	dg.AddHandler(s.ready)
	dg.AddHandler(s.interactionCreate) // We only need the interaction handler now

	dg.Identify.Intents = discordgo.IntentsGuildMessages

	if err := dg.Open(); err != nil {
		log.Fatalf("[BOT] FATAL: Error opening connection: %v", err)
	}
	defer dg.Close()

	log.Println("✅ [BOT] Service is running. Press CTRL-C to exit.")
	<-quit
	log.Println("[BOT] Shutdown signal received, exiting.")
}

// --- Discord Event Handlers ---

func (s *Service) ready(sess *discordgo.Session, event *discordgo.Ready) {
	log.Printf("[BOT] Logged in as: %v#%v\n", sess.State.User.Username, sess.State.User.Discriminator)

	// Define and register the /route slash command
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "route",
			Description: "Calculates the shortest route between two solar systems.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "start",
					Description: "The starting solar system.",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "end",
					Description: "The destination solar system.",
					Required:    true,
				},
			},
		},
	}

	_, err := sess.ApplicationCommandBulkOverwrite(sess.State.User.ID, "", commands)
	if err != nil {
		log.Fatalf("[BOT] FATAL: Could not register slash commands: %v", err)
	}
	log.Println("[BOT] Slash commands registered.")
}

func (s *Service) interactionCreate(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "route" {
		return
	}

	err := sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[BOT] ERROR: Failed to defer interaction response: %v", err)
		return
	}

	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	startName := optionMap["start"].StringValue()
	endName := optionMap["end"].StringValue()

	startID, err1 := s.esiClient.GetSystemID(startName)
	endID, err2 := s.esiClient.GetSystemID(endName)

	var embed *discordgo.MessageEmbed
	if err1 != nil || err2 != nil {
		embed = &discordgo.MessageEmbed{
			Title:       "Error: Invalid System Name",
			Description: "Sorry, I couldn't recognise one of those system names. Please check for typos.",
			Color:       0xff0000, // Red
		}
	} else {

		avoidList := make(map[int]bool)
		avoidList[30100000] = true
		s.graphMutex.RLock()
		pathNames := FindAndConvertPath(s.universeGraph, startID, endID, s.esiClient, avoidList) // This line was already correct
		s.graphMutex.RUnlock()

		if pathNames == nil {
			embed = &discordgo.MessageEmbed{

				Description: fmt.Sprintf("No shortcut possible between **%s** and **%s**.", startName, endName),
				Color:       0xff0000, // Red
			}

		} else {
			// Format the path with a block quote for better readability
			routeString := fmt.Sprintf("> %s", strings.Join(pathNames, "\n> → "))

			// Choose a color based on the number of jumps
			jumpCount := len(pathNames) - 1
			embedColor := 0x4CAF50 // Green for short routes
			if jumpCount > 10 {
				embedColor = 0xFFC107 // Amber for medium routes
			}
			if jumpCount > 20 {
				embedColor = 0xF44336 // Red for long routes
			}

			embed = &discordgo.MessageEmbed{
				Author: &discordgo.MessageEmbedAuthor{
					Name:    "ShortCircuit Route Planner",
					IconURL: "https://images.evetech.net/corporations/98330748/logo?size=64",
				},
				Title:       "Route Calculated",
				Color:       embedColor,
				Timestamp:   time.Now().Format(time.RFC3339),
				Description: routeString,
				Fields: []*discordgo.MessageEmbedField{
					{
						Name:   "Start",
						Value:  startName,
						Inline: true,
					},
					{
						Name:   "End",
						Value:  endName,
						Inline: true,
					},
					{
						Name:   "Jumps",
						Value:  fmt.Sprintf("%d", jumpCount),
						Inline: true,
					},
				},
				Footer: &discordgo.MessageEmbedFooter{
					Text: "Zarzakh is currently avoided.",
				},
			}
		}
	}

	_, err = sess.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		log.Printf("[BOT] ERROR: Failed to send webhook edit: %v", err)
	}
}

// --- Helper Functions ---

func FindAndConvertPath(graph map[int][]int, startID, endID int, esi *ESIClient, avoidList map[int]bool) []string {
	pathIDs := FindShortestPath(graph, startID, endID, avoidList)
	if pathIDs == nil {
		return nil
	}

	var pathNames []string
	for _, id := range pathIDs {
		if sysInfo, err := esi.GetSystemDetails(id); err == nil {
			pathNames = append(pathNames, sysInfo.Name)
		} else {
			pathNames = append(pathNames, fmt.Sprintf("Unknown (%d)", id))
		}
	}
	return pathNames
}

func startHealthCheckServer() {

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Short Circuit Bot is running!")
	})

	log.Printf("Health check server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start health check server: %v", err)
	}
}
