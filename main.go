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

    // 1. Load all configuration from .env file.
    cfg, err := Load() // Config load function from config.go in same package
    if err != nil {
        log.Fatalf("FATAL: Could not load configuration: %v", err)
    }
    log.Println("Configuration loaded.")

    // 2. Create the Fetcher and Bot services.
    fetcherService, err := New(cfg.TripwireURL, cfg.TripwireUser, cfg.TripwirePass) // Fetcher ctor
    if err != nil {
        log.Fatalf("FATAL: Could not create fetcher service: %v", err)
    }

    botService := NewService(cfg.BotToken) // Bot ctor

    // 3. Set up channels and a WaitGroup for graceful shutdown.
    var wg sync.WaitGroup
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

    // 4. Launch the services in their own goroutines.
    wg.Add(2) // We are waiting for two services to finish.
    go fetcherService.Start(&wg, quit)
    go botService.Start(&wg, quit)

    // 5. Wait for all services to shut down cleanly.
    wg.Wait()
    log.Println("--- All services have shut down. Exiting. ---")
}
