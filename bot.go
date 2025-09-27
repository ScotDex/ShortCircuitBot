package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// --- Service Definition ---

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

// --- Service Lifecycle ---

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
		avoidList[30100000] = true // Zarzakh's System ID
		s.graphMutex.RLock()
		pathNames := FindAndConvertPath(s.universeGraph, startID, endID, s.esiClient, avoidList)
		s.graphMutex.RUnlock()

		if pathNames == nil {
			embed = &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("Route Not Found"),
				Description: fmt.Sprintf("No path could be found between **%s** and **%s**.", startName, endName),
				Color:       0xff0000, // Red
			}
		} else {
			embed = &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("Route from %s to %s", startName, endName),
				Description: fmt.Sprintf("`%s`", strings.Join(pathNames, " → ")),
				Color:       0x00ff00, // Green
				Footer: &discordgo.MessageEmbedFooter{
					Text: fmt.Sprintf("%d jumps", len(pathNames)-1),
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
