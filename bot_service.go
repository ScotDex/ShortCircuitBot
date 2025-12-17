package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
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
						{Name: "Safest (High-Sec first)", Value: "safer"},
						{Name: "Unsafe (Low/Null-Sec first)", Value: "unsafe"},
						{Name: "Shortest (Default)", Value: "shortest"},
					},
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

// ---- interactionCreate (dispatcher) ----
func (s *Service) interactionCreate(sess *discordgo.Session, i *discordgo.InteractionCreate) {
	// Button clicks
	if i.Type == discordgo.InteractionMessageComponent {
		if err := s.handleButtonClick(sess, i); err != nil {
			log.Printf("[BOT] ERROR handling button: %v", err)
		}
		return
	}

	// Slash command: route
	if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "route" {
		return
	}

	if err := sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		log.Printf("[BOT] ERROR: Failed to defer interaction response: %v", err)
		return
	}

	// Process the route command
	if err := s.handleRouteCommand(sess, i); err != nil {
		log.Printf("[BOT] ERROR processing route: %v", err)
	}
}

// ---- button handler: Copy Route ----
func (s *Service) handleButtonClick(sess *discordgo.Session, i *discordgo.InteractionCreate) error {
	data := i.MessageComponentData()
	if data.CustomID != "copy_route_button" {
		return nil
	}

	if len(i.Message.Embeds) == 0 {
		return nil
	}
	embed := i.Message.Embeds[0]

	// find Route Details field and pull bolded system names
	var systems []string
	for _, f := range embed.Fields {
		if f.Name == "Route Details" {
			for _, line := range strings.Split(f.Value, "\n") {
				// the Name was formatted like: "🟢◦ **SystemName (0.9)** — ..." so extract between ** **
				if strings.Contains(line, "**") {
					parts := strings.Split(line, "**")
					if len(parts) > 1 {
						// parts[1] contains "SystemName (0.9)"; strip trailing sec paren to return raw name only if desired
						namePart := parts[1]
						// optionally trim "(sec)" from the name if present
						if idx := strings.LastIndex(namePart, " ("); idx > 0 {
							namePart = namePart[:idx]
						}
						systems = append(systems, strings.TrimSpace(namePart))
					}
				}
			}
			break
		}
	}

	copyableText := strings.Join(systems, "\n")
	return sess.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "```\n" + copyableText + "\n```",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

// ---- main route handler ----
func (s *Service) handleRouteCommand(sess *discordgo.Session, i *discordgo.InteractionCreate) error {
	opts := s.parseOptions(i.ApplicationCommandData().Options)
	startName, endName := opts["start"], opts["end"]
	excludeInput := opts["exclude"]
	preference := "shortest"
	if v, ok := opts["preference"]; ok && v != "" {
		preference = v
	}

	startID, err1 := s.esiClient.GetSystemID(startName)
	endID, err2 := s.esiClient.GetSystemID(endName)

	embedAuthor := &discordgo.MessageEmbedAuthor{
		Name:    "Short Circuit Bot",
		IconURL: "https://images.evetech.net/corporations/98330748/logo?size=64",
	}

	var embed *discordgo.MessageEmbed
	var components []discordgo.MessageComponent

	// invalid system names
	if err1 != nil || err2 != nil {
		embed = &discordgo.MessageEmbed{
			Author:      embedAuthor,
			Title:       "Error: Invalid System Name",
			Description: "Sorry, I couldn't recognise one of those system names. Please check for typos.",
			Color:       0xff0000,
		}
	} else {
		avoidList := s.buildAvoidList(excludeInput)
		avoidList[30100000] = true // Zarzakh always excluded

		// pathfinding (guarded by RLock)
		s.graphMutex.RLock()
		pathIDs := FindPreferredPath(s.universeGraph, startID, endID, s.esiClient, preference, avoidList)
		s.graphMutex.RUnlock()

		if pathIDs == nil {
			embed = &discordgo.MessageEmbed{
				Author:      embedAuthor,
				Description: fmt.Sprintf("No route possible between **%s** and **%s**.", startName, endName),
				Color:       0xff0000,
			}
		} else {
			// load supporting data (file reads)
			killMap := s.loadKills("system_kills.json")
			sigMap, eolMap := s.loadTripwire("tripwire_data.json")

			// fetch system intel concurrently
			intelMap := s.fetchIntelForPath(pathIDs, killMap, sigMap, eolMap)

			// format route lines (detailed style with small colored dots)
			routeString := s.formatRouteString(pathIDs, intelMap)

			jumpCount := len(pathIDs) - 1
			embedColor := 0x4CAF50
			if jumpCount > 10 {
				embedColor = 0xFFC107
			}
			if jumpCount > 20 {
				embedColor = 0xF44336
			}

			// excluded system names for display
			var excludedSysNames []string
			for sysID := range avoidList {
				if sysInfo, err := s.esiClient.GetSystemDetails(sysID); err == nil {
					excludedSysNames = append(excludedSysNames, sysInfo.Name)
				}
			}

			embed = &discordgo.MessageEmbed{
				Author:    embedAuthor,
				Title:     "Route Calculated",
				Color:     embedColor,
				Timestamp: time.Now().Format(time.RFC3339),
				Fields: []*discordgo.MessageEmbedField{
					{Name: "Start", Value: startName, Inline: true},
					{Name: "End", Value: endName, Inline: true},
					{Name: "Jumps", Value: fmt.Sprintf("%d", jumpCount), Inline: true},
					{Name: "Route Details", Value: routeString},
					{Name: "Excluded Systems", Value: strings.Join(excludedSysNames, ", ")},
				},
				Footer: &discordgo.MessageEmbedFooter{
					Text: "Zarzakh is ALWAYS excluded. Kills are up to 60min old.",
				},
			}

			components = []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Copy Route",
							Style:    discordgo.SecondaryButton,
							Emoji:    &discordgo.ComponentEmoji{Name: "📋"},
							CustomID: "copy_route_button",
						},
					},
				},
			}
		}
	}

	webhookEdit := discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	}
	_, err := sess.InteractionResponseEdit(i.Interaction, &webhookEdit)
	return err
}

