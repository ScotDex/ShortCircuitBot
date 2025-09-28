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
	"strconv"
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

func (s *Scraper) FetchData() (*TripwireData, error) {
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

	var data TripwireData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tripwire data: %w", err)
	}

	return &data, nil
}

type Fetcher struct {
	scraper           *Scraper
	universeGraph     map[int][]int
	graphMutex        *sync.RWMutex
	eveScoutClient    *EveScoutClient
	esiClient         *ESIClient // Add ESIClient here
	baseStargateGraph map[int][]int
	// esiClient:         esi, // Add ESIClient here
}

func New(url, user, pass string, graph map[int][]int, mutex *sync.RWMutex, esc *EveScoutClient, esiClient *ESIClient) (*Fetcher, error) {
	scraper, err := NewScraper(url, user, pass)
	if err != nil {
		return nil, err
	}

	baseGraph := make(map[int][]int, len(graph))
	for k, v := range graph {
		// Ensure the slice is also copied, not just referenced
		newSlice := make([]int, len(v))
		copy(newSlice, v)
		baseGraph[k] = newSlice
	}

	return &Fetcher{
		scraper:           scraper,
		universeGraph:     graph,
		graphMutex:        mutex,
		eveScoutClient:    NewEveScoutClient("ShortCircuitBot/0.1"), // Initialize EveScoutClient here
		esiClient:         nil,                                      // ESIClient needs to be passed in or initialized
		baseStargateGraph: baseGraph,
	}, nil
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

// In fetcher_service.go
func (s *Fetcher) fetchAndSaveData() {
	log.Println("[FETCHER] Concurrently fetching all wormhole data...")
	var wg sync.WaitGroup
	wg.Add(2)

	var tripwireData *TripwireData
	var allScoutRoutes []Route // Changed from theraConnections
	var tripwireErr, scoutErr error

	// Task 1: Fetch Tripwire data
	go func() {
		defer wg.Done()
		tripwireData, tripwireErr = s.scraper.FetchData()
	}()

	// Task 2: Fetch all Eve-Scout routes
	go func() {
		defer wg.Done()
		allScoutRoutes, scoutErr = s.eveScoutClient.GetAllRoutes()
	}()

	wg.Wait() // Wait for both fetches to complete

	// Check for errors
	if tripwireErr != nil {
		log.Printf("[FETCHER] WARNING: Tripwire data fetch failed: %v", tripwireErr)
		return // Exit if Tripwire fails
	}
	if scoutErr != nil {
		log.Printf("[FETCHER] WARNING: Eve-Scout data fetch failed: %v", scoutErr)
	}
	log.Println("✅ [FETCHER] All data fetched successfully.")

	// --- EFFICIENT GRAPH UPDATE ---

	// 1. Create a fresh copy of the cached stargate map. This is much faster than reading from disk.
	newGraph := make(map[int][]int, len(s.baseStargateGraph))
	for k, v := range s.baseStargateGraph {
		newSlice := make([]int, len(v))
		copy(newSlice, v)
		newGraph[k] = newSlice
	}

	// 2. Add the new data
	if tripwireData != nil {
		log.Println("[FETCHER] Processing and validating Tripwire data...")
		for _, wh := range tripwireData.Wormholes {
			// This is the validation check
			if wh.InitialID == "???" || wh.SecondaryID == "???" {
				continue // Skip this wormhole
			}

			sigA, okA := tripwireData.Signatures[wh.InitialID]
			sigB, okB := tripwireData.Signatures[wh.SecondaryID]

			if okA && okB {
				sysA_ID, _ := strconv.Atoi(sigA.SystemID)
				sysB_ID, _ := strconv.Atoi(sigB.SystemID)

				if sysA_ID != 0 && sysB_ID != 0 {
					newGraph[sysA_ID] = append(newGraph[sysA_ID], sysB_ID)
					newGraph[sysB_ID] = append(newGraph[sysB_ID], sysA_ID)
					s.esiClient.GetSystemName(sysA_ID)
					s.esiClient.GetSystemName(sysB_ID)
				}
			}
		}
	} // s.esiClient is not available here, pass nil for now
	for _, route := range allScoutRoutes {
		newGraph[route.InSystemID] = append(newGraph[route.InSystemID], route.OutSystemID)
		newGraph[route.OutSystemID] = append(newGraph[route.OutSystemID], route.InSystemID)
	}
	DeduplicateNeighbors(newGraph)

	// 3. Lock, atomically swap the old graph with the new one, then unlock.
	s.graphMutex.Lock()
	s.universeGraph = newGraph
	s.graphMutex.Unlock()
	log.Println("✅ [FETCHER] Universe graph has been updated.")

	// 4. Save the new Tripwire data to a local file for the next startup.
	jsonData, err := json.MarshalIndent(tripwireData, "", "  ")
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
