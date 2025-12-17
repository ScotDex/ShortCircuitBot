package main

import "time"

// --- Structs (from your models package) ---
// This section has been updated to fully model the JSON response.

// TripwireData matches the top-level JSON structure.
type TripwireData struct {
	Oauth      OauthData            `json:"oauth"`
	Esi        map[string]EsiData   `json:"esi"`
	Sync       string               `json:"sync"`
	Signatures map[string]Signature `json:"signatures"`
	Wormholes  map[string]Wormhole  `json:"wormholes"`
}

// OauthData holds the main OAuth token information.
type OauthData struct {
	Subject      int    `json:"subject"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenExpire  string `json:"tokenExpire"`
}

// EsiData holds the ESI information for a specific character.
type EsiData struct {
	CharacterID   string `json:"characterID"`
	CharacterName string `json:"characterName"`
	AccessToken   string `json:"accessToken"`
	RefreshToken  string `json:"refreshToken"`
	TokenExpire   string `json:"tokenExpire"`
}

// Signature holds all the details for a single cosmic signature.
// We use pointers for fields that can be null in the JSON response.
type Signature struct {
	ID             string      `json:"id"`
	SignatureID    *string     `json:"signatureID"`
	SystemID       string      `json:"systemID"`
	Type           string      `json:"type"`
	Name           *string     `json:"name"`
	Bookmark       interface{} `json:"bookmark"` // Use interface{} for mixed or null types
	LifeTime       string      `json:"lifeTime"`
	LifeLeft       string      `json:"lifeLeft"`
	LifeLength     string      `json:"lifeLength"`
	CreatedByID    string      `json:"createdByID"`
	CreatedByName  string      `json:"createdByName"`
	ModifiedByID   string      `json:"modifiedByID"`
	ModifiedByName string      `json:"modifiedByName"`
	ModifiedTime   string      `json:"modifiedTime"`
	MaskID         string      `json:"maskID"`
}

// Wormhole holds data for a wormhole connection. Its structure is still
// partially assumed until a full wormhole object is seen in the JSON.
type Wormhole struct {
	ID          string `json:id`
	InitialID   string `json:"initialID"`
	SecondaryID string `json:"secondaryID"`
	Type        string `json:"type"`
	Parent      string `json:"parent"`
	Life        string `json:"life"`
	Mass        string `json:"mass"`
}

type Direction struct {
	ID          string  `json:"id"`
	SystemID    string  `json:"systemID"`
	Type        string  `json:"type"`
	SignatureID *string `json:"signatureID"`
}

// --- Structs (kept the same) ---

type ApiHealth struct {
	Version string `json:"api_version"`
}

type Route struct {
	ID              string    `json:"id"`
	CreatedAt       time.Time `json:"created_at"`
	CreatedByID     int       `json:"created_by_id"`
	CreatedByName   string    `json:"created_by_name"`
	UpdatedAt       time.Time `json:"updated_at"`
	UpdatedByID     int       `json:"updated_by_id"`
	UpdatedByName   string    `json:"updated_by_name"`
	CompletedAt     time.Time `json:"completed_at"`
	CompletedByID   int       `json:"completed_by_id"`
	CompletedByName string    `json:"completed_by_name"`
	Completed       bool      `json:"completed"`
	WhExitsOutward  bool      `json:"wh_exits_outward"`
	WhType          string    `json:"wh_type"`
	MaxShipSize     string    `json:"max_ship_size"`
	ExpiresAt       time.Time `json:"expires_at"`
	RemainingHours  int       `json:"remaining_hours"`
	SignatureType   string    `json:"signature_type"`
	OutSystemID     int       `json:"out_system_id"`
	OutSystemName   string    `json:"out_system_name"`
	OutSignature    string    `json:"out_signature"`
	InSystemID      int       `json:"in_system_id"`
	InSystemClass   string    `json:"in_system_class"`
	InSystemName    string    `json:"in_system_name"`
	InRegionID      int       `json:"in_region_id"`
	InRegionName    string    `json:"in_region_name"`
	InSignature     string    `json:"in_signature"`
	Comment         string    `json:"comment"`
}

// --- API Client ---
