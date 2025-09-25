package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

// --- Scraper Logic (from your tripwire package) ---
type Scraper struct {
	client   *http.Client
	baseURL  string
	username string
	password string
}

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"

// NewScraper creates a scraper that will use a username and password.
func NewScraper(baseURL, user, pass string) (*Scraper, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}
	client := &http.Client{Jar: jar}
	return &Scraper{
		client:   client,
		baseURL:  baseURL,
		username: user,
		password: pass,
	}, nil
}

// Login performs a direct login using admin credentials.
func (s *Scraper) Login() error {
	loginURL := fmt.Sprintf("%s/login.php", s.baseURL)
	formData := url.Values{
		"username": {s.username},
		"password": {s.password},
		"mode":     {"login"},
	}
	req, err := http.NewRequest("POST", loginURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Referer", loginURL)

	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("login POST request failed: %w", err)
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read login response body: %w", err)
	}

	if strings.Contains(string(bodyBytes), "Password incorrect") {
		return errors.New("login failed: password incorrect")
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed with status code: %d", res.StatusCode)
	}

	log.Println("✅ Successfully logged into Tripwire with admin account.")
	return nil
}

// FetchData uses the authenticated session to get map data.
func (s *Scraper) FetchData() (*TripwireData, error) {
	refreshURL := fmt.Sprintf("%s/refresh.php", s.baseURL)
	systemName := "Jita"

	formData := url.Values{
		"mode":       {"init"},
		"systemName": {systemName},
		"systemID":   {"30000142"},
	}

	req, err := http.NewRequest("POST", refreshURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create data fetch request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", fmt.Sprintf("%s/?system=%s", s.baseURL, systemName))
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	res, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("data fetch request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("data fetch failed with status code: %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if len(body) == 0 || body[0] != '{' {
		return nil, errors.New("response was not JSON (session may be invalid or expired)")
	}

	var data TripwireData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tripwire data: %w", err)
	}

	return &data, nil
}

// --- NEW: Function to fetch data and save it to a file ---
func fetchAndSaveData(scraper *Scraper) {
	log.Println("Fetching latest Tripwire data...")
	data, err := scraper.FetchData()
	if err != nil {
		// If the fetch fails (e.g., expired session), try to re-login once.
		log.Printf("WARNING: Data fetch failed: %v. Attempting to re-login...", err)
		if loginErr := scraper.Login(); loginErr != nil {
			log.Printf("ERROR: Re-login failed: %v", loginErr)
			return
		}
		// Retry fetching data after successful re-login.
		data, err = scraper.FetchData()
		if err != nil {
			log.Printf("ERROR: Data fetch failed even after re-login: %v", err)
			return
		}
	}

	log.Printf("✅ Successfully fetched data for %d signatures and %d wormholes.", len(data.Signatures), len(data.Wormholes))

	// Marshal the data into a nicely formatted JSON byte slice.
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("ERROR: Failed to marshal data to JSON: %v", err)
		return
	}

	// Write the JSON data to a local file, overwriting it if it exists.
	err = os.WriteFile("tripwire_data.json", jsonData, 0644)
	if err != nil {
		log.Printf("ERROR: Failed to write data to file: %v", err)
		return
	}

	log.Println("✅ Successfully updated local data file: tripwire_data.json")
}

// --- Main function now runs a continuous service ---
func main() {
	log.Println("--- Starting Tripwire Data Fetcher Service ---")
	if err := godotenv.Load(); err != nil {
		log.Fatalf("FATAL: Error loading .env file: %v", err)
	}
	tripwireURL := os.Getenv("TRIPWIRE_URL")
	tripwireUser := os.Getenv("TRIPWIRE_USER")
	tripwirePass := os.Getenv("TRIPWIRE_PASS")
	log.Println("Configuration loaded.")

	scraper, err := NewScraper(tripwireURL, tripwireUser, tripwirePass)
	if err != nil {
		log.Fatalf("FATAL: Could not create scraper: %v", err)
	}
	log.Println("Scraper created.")

	if err := scraper.Login(); err != nil {
		log.Fatalf("FATAL: Initial Tripwire login failed: %v", err)
	}

	// Perform an initial fetch as soon as the service starts.
	fetchAndSaveData(scraper)

	// Set up a ticker to run the fetch function every 10 minutes.
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	// Set up a channel to listen for shutdown signals (like Ctrl+C).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	log.Println("--- ✅ Service is running. Will fetch data every 10 minutes. Press CTRL+C to exit. ---")

	// This is the main service loop.
	for {
		select {
		case <-ticker.C:
			// This case is triggered every 10 minutes.
			fetchAndSaveData(scraper)
		case <-quit:
			// This case is triggered when you press Ctrl+C.
			log.Println("Shutdown signal received, exiting.")
			return
		}
	}
}
