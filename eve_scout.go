package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// NewEveScoutClient creates a new client for the EVE-Scout API.
func NewEveScoutClient(userAgent string) *EveScoutClient {
	return &EveScoutClient{
		baseURL:   "https://api.eve-scout.com/v2",
		userAgent: userAgent,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// makeRequest handles the GET request and JSON decoding.
func (c *EveScoutClient) makeRequest(endpoint string, target interface{}) error {
	req, err := http.NewRequest("GET", c.baseURL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api returned non-200 status: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode json response: %w", err)
	}
	return nil
}

// GetTheraConnections fetches all public wormhole connections for Thera.
func (c *EveScoutClient) GetTheraConnections() ([]TheraConnection, error) {
	var connections []TheraConnection
	endpoint := "/public/signatures?system_name=thera"
	err := c.makeRequest(endpoint, &connections)
	return connections, err
}

// --- Thera Updater Service ---

// TheraUpdater manages the background fetching of Thera connections.
type TheraUpdater struct {
	eveScoutClient *EveScoutClient
	universeGraph  map[int][]int
	graphMutex     *sync.RWMutex
}

// NewTheraUpdater creates a new Thera data updater service.
func NewTheraUpdater(client *EveScoutClient, graph map[int][]int, mutex *sync.RWMutex) *TheraUpdater {
	return &TheraUpdater{
		eveScoutClient: client,
		universeGraph:  graph,
		graphMutex:     mutex,
	}
}

// Start launches the background updater. The API cache TTL is 5 minutes.
func (u *TheraUpdater) Start(wg *sync.WaitGroup, quit chan struct{}) {
	defer wg.Done()
	log.Println("[THERA UPDATER] Starting service...")

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	u.updateGraph() // Run once immediately on startup

	for {
		select {
		case <-ticker.C:
			u.updateGraph()
		case <-quit:
			log.Println("[THERA UPDATER] Shutdown signal received, exiting.")
			return
		}
	}
}

// updateGraph fetches Thera connections and merges them into the main graph.
func (u *TheraUpdater) updateGraph() {
	log.Println("[THERA UPDATER] Fetching Thera connections from EVE-Scout...")
	theraConnections, err := u.eveScoutClient.GetTheraConnections()
	if err != nil {
		log.Printf("[THERA UPDATER] ERROR: Failed to fetch Thera data: %v", err)
		return
	}

	u.graphMutex.Lock()
	defer u.graphMutex.Unlock()

	const theraSystemID = 31000005
	theraAdded := 0
	for _, conn := range theraConnections {
		if conn.DestinationSystem != nil {
			destID := conn.DestinationSystem.ID
			u.universeGraph[theraSystemID] = append(u.universeGraph[theraSystemID], destID)
			u.universeGraph[destID] = append(u.universeGraph[destID], theraSystemID)
			theraAdded++
		}
	}

	DeduplicateNeighbors(u.universeGraph)
	log.Printf("[THERA UPDATER] ✅ Merged %d Thera connections into the graph.", theraAdded)
}