// ---- small helpers ----

func (s *Service) parseOptions(options []*discordgo.ApplicationCommandInteractionDataOption) map[string]string {
	result := map[string]string{}
	for _, opt := range options {
		if opt == nil || opt.Name == "" {
			continue
		}
		result[opt.Name] = opt.StringValue()
	}
	return result
}

func (s *Service) buildAvoidList(excludeInput string) map[int]bool {
	avoid := make(map[int]bool)
	if excludeInput == "" {
		return avoid
	}
	for _, sysName := range strings.Split(excludeInput, ",") {
		sysName = strings.TrimSpace(sysName)
		if sysName == "" {
			continue
		}
		if sysID, err := s.esiClient.GetSystemID(sysName); err == nil {
			avoid[sysID] = true
		}
	}
	return avoid
}

func (s *Service) loadKills(path string) map[int]int {
	killMap := make(map[int]int)
	b, err := os.ReadFile(path)
	if err != nil {
		return killMap
	}
	var all []EsiSystemKills
	if err := json.Unmarshal(b, &all); err != nil {
		log.Printf("[BOT] WARN: failed to parse %s: %v", path, err)
		return killMap
	}
	for _, k := range all {
		killMap[k.SystemID] = k.ShipKills
	}
	return killMap
}

func (s *Service) loadTripwire(path string) (map[int]string, map[int]time.Time) {
	sigMap := make(map[int]string)
	eolMap := make(map[int]time.Time)

	b, err := os.ReadFile(path)
	if err != nil {
		return sigMap, eolMap
	}
	var td TripwireData
	if err := json.Unmarshal(b, &td); err != nil {
		log.Printf("[BOT] WARN: failed to parse %s: %v", path, err)
		return sigMap, eolMap
	}
	for _, sig := range td.Signatures {
		sysID, _ := strconv.Atoi(sig.SystemID)
		if sysID == 0 {
			continue
		}
		if sig.SignatureID != nil {
			sigMap[sysID] = strings.ToUpper(*sig.SignatureID)
		}
		if sig.LifeLeft != "" {
			if eol, err := time.Parse("2006-01-02 15:04:05", sig.LifeLeft); err == nil {
				eolMap[sysID] = eol
			}
		}
	}
	return sigMap, eolMap
}

type SystemIntel struct {
	Name        string
	KillCount   int
	SecDisplay  string
	SignatureID string
	EolInfo     string
}

