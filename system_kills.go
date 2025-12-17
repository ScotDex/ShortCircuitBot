package main

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

// KillDataUpdater manages the background fetching service.
type KillDataUpdater struct {
	esiClient *ESIClient
	filePath  string
}

// NewKillDataUpdater creates a new updater service.
func NewKillDataUpdater(client *ESIClient, filePath string) *KillDataUpdater {
	return &KillDataUpdater{
		esiClient: client,
		filePath:  filePath,
	}
}

// Start launches the background updater. Run this as a goroutine.
func (u *KillDataUpdater) Start(wg *sync.WaitGroup, quit chan struct{}) {
	defer wg.Done()
	log.Println("[UPDATER] Starting background kill data updater...")

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	u.fetchAndSave() // Run once immediately on startup.

	for {
		select {
		case <-ticker.C:
			u.fetchAndSave()
		case <-quit:
			log.Println("[UPDATER] Shutdown signal received, exiting.")
			return
		}
	}
}

// fetchAndSave gets the data from ESI and atomically writes it to the local file.
func (u *KillDataUpdater) fetchAndSave() {
	log.Println("[UPDATER] Fetching latest system kill data from ESI...")
	kills, err := u.esiClient.GetSystemKills()
	if err != nil {
		log.Printf("[UPDATER] ERROR: Failed to fetch kills from ESI: %v", err)
		return
	}

	jsonData, err := json.Marshal(kills)
	if err != nil {
		log.Printf("[UPDATER] ERROR: Failed to convert kills to JSON: %v", err)
		return
	}

	// --- PERFORMANCE & SAFETY IMPROVEMENT ---
	// Write to a temporary file first.
	tempFilePath := u.filePath + ".tmp"
	err = os.WriteFile(tempFilePath, jsonData, 0644)
	if err != nil {
		log.Printf("[UPDATER] ERROR: Failed to write to temporary file '%s': %v", tempFilePath, err)
		return
	}

	// Atomically rename the temporary file to the final destination.
	// This is an instant operation and prevents file corruption.
	err = os.Rename(tempFilePath, u.filePath)
	if err != nil {
		log.Printf("[UPDATER] ERROR: Failed to rename temp file to '%s': %v", u.filePath, err)
		return
	}
	// --- END IMPROVEMENT ---

	log.Printf("[UPDATER] ✅ Successfully saved kill data to %s.", u.filePath)
}
