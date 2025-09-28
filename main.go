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

	esiClient := NewESIClient("Your Character Name or Email")
	eveScoutClient := NewEveScoutClient("ShortCircuitBot/0.1")

	// 2. Concurrently load all initial data from files
	log.Println("--- Concurrently loading initial data ---")
	var dataWg sync.WaitGroup
	dataWg.Add(3) // We have 3 loading tasks

	var universeGraph map[int][]int
	var tripwireData *TripwireData
	var graphErr, tripwireErr, cacheErr error

	go func() {
		defer dataWg.Done()
		universeGraph, graphErr = BuildGraphFromCSV("mapSolarSystemJumps.csv")
	}()

	go func() {
		defer dataWg.Done()
		tripwireData, tripwireErr = loadTripwireData("tripwire_data.json")
	}()

	go func() {
		defer dataWg.Done()
		cacheErr = esiClient.LoadSystemCache("system_cache.json")
	}()

	dataWg.Wait() // Wait for all loading to finish

	// Check for errors from the concurrent tasks
	if graphErr != nil {
		log.Fatalf("FATAL: Could not build stargate graph: %v", graphErr)
	}
	if tripwireErr != nil {
		log.Fatalf("FATAL: Could not load tripwire data: %v", tripwireErr)
	}
	if cacheErr != nil {
		log.Printf("WARN: Could not load system cache: %v", cacheErr)
	}
	log.Println("--- Initial data loaded successfully ---")

	// 3. Combine the loaded data (this is fast)
	AddTripwireWormholesToGraph(universeGraph, tripwireData, esiClient) // Assuming this needs esiClient now
	DeduplicateNeighbors(universeGraph)
	log.Printf("Graph built with %d systems.", len(universeGraph))

	// 4. Create and run services (same as before)
	var graphMutex sync.RWMutex
	fetcherService, err := New(cfg.TripwireURL, cfg.TripwireUser, cfg.TripwirePass, universeGraph, &graphMutex, eveScoutClient)
	if err != nil {
		log.Fatalf("FATAL: Could not create fetcher service: %v", err)
	}
	botService := NewService(cfg.BotToken, universeGraph, &graphMutex, esiClient)

	var servicesWg sync.WaitGroup
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	servicesWg.Add(2)
	go fetcherService.Start(&servicesWg, quit)
	go botService.Start(&servicesWg, quit)

	servicesWg.Wait()
	log.Println("--- All services have shut down. Exiting. ---")
}
