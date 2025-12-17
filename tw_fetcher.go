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
	universeGraph     map[int][]int // This is a pointer to the main graph
	graphMutex        *sync.RWMutex
	baseStargateGraph map[int][]int // This is a clean copy
}

// New Fetcher is now simpler, it doesn't need the EVE-Scout or ESI clients.
func New(url, user, pass string, graph map[int][]int, mutex *sync.RWMutex) (*Fetcher, error) {
	scraper, err := NewScraper(url, user, pass)
	if err != nil {
		return nil, err
	}

	baseGraph := make(map[int][]int, len(graph))
	for k, v := range graph {
		newSlice := make([]int, len(v))
		copy(newSlice, v)
		baseGraph[k] = newSlice
	}

	return &Fetcher{
		scraper:           scraper,
		universeGraph:     graph,
		graphMutex:        mutex,
		baseStargateGraph: baseGraph,
	}, nil
}

// Start begins the background fetching service.
func (s *Fetcher) Start(wg *sync.WaitGroup, quit chan struct{}) {
	defer wg.Done()
	log.Println("[FETCHER] Starting service...")

	if err := s.scraper.Login(); err != nil {
		log.Fatalf("[FETCHER] FATAL: Initial Tripwire login failed: %v", err)
	}

	s.updateTripwireData() // Renamed for clarity

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	log.Println("✅ [FETCHER] Service is running. Will fetch Tripwire data every 10 minutes.")

	for {
		select {
		case <-ticker.C:
			s.updateTripwireData()
		case <-quit:
			log.Println("[FETCHER] Shutdown signal received, exiting.")
			return
		}
	}
}

func (s *Fetcher) updateTripwireData() {
	log.Println("[FETCHER] Fetching Tripwire data...")
	tripwireData, err := s.scraper.FetchData()
	if err != nil {
		log.Printf("[FETCHER] WARNING: Tripwire data fetch failed: %v", err)
		return
	}

	log.Println("[FETCHER] ✅ Tripwire data fetched successfully.")

	newGraph := make(map[int][]int, len(s.baseStargateGraph))
	for k, v := range s.baseStargateGraph {
		newSlice := make([]int, len(v))
		copy(newSlice, v)
		newGraph[k] = newSlice
	}

	if tripwireData != nil {
		for _, wh := range tripwireData.Wormholes {
			sigA, okA := tripwireData.Signatures[wh.InitialID]
			sigB, okB := tripwireData.Signatures[wh.SecondaryID]

			if okA && okB {
				sysA_ID, _ := strconv.Atoi(sigA.SystemID)
				sysB_ID, _ := strconv.Atoi(sigB.SystemID)

				if sysA_ID != 0 && sysB_ID != 0 {
					newGraph[sysA_ID] = append(newGraph[sysA_ID], sysB_ID)
					newGraph[sysB_ID] = append(newGraph[sysB_ID], sysA_ID)
				}
			}
		}
	}

	s.graphMutex.Lock()
	s.universeGraph = newGraph
	s.graphMutex.Unlock()
	log.Println("✅ [FETCHER] Merged Tripwire data into the graph.")

	// Save data to local file
	jsonData, err := json.MarshalIndent(tripwireData, "", "  ")
	if err != nil {
		log.Printf("[FETCHER] ERROR: Failed to marshal data: %v", err)
		return
	}
	err = os.WriteFile("tripwire_data.json", jsonData, 0644)
	if err != nil {
		log.Printf("[FETCHER] ERROR: Failed to write data to file: %v", err)
	}
}
