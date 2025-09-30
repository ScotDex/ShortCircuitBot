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
		if excludeInput != "" {
			excludeSystems := strings.Split(excludeInput, ",")
			for _, sysName := range excludeSystems {
				sysName = strings.TrimSpace(sysName)
				if sysName == "" {
					continue
				}
				if sysID, err := s.esiClient.GetSystemID(sysName); err == nil {
					avoidList[sysID] = true
				} else {
					log.Printf("[BOT] Warning: Unable to resolve system name for exclude: %s", sysName)
				}
			}
		}
		avoidList[30100000] = true // Zarzakh

		s.graphMutex.RLock()
		pathIDs := FindShortestPath(s.universeGraph, startID, endID, avoidList)
		s.graphMutex.RUnlock()

		if pathIDs == nil {
			embed = &discordgo.MessageEmbed{
				Description: fmt.Sprintf("No route possible between **%s** and **%s**.", startName, endName),
				Color:       0xff0000,
			}
		} else {
			// --- 1. Load kill data from the local file ---
			killMap := make(map[int]int)
			killFile, err := os.ReadFile("system_kills.json")
			if err != nil {
				log.Printf("[BOT] WARN: Could not read local kill data file: %v", err)
			} else {
				var allKills []EsiSystemKills
				if err := json.Unmarshal(killFile, &allKills); err == nil {
					for _, k := range allKills {
						killMap[k.SystemID] = k.ShipKills
					}
				}
			}

			// --- 2. Concurrently fetch system names and combine with local kill data ---
			type SystemIntel struct {
				Name            string
				ThreatIndicator string
			}
			intelMap := make(map[int]SystemIntel)
			var intelMutex sync.Mutex
			var wg sync.WaitGroup // The WaitGroup to fix the race condition

			for _, systemID := range pathIDs {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()

					systemName := fmt.Sprintf("Unknown (%d)", id)
					if sysInfo, err := s.esiClient.GetSystemDetails(id); err == nil {
						systemName = sysInfo.Name
					}

					shipKills := 0
					if kills, ok := killMap[id]; ok {
						shipKills = kills
					}

					var threatIndicator string
					if shipKills >= 10 {
						threatIndicator = fmt.Sprintf("🔥 (%d)", shipKills)
					} else if shipKills > 0 {
						threatIndicator = fmt.Sprintf("⚠️ (%d)", shipKills)
					} else {
						threatIndicator = fmt.Sprintf("✅ (%d)", shipKills)
					}

					intelMutex.Lock()
					intelMap[id] = SystemIntel{Name: systemName, ThreatIndicator: threatIndicator}
					intelMutex.Unlock()
				}(systemID)
			}
			wg.Wait() // Wait for all goroutines to finish before proceeding

			// --- 3. Build the final response ---
			var pathWithKills []string
			for _, systemID := range pathIDs {
				intel := intelMap[systemID]
				pathWithKills = append(pathWithKills, fmt.Sprintf("%s %s", intel.Name, intel.ThreatIndicator))
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

			var excludedSysNames []string
			for sysID := range avoidList {
				if sysInfo, err := s.esiClient.GetSystemDetails(sysID); err == nil {
					excludedSysNames = append(excludedSysNames, sysInfo.Name)
				}
			}

			embed = &discordgo.MessageEmbed{
				Author:      &discordgo.MessageEmbedAuthor{Name: "ShortCircuit Route Planner", IconURL: "https://images.evetech.net/corporations/98330748/logo?size=64"},
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
				Footer: &discordgo.MessageEmbedFooter{Text: "Zarzakh is always avoided. Kill data is for the last hour."},
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

func FindShortestPath(graph map[int][]int, startID, endID int, avoidList map[int]bool) []int {
	if avoidList[startID] || avoidList[endID] {
		return nil
	}
	queue := []int{startID}
	visited := make(map[int]int)
	visited[startID] = -1

	head := 0
	for head < len(queue) {
		currentSystem := queue[head]
		head++
		if currentSystem == endID {
			path := []int{}
			temp := currentSystem
			for temp != -1 {
				path = append(path, temp)
				temp = visited[temp]
			}
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}
		for _, neighbor := range graph[currentSystem] {
			if _, found := visited[neighbor]; !found && !avoidList[neighbor] {
				visited[neighbor] = currentSystem
				queue = append(queue, neighbor)
			}
		}
	}
	return nil
}
