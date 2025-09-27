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

	// 1. Load config
	cfg, err := Load()
	if err != nil {
		log.Fatalf("FATAL: Could not load configuration: %v", err)
	}
	log.Println("Configuration loaded.")

	// 2. Create the ESIClient
	esiClient := NewESIClient("Your Character Name or Email")
	if err := esiClient.LoadSystemCache("system_cache.json"); err != nil {
		log.Printf("WARN: Could not load system cache: %v", err)
	}

	eveScoutClient := NewEveScoutClient("ShortCircuitBot/0.1")

	// 3. Load the specific Tripwire data for the test
	tripwireData, err := loadTripwireData("tripwire_data.json")
	if err != nil {
		log.Fatalf("FATAL: Could not load tripwire data: %v", err)
	}

	// 4. Build the single, definitive graph using the test data
	universeGraph, err := BuildGraphFromCSV("mapSolarSystemJumps.csv")
	if err != nil {
		log.Fatalf("FATAL: Could not build stargate graph: %v", err)
	}
	AddTripwireWormholesToGraph(universeGraph, tripwireData)
	DeduplicateNeighbors(universeGraph)
	log.Printf("Graph built with %d systems using test data.", len(universeGraph))

	// 5. Create services using the graph we just built and tested
	var graphMutex sync.RWMutex
	fetcherService, err := New(cfg.TripwireURL, cfg.TripwireUser, cfg.TripwirePass, universeGraph, &graphMutex, eveScoutClient)
	if err != nil {
		log.Fatalf("FATAL: Could not create fetcher service: %v", err)
	}
	botService := NewService(cfg.BotToken, universeGraph, &graphMutex, esiClient)
	//	killMonitorService := NewKillMonitor(universeGraph, &graphMutex, esiClient, cfg.DiscordWebHook)

	// 6. Set up and run the services
	var wg sync.WaitGroup
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	wg.Add(2)
	go fetcherService.Start(&wg, quit)
	go botService.Start(&wg, quit)
	//go killMonitorService.Start(&wg)
	wg.Wait()
	log.Println("--- All services have shut down. Exiting. ---")
}
