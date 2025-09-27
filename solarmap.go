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

// AddTripwireWormholesToGraph integrates real-time wormhole data to the graph.
// tripwireData is a map of signature ID to Signature, representing wormhole signatures.
// Pass the whole TripwireData object to the function now
func AddTripwireWormholesToGraph(graph map[int][]int, data *TripwireData) {
	// Loop through each wormhole connection provided by the data
	for _, wh := range data.Wormholes {
		// Find the full signature details for each end of the wormhole
		sigA, okA := data.Signatures[wh.InitialID]
		sigB, okB := data.Signatures[wh.SecondaryID]

		// If both ends exist in the signatures map...
		if okA && okB {
			// ...get their system IDs
			sysA, errA := strconv.Atoi(sigA.SystemID)
			sysB, errB := strconv.Atoi(sigB.SystemID)

			if errA == nil && errB == nil {
				// ...and add the two-way jump to the graph
				graph[sysA] = append(graph[sysA], sysB)
				graph[sysB] = append(graph[sysB], sysA)
			}
		}
	}

	log.Printf("Successfully processed and added %d wormhole connections.", len(data.Wormholes))
}

// The parameter needs to change to accept all the new data
func GraphBuilder(data *TripwireData) (map[int][]int, error) {
	graph, err := BuildGraphFromCSV("mapSolarSystemJumps.csv")
	if err != nil {
		// It's better to return an error than to call log.Fatal here
		return nil, err
	}
	DeduplicateNeighbors(graph)

	// Now, call your new and improved function to add wormholes!
	if data != nil {
		AddTripwireWormholesToGraph(graph, data)
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
		return nil, err
	}

	var data TripwireData
	if err := json.Unmarshal(file, &data); err != nil {
		return nil, err
	}

	return &data, nil
}

// FindShortestPath uses Breadth-First Search to find the shortest path in jumps.
func FindShortestPath(graph map[int][]int, startID, endID int) []int {
	// A queue of paths to check
	queue := [][]int{{startID}}
	// A map to keep track of systems we've already visited to avoid loops
	visited := make(map[int]bool)
	visited[startID] = true

	for len(queue) > 0 {
		// Get the first path from the queue
		path := queue[0]
		queue = queue[1:]

		// Get the last system in the current path
		currentSystem := path[len(path)-1]

		// If we've found our destination, we're done!
		if currentSystem == endID {
			return path
		}

		// Otherwise, look at its neighbors
		for _, neighbor := range graph[currentSystem] {
			if !visited[neighbor] {
				visited[neighbor] = true
				// Create a new path by adding the neighbor to the current one
				newPath := make([]int, len(path))
				copy(newPath, path)
				newPath = append(newPath, neighbor)
				queue = append(queue, newPath)
			}
		}
	}

	// If the queue runs out and we haven't found the end, no path exists
	return nil
}
