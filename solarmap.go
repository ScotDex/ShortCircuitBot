package main

import (
	"log"
	"strconv"

	"gonum.org/v1/gonum/graph/simple"
)

// SolarMap holds the graph representation of the EVE universe.
type SolarMap struct {
	Graph *simple.DirectedGraph
	// A helper map to quickly find a graph node by its EVE System ID.
	systemIDToNode map[int64]int64
}

// NewSolarMapFromData creates a new SolarMap and populates it with data from Tripwire.
func NewSolarMapFromData(data *TripwireData) (*SolarMap, error) {
	g := simple.NewDirectedGraph()
	solarMap := &SolarMap{
		Graph:          g,
		systemIDToNode: make(map[int64]int64),
	}

	// First, add all unique systems from the signatures as nodes in our graph.
	for _, sig := range data.Signatures {
		systemID, err := strconv.ParseInt(sig.SystemID, 10, 64)
		if err != nil {
			// Skip signatures with invalid system IDs.
			continue
		}

		// If we haven't seen this system before, add it to the graph.
		if _, exists := solarMap.systemIDToNode[systemID]; !exists {
			node := g.NewNode()
			g.AddNode(node)
			solarMap.systemIDToNode[systemID] = node.ID()
		}
	}

	log.Printf("Added %d unique systems to the map.", len(solarMap.systemIDToNode))

	// Second, iterate through the wormholes to create the connections (edges).
	wormholeConnections := 0
	for _, wh := range data.Wormholes {
		// Look up the signatures for both sides of the wormhole.
		sigA, existsA := data.Signatures[wh.InitialID]
		sigB, existsB := data.Signatures[wh.SecondaryID]

		if !existsA || !existsB {
			continue // Skip wormholes with missing signature data.
		}

		// Get the system IDs for each side.
		systemA, _ := strconv.ParseInt(sigA.SystemID, 10, 64)
		systemB, _ := strconv.ParseInt(sigB.SystemID, 10, 64)

		// Get the graph nodes for each system.
		nodeA, existsA := solarMap.systemIDToNode[systemA]
		nodeB, existsB := solarMap.systemIDToNode[systemB]

		if !existsA || !existsB {
			continue // Skip if one of the systems wasn't valid.
		}

		// Add the connection in both directions.
		g.SetEdge(g.NewEdge(g.Node(nodeA), g.Node(nodeB)))
		g.SetEdge(g.NewEdge(g.Node(nodeB), g.Node(nodeA)))
		wormholeConnections++
	}

	log.Printf("Added %d wormhole connections to the map.", wormholeConnections)

	// TODO: In the future, we will also add all the static stargate connections here.

	return solarMap, nil
}
