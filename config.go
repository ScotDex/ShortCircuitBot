package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application.
type Config struct {
	BotToken     string
	TripwireURL  string
	TripwireUser string
	TripwirePass string
}

// Load reads configuration from a .env file and the environment.
func Load() (*Config, error) {
	// Load values from .env file. It's safe to ignore the error if it doesn't exist.
	_ = godotenv.Load()

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN is not set in the environment")
	}

	tripwireURL := os.Getenv("TRIPWIRE_URL")
	if tripwireURL == "" {
		return nil, fmt.Errorf("TRIPWIRE_URL is not set in the environment")
	}

	tripwireUser := os.Getenv("TRIPWIRE_USER")
	if tripwireUser == "" {
		return nil, fmt.Errorf("TRIPWIRE_USER is not set in the environment")
	}

	tripwirePass := os.Getenv("TRIPWIRE_PASS")
	if tripwirePass == "" {
		return nil, fmt.Errorf("TRIPWIRE_PASS is not set in the environment")
	}

	return &Config{
		BotToken:     botToken,
		TripwireURL:  tripwireURL,
		TripwireUser: tripwireUser,
		TripwirePass: tripwirePass,
	}, nil
}
