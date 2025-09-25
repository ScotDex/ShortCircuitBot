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
	"strings"
	"sync"
	"time"
)

// --- Scraper Logic ---
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

	log.Println("✅ [FETCHER] Successfully logged into Tripwire.")
	return nil
}

// FetchData uses the authenticated session to get map data.
func (s *Scraper) FetchData() (*models.TripwireData, error) {
	refreshURL := fmt.Sprintf("%s/refresh.php", s.baseURL)
	formData := url.Values{
		"mode":     {"init"},
		"systemID": {"30000142"},
	}

	req, err := http.NewRequest("POST", refreshURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create data fetch request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", fmt.Sprintf("%s/?system=Jita", s.baseURL))
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

	var data models.TripwireData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tripwire data: %w", err)
	}

	return &data, nil
}

// --- Fetcher Service ---
// RENAMED: from Service to Fetcher to avoid name collisions.
type Fetcher struct {
	scraper *Scraper
}

// RENAMED: from NewService to New to be more idiomatic.
func New(url, user, pass string) (*Fetcher, error) {
	scraper, err := NewScraper(url, user, pass)
	if err != nil {
		return nil, err
	}
	return &Fetcher{scraper: scraper}, nil
}

// Start begins the background fetching service.
func (s *Fetcher) Start(wg *sync.WaitGroup, quit chan os.Signal) {
	defer wg.Done()
	log.Println("[FETCHER] Starting service...")

	if err := s.scraper.Login(); err != nil {
		log.Fatalf("[FETCHER] FATAL: Initial Tripwire login failed: %v", err)
	}

	s.fetchAndSaveData()

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	log.Println("✅ [FETCHER] Service is running. Will fetch data every 10 minutes.")

	for {
		select {
		case <-ticker.C:
			s.fetchAndSaveData()
		case <-quit:
			log.Println("[FETCHER] Shutdown signal received, exiting.")
			return
		}
	}
}

func (s *Fetcher) fetchAndSaveData() {
	log.Println("[FETCHER] Fetching latest Tripwire data...")
	data, err := s.scraper.FetchData()
	if err != nil {
		log.Printf("[FETCHER] WARNING: Data fetch failed: %v. Attempting to re-login...", err)
		if loginErr := s.scraper.Login(); loginErr != nil {
			log.Printf("[FETCHER] ERROR: Re-login failed: %v", loginErr)
			return
		}
		data, err = s.scraper.FetchData()
		if err != nil {
			log.Printf("[FETCHER] ERROR: Data fetch failed after re-login: %v", err)
			return
		}
	}

	log.Printf("✅ [FETCHER] Successfully fetched data for %d signatures and %d wormholes.", len(data.Signatures), len(data.Wormholes))

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("[FETCHER] ERROR: Failed to marshal data to JSON: %v", err)
		return
	}

	err = os.WriteFile("tripwire_data.json", jsonData, 0644)
	if err != nil {
		log.Printf("[FETCHER] ERROR: Failed to write data to file: %v", err)
		return
	}

	log.Println("✅ [FETCHER] Successfully updated local data file: tripwire_data.json")
}

