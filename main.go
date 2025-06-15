package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/rodgon/valkyrie/pkg/agent"
)

func main() {
	// Get the kubeconfig path from environment variable or use default
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Error getting user home directory: %v", err)
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	// Create the agent
	agent, err := agent.NewAgent(kubeconfig)
	if err != nil {
		log.Fatalf("Error creating agent: %v", err)
	}

	// Main learning loop
	for {
		// Observe current state
		err := agent.ObserveCluster()
		if err != nil {
			log.Printf("Error observing cluster: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		// Learn from the current state
		agent.Learn()

		// Detect anomalies
		anomalies := agent.DetectAnomalies()
		agent.PrintAnomalies(anomalies)

		// Print current state
		agent.PrintState()

		// Wait before next observation
		time.Sleep(30 * time.Second)
	}
}
