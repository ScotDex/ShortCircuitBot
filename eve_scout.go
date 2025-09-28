package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// EveScoutClient manages all communication with the EVE-Scout API.
type EveScoutClient struct {
	baseURL    string
	userAgent  string
	httpClient *http.Client
}

// NewEveScoutClient creates a new client for the EVE-Scout API.
func NewEveScoutClient(userAgent string) *EveScoutClient {
	return &EveScoutClient{
		baseURL:   "https://api.eve-scout.com/v2",
		userAgent: userAgent,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// makeRequest is a generic helper that handles all GET requests and JSON decoding.
func (c *EveScoutClient) makeRequest(endpoint string, target interface{}) error {
	req, err := http.NewRequest("GET", c.baseURL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api returned non-200 status: %d", resp.StatusCode)
	}

	// Decode the JSON response directly into the provided target struct.
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode json response: %w", err)
	}

	return nil
}

// GetRoutesBySystem fetches all public signatures for a specific system.
// This single function replaces both TheraRoutes and TurnurRoutes.
func (c *EveScoutClient) GetRoutesBySystem(systemName string) ([]Route, error) {
	var routes []Route
	endpoint := fmt.Sprintf("/public/signatures?system_name=%s", systemName)
	err := c.makeRequest(endpoint, &routes)
	if err != nil {
		return nil, err
	}
	return routes, nil
}

// GetAllRoutes fetches all public signatures from the API.
func (c *EveScoutClient) GetAllRoutes() ([]Route, error) {
	var routes []Route
	err := c.makeRequest("/public/signatures", &routes)
	if err != nil {
		return nil, err
	}
	return routes, nil
}
