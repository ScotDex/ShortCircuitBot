package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
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

	// --- 1. Build the complete initial graph from all sources ---
	log.Println("--- Building initial universe graph ---")
	universeGraph, err := BuildGraphFromCSV("mapSolarSystemJumps.csv")
	if err != nil {
		log.Fatalf("FATAL: Could not build stargate graph: %v", err)
	}

	// Add connections from local Tripwire cache
	tripwireData, err := loadTripwireData("tripwire_data.json")
	if err != nil {
		log.Printf("WARN: Could not load initial tripwire data: %v", err)
	}
	if tripwireData != nil {
		AddTripwireWormholesToGraph(universeGraph, tripwireData, esiClient)
	}

	// Add live Thera connections from EVE-Scout
	theraConnections, err := eveScoutClient.GetTheraConnections()
	if err != nil {
		log.Printf("WARN: Could not fetch initial Thera connections: %v", err)
	} else {
		const theraSystemID = 31000005
		for _, conn := range theraConnections {
			if conn.DestinationSystem != nil {
				destID := conn.DestinationSystem.ID
				universeGraph[theraSystemID] = append(universeGraph[theraSystemID], destID)
				universeGraph[destID] = append(universeGraph[destID], theraSystemID)
			}
		}
		log.Printf("✅ Added %d initial Thera connections.", len(theraConnections))
	}

	DeduplicateNeighbors(universeGraph)
	log.Printf("✅ Graph built with %d systems.", len(universeGraph))

	// --- 2. Create services with the fully-built graph ---
	var graphMutex sync.RWMutex
	fetcherService, err := New(cfg.TripwireURL, cfg.TripwireUser, cfg.TripwirePass, universeGraph, &graphMutex)
	if err != nil {
		log.Fatalf("FATAL: Could not create fetcher service: %v", err)
	}
	botService := NewService(cfg.BotToken, universeGraph, &graphMutex, esiClient)
	killUpdater := NewKillDataUpdater(esiClient, "system_kills.json")
	theraUpdater := NewTheraUpdater(eveScoutClient, universeGraph, &graphMutex)

	// --- 3. Start services and handle shutdown ---
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

	go killUpdater.Start()
	go startHealthCheckServer()

	servicesWg.Wait()
	log.Println("--- All services have shut down. Exiting. ---")
}
