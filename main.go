package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rodgon/valkyrie/pkg/agent"
	"github.com/rodgon/valkyrie/pkg/config"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create agent
	a, err := agent.NewAgent(cfg)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Main loop
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	go a.StartMetricsServer() // Starts on :8080

	for {
		select {
		case <-sigChan:
			log.Println("Received shutdown signal")
			return
		case <-ticker.C:
			// Observe cluster state
			if err := a.ObserveCluster(); err != nil {
				log.Printf("Error observing cluster: %v", err)
				continue
			}

			// Learn from observations
			if err := a.Learn(); err != nil {
				log.Printf("Error learning: %v", err)
				continue
			}

			// Detect anomalies
			anomalies, err := a.DetectAnomalies()
			if err != nil {
				log.Printf("Error detecting anomalies: %v", err)
				continue
			}

			// Print results
			a.PrintAnomalies(anomalies)
			a.PrintState()
		}
	}

}
