package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const (
	logSuccess = "✅"
	logWarn    = "⚠️"
)

func main() {
	log.Println("--- Starting ShortCircuitBot ---")

	cfg, err := Load()
	if err != nil {
		log.Fatalf("FATAL: Could not load configuration: %v", err)
	}
	log.Println("Configuration loaded.")

	esiClient := NewESIClient("YourApp/ContactEmail")
	eveScoutClient := NewEveScoutClient("ShortCircuitBot/0.1")

	// --- 1. Load ESI System Cache ---
	// This must be done first so the ESI client knows system names.
	if err := esiClient.LoadSystemCache("system_cache.json"); err != nil {
		log.Printf("%s Could not load system cache: %v. Names will be fetched live.", logWarn, err)
	}

	// --- 2. Build the complete initial graph from all sources ---
	log.Println("--- Building initial universe graph ---")
	universeGraph, err := BuildGraphFromCSV("mapSolarSystemJumps.csv")
	if err != nil {
		log.Fatalf("FATAL: Could not build stargate graph: %v", err)
	}

	// Add connections from local Tripwire cache
	tripwireData, err := loadTripwireData("tripwire_data.json")
	if err != nil {
		log.Printf("%s Could not load initial tripwire data: %v", logWarn, err)
	}
	if tripwireData != nil {
		AddTripwireWormholesToGraph(universeGraph, tripwireData, esiClient)
	}

	// Add live Thera connections from EVE-Scout
	theraConnections, err := eveScoutClient.GetTheraConnections()
	if err != nil {
		log.Printf("%s Could not fetch initial Thera connections: %v", logWarn, err)
	} else {
		const theraSystemID = 31000005
		for _, conn := range theraConnections {
			if conn.DestinationSystem != nil {
				destID := conn.DestinationSystem.ID
				universeGraph[theraSystemID] = append(universeGraph[theraSystemID], destID)
				universeGraph[destID] = append(universeGraph[destID], theraSystemID)
			}
		}
		log.Printf("%s Added %d initial Thera connections.", logSuccess, len(theraConnections))
	}

	DeduplicateNeighbors(universeGraph)
	log.Printf("%s Graph built with %d systems.", logSuccess, len(universeGraph))

	// --- 3. Create services with the fully-built graph ---
	var graphMutex sync.RWMutex
	fetcherService, err := New(cfg.TripwireURL, cfg.TripwireUser, cfg.TripwirePass, universeGraph, &graphMutex)
	if err != nil {
		log.Fatalf("FATAL: Could not create fetcher service: %v", err)
	}
	botService := NewService(cfg.BotToken, universeGraph, &graphMutex, esiClient)
	killUpdater := NewKillDataUpdater(esiClient, "system_kills.json")
	theraUpdater := NewTheraUpdater(eveScoutClient, universeGraph, &graphMutex)

	// --- 4. Start services and handle shutdown ---
	var servicesWg sync.WaitGroup
	quit := make(chan struct{})

	go func() {
		osSignal := make(chan os.Signal, 1)
		signal.Notify(osSignal, os.Interrupt, syscall.SIGTERM)
		<-osSignal
		log.Println("--- Shutdown signal received, stopping services. ---")
		close(quit)
	}()

	servicesWg.Add(3)
	go fetcherService.Start(&servicesWg, quit)
	go botService.Start(&servicesWg, quit)
	go theraUpdater.Start(&servicesWg, quit)

	go killUpdater.Start(&servicesWg, quit)
	go startHealthCheckServer()

	servicesWg.Wait()
	log.Println("--- All services have shut down. Exiting. ---")
}
