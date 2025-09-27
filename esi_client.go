package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// --- Structs ---
type (
	ESINameResponse struct {
		Name string `json:"name"`
	}
	ESIIDResponse struct {
		Characters []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"characters"`
		Systems []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"systems"`
	}
	ESISystemInfo struct {
		Name            string  `json:"name"`
		SecurityStatus  float64 `json:"security_status"`
		Stargates       []int   `json:"stargates"`
		Stations        []int   `json:"stations"`
		SystemID        int     `json:"system_id"`
		ConstellationID int     `json:"constellation_id"`
		RegionID        int     `json:"region_id"`
	}
	ESIRegionInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		RegionID    int    `json:"region_id"`
	}
	ESIClient struct {
		httpClient *http.Client
		baseURL    string
		userAgent  string

		cacheMutex       sync.RWMutex
		characterNames   map[int]string
		corporationNames map[int]string
		shipNames        map[int]string
		systemNames      map[int]string
		characterIDs     map[string]int
		systemInfoCache  map[int]*ESISystemInfo

		regionNames        map[int]string
		constellationNames map[int]string
	}
)

// --- Constructor ---
func NewESIClient(contactInfo string) *ESIClient {
	return &ESIClient{
		httpClient: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &http.Transport{DisableCompression: false},
		},
		baseURL:          "https://esi.evetech.net/latest",
		userAgent:        fmt.Sprintf("Firehawk Discord Bot (%s)", contactInfo),
		characterNames:   map[int]string{},
		corporationNames: map[int]string{},
		shipNames:        map[int]string{},
		systemNames:      map[int]string{},
		characterIDs:     map[string]int{},
		systemInfoCache:  map[int]*ESISystemInfo{},

		regionNames:        map[int]string{},
		constellationNames: map[int]string{},
	}
}

// --- Core HTTP ---
func (c *ESIClient) makeRequest(method, url string, body io.Reader, target interface{}) error {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ESI returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

// --- Character ID <-> Name ---
func (c *ESIClient) GetCharacterID(name string) (int, error) {
	c.cacheMutex.RLock()
	if id, ok := c.characterIDs[name]; ok {
		c.cacheMutex.RUnlock()
		return id, nil
	}
	c.cacheMutex.RUnlock()

	var idData ESIIDResponse
	body, _ := json.Marshal([]string{name})
	if err := c.makeRequest(http.MethodPost, c.baseURL+"/universe/ids/", bytes.NewBuffer(body), &idData); err != nil {
		return 0, err
	}
	if len(idData.Characters) == 0 {
		return 0, fmt.Errorf("character not found: %s", name)
	}

	id := idData.Characters[0].ID
	c.cacheMutex.Lock()
	c.characterIDs[name] = id
	c.cacheMutex.Unlock()
	return id, nil
}

// --- Generic ID -> Name ---
func (c *ESIClient) getName(id int, category string, cache map[int]string) string {
	if id == 0 {
		return "Unknown"
	}
	c.cacheMutex.RLock()
	if name, ok := cache[id]; ok {
		c.cacheMutex.RUnlock()
		return name
	}
	c.cacheMutex.RUnlock()

	var resp ESINameResponse
	url := fmt.Sprintf("%s/%s/%d/", c.baseURL, category, id)
	if err := c.makeRequest(http.MethodGet, url, nil, &resp); err != nil {
		log.Printf("Failed to get name for ID %d (%s): %v", id, category, err)
		return "Unknown"
	}

	c.cacheMutex.Lock()
	cache[id] = resp.Name
	c.cacheMutex.Unlock()
	return resp.Name
}

// --- Public Name Helpers ---
func (c *ESIClient) GetCharacterName(id int) string {
	return c.getName(id, "characters", c.characterNames)
}
func (c *ESIClient) GetCorporationName(id int) string {
	return c.getName(id, "corporations", c.corporationNames)
}
func (c *ESIClient) GetShipName(id int) string { return c.getName(id, "universe/types", c.shipNames) }
func (c *ESIClient) GetConstellationName(id int) string {
	return c.getName(id, "universe/constellations", c.constellationNames)
}

func (c *ESIClient) GetSystemName(id int) string {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()
	if sys, ok := c.systemInfoCache[id]; ok {
		return sys.Name
	}
	return "Unknown"
}

func (c *ESIClient) GetRegionName(id int) string {
	if id == 0 {
		return "Unknown"
	}
	c.cacheMutex.RLock()
	if name, ok := c.regionNames[id]; ok {
		c.cacheMutex.RUnlock()
		return name
	}
	c.cacheMutex.RUnlock()

	var region ESIRegionInfo
	url := fmt.Sprintf("%s/universe/regions/%d/", c.baseURL, id)
	if err := c.makeRequest(http.MethodGet, url, nil, &region); err != nil {
		log.Printf("Failed to get region name for ID %d: %v", id, err)
		return "Unknown"
	}

	c.cacheMutex.Lock()
	c.regionNames[id] = region.Name
	c.cacheMutex.Unlock()
	return region.Name
}

// --- System Cache ---
func (c *ESIClient) LoadSystemCache(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open cache file: %w", err)
	}
	defer f.Close()

	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()
	if err := json.NewDecoder(f).Decode(&c.systemInfoCache); err != nil {
		return fmt.Errorf("failed to unmarshal system cache: %w", err)
	}
	log.Printf("Loaded %d systems from cache.", len(c.systemInfoCache))
	return nil
}

func (c *ESIClient) GetSystemDetails(id int) (*ESISystemInfo, error) {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()
	if sys, ok := c.systemInfoCache[id]; ok {
		return sys, nil
	}
	return nil, fmt.Errorf("system ID %d not found", id)
}

// --- Misc ---
func (c *ESIClient) GetRandomCorporationLogoURL() string {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()
	if len(c.corporationNames) == 0 {
		return "https://images.evetech.net/corporations/109299958/logo?size=128"
	}
	ids := make([]int, 0, len(c.corporationNames))
	for id := range c.corporationNames {
		ids = append(ids, id)
	}
	return fmt.Sprintf("https://images.evetech.net/corporations/%d/logo?size=128", ids[rand.Intn(len(ids))])
}

func (c *ESIClient) GetSystemID(name string) (int, error) {
	// For case-insensitivity, we can use a local cache
	c.cacheMutex.RLock()
	// This is a simple loop, but effective for a small number of cached systems
	for id, sysInfo := range c.systemInfoCache {
		if strings.EqualFold(sysInfo.Name, name) {
			c.cacheMutex.RUnlock()
			return id, nil
		}
	}
	c.cacheMutex.RUnlock()

	// If not in cache, query ESI
	var idData ESIIDResponse
	body, _ := json.Marshal([]string{name})
	if err := c.makeRequest(http.MethodPost, c.baseURL+"/universe/ids/", bytes.NewBuffer(body), &idData); err != nil {
		return 0, err
	}
	if len(idData.Systems) == 0 {
		return 0, fmt.Errorf("system not found: %s", name)
	}

	// Note: We don't cache the result here because the systemInfoCache is loaded from a file.
	// A more advanced implementation might add it to the cache.
	return idData.Systems[0].ID, nil
}
