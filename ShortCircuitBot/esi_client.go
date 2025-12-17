package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// --- Structs ---

// esiName represents the universal ESI response for an ID-to-name lookup.
type esiName struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// esiIDResults holds the results from the /universe/ids endpoint.
type esiIDResults struct {
	Systems []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"solar_systems"`
}

// ESISystemInfo holds detailed information about a solar system.
type ESISystemInfo struct {
	Name           string  `json:"name"`
	SecurityStatus float64 `json:"security_status"`
	SystemID       int     `json:"system_id"`
}

// EsiSystemKills defines the structure for the system kills endpoint.
type EsiSystemKills struct {
	NpcKills  int `json:"npc_kills"`
	PodKills  int `json:"pod_kills"`
	ShipKills int `json:"ship_kills"`
	SystemID  int `json:"system_id"`
}

// ESIClient manages all communication with the EVE Online ESI.
type ESIClient struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string

	cacheMutex      sync.RWMutex
	nameCache       map[int]string         // A single cache for all ID->Name lookups
	systemInfoCache map[int]*ESISystemInfo // Cache for detailed system info
	systemIDCache   map[string]int         // Cache for Name->ID lookups
}

// --- Constructor ---

func NewESIClient(contactInfo string) *ESIClient {
	return &ESIClient{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		baseURL:         "https://esi.evetech.net/latest",
		userAgent:       fmt.Sprintf("ShortCircuit Bot/0.1 (%s)", contactInfo),
		nameCache:       make(map[int]string),
		systemInfoCache: make(map[int]*ESISystemInfo),
		systemIDCache:   make(map[string]int),
	}
}

// --- Core HTTP Logic ---

func (c *ESIClient) makeRequest(method, endpoint string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, c.baseURL+endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESI returned %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// --- Name & ID Resolution ---

// GetSystemID resolves a system name to its ID.
func (c *ESIClient) GetSystemID(name string) (int, error) {
	c.cacheMutex.RLock()
	if id, ok := c.systemIDCache[strings.ToLower(name)]; ok {
		c.cacheMutex.RUnlock()
		return id, nil
	}
	for id, sysInfo := range c.systemInfoCache {
		if strings.EqualFold(sysInfo.Name, name) {
			c.cacheMutex.RUnlock()
			return id, nil
		}
	}
	c.cacheMutex.RUnlock()

	body, _ := json.Marshal([]string{name})
	respBytes, err := c.makeRequest(http.MethodPost, "/universe/ids/", bytes.NewBuffer(body))
	if err != nil {
		return 0, err
	}

	var idData esiIDResults
	if err := json.Unmarshal(respBytes, &idData); err != nil {
		return 0, err
	}
	if len(idData.Systems) == 0 {
		return 0, fmt.Errorf("system not found: %s", name)
	}

	id := idData.Systems[0].ID
	c.cacheMutex.Lock()
	c.systemIDCache[strings.ToLower(name)] = id
	c.cacheMutex.Unlock()
	return id, nil
}

// getName resolves a single ID to its name for a given category.
func (c *ESIClient) getName(id int, category string) string {
	if id == 0 {
		return "Unknown"
	}
	c.cacheMutex.RLock()
	if name, ok := c.nameCache[id]; ok {
		c.cacheMutex.RUnlock()
		return name
	}
	c.cacheMutex.RUnlock()

	// Bulk endpoint for names is more efficient
	ids := []int{id}
	body, _ := json.Marshal(ids)
	respBytes, err := c.makeRequest(http.MethodPost, "/universe/names/", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Failed to get name for ID %d: %v", id, err)
		return "Unknown"
	}

	var names []esiName
	if err := json.Unmarshal(respBytes, &names); err != nil || len(names) == 0 {
		log.Printf("Failed to decode name for ID %d: %v", id, err)
		return "Unknown"
	}

	name := names[0].Name
	c.cacheMutex.Lock()
	c.nameCache[id] = name
	c.cacheMutex.Unlock()
	return name
}

// --- Public Name Helpers ---

func (c *ESIClient) GetSystemName(id int) string { return c.getName(id, "solar_system") }
func (c *ESIClient) GetRegionName(id int) string { return c.getName(id, "region") }

// --- System Data ---

// LoadSystemCache loads the system info from a local file.
func (c *ESIClient) LoadSystemCache(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open cache file: %w", err)
	}
	defer f.Close()

	var cacheData map[int]*ESISystemInfo
	if err := json.NewDecoder(f).Decode(&cacheData); err != nil {
		return fmt.Errorf("failed to unmarshal system cache: %w", err)
	}

	c.cacheMutex.Lock()
	c.systemInfoCache = cacheData
	// Pre-populate the Name->ID cache for faster lookups
	for id, sysInfo := range cacheData {
		c.systemIDCache[strings.ToLower(sysInfo.Name)] = id
	}
	c.cacheMutex.Unlock()

	log.Printf("Loaded %d systems from cache.", len(c.systemInfoCache))
	return nil
}

// GetSystemDetails retrieves full system details from the cache.
func (c *ESIClient) GetSystemDetails(id int) (*ESISystemInfo, error) {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()
	if sys, ok := c.systemInfoCache[id]; ok {
		return sys, nil
	}
	// Fallback to live API if not in cache (optional, but robust)
	// For now, we'll just return an error as per original logic.
	return nil, fmt.Errorf("system ID %d not found in cache", id)
}

// GetSystemKills fetches recent kill data.
func (c *ESIClient) GetSystemKills() ([]EsiSystemKills, error) {
	respBytes, err := c.makeRequest(http.MethodGet, "/universe/system_kills/", nil)
	if err != nil {
		return nil, err
	}

	var kills []EsiSystemKills
	if err := json.Unmarshal(respBytes, &kills); err != nil {
		return nil, err
	}
	return kills, nil
}
