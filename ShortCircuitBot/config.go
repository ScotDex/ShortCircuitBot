package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application.
type Config struct {
	BotToken       string
	TripwireURL    string
	TripwireUser   string
	TripwirePass   string
	DiscordWebHook string
}

// Load reads configuration from a .env file and the environment.
func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		fmt.Printf("[WARNING] Failed to load .env file: %v\n", err)
	}

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN is not set in the environment")
	}

	tripwireURL := os.Getenv("TRIPWIRE_URL")
	if tripwireURL == "" {
		return nil, fmt.Errorf("TRIPWIRE_URL is not set in the environment")
	}
	if _, err := url.ParseRequestURI(tripwireURL); err != nil {
		return nil, fmt.Errorf("TRIPWIRE_URL is invalid: %v", err)
	}

	tripwireUser := os.Getenv("TRIPWIRE_USER")
	if tripwireUser == "" {
		return nil, fmt.Errorf("TRIPWIRE_USER is not set in the environment")
	}

	tripwirePass := os.Getenv("TRIPWIRE_PASS")
	if tripwirePass == "" {
		return nil, fmt.Errorf("TRIPWIRE_PASS is not set in the environment")
	}

	DiscordWebHook := os.Getenv("DISCORD_WEB_HOOK")
	if tripwirePass == "" {
		return nil, fmt.Errorf("DISCORD_WEB_HOOK is not set in the environment")
	}

	return &Config{
		BotToken:       botToken,
		TripwireURL:    tripwireURL,
		TripwireUser:   tripwireUser,
		TripwirePass:   tripwirePass,
		DiscordWebHook: DiscordWebHook,
	}, nil
}

func startHealthCheckServer() {

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Short Circuit Bot is running!")
	})

	log.Printf("Health check server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start health check server: %v", err)
	}
}
