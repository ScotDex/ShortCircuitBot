package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

// Health service to keep it running on cloud run - not needed if you want to run it on digital ocean/container

func startHealthCheckServer() {

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Short Circuit Bot is running!")
	})

	log.Printf("Health check server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start health check server: %v", err)
	}
}
