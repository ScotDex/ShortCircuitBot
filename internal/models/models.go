// in internal/models/models.go
package models

// TripwireData matches the top-level JSON structure from the scraper.
type TripwireData struct {
	Wormholes  map[string]Wormhole  `json:"wormholes"`
	Signatures map[string]Signature `json:"signatures"`
}

// Wormhole defines the fields for a single wormhole.
type Wormhole struct {
	InitialID   int    `json:"initialID"`
	SecondaryID int    `json:"secondaryID"`
	Type        string `json:"type"`
	Life        string `json:"life"`
	Mass        string `json:"mass"`
}

// Signature defines the fields for a cosmic signature.
type Signature struct {
	SystemID     int    `json:"systemID"`
	SignatureID  string `json:"signatureID"`
	ModifiedTime string `json:"modifiedTime"`
}
