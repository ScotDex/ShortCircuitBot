//go:build test

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// ZKillboardResponse defines the structure for kills from the API.
type ZKillboardResponse []struct {
	KillmailID int `json:"killmail_id"`
	Zkb        struct {
		TotalValue float64 `json:"totalValue"`
	} `json:"zkb"`
}

// KillMonitor service checks for recent kills in wormhole systems.
type KillMonitor struct {
	universeGraph map[int][]int
	graphMutex    *sync.RWMutex
	esiClient     *ESIClient
	webhookURL    string
	httpClient    *http.Client
}

// NewKillMonitor creates a new instance of the KillMonitor service.
func NewKillMonitor(graph map[int][]int, mutex *sync.RWMutex, esi *ESIClient, webhookURL string) *KillMonitor {
	return &KillMonitor{
		universeGraph: graph,
		graphMutex:    mutex,
		esiClient:     esi,
		webhookURL:    webhookURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Start begins the monitoring loop.
func (km *KillMonitor) Start(wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("[KILLMONITOR] Starting service...")

	// Run the check once at startup.
	km.checkForKills()

	// Then, run it on a timer.
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		km.checkForKills()
	}
}

// checkForKills identifies wormhole systems and queries zKillboard for activity.
func (km *KillMonitor) checkForKills() {
	// 1. Get a list of current wormhole systems from the graph.
	km.graphMutex.RLock()
	var whSystems []int
	for systemID := range km.universeGraph {
		// J-space system IDs start with 310...
		if systemID > 31000000 && systemID < 32000000 {
			whSystems = append(whSystems, systemID)
		}
	}
	km.graphMutex.RUnlock()

	if len(whSystems) == 0 {
		return // No wormholes to monitor
	}

	log.Printf("[KILLMONITOR] Checking %d wormhole systems for recent activity...", len(whSystems))

	// 2. Check each system for kills in the last 60 seconds.
	for _, systemID := range whSystems {
		url := fmt.Sprintf("https://zkillboard.com/system/%d/", systemID)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			continue
		}
		// It's good practice to set a custom User-Agent.
		req.Header.Set("User-Agent", "ShortCircuitKillMonitor/1.0")

		resp, err := km.httpClient.Do(req)
		if err != nil {
			log.Printf("[KILLMONITOR] ERROR: Failed to fetch kills for system %d: %v", systemID, err)
			continue
		}
		defer resp.Body.Close()

		var kills ZKillboardResponse
		if err := json.NewDecoder(resp.Body).Decode(&kills); err != nil {
			continue
		}

		// 3. If kills are found, send a notification.
		if len(kills) > 0 {
			systemName := km.esiClient.GetSystemName(systemID)
			killValue := kills[0].Zkb.TotalValue
			killID := kills[0].KillmailID
			message := fmt.Sprintf("💥 **Kill Detected in %s!** Value: %.2f ISK. [View Killmail](https://zkillboard.com/kill/%d/)", systemName, killValue, killID)
			km.sendDiscordNotification(message)
		}
	}
}

// sendDiscordNotification sends a message to a Discord webhook.
func (km *KillMonitor) sendDiscordNotification(message string) {
	payload := map[string]string{"content": message}
	jsonPayload, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", km.webhookURL, bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")

	_, err := km.httpClient.Do(req)
	if err != nil {
		log.Printf("[KILLMONITOR] ERROR: Failed to send Discord notification: %v", err)
	}
}