func (s *Service) fetchIntelForPath(path []int, killMap map[int]int, sigMap map[int]string, eolMap map[int]time.Time) map[int]SystemIntel {
	intelMap := make(map[int]SystemIntel)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, id := range path {
		wg.Add(1)
		go func(sysID int) {
			defer wg.Done()
			intel := SystemIntel{Name: fmt.Sprintf("Unknown (%d)", sysID), SecDisplay: "N/A"}

			if si, err := s.esiClient.GetSystemDetails(sysID); err == nil {
				intel.Name = si.Name
				intel.SecDisplay = fmt.Sprintf("%.1f", si.SecurityStatus)
			}
			if k := killMap[sysID]; k != 0 {
				intel.KillCount = k
			}
			if sig, ok := sigMap[sysID]; ok {
				intel.SignatureID = sig
			}
			if eol, ok := eolMap[sysID]; ok {
				if remaining := time.Until(eol); remaining > 0 {
					intel.EolInfo = fmt.Sprintf("EOL: ~%dh", int(remaining.Hours()))
				} else {
					intel.EolInfo = "EOL"
				}
			}

			mu.Lock()
			intelMap[sysID] = intel
			mu.Unlock()
		}(id)
	}

	wg.Wait()
	return intelMap
}

// formatRouteString builds the detailed embed field using small colored dots + tiny hollow dot suffix
func (s *Service) formatRouteString(path []int, intelMap map[int]SystemIntel) string {
	lines := make([]string, 0, len(path))
	for _, sysID := range path {
		intel := intelMap[sysID]
		secFloat, _ := strconv.ParseFloat(intel.SecDisplay, 64)

		// small colored dot + tiny hollow suffix to reduce visual weight: e.g. "🟢◦"
		secMarker := "🟢◦"
		if secFloat < 0.5 && secFloat >= 0.1 {
			secMarker = "🟠◦"
		} else if secFloat < 0.1 {
			secMarker = "🔴◦"
		}

		// Build line: marker, bold name (with sec), optional bits
		line := fmt.Sprintf("%s **%s (%s)**", secMarker, intel.Name, intel.SecDisplay)
		if intel.KillCount > 0 {
			line += fmt.Sprintf(" — 🔥 %d kills", intel.KillCount)
		}
		if intel.SignatureID != "" {
			line += fmt.Sprintf(" — WH: %s", intel.SignatureID)
		}
		if intel.EolInfo != "" {
			line += fmt.Sprintf(" — %s", intel.EolInfo)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// ---- Pathfinding unchanged in behaviour (minor safety fix in path recovery) ----
func FindPreferredPath(graph map[int][]int, startID, endID int, esiClient *ESIClient, preference string, avoidList map[int]bool) []int {
	costs := make(map[int]float64)
	for id := range graph {
		costs[id] = 1e9
	}
	costs[startID] = 0
	parents := make(map[int]int)
	pq := []int{startID}

	for len(pq) > 0 {
		var currentID int
		var lowestCost = 1e10
		var currentIndex = -1
		for i, id := range pq {
			if costs[id] < lowestCost {
				lowestCost, currentID, currentIndex = costs[id], id, i
			}
		}
		if currentIndex == -1 {
			break
		}

		// pop current
		pq = append(pq[:currentIndex], pq[currentIndex+1:]...)

		if currentID == endID {
			// Reconstruct path safely (stop if parent missing)
			path := []int{}
			at := endID
			for {
				path = append([]int{at}, path...)
				if at == startID {
					break
				}
				parent, ok := parents[at]
				if !ok {
					// Broken parent chain: abort
					return nil
				}
				at = parent
			}
			return path
		}

		for _, neighborID := range graph[currentID] {
			if avoidList[neighborID] {
				continue
			}

			cost := 1.0
			if preference != "shortest" {
				if sysInfo, err := esiClient.GetSystemDetails(neighborID); err == nil {
					isHighSec := sysInfo.SecurityStatus >= 0.5
					if preference == "safer" && !isHighSec {
						cost += 100.0
					} else if preference == "unsafe" && isHighSec {
						cost += 100.0
					}
				}
			}

			newCost := costs[currentID] + cost
			if newCost < costs[neighborID] {
				costs[neighborID], parents[neighborID] = newCost, currentID
				pq = append(pq, neighborID)
			}
		}
	}
	return nil
}
