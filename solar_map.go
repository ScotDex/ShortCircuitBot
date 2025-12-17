package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
)

// BuildGraphFromCSV reads mapSolarSystemJumps.csv and returns a graph as adjacency list.
func BuildGraphFromCSV(filename string) (map[int][]int, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV data: %w", err)
	}

	graph := make(map[int][]int)

	// Expected columns: fromRegionID,fromConstellationID,fromSolarSystemID,toSolarSystemID,toConstellationID,toRegionID
	for i, rec := range records {
		if i == 0 {
			continue // skip header
		}
		if len(rec) < 6 {
			log.Printf("Skipping incomplete row %d", i+1)
			continue
		}
		fromSystem, err1 := strconv.Atoi(rec[2])
		toSystem, err2 := strconv.Atoi(rec[3])
		if err1 != nil || err2 != nil {
			log.Printf("Invalid system ID at row %d: %v %v", i+1, err1, err2)
			continue
		}
		graph[fromSystem] = append(graph[fromSystem], toSystem)
		graph[toSystem] = append(graph[toSystem], fromSystem)
	}

	return graph, nil
}

// DeduplicateNeighbors ensures no duplicate edges in adjacency lists.
func DeduplicateNeighbors(graph map[int][]int) {
	for systemID, neighbors := range graph {
		unique := make(map[int]bool)
		deduped := make([]int, 0, len(neighbors))
		for _, n := range neighbors {
			if !unique[n] {
				unique[n] = true
				deduped = append(deduped, n)
			}
		}
		graph[systemID] = deduped
	}
}

// In the file with your graph logic

func AddTripwireWormholesToGraph(graph map[int][]int, data *TripwireData, esiClient *ESIClient) {
	if data == nil {
		return
	}

	addedCount := 0
	for _, wh := range data.Wormholes {
		sigA, okA := data.Signatures[wh.InitialID]
		sigB, okB := data.Signatures[wh.SecondaryID]

		if okA && okB {
			sysA_ID, _ := strconv.Atoi(sigA.SystemID)
			sysB_ID, _ := strconv.Atoi(sigB.SystemID)

			// Define the complete guard conditions
			isSigAValid := sysA_ID != 0 && sigA.SignatureID != nil && *sigA.SignatureID != "???"
			isSigBValid := sysB_ID != 0 && sigB.SignatureID != nil && *sigB.SignatureID != "???"

			// Use the complete guard. The old if block has been removed.
			if isSigAValid && isSigBValid {
				graph[sysA_ID] = append(graph[sysA_ID], sysB_ID)
				graph[sysB_ID] = append(graph[sysB_ID], sysA_ID)
				addedCount++

				// Proactively look up the names for these systems and cache them.
				if esiClient != nil {
					esiClient.GetSystemName(sysA_ID)
					esiClient.GetSystemName(sysB_ID)
				}
			}
		}
	}
	log.Printf("Successfully processed and added %d wormhole connections from Tripwire.", addedCount)
}

// The parameter needs to change to accept all the new data
func GraphBuilder(data *TripwireData, esiClient *ESIClient) (map[int][]int, error) {
	graph, err := BuildGraphFromCSV("mapSolarSystemJumps.csv")
	if err != nil {
		// It's better to return an error than to call log.Fatal here
		return nil, err
	}
	DeduplicateNeighbors(graph)

	// Now, call your new and improved function to add wormholes!
	if data != nil {
		AddTripwireWormholesToGraph(graph, data, esiClient) // Pass nil for esiClient for now
	}

	// This debug printing is great for checking your work
	fmt.Printf("Graph contains %d systems.\n", len(graph))
	exampleSystemID := 30000142 // Jita
	if neighbors, ok := graph[exampleSystemID]; ok {
		fmt.Printf("System %d has %d direct jumps:\n", exampleSystemID, len(neighbors))
		for _, n := range neighbors {
			fmt.Printf("  -> %d\n", n)
		}
	}

	return graph, nil
}

func loadTripwireData(filename string) (*TripwireData, error) {
	file, err := os.ReadFile(filename)
	if err != nil {
		// If the file doesn't exist, that's okay.
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(file) == 0 {
		return nil, nil
	}
	var data TripwireData
	if err := json.Unmarshal(file, &data); err != nil {
		return nil, err
	}

	return &data, nil
}
