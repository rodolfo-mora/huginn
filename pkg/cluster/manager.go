package cluster

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rodolfo-mora/huginn/pkg/config"
	"github.com/rodolfo-mora/huginn/pkg/types"
)

// Manager handles multiple cluster operations
type Manager struct {
	clusters map[string]*ClusterAgent
	config   *config.Config
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// ClusterAgent represents an agent for a single cluster
type ClusterAgent struct {
	ClusterConfig *config.ClusterConfig
	State         *types.ClusterState
	LastUpdated   time.Time
	Healthy       bool
	Error         error
}

// NewManager creates a new cluster manager
func NewManager(cfg *config.Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		clusters: make(map[string]*ClusterAgent),
		config:   cfg,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// InitializeClusters initializes all enabled clusters
func (m *Manager) InitializeClusters() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, clusterConfig := range m.config.Clusters {
		if !clusterConfig.Enabled {
			continue
		}

		clusterAgent := &ClusterAgent{
			ClusterConfig: &clusterConfig,
			State: &types.ClusterState{
				ClusterID:   clusterConfig.ID,
				ClusterName: clusterConfig.Name,
				Namespaces:  []string{},
				Nodes:       []types.Node{},
				Resources:   make(map[string]types.ResourceList),
				Events:      []types.ClusterEvent{},
			},
			LastUpdated: time.Now(),
			Healthy:     true,
		}

		m.clusters[clusterConfig.ID] = clusterAgent
		log.Printf("Initialized cluster: %s (%s)", clusterConfig.Name, clusterConfig.ID)
	}

	return nil
}

// GetCluster returns a specific cluster agent
func (m *Manager) GetCluster(clusterID string) (*ClusterAgent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cluster, exists := m.clusters[clusterID]
	return cluster, exists
}

// GetAllClusters returns all cluster agents
func (m *Manager) GetAllClusters() map[string]*ClusterAgent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ClusterAgent)
	for id, cluster := range m.clusters {
		result[id] = cluster
	}
	return result
}

// UpdateClusterState updates the state of a specific cluster
func (m *Manager) UpdateClusterState(clusterID string, state *types.ClusterState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cluster, exists := m.clusters[clusterID]
	if !exists {
		return fmt.Errorf("cluster %s not found", clusterID)
	}

	cluster.State = state
	cluster.LastUpdated = time.Now()
	cluster.Healthy = true
	cluster.Error = nil

	return nil
}

// SetClusterHealth sets the health status of a cluster
func (m *Manager) SetClusterHealth(clusterID string, healthy bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cluster, exists := m.clusters[clusterID]; exists {
		cluster.Healthy = healthy
		cluster.Error = err
		cluster.LastUpdated = time.Now()
	}
}

// GetMultiClusterState returns the aggregated state of all clusters
func (m *Manager) GetMultiClusterState() *types.MultiClusterState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	multiState := &types.MultiClusterState{
		Clusters: make(map[string]types.ClusterState),
		Summary: types.ClusterSummary{
			LastUpdated: time.Now(),
		},
	}

	for id, cluster := range m.clusters {
		if cluster.State != nil {
			multiState.Clusters[id] = *cluster.State
		}

		multiState.Summary.TotalClusters++
		if cluster.Healthy {
			multiState.Summary.HealthyClusters++
		} else {
			multiState.Summary.UnhealthyClusters++
		}

		if cluster.State != nil {
			multiState.Summary.TotalNodes += len(cluster.State.Nodes)
		}
	}

	return multiState
}

// GetClusterSummary returns a summary of cluster health
func (m *Manager) GetClusterSummary() types.ClusterSummary {
	multiState := m.GetMultiClusterState()
	return multiState.Summary
}

// Stop stops the cluster manager
func (m *Manager) Stop() {
	m.cancel()
}

// GetContext returns the manager's context
func (m *Manager) GetContext() context.Context {
	return m.ctx
}
