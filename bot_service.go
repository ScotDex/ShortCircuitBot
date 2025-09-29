package main

import (
	"fmt"
	"log"
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
	dg.AddHandler(s.interactionCreate)

	dg.Identify.Intents = discordgo.IntentsGuildMessages

	if err := dg.Open(); err != nil {
		log.Fatalf("[BOT] FATAL: Error opening connection: %v", err)
	}
	defer dg.Close()

	log.Println("✅ [BOT] Service is running. Press CTRL-C to exit.")
	<-quit
	log.Println("[BOT] Shutdown signal received, exiting.")
}

func (s *Service) ready(sess *discordgo.Session, event *discordgo.Ready) {
	log.Printf("[BOT] Logged in as: %v#%v\n", sess.State.User.Username, sess.State.User.Discriminator)

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
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "exclude",
					Description: "Comma-separated list of systems to avoid",
					Required:    false,
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

	excludeInput := ""
	if opt, exists := optionMap["exclude"]; exists {
		excludeInput = opt.StringValue()
	}

	startID, err1 := s.esiClient.GetSystemID(startName)
	endID, err2 := s.esiClient.GetSystemID(endName)

	var embed *discordgo.MessageEmbed
	if err1 != nil || err2 != nil {
		embed = &discordgo.MessageEmbed{
			Title:       "Error: Invalid System Name",
			Description: "Sorry, I couldn't recognise one of those system names. Please check for typos.",
			Color:       0xff0000,
		}
	} else {
		avoidList := make(map[int]bool)

		// Parse exclude systems and build avoid list
		if excludeInput != "" {
			excludeSystems := strings.Split(excludeInput, ",")
			for _, sysName := range excludeSystems {
				sysName = strings.TrimSpace(sysName)
				if sysName == "" {
					continue
				}
				sysID, err := s.esiClient.GetSystemID(sysName)
				if err != nil {
					log.Printf("[BOT] Warning: Unable to resolve system name for exclude: %s", sysName)
					continue
				}
				avoidList[sysID] = true
			}
		}

		// Optionally add default avoids here
		avoidList[30100000] = true // Example: Zarzakh

		// Search path with avoid list
		s.graphMutex.RLock()
		pathIDs := FindShortestPath(s.universeGraph, startID, endID, avoidList)
		s.graphMutex.RUnlock()

		if pathIDs == nil {
			embed = &discordgo.MessageEmbed{
				Description: fmt.Sprintf("No route possible between **%s** and **%s**.", startName, endName),
				Color:       0xff0000,
			}
		} else {
			// NEW: Loop through the path to get names AND kill data
			var pathWithKills []string
			for _, systemID := range pathIDs {
				systemName := fmt.Sprintf("Unknown (%d)", systemID)
				if sysInfo, err := s.esiClient.GetSystemDetails(systemID); err == nil {
					systemName = sysInfo.Name
				}

				// Fetch kill data and create a threat indicator
				var threatIndicator string
				if kills, err := s.esiClient.GetSystemKills(systemID); err == nil && len(kills) > 0 {
					shipKills := kills[0].ShipKills
					if shipKills >= 10 {
						threatIndicator = fmt.Sprintf("🔥 (%d)", shipKills) // High threat
					} else if shipKills > 0 {
						threatIndicator = fmt.Sprintf("⚠️ (%d)", shipKills) // Medium threat
					}
				}

				pathWithKills = append(pathWithKills, fmt.Sprintf("%s %s", systemName, threatIndicator))
			}

			routeString := fmt.Sprintf("> %s", strings.Join(pathWithKills, "\n> → "))
			jumpCount := len(pathIDs) - 1
			embedColor := 0x4CAF50
			if jumpCount > 10 {
				embedColor = 0xFFC107
			}
			if jumpCount > 20 {
				embedColor = 0xF44336
			}

			// List excluded system names for embed field
			excludedSysNames := []string{}
			for sysID := range avoidList {
				if sysInfo, err := s.esiClient.GetSystemDetails(sysID); err == nil {
					excludedSysNames = append(excludedSysNames, sysInfo.Name)
				}
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
					{
						Name:  "Excluded Systems",
						Value: strings.Join(excludedSysNames, ", "),
					},
				},
				Footer: &discordgo.MessageEmbedFooter{
					Text: "Zarzakh is always avoided. Kill data is for the last hour.",
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

// FindAndConvertPath can now be removed if it's no longer used elsewhere.
// I'll leave it here for now in case other parts of your code need it.
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
