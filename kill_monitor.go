// You can place this in a new file like 'updater.go'
package main

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

// KillDataUpdater manages the background fetching service.
type KillDataUpdater struct {
	esiClient *ESIClient
	filePath  string
	ticker    *time.Ticker
	quit      chan struct{}
}

// NewKillDataUpdater creates a new updater service.
func NewKillDataUpdater(client *ESIClient, filePath string) *KillDataUpdater {
	return &KillDataUpdater{
		esiClient: client,
		filePath:  filePath,
		// The ticker will fire every hour to trigger an update.
		ticker: time.NewTicker(1 * time.Hour),
		quit:   make(chan struct{}),
	}
}

// Start launches the background updater. Run this as a goroutine.
func (u *KillDataUpdater) Start() {
	log.Println("[UPDATER] Starting background kill data updater...")

	// Run once immediately on startup.
	u.fetchAndSave()

	// Loop forever, waiting for the ticker or a quit signal.
	for {
		select {
		case <-u.ticker.C:
			// The hourly ticker has fired, so fetch new data.
			u.fetchAndSave()
		case <-u.quit:
			// The service is stopping.
			u.ticker.Stop()
			return
		}
	}
}

// Stop safely shuts down the updater service.
func (u *KillDataUpdater) Stop() {
	log.Println("[UPDATER] Stopping background kill data updater...")
	close(u.quit)
}

// fetchAndSave gets the data from ESI and writes it to the local file.
func (u *KillDataUpdater) fetchAndSave() {
	log.Println("[UPDATER] Fetching latest system kill data from ESI...")
	// Assumes GetSystemKills fetches data for ALL systems.
	kills, err := u.esiClient.GetSystemKills()
	if err != nil {
		log.Printf("[UPDATER] ERROR: Failed to fetch kills from ESI: %v", err)
		return
	}

	// Convert the data to JSON format.
	jsonData, err := json.Marshal(kills)
	if err != nil {
		log.Printf("[UPDATER] ERROR: Failed to convert kills to JSON: %v", err)
		return
	}

	// Write the JSON data to the file, overwriting it if it exists.
	err = os.WriteFile(u.filePath, jsonData, 0644)
	if err != nil {
		log.Printf("[UPDATER] ERROR: Failed to write kills to file '%s': %v", u.filePath, err)
		return
	}

	log.Printf("[UPDATER] ✅ Successfully saved kill data to %s.", u.filePath)
}
