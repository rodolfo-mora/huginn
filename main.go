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
	printAnomalies := flag.Bool("print-anomalies", false, "Print detected anomalies")
	printState := flag.Bool("print-state", false, "Print cluster state")
	printConfig := flag.Bool("print-config", false, "Print configuration")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create multi-cluster agent
	multiAgent, err := agent.NewMultiClusterAgent(cfg)
	if err != nil {
		log.Fatalf("Failed to create multi-cluster agent: %v", err)
	}
	defer multiAgent.Stop()

	// Print configuration if requested
	if *printConfig {
		multiAgent.PrintConfig()
		return
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Main loop
	ticker := time.NewTicker(time.Duration(cfg.ObservationInterval) * time.Second)
	defer ticker.Stop()

	go multiAgent.StartMetricsServer() // Starts on :8080

	log.Printf("Multi-cluster agent started with %d clusters", len(cfg.Clusters))

	for {
		select {
		case <-sigChan:
			log.Println("Received shutdown signal")
			return
		case <-multiAgent.GetContext().Done():
			log.Println("Context cancelled, shutting down")
			return
		case <-ticker.C:
			// Check for shutdown signal before starting operations
			select {
			case <-sigChan:
				log.Println("Received shutdown signal during operation")
				return
			case <-multiAgent.GetContext().Done():
				log.Println("Context cancelled during operation")
				return
			default:
				// Continue with operations
			}

			// Observe all clusters with context
			if err := multiAgent.ObserveAllClustersWithContext(multiAgent.GetContext()); err != nil {
				if multiAgent.GetContext().Err() != nil {
					log.Println("Observation cancelled due to shutdown")
					return
				}
				log.Printf("Error observing clusters: %v", err)
				continue
			}

			// Check for shutdown again
			select {
			case <-sigChan:
				log.Println("Received shutdown signal after observation")
				return
			case <-multiAgent.GetContext().Done():
				return
			default:
			}

			// Learn from observations across all clusters
			if err := multiAgent.LearnFromAllClusters(); err != nil {
				log.Printf("Error learning: %v", err)
				continue
			}

			// Check for shutdown again
			select {
			case <-sigChan:
				log.Println("Received shutdown signal after learning")
				return
			case <-multiAgent.GetContext().Done():
				return
			default:
			}

			// Detect anomalies across all clusters with context
			anomalies, err := multiAgent.DetectAllAnomaliesWithContext(multiAgent.GetContext())
			if err != nil {
				if multiAgent.GetContext().Err() != nil {
					log.Println("Anomaly detection cancelled due to shutdown")
					return
				}
				log.Printf("Error detecting anomalies: %v", err)
				continue
			}

			// Print results based on flags
			if *printAnomalies {
				multiAgent.PrintAllAnomalies(anomalies)
			}
			if *printState {
				multiAgent.PrintMultiClusterState()
			}
		}
	}
}
