package main

import (
	"encoding/json"
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

func (s *Service) Start(wg *sync.WaitGroup, quit chan struct{}) {
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
				{Type: discordgo.ApplicationCommandOptionString, Name: "start", Description: "The starting solar system.", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "end", Description: "The destination solar system.", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "exclude", Description: "Comma-separated list of systems to avoid", Required: false},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "preference",
					Description: "The type of route to prefer.",
					Required:    false,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{
							Name:  "Safest (High-Sec first)",
							Value: "safer",
						},
						{
							Name:  "Unsafe (Low/Null-Sec first)",
							Value: "unsafe",
						},
						{
							Name:  "Shortest (Default)",
							Value: "shortest",
						},
					},
				},
			}, // <-- Missing brace for the Options slice
		}, // <-- Missing brace for the ApplicationCommand struct
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
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
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

	// --- ADD THIS BLOCK HERE ---
	// Get the user's preference, defaulting to "shortest" if not provided.
	preference := "shortest"
	if opt, exists := optionMap["preference"]; exists {
		preference = opt.StringValue()
	}
	// --- END OF NEW BLOCK ---

	var embed *discordgo.MessageEmbed
	var embedAuthor = &discordgo.MessageEmbedAuthor{Name: "ShortCircuit Route Planner", IconURL: "https://images.evetech.net/corporations/98330748/logo?size=64"}

	if err1 != nil || err2 != nil {
		embed = &discordgo.MessageEmbed{
			Author:      embedAuthor,
			Title:       "Error: Invalid System Name",
			Description: "Sorry, I couldn't recognise one of those system names. Please check for typos.",
			Color:       0xff0000,
		}
	} else {
		avoidList := make(map[int]bool)
		if excludeInput != "" {
			for _, sysName := range strings.Split(excludeInput, ",") {
				sysName = strings.TrimSpace(sysName)
				if sysName != "" {
					if sysID, err := s.esiClient.GetSystemID(sysName); err == nil {
						avoidList[sysID] = true
					}
				}
			}
		}
		avoidList[30100000] = true // Zarzakh

		s.graphMutex.RLock()
		pathIDs := FindShortestPath(s.universeGraph, startID, endID, s.esiClient, avoidList, preference)
		s.graphMutex.RUnlock()

		if pathIDs == nil {
			embed = &discordgo.MessageEmbed{
				Author:      embedAuthor,
				Description: fmt.Sprintf("No route possible between **%s** and **%s**.", startName, endName),
				Color:       0xff0000,
			}
		} else {
			killMap := make(map[int]int)
			killFile, err := os.ReadFile("system_kills.json")
			if err == nil {
				var allKills []EsiSystemKills
				if json.Unmarshal(killFile, &allKills) == nil {
					for _, k := range allKills {
						killMap[k.SystemID] = k.ShipKills
					}
				}
			}

			type SystemIntel struct {
				Name       string
				KillCount  int
				SecStatus  float64
				SecDisplay string
			}
			intelMap := make(map[int]SystemIntel)
			var intelMutex sync.Mutex
			var wg sync.WaitGroup

			for _, systemID := range pathIDs {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()

					intel := SystemIntel{Name: fmt.Sprintf("Unknown (%d)", id), SecDisplay: "N/A"}
					if sysInfo, err := s.esiClient.GetSystemDetails(id); err == nil {
						intel.Name = sysInfo.Name
						intel.SecStatus = sysInfo.SecurityStatus
						intel.SecDisplay = fmt.Sprintf("%.1f", sysInfo.SecurityStatus)
					}
					if kills, ok := killMap[id]; ok {
						intel.KillCount = kills
					}

					intelMutex.Lock()
					intelMap[id] = intel
					intelMutex.Unlock()
				}(systemID)
			}
			wg.Wait()

			var routeLines []string
			header := fmt.Sprintf("%-14s | %-4s | %s", "System", "Sec", "Kills (1hr)")
			routeLines = append(routeLines, header)
			routeLines = append(routeLines, strings.Repeat("-", len(header)+2))

			for i, systemID := range pathIDs {
				intel := intelMap[systemID]
				arrow := "→ "
				if i == 0 {
					arrow = "  "
				}

				line := fmt.Sprintf("%s%-12s | %-4s | 🔥 %d", arrow, intel.Name, intel.SecDisplay, intel.KillCount)
				routeLines = append(routeLines, line)
			}
			routeString := "```\n" + strings.Join(routeLines, "\n") + "\n```"

			jumpCount := len(pathIDs) - 1
			embedColor := 0x4CAF50
			if jumpCount > 10 {
				embedColor = 0xFFC107
			}
			if jumpCount > 20 {
				embedColor = 0xF44336
			}

			var excludedSysNames []string
			for sysID := range avoidList {
				if sysInfo, err := s.esiClient.GetSystemDetails(sysID); err == nil {
					excludedSysNames = append(excludedSysNames, sysInfo.Name)
				}
			}

			embed = &discordgo.MessageEmbed{
				Author:      embedAuthor,
				Title:       "Route Calculated",
				Color:       embedColor,
				Timestamp:   time.Now().Format(time.RFC3339),
				Description: routeString,
				Fields: []*discordgo.MessageEmbedField{
					{Name: "Start", Value: startName, Inline: true},
					{Name: "End", Value: endName, Inline: true},
					{Name: "Jumps", Value: fmt.Sprintf("%d", jumpCount), Inline: true},
					{Name: "Excluded Systems", Value: strings.Join(excludedSysNames, ", ")},
				},
				Footer: &discordgo.MessageEmbedFooter{Text: "Zarzakh is ALWAYS excluded. This is a test Bot."},
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

// This function replaces your old FindShortestPath
func FindShortestPath(graph map[int][]int, startID, endID int, esiClient *ESIClient, avoidList map[int]bool, preference string) []int {
	// A map to store the cost to reach each system from the start
	costs := make(map[int]float64)
	for id := range graph {
		costs[id] = 1e9 // Initialize all costs to infinity
	}
	costs[startID] = 0

	// A map to reconstruct the path
	parents := make(map[int]int)

	// A simple priority queue to hold systems to visit
	pq := []int{startID}

	for len(pq) > 0 {
		// Find the node in the queue with the lowest cost
		var currentID int
		var lowestCost = 1e10
		var currentIndex = -1
		for i, id := range pq {
			if costs[id] < lowestCost {
				lowestCost = costs[id]
				currentID = id
				currentIndex = i
			}
		}
		if currentIndex == -1 {
			break
		}

		// Remove from queue
		pq = append(pq[:currentIndex], pq[currentIndex+1:]...)

		if currentID == endID {
			// Path found, reconstruct it from the parents map...
			path := []int{}
			for at := endID; at != 0; at = parents[at] {
				path = append([]int{at}, path...)
				if at == startID { // Stop if we've reached the start
					break
				}
			}
			return path
		}

		for _, neighborID := range graph[currentID] {
			// --- THIS IS THE KEY LOGIC ---
			cost := 1.0 // Default cost for one jump

			if preference == "safer" {
				sysInfo, _ := esiClient.GetSystemDetails(neighborID)
				if sysInfo != nil && sysInfo.SecurityStatus < 0.5 {
					cost += 100.0 // Add a huge penalty for non-high-sec systems
				}
			} else if preference == "unsafe" {
				sysInfo, _ := esiClient.GetSystemDetails(neighborID)
				if sysInfo != nil && sysInfo.SecurityStatus >= 0.5 {
					cost += 100.0 // Add a huge penalty for high-sec systems
				}
			}

			newCost := costs[currentID] + cost
			if newCost < costs[neighborID] {
				costs[neighborID] = newCost
				parents[neighborID] = currentID
				pq = append(pq, neighborID)
			}
		}
	}
	return nil // No path found
}

// formatSecurity is no longer needed as we are using the raw value for alignment.
